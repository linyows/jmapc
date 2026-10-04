package jmapc

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"
)

func b64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("decoding %s: %v", s, err)
	}
	return b
}

// TestDecryptTheExampleOfRFC8291 decrypts the push of RFC 8291, Appendix A,
// with the user agent's keys it gives.
func TestDecryptTheExampleOfRFC8291(t *testing.T) {
	private, err := ecdh.P256().NewPrivateKey(b64(t, "q1dXpw3UpT5VOmu_cf_v6ih07Aems3njxI-JWgLcM94"))
	if err != nil {
		t.Fatalf("the user agent's private key: %v", err)
	}
	if got := base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes()); got != "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4" {
		t.Fatalf("the user agent's public key is %s, not the one the RFC gives", got)
	}
	keys := &pushKeys{private: private, auth: b64(t, "BTBZMqHH6r4Tts7J_aSIgg")}
	got, err := keys.decrypt(b64(t, "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"))
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(got) != "When I grow up, I want to be a watermelon" {
		t.Errorf("decrypted %q", got)
	}
}

// encryptPush encrypts plaintext for the keys of a subscription as a server
// does, RFC 8291 over RFC 8188, in records of recordSize octets.
func encryptPush(t *testing.T, keys map[string]string, plaintext []byte, recordSize int) []byte {
	t.Helper()
	userKey, err := ecdh.P256().NewPublicKey(b64(t, keys["p256dh"]))
	if err != nil {
		t.Fatalf("the subscription's public key: %v", err)
	}
	auth := b64(t, keys["auth"])
	server, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := server.ECDH(userKey)
	if err != nil {
		t.Fatal(err)
	}
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	info := "WebPush: info\x00" + string(userKey.Bytes()) + string(server.PublicKey().Bytes())
	ikm, _ := hkdf.Key(sha256.New, shared, auth, info, 32)
	key, _ := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)

	var out bytes.Buffer
	out.Write(salt)
	_ = binary.Write(&out, binary.BigEndian, uint32(recordSize))
	out.WriteByte(byte(len(server.PublicKey().Bytes())))
	out.Write(server.PublicKey().Bytes())
	room := recordSize - 16 - 1
	for seq := uint64(0); ; seq++ {
		n := min(room, len(plaintext))
		last := n == len(plaintext)
		record := append(append([]byte(nil), plaintext[:n]...), 1)
		if last {
			record[len(record)-1] = 2
		}
		recordNonce := append([]byte(nil), nonce...)
		var counter [8]byte
		binary.BigEndian.PutUint64(counter[:], seq)
		for i := range counter {
			recordNonce[4+i] ^= counter[i]
		}
		out.Write(gcm.Seal(nil, recordNonce, record, nil))
		plaintext = plaintext[n:]
		if last {
			return out.Bytes()
		}
	}
}

func TestDecryptWhatIsEncryptedForTheKeys(t *testing.T) {
	keys, err := newPushKeys()
	if err != nil {
		t.Fatal(err)
	}
	message := []byte(strings.Repeat("a state change ", 20))
	for _, size := range []int{4096, 64} {
		got, err := keys.decrypt(encryptPush(t, keys.subscription(), message, size))
		if err != nil {
			t.Fatalf("records of %d: decrypt: %v", size, err)
		}
		if !bytes.Equal(got, message) {
			t.Errorf("records of %d: decrypted %q", size, got)
		}
	}
}

// TestDecryptRefusesWhatWasNotEncryptedForTheKeys checks that a push made for
// other keys, or altered on the way, does not decrypt: the keys are the only
// thing that says a push came from the server.
func TestDecryptRefusesWhatWasNotEncryptedForTheKeys(t *testing.T) {
	ours, _ := newPushKeys()
	theirs, _ := newPushKeys()
	message := []byte(`{"@type":"StateChange","changed":{}}`)

	if _, err := ours.decrypt(encryptPush(t, theirs.subscription(), message, 4096)); err == nil {
		t.Error("a push encrypted for other keys decrypted")
	}
	altered := encryptPush(t, ours.subscription(), message, 4096)
	altered[len(altered)-1] ^= 1
	if _, err := ours.decrypt(altered); err == nil {
		t.Error("an altered push decrypted")
	}
	for _, short := range [][]byte{nil, make([]byte, 10), make([]byte, headerLength)} {
		if _, err := ours.decrypt(short); err == nil {
			t.Errorf("a push of %d octets decrypted", len(short))
		}
	}
}
