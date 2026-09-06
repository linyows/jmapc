package jmapc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

// BlobInfo describes a blob the server has accepted, as returned by the upload
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
//
// The content is read from the server as it is read from here, so a blob
// larger than memory is written straight to a file without being held.
type Blob struct {
	// ReadCloser carries the blob's content.
	io.ReadCloser
	// Type is the media type the server served the blob as.
	Type string
	// Size is the size in octets of what is being read, which is the size of
	// the part where a range was requested, or -1 where the server did not
	// report it.
	Size int64
	// Range is the part of the blob the server returned, and is nil where the
	// whole of it was requested.
	Range *BlobRange
	// Name is the filename from the Content-Disposition header, if the server
	// sent one. It is whatever the server sent, which may be a path rather
	// than a name: take the base of it before writing anything under it.
	Name string
}

// DownloadOptions are the parameters a download may send beyond the blob id.
type DownloadOptions struct {
	// Name is the filename to request the blob be offered under. Servers use
	// it in the Content-Disposition header.
	Name string
	// Type is the media type to request the blob be served as. Servers use it
	// in the Content-Type header, and may refuse a type they consider
	// unsafe.
	Type string
	// From is the first octet to fetch, counted from the start of the blob.
	// Zero starts at the beginning.
	//
	// From and Length are sent as an HTTP Range header. JMAP does not define
	// one for the download endpoint, so a server is free to ignore it and
	// return the whole blob; where that happens the download fails rather than
	// returning content the caller would write at the wrong offset.
	From int64
	// Length is how many octets to fetch, and zero fetches to the end of the
	// blob.
	Length int64
}

// BlobRange is the part of a blob a server returned, as its Content-Range
// header reported it.
type BlobRange struct {
	// From and To are the first and last octet returned, counted from the
	// start of the blob and both included.
	From, To int64
	// Total is the size of the whole blob, or -1 where the server did not
	// report it.
	Total int64
}

// header renders the range as the value of an HTTP Range header.
func (o *DownloadOptions) header() string {
	switch {
	case o.From == 0 && o.Length == 0:
		return ""
	case o.Length == 0:
		return fmt.Sprintf("bytes=%d-", o.From)
	default:
		return fmt.Sprintf("bytes=%d-%d", o.From, o.From+o.Length-1)
	}
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
// blob is unreferenced until something points at it, such as an Email/set that
// names it in a body part, and a server may discard a blob nothing refers to.
//
// The contentType is a hint. The server records the type it determines for the
// blob, which is what BlobInfo reports.
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

	// The server states how many uploads it accepts at once, and this is one.
	release, err := c.uploads.hold(ctx, c.limit(func(core *CoreCapability) UnsignedInt {
		return core.MaxConcurrentUpload
	}), c.waiting(ctx, KindUpload))
	if err != nil {
		return nil, err
	}
	defer release()

	resp, err := c.sendWithRetry(req, KindUpload)
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
// A blob has no type of its own: the server serves it as whatever type the
// download requests, within what it considers safe. Pass the type from the body
// part that referred to the blob.
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

	if opts.From < 0 || opts.Length < 0 {
		return nil, fmt.Errorf("jmapc: a download range counts octets from the start of the blob, and neither part of it is negative")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("jmapc: building download request: %w", err)
	}
	wanted := opts.header()
	if wanted != "" {
		req.Header.Set("Range", wanted)
	}
	resp, err := c.sendWithRetry(req, KindDownload)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		defer resp.Body.Close()
		return nil, c.requestError(resp)
	}
	if wanted != "" && resp.StatusCode == http.StatusOK {
		// The whole blob, where part of it was asked for. Returning it would
		// leave the caller to write the start of the blob at the offset it
		// asked to continue from.
		resp.Body.Close()
		return nil, fmt.Errorf("jmapc: the server ignored the range %q and answered with the whole blob", wanted)
	}
	blob := &Blob{
		ReadCloser: resp.Body,
		Type:       resp.Header.Get("Content-Type"),
		Size:       resp.ContentLength,
		Name:       filenameFrom(resp.Header.Get("Content-Disposition")),
	}
	if wanted != "" {
		part, err := parseContentRange(resp.Header.Get("Content-Range"))
		if err != nil {
			resp.Body.Close()
			return nil, err
		}
		blob.Range = part
	}
	return blob, nil
}

// parseContentRange reads the part of a blob a server reported returning,
// which RFC 9110, Section 14.4 writes as "bytes 0-99/1234", with the size of
// the whole written as "*" where the server does not report it.
func parseContentRange(header string) (*BlobRange, error) {
	value, ok := strings.CutPrefix(strings.TrimSpace(header), "bytes ")
	if !ok {
		return nil, fmt.Errorf("jmapc: the server answered with part of the blob and a Content-Range of %q", header)
	}
	span, total, ok := strings.Cut(value, "/")
	if !ok {
		return nil, fmt.Errorf("jmapc: the server answered with part of the blob and a Content-Range of %q", header)
	}
	first, last, ok := strings.Cut(span, "-")
	if !ok {
		return nil, fmt.Errorf("jmapc: the server answered with part of the blob and a Content-Range of %q", header)
	}
	from, err := strconv.ParseInt(first, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("jmapc: the server answered with part of the blob and a Content-Range of %q", header)
	}
	to, err := strconv.ParseInt(last, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("jmapc: the server answered with part of the blob and a Content-Range of %q", header)
	}
	part := &BlobRange{From: from, To: to, Total: -1}
	if total != "*" {
		size, err := strconv.ParseInt(total, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("jmapc: the server answered with part of the blob and a Content-Range of %q", header)
		}
		part.Total = size
	}
	return part, nil
}

// filenameFrom extracts the filename from a Content-Disposition header, and
// returns an empty string where there is none.
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
