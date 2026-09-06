package jmapc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestExpandURITemplate(t *testing.T) {
	tests := []struct {
		tmpl string
		vars map[string]string
		want string
	}{{
		tmpl: "https://x/upload/{accountId}/",
		vars: map[string]string{"accountId": "a1"},
		want: "https://x/upload/a1/",
	}, {
		tmpl: "https://x/dl/{accountId}/{blobId}/{name}?accept={type}",
		vars: map[string]string{"accountId": "a1", "blobId": "b2", "name": "report.pdf", "type": "application/pdf"},
		want: "https://x/dl/a1/b2/report.pdf?accept=application%2Fpdf",
	}, {
		// A filename is arbitrary text, so everything outside the unreserved
		// set has to be escaped, in the path as well as in the query.
		tmpl: "https://x/dl/{name}",
		vars: map[string]string{"name": "a b/c?d&e=f#g.txt"},
		want: "https://x/dl/a%20b%2Fc%3Fd%26e%3Df%23g.txt",
	}, {
		tmpl: "https://x/dl/no-variables",
		vars: map[string]string{},
		want: "https://x/dl/no-variables",
	}}
	for _, tt := range tests {
		got, err := expandURITemplate(tt.tmpl, tt.vars)
		if err != nil {
			t.Errorf("expandURITemplate(%q): %v", tt.tmpl, err)
			continue
		}
		if got != tt.want {
			t.Errorf("expandURITemplate(%q) = %q, want %q", tt.tmpl, got, tt.want)
		}
	}
}

func TestExpandURITemplateErrors(t *testing.T) {
	for _, tt := range []struct {
		tmpl string
		vars map[string]string
	}{
		{"https://x/{accountId", map[string]string{"accountId": "a"}},
		{"https://x/{unknown}", map[string]string{"accountId": "a"}},
	} {
		if _, err := expandURITemplate(tt.tmpl, tt.vars); err == nil {
			t.Errorf("expandURITemplate(%q) succeeded, want an error", tt.tmpl)
		}
	}
}

// blobServer serves the session, the upload endpoint, and the download
// endpoint.
type blobServer struct {
	*testServer
	// uploadedType and uploadedBody record what the last upload carried.
	uploadedType string
	uploadedBody string
	// downloadPath records the path the last download asked for.
	downloadPath string
	// downloadRange records the Range header the last download sent.
	downloadRange string
	// ignoreRange makes the download endpoint answer with the whole blob even
	// where part of it was asked for, as a server that does not implement
	// ranges does.
	ignoreRange bool
	// maxSizeUpload is advertised by the session.
	maxSizeUpload int
}

// blobBody is what the download endpoint serves.
const blobBody = "%PDF-1.4 pretend"

func newBlobServer(t *testing.T) *blobServer {
	t.Helper()
	bs := &blobServer{maxSizeUpload: 1 << 20}
	ts := newTestServer(t)
	bs.testServer = ts

	mux, ok := ts.Config.Handler.(*http.ServeMux)
	if !ok {
		t.Fatalf("test server handler is %T, want *http.ServeMux", ts.Config.Handler)
	}
	// Replace the session so that it advertises the blob endpoints too.
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		ts.sessionHits.Add(1)
		fmt.Fprintf(w, `{
		  "capabilities": {"urn:ietf:params:jmap:core": {"maxSizeUpload": %d}},
		  "accounts": {"a1": {"name": "someone", "isPersonal": true}},
		  "primaryAccounts": {"urn:ietf:params:jmap:mail": "a1"},
		  "username": "someone",
		  "apiUrl": %q,
		  "uploadUrl": %q,
		  "downloadUrl": %q,
		  "state": "sess1"
		}`, bs.maxSizeUpload, ts.URL+"/api", ts.URL+"/upload/{accountId}",
			ts.URL+"/dl/{accountId}/{blobId}/{name}?accept={type}")
	})
	mux.HandleFunc("/upload/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bs.uploadedType = r.Header.Get("Content-Type")
		bs.uploadedBody = string(body)
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"accountId":"a1","blobId":"blob9","type":%q,"size":%d}`,
			r.Header.Get("Content-Type"), len(body))
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		bs.downloadPath = r.URL.RequestURI()
		bs.downloadRange = r.Header.Get("Range")
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="report.pdf"`)
		if bs.downloadRange == "" || bs.ignoreRange {
			fmt.Fprint(w, blobBody)
			return
		}
		from, to := parseTestRange(t, bs.downloadRange, len(blobBody))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, to, len(blobBody)))
		w.WriteHeader(http.StatusPartialContent)
		fmt.Fprint(w, blobBody[from:to+1])
	})
	return bs
}

// client returns a client whose session advertises the blob endpoints.
func (bs *blobServer) client(opts ...Option) *Client {
	return New(bs.URL+"/session", opts...)
}

// TestDownloadWithRelativeSessionURLs checks that a downloadUrl sent as a
// path, template braces included, still resolves and expands correctly, and
// is not mangled into percent-escaped braces along the way.
func TestDownloadWithRelativeSessionURLs(t *testing.T) {
	var downloadPath string
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{
		  "capabilities": {"urn:ietf:params:jmap:core": {}},
		  "accounts": {"a1": {"name": "someone", "isPersonal": true}},
		  "primaryAccounts": {}, "username": "u",
		  "apiUrl": "/api",
		  "downloadUrl": "/dl/{accountId}/{blobId}/{name}?accept={type}",
		  "state": "s"
		}`)
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		downloadPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/pdf")
		fmt.Fprint(w, "%PDF-1.4 pretend")
	})

	c := New(srv.URL + "/session")
	blob, err := c.Download(context.Background(), "a1", "blob9", &DownloadOptions{
		Name: "report.pdf",
		Type: "application/pdf",
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	blob.Close()

	want := "/dl/a1/blob9/report.pdf?accept=application%2Fpdf"
	if downloadPath != want {
		t.Errorf("downloaded from %q, want %q", downloadPath, want)
	}
}

func TestUpload(t *testing.T) {
	bs := newBlobServer(t)
	info, err := bs.client().Upload(context.Background(), "a1", "text/plain", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if bs.uploadedBody != "hello" {
		t.Errorf("the server received %q, want %q", bs.uploadedBody, "hello")
	}
	if bs.uploadedType != "text/plain" {
		t.Errorf("Content-Type = %q, want text/plain", bs.uploadedType)
	}
	if info.BlobID != "blob9" || info.Size != 5 {
		t.Errorf("blob info = %+v, want blob9 of 5 octets", info)
	}
}

// TestUploadDefaultsContentType checks that a blob offered with no type is
// still sent with one, because the endpoint requires it.
func TestUploadDefaultsContentType(t *testing.T) {
	bs := newBlobServer(t)
	if _, err := bs.client().Upload(context.Background(), "a1", "", strings.NewReader("x")); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if bs.uploadedType != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want application/octet-stream", bs.uploadedType)
	}
}

// TestUploadRejectsOversizedBlob checks that a blob the session already shows
// is too large never leaves the machine.
func TestUploadRejectsOversizedBlob(t *testing.T) {
	bs := newBlobServer(t)
	bs.maxSizeUpload = 4

	_, err := bs.client().Upload(context.Background(), "a1", "text/plain", strings.NewReader("far too long"))
	if err == nil {
		t.Fatal("expected an error")
	}
	var reqErr *RequestError
	if !errors.As(err, &reqErr) {
		t.Fatalf("error is %T, want *RequestError", err)
	}
	if reqErr.Limit != "maxSizeUpload" {
		t.Errorf("limit = %q, want maxSizeUpload", reqErr.Limit)
	}
	if bs.uploadedBody != "" {
		t.Errorf("the blob was sent anyway: %q", bs.uploadedBody)
	}
}

func TestUploadNeedsAccountID(t *testing.T) {
	bs := newBlobServer(t)
	if _, err := bs.client().Upload(context.Background(), "", "text/plain", strings.NewReader("x")); err == nil {
		t.Error("expected an error for an empty account id")
	}
}

func TestDownload(t *testing.T) {
	bs := newBlobServer(t)
	blob, err := bs.client().Download(context.Background(), "a1", "blob9", &DownloadOptions{
		Name: "report.pdf",
		Type: "application/pdf",
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer blob.Close()

	want := "/dl/a1/blob9/report.pdf?accept=application%2Fpdf"
	if bs.downloadPath != want {
		t.Errorf("downloaded from %q, want %q", bs.downloadPath, want)
	}
	if blob.Type != "application/pdf" {
		t.Errorf("Type = %q, want application/pdf", blob.Type)
	}
	if blob.Name != "report.pdf" {
		t.Errorf("Name = %q, want report.pdf", blob.Name)
	}
	body, err := io.ReadAll(blob)
	if err != nil {
		t.Fatalf("reading the blob: %v", err)
	}
	if !strings.HasPrefix(string(body), "%PDF") {
		t.Errorf("body = %q", body)
	}
}

// TestDownloadWithoutOptions checks that the blob id stands in for the name and
// that a type is always asked for, since the template has no way to leave a
// variable out.
func TestDownloadWithoutOptions(t *testing.T) {
	bs := newBlobServer(t)
	blob, err := bs.client().Download(context.Background(), "a1", "blob9", nil)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	blob.Close()
	want := "/dl/a1/blob9/blob9?accept=application%2Foctet-stream"
	if bs.downloadPath != want {
		t.Errorf("downloaded from %q, want %q", bs.downloadPath, want)
	}
}

func TestDownloadError(t *testing.T) {
	bs := newBlobServer(t)
	mux := bs.Config.Handler.(*http.ServeMux)
	mux.HandleFunc("/dl/a1/missing/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"type":"urn:ietf:params:jmap:error:notFound","status":404,"detail":"no such blob"}`)
	})
	_, err := bs.client().Download(context.Background(), "a1", "missing", nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	var reqErr *RequestError
	if !errors.As(err, &reqErr) {
		t.Fatalf("error is %T (%v), want *RequestError", err, err)
	}
	if reqErr.Status != http.StatusNotFound || reqErr.Detail != "no such blob" {
		t.Errorf("error = %+v", reqErr)
	}
}

func TestClientPrimaryAccountID(t *testing.T) {
	ts := newTestServer(t)
	got, err := ts.client().PrimaryAccountID(context.Background(), CapabilityMail)
	if err != nil {
		t.Fatalf("PrimaryAccountID: %v", err)
	}
	if got != "a1" {
		t.Errorf("primary account = %q, want a1", got)
	}
}

func TestFilenameFrom(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{`attachment; filename="report.pdf"`, "report.pdf"},
		{"attachment", ""},
		{"nonsense; ;", ""},
	}
	for _, tt := range tests {
		if got := filenameFrom(tt.in); got != tt.want {
			t.Errorf("filenameFrom(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// parseTestRange reads the "bytes=a-b" a test sent, with an open end meaning
// the rest of the blob.
func parseTestRange(t *testing.T, header string, size int) (int, int) {
	t.Helper()
	span, ok := strings.CutPrefix(header, "bytes=")
	if !ok {
		t.Fatalf("the client sent a Range of %q", header)
	}
	first, last, ok := strings.Cut(span, "-")
	if !ok {
		t.Fatalf("the client sent a Range of %q", header)
	}
	from, err := strconv.Atoi(first)
	if err != nil {
		t.Fatalf("the client sent a Range of %q", header)
	}
	if last == "" {
		return from, size - 1
	}
	to, err := strconv.Atoi(last)
	if err != nil {
		t.Fatalf("the client sent a Range of %q", header)
	}
	return from, to
}

func TestDownloadARange(t *testing.T) {
	bs := newBlobServer(t)
	blob, err := bs.client().Download(context.Background(), "a1", "blob9", &DownloadOptions{From: 5, Length: 4})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer blob.Close()

	if bs.downloadRange != "bytes=5-8" {
		t.Errorf("the client sent a Range of %q, want bytes=5-8", bs.downloadRange)
	}
	body, err := io.ReadAll(blob)
	if err != nil {
		t.Fatalf("reading the blob: %v", err)
	}
	if string(body) != blobBody[5:9] {
		t.Errorf("body = %q, want %q", body, blobBody[5:9])
	}
	if blob.Range == nil {
		t.Fatal("the blob reports no range")
	}
	if blob.Range.From != 5 || blob.Range.To != 8 || blob.Range.Total != int64(len(blobBody)) {
		t.Errorf("range = %+v, want 5-8 of %d", *blob.Range, len(blobBody))
	}
}

// A download resumed after an interruption asks for the rest of the blob
// without knowing how much of it is left.
func TestDownloadFromAnOffsetToTheEnd(t *testing.T) {
	bs := newBlobServer(t)
	blob, err := bs.client().Download(context.Background(), "a1", "blob9", &DownloadOptions{From: 9})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer blob.Close()

	if bs.downloadRange != "bytes=9-" {
		t.Errorf("the client sent a Range of %q, want bytes=9-", bs.downloadRange)
	}
	body, err := io.ReadAll(blob)
	if err != nil {
		t.Fatalf("reading the blob: %v", err)
	}
	if string(body) != blobBody[9:] {
		t.Errorf("body = %q, want %q", body, blobBody[9:])
	}
}

// JMAP defines no range for the download endpoint, so a server may answer with
// the whole blob. Returning it would leave the caller to write the start of the
// blob at the offset it asked to continue from.
func TestADownloadWhoseRangeWasIgnoredFails(t *testing.T) {
	bs := newBlobServer(t)
	bs.ignoreRange = true
	_, err := bs.client().Download(context.Background(), "a1", "blob9", &DownloadOptions{From: 5, Length: 4})
	if err == nil {
		t.Fatal("Download succeeded where the server ignored the range")
	}
	if !strings.Contains(err.Error(), "ignored the range") {
		t.Errorf("error = %v, want it to report the range was ignored", err)
	}
}

func TestADownloadWithoutARangeSendsNone(t *testing.T) {
	bs := newBlobServer(t)
	blob, err := bs.client().Download(context.Background(), "a1", "blob9", &DownloadOptions{Name: "report.pdf"})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer blob.Close()
	if bs.downloadRange != "" {
		t.Errorf("the client sent a Range of %q, want none", bs.downloadRange)
	}
	if blob.Range != nil {
		t.Errorf("the blob reports a range of %+v, want none", *blob.Range)
	}
}

func TestADownloadRangeCountsFromTheStart(t *testing.T) {
	bs := newBlobServer(t)
	for _, opts := range []*DownloadOptions{{From: -1}, {Length: -1}} {
		if _, err := bs.client().Download(context.Background(), "a1", "blob9", opts); err == nil {
			t.Errorf("Download(%+v) succeeded, want an error", *opts)
		}
	}
}

func TestParseContentRange(t *testing.T) {
	for _, tt := range []struct {
		header string
		want   BlobRange
	}{
		{"bytes 0-99/1234", BlobRange{From: 0, To: 99, Total: 1234}},
		{"bytes 5-8/*", BlobRange{From: 5, To: 8, Total: -1}},
	} {
		got, err := parseContentRange(tt.header)
		if err != nil {
			t.Errorf("parseContentRange(%q): %v", tt.header, err)
			continue
		}
		if *got != tt.want {
			t.Errorf("parseContentRange(%q) = %+v, want %+v", tt.header, *got, tt.want)
		}
	}
	for _, header := range []string{"", "items 0-99/1234", "bytes 0-99", "bytes x-99/1234", "bytes 0-99/many"} {
		if _, err := parseContentRange(header); err == nil {
			t.Errorf("parseContentRange(%q) succeeded, want an error", header)
		}
	}
}
