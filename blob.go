package jmapc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

// BlobInfo describes a blob the server has taken in, as returned by the upload
// endpoint of RFC 8620, Section 6.1.
type BlobInfo struct {
	// AccountID is the account the blob was uploaded to.
	AccountID ID `json:"accountId"`
	// BlobID is the id to refer to the blob by, such as in the blobId of an
	// EmailBodyPart.
	BlobID ID `json:"blobId"`
	// Type is the media type the server recorded for the blob, which may
	// differ from the one that was offered.
	Type string `json:"type"`
	// Size is the size of the blob in octets.
	Size UnsignedInt `json:"size"`
}

// Blob is a blob being downloaded. The caller must close it.
type Blob struct {
	// ReadCloser carries the blob's content.
	io.ReadCloser
	// Type is the media type the server served the blob as.
	Type string
	// Size is the size in octets, or -1 when the server did not say.
	Size int64
	// Name is the filename from the Content-Disposition header, if the server
	// sent one. It is whatever the server said, which may be a path rather
	// than a name: take the base of it before writing anything under it.
	Name string
}

// DownloadOptions are the things a download may ask the server for beyond the
// blob itself.
type DownloadOptions struct {
	// Name is the filename to ask the server to offer the blob under. Servers
	// use it in the Content-Disposition header.
	Name string
	// Type is the media type to ask the server to serve the blob as. Servers
	// use it in the Content-Type header, and may refuse a type they consider
	// unsafe.
	Type string
}

// PrimaryAccountID returns the id of the account to use by default for a
// capability, fetching the session if it has not been fetched yet.
func (c *Client) PrimaryAccountID(ctx context.Context, capability string) (ID, error) {
	s, err := c.Session(ctx)
	if err != nil {
		return "", err
	}
	return s.PrimaryAccountID(capability)
}

// Upload sends a blob to the account and returns the id to refer to it by. A
// blob is untethered until something points at it, such as an Email/set that
// names it in a body part; servers are free to discard one nothing refers to.
//
// The contentType is a hint. The server records what it decides the blob is,
// which is what BlobInfo reports back.
func (c *Client) Upload(ctx context.Context, accountID ID, contentType string, body io.Reader) (*BlobInfo, error) {
	if accountID == "" {
		return nil, fmt.Errorf("jmapc: uploading a blob needs an account id")
	}
	s, err := c.Session(ctx)
	if err != nil {
		return nil, err
	}
	if s.UploadURL == "" {
		return nil, fmt.Errorf("jmapc: the session advertises no uploadUrl")
	}
	url, err := expandURITemplate(s.UploadURL, map[string]string{"accountId": string(accountID)})
	if err != nil {
		return nil, fmt.Errorf("jmapc: expanding uploadUrl: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("jmapc: building upload request: %w", err)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	if err := c.checkUploadSize(ctx, req.ContentLength); err != nil {
		return nil, err
	}

	resp, err := c.send(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, c.requestError(resp)
	}
	var info BlobInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("jmapc: decoding upload response: %w", err)
	}
	return &info, nil
}

// checkUploadSize rejects an upload the session already shows is too large,
// when the size is known ahead of time.
func (c *Client) checkUploadSize(ctx context.Context, size int64) error {
	if !c.strict || size <= 0 {
		return nil
	}
	s, err := c.Session(ctx)
	if err != nil {
		return err
	}
	core, err := s.Core()
	if err != nil {
		return nil
	}
	if max := core.MaxSizeUpload; max > 0 && UnsignedInt(size) > max {
		return &RequestError{
			Type:   ErrTypeLimit,
			Limit:  "maxSizeUpload",
			Detail: fmt.Sprintf("the blob is %d octets, and the server accepts at most %d", size, max),
		}
	}
	return nil
}

// Download fetches a blob's content. The caller must close the returned blob.
//
// A blob has no type of its own: the server serves it as whatever the download
// asks for, within what it considers safe. Pass the type from the body part
// that referred to the blob.
func (c *Client) Download(ctx context.Context, accountID, blobID ID, opts *DownloadOptions) (*Blob, error) {
	if accountID == "" {
		return nil, fmt.Errorf("jmapc: downloading a blob needs an account id")
	}
	if blobID == "" {
		return nil, fmt.Errorf("jmapc: downloading a blob needs a blob id")
	}
	s, err := c.Session(ctx)
	if err != nil {
		return nil, err
	}
	if s.DownloadURL == "" {
		return nil, fmt.Errorf("jmapc: the session advertises no downloadUrl")
	}
	if opts == nil {
		opts = &DownloadOptions{}
	}
	name := opts.Name
	if name == "" {
		name = string(blobID)
	}
	mediaType := opts.Type
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	url, err := expandURITemplate(s.DownloadURL, map[string]string{
		"accountId": string(accountID),
		"blobId":    string(blobID),
		"name":      name,
		"type":      mediaType,
	})
	if err != nil {
		return nil, fmt.Errorf("jmapc: expanding downloadUrl: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("jmapc: building download request: %w", err)
	}
	resp, err := c.send(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, c.requestError(resp)
	}
	return &Blob{
		ReadCloser: resp.Body,
		Type:       resp.Header.Get("Content-Type"),
		Size:       resp.ContentLength,
		Name:       filenameFrom(resp.Header.Get("Content-Disposition")),
	}, nil
}

// filenameFrom pulls the filename out of a Content-Disposition header, and
// returns an empty string when there is none to be had.
func filenameFrom(disposition string) string {
	if disposition == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(disposition)
	if err != nil {
		return ""
	}
	return params["filename"]
}

// expandURITemplate fills in a level 1 URI template as defined in RFC 6570,
// which is the form the session's upload and download URLs take.
func expandURITemplate(tmpl string, vars map[string]string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(tmpl); {
		if tmpl[i] != '{' {
			b.WriteByte(tmpl[i])
			i++
			continue
		}
		end := strings.IndexByte(tmpl[i:], '}')
		if end < 0 {
			return "", fmt.Errorf("unclosed %q in %q", "{", tmpl)
		}
		name := tmpl[i+1 : i+end]
		value, ok := vars[name]
		if !ok {
			return "", fmt.Errorf("no value for the variable %q in %q", name, tmpl)
		}
		b.WriteString(escapeTemplateValue(value))
		i += end + 1
	}
	return b.String(), nil
}

// escapeTemplateValue percent-encodes everything outside the unreserved set of
// RFC 3986, which is what a level 1 template expansion calls for. The escaping
// in net/url is not quite this: it leaves some reserved characters alone
// depending on which part of a URL it is escaping for, and a blob id or a
// filename goes into both the path and the query.
func escapeTemplateValue(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isUnreserved(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0x0f])
	}
	return b.String()
}

// isUnreserved reports whether a byte may appear in a URI without escaping.
func isUnreserved(c byte) bool {
	switch {
	case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		return true
	case c == '-', c == '.', c == '_', c == '~':
		return true
	}
	return false
}
