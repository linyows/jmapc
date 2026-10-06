package jmapc

import (
	"context"
	"encoding/json"
	"errors"
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
	// returning content the caller would write at the wrong offset, with an
	// error IsRangeIgnored reports.
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

// rangeIgnoredError is the failure of a download whose range the server
// ignored, answering with the whole blob, or with a part whose bounds do not
// fit the one asked for, or that it does not say the bounds of.
type rangeIgnoredError struct {
	// wanted is the Range header that was sent.
	wanted string
	// partial says the server answered with part of the blob, and answered
	// is the Content-Range it gave the part, empty where it gave none.
	partial  bool
	answered string
}

func (e *rangeIgnoredError) Error() string {
	if e.partial {
		return fmt.Sprintf("jmapc: the server answered the range %q with a part whose Content-Range is %q", e.wanted, e.answered)
	}
	return fmt.Sprintf("jmapc: the server ignored the range %q and answered with the whole blob", e.wanted)
}

// IsRangeIgnored reports whether err is a download that asked for part of a
// blob from a server that answered with the whole of it, or with a part that
// does not fit what was asked: one that starts elsewhere, ends past where it
// was asked to, ends sooner without the blob ending there, or whose bounds the
// server did not state. JMAP does not define
// ranges on the download endpoint, so a server that does not offer them is not
// at fault, and asking again will not change its answer: a caller resuming a
// download downloads the whole blob instead.
func IsRangeIgnored(err error) bool {
	var ignored *rangeIgnoredError
	return errors.As(err, &ignored)
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
		return nil, &rangeIgnoredError{wanted: wanted}
	}
	blob := &Blob{
		ReadCloser: resp.Body,
		Type:       resp.Header.Get("Content-Type"),
		Size:       resp.ContentLength,
		Name:       filenameFrom(resp.Header.Get("Content-Disposition")),
	}
	if wanted != "" {
		answered := resp.Header.Get("Content-Range")
		// The part has to start where it was asked to, or the caller writes
		// it at the wrong offset, and end where it was asked to, or the
		// caller is handed less than it asked for. It ends sooner only where
		// the blob does, and never past the end of the blob. A part whose
		// bounds cannot be read cannot be placed at all.
		// Where the server says how long the body is, it has to be as long as
		// the part it says the body is.
		part, err := parseContentRange(answered)
		if err != nil || part.From != opts.From || part.To < part.From ||
			!endsWhereAsked(part, opts) ||
			(resp.ContentLength >= 0 && resp.ContentLength != part.To-part.From+1) {
			resp.Body.Close()
			return nil, &rangeIgnoredError{wanted: wanted, partial: true, answered: answered}
		}
		blob.Range = part
		// A body sent in chunks says how long it is only by ending, so it is
		// held to the length of the part as it is read.
		if resp.ContentLength < 0 {
			blob.ReadCloser = &partReader{body: resp.Body, left: part.To - part.From + 1, part: answered, wanted: wanted}
		}
	}
	return blob, nil
}

// partReader reads the body of a part whose length the server did not state
// beforehand, failing where it ends sooner or goes on longer than the part it
// says it is: the caller would otherwise take a short body for the whole part,
// or read octets of the blob it did not ask for. A body that goes on longer is
// the server answering the range wrongly, which IsRangeIgnored reports; one
// that ends sooner may as well be a connection that dropped, and is
// io.ErrUnexpectedEOF, which IsTemporary takes as worth another attempt.
type partReader struct {
	body   io.ReadCloser
	left   int64
	part   string
	wanted string
}

func (r *partReader) Read(p []byte) (int, error) {
	if r.left == 0 {
		// The part is all here. What follows has to be the end of the body,
		// and a failure to reach it is the failure it is.
		var extra [1]byte
		for {
			n, err := r.body.Read(extra[:])
			switch {
			case n > 0:
				// The server answered with more than the part it says,
				// and will again: as permanent as a part that starts in
				// the wrong place.
				return 0, &rangeIgnoredError{wanted: r.wanted, partial: true, answered: r.part}
			case errors.Is(err, io.EOF):
				return 0, io.EOF
			case err != nil:
				return 0, err
			}
		}
	}
	if int64(len(p)) > r.left {
		p = p[:r.left]
	}
	n, err := r.body.Read(p)
	r.left -= int64(n)
	if errors.Is(err, io.EOF) && r.left > 0 {
		return n, fmt.Errorf("jmapc: the part ended %d octets short of the %s its Content-Range says: %w",
			r.left, r.part, io.ErrUnexpectedEOF)
	}
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return n, err
}

func (r *partReader) Close() error { return r.body.Close() }

// endsWhereAsked reports whether a part ends where a download asked it to: at
// the end of the range asked for, or sooner where the server says the blob ends
// first. A part ending sooner where the server does not say how long the blob
// is proves nothing about where the blob ends, and is taken at its word only
// where the download asked for the rest of the blob, however long that is.
func endsWhereAsked(part *BlobRange, opts *DownloadOptions) bool {
	if part.Total >= 0 && part.To >= part.Total {
		return false
	}
	if opts.Length > 0 {
		last := opts.From + opts.Length - 1
		switch {
		case part.To > last:
			return false
		case part.To == last:
			return true
		}
		return part.Total >= 0 && part.To == part.Total-1
	}
	return part.Total < 0 || part.To == part.Total-1
}

// parseContentRange reads the part of a blob a server reported returning,
// which RFC 9110, Section 14.4 writes as "bytes 0-99/1234", with the size of
// the whole written as "*" where the server does not report it.
func parseContentRange(header string) (*BlobRange, error) {
	malformed := fmt.Errorf("jmapc: the server answered with part of the blob and a Content-Range of %q", header)
	value, ok := strings.CutPrefix(strings.TrimSpace(header), "bytes ")
	if !ok {
		return nil, malformed
	}
	span, total, ok := strings.Cut(value, "/")
	if !ok {
		return nil, malformed
	}
	first, last, ok := strings.Cut(span, "-")
	if !ok {
		return nil, malformed
	}
	from, ok := octets(first)
	if !ok {
		return nil, malformed
	}
	to, ok := octets(last)
	if !ok {
		return nil, malformed
	}
	part := &BlobRange{From: from, To: to, Total: -1}
	if total != "*" {
		size, ok := octets(total)
		if !ok {
			return nil, malformed
		}
		part.Total = size
	}
	return part, nil
}

// octets reads a count of octets in a Content-Range, which RFC 9110 writes as
// digits and nothing else: no sign, so that "-1" is not taken for the "*" a
// server writes where it does not know the size.
func octets(s string) (int64, bool) {
	if s == "" || strings.TrimLeft(s, "0123456789") != "" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil
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
