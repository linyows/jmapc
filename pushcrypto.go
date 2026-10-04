package jmapc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
)

// pushKeys are the keys a push subscription is made with, as RFC 8620,
// Section 7.2 has a client give them: the server encrypts each push for them as
// RFC 8291 describes, and only the holder of the private key and the auth
// secret can read it, or write one that decrypts.
type pushKeys struct {
	private *ecdh.PrivateKey
	auth    []byte
}

// newPushKeys makes a P-256 key pair and a 16-octet auth secret.
func newPushKeys() (*pushKeys, error) {
	private, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("jmapc: making the push keys: %w", err)
	}
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		return nil, fmt.Errorf("jmapc: making the push keys: %w", err)
	}
	return &pushKeys{private: private, auth: auth}, nil
}

// subscription returns the keys as a push subscription carries them: the
// public key uncompressed and the auth secret, each base64url without padding.
func (k *pushKeys) subscription() map[string]string {
	return map[string]string{
		"p256dh": base64.RawURLEncoding.EncodeToString(k.private.PublicKey().Bytes()),
		"auth":   base64.RawURLEncoding.EncodeToString(k.auth),
	}
}

// The parts of the aes128gcm content coding of RFC 8188.
const (
	saltLength   = 16
	tagLength    = 16
	headerLength = saltLength + 4 + 1
)

// decrypt reads a push encrypted with the aes128gcm content coding of RFC
// 8188, with the key and nonce derived as RFC 8291 has a push service derive
// them. The header carries the server's public key as its key id, and the
// records each end in a delimiter, then padding.
func (k *pushKeys) decrypt(body []byte) ([]byte, error) {
	if len(body) < headerLength {
		return nil, errors.New("the push is shorter than its header")
	}
	salt := body[:saltLength]
	recordSize := int(binary.BigEndian.Uint32(body[saltLength : saltLength+4]))
	idLength := int(body[saltLength+4])
	if len(body) < headerLength+idLength {
		return nil, errors.New("the push is shorter than its header says")
	}
	if recordSize <= tagLength+1 {
		return nil, fmt.Errorf("the push has records of %d octets, too small to hold one", recordSize)
	}
	serverKey, err := ecdh.P256().NewPublicKey(body[headerLength : headerLength+idLength])
	if err != nil {
		return nil, fmt.Errorf("the push does not carry the server's public key: %w", err)
	}
	ciphertext := body[headerLength+idLength:]

	// RFC 8291, Section 3.4: the shared secret and the auth secret make the
	// input keying material, which RFC 8188 then derives the key and the
	// nonce from, with the salt.
	shared, err := k.private.ECDH(serverKey)
	if err != nil {
		return nil, fmt.Errorf("the push's key does not agree with ours: %w", err)
	}
	info := "WebPush: info\x00" + string(k.private.PublicKey().Bytes()) + string(serverKey.Bytes())
	ikm, err := hkdf.Key(sha256.New, shared, k.auth, info, 32)
	if err != nil {
		return nil, err
	}
	key, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	var plaintext []byte
	for seq := uint64(0); len(ciphertext) > 0; seq++ {
		n := min(recordSize, len(ciphertext))
		last := n == len(ciphertext)
		// RFC 8188, Section 2.3: the nonce of each record is the derived one
		// with the record's sequence number XORed into its end.
		recordNonce := append([]byte(nil), nonce...)
		var counter [8]byte
		binary.BigEndian.PutUint64(counter[:], seq)
		for i := range counter {
			recordNonce[4+i] ^= counter[i]
		}
		record, err := gcm.Open(nil, recordNonce, ciphertext[:n], nil)
		if err != nil {
			return nil, errors.New("the push does not decrypt with this subscription's keys")
		}
		// Padding is zeros, after a delimiter that is 2 in the last record
		// and 1 in any other.
		end := len(record) - 1
		for end >= 0 && record[end] == 0 {
			end--
		}
		want := byte(1)
		if last {
			want = 2
		}
		if end < 0 || record[end] != want {
			return nil, errors.New("the push's padding is not what RFC 8188 sets")
		}
		plaintext = append(plaintext, record[:end]...)
		ciphertext = ciphertext[n:]
	}
	return plaintext, nil
}
