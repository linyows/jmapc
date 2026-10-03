// Command driver is the jmapc side of the end-to-end tests. Each subcommand
// makes one call through the runtime or the generated client and prints what
// came back as JSON, which the probe workflows compare with what the server
// says over a path that does not go through jmapc.
//
// It reads the server and the account from the environment:
//
//	JMAPC_E2E_SESSION_URL  the session URL, such as http://localhost:18080/.well-known/jmap
//	JMAPC_E2E_USER         the account to log in as
//	JMAPC_E2E_PASSWORD     its password
//
// Usage:
//
//	driver session
//	driver find-emails -subject <phrase>
//	driver import-email -subject <subject>
//	driver blob -data <content> -from <offset> -length <octets>
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/linyows/jmapc"
	"github.com/linyows/jmapc/e2e/client"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "driver:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("no subcommand given")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := newClient()
	if err != nil {
		return err
	}
	switch args[0] {
	case "session":
		return session(ctx, c)
	case "find-emails":
		fs := flag.NewFlagSet("find-emails", flag.ContinueOnError)
		subject := fs.String("subject", "", "phrase the subject contains")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return findEmails(ctx, c, *subject)
	case "import-email":
		fs := flag.NewFlagSet("import-email", flag.ContinueOnError)
		subject := fs.String("subject", "", "subject of the message to import")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return importEmail(ctx, c, *subject)
	case "blob":
		fs := flag.NewFlagSet("blob", flag.ContinueOnError)
		data := fs.String("data", "", "content to upload")
		from := fs.Int64("from", 0, "first octet of the range to download")
		length := fs.Int64("length", 0, "octets in the range to download")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return blob(ctx, c, *data, *from, *length)
	default:
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

// newClient builds a client for the account the environment names.
func newClient() (*jmapc.Client, error) {
	url, user, password := os.Getenv("JMAPC_E2E_SESSION_URL"), os.Getenv("JMAPC_E2E_USER"), os.Getenv("JMAPC_E2E_PASSWORD")
	if url == "" || user == "" || password == "" {
		return nil, errors.New("JMAPC_E2E_SESSION_URL, JMAPC_E2E_USER and JMAPC_E2E_PASSWORD must all be set")
	}
	return jmapc.New(url, jmapc.WithBasicAuth(user, password)), nil
}

// session prints the parts of the session a client depends on, as jmapc read
// them: the URLs after resolving them, the account it would use for mail, and
// the limits it would hold requests to.
func session(ctx context.Context, c *jmapc.Client) error {
	s, err := c.Session(ctx)
	if err != nil {
		return err
	}
	mail, err := s.PrimaryAccountID(jmapc.CapabilityMail)
	if err != nil {
		return err
	}
	core, err := s.Core()
	if err != nil {
		return err
	}
	capabilities := make([]string, 0, len(s.Capabilities))
	for uri := range s.Capabilities {
		capabilities = append(capabilities, uri)
	}
	slices.Sort(capabilities)
	return emit(map[string]any{
		"username":          s.Username,
		"apiUrl":            s.APIURL,
		"downloadUrl":       s.DownloadURL,
		"uploadUrl":         s.UploadURL,
		"eventSourceUrl":    s.EventSourceURL,
		"mailAccountId":     mail,
		"capabilities":      capabilities,
		"maxCallsInRequest": core.MaxCallsInRequest,
		"maxObjectsInGet":   core.MaxObjectsInGet,
	})
}

// findEmails prints the emails whose subject contains a phrase, through the
// client generated from requests/FindEmailsBySubject.jmap.json.
func findEmails(ctx context.Context, c *jmapc.Client, subject string) error {
	if subject == "" {
		return errors.New("find-emails needs -subject")
	}
	resp, err := client.FindEmailsBySubject(ctx, c, client.FindEmailsBySubjectParams{Subject: subject})
	if err != nil {
		return err
	}
	emails := make([]map[string]any, 0, len(resp.List))
	for _, e := range resp.List {
		from := make([]string, 0, len(e.From))
		for _, a := range e.From {
			from = append(from, a.Email)
		}
		to := make([]string, 0, len(e.To))
		for _, a := range e.To {
			to = append(to, a.Email)
		}
		keywords := make([]string, 0, len(e.Keywords))
		for k, set := range e.Keywords {
			if set {
				keywords = append(keywords, k)
			}
		}
		slices.Sort(keywords)
		emails = append(emails, map[string]any{
			"id":         e.ID,
			"keywords":   keywords,
			"subject":    e.Subject,
			"from":       from,
			"to":         to,
			"mailboxes":  len(e.MailboxIDs),
			"receivedAt": e.ReceivedAt,
		})
	}
	return emit(map[string]any{"count": len(emails), "emails": emails})
}

// importEmail uploads a message and imports it into the inbox through the
// client generated from requests/ImportEmail.jmap.json, which flags it.
func importEmail(ctx context.Context, c *jmapc.Client, subject string) error {
	if subject == "" {
		return errors.New("import-email needs -subject")
	}
	s, err := c.Session(ctx)
	if err != nil {
		return err
	}
	account, err := s.PrimaryAccountID(jmapc.CapabilityMail)
	if err != nil {
		return err
	}
	boxes, err := client.FindMailboxByRole(ctx, c, client.FindMailboxByRoleParams{Role: "inbox"})
	if err != nil {
		return err
	}
	if len(boxes.List) != 1 {
		return fmt.Errorf("found %d mailboxes with the inbox role, want 1", len(boxes.List))
	}
	inbox := boxes.List[0].ID

	message := strings.Join([]string{
		"From: jmapc <jmapc@example.test>",
		"To: " + s.Username,
		"Subject: " + subject,
		"Date: " + time.Now().UTC().Format(time.RFC1123Z),
		"Message-ID: <" + fmt.Sprint(time.Now().UnixNano()) + "@jmapc.example.test>",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Imported through jmapc.",
		"",
	}, "\r\n")
	uploaded, err := c.Upload(ctx, account, "message/rfc822", strings.NewReader(message))
	if err != nil {
		return err
	}
	resp, err := client.ImportEmail(ctx, c, client.ImportEmailParams{BlobID: uploaded.BlobID, MailboxID: inbox})
	if err != nil {
		return err
	}
	imported, ok := resp.Import.Created["imported"]
	if !ok {
		return fmt.Errorf("the import reported no email created: %+v", resp.Import.NotCreated)
	}
	return emit(map[string]any{"id": imported.ID, "blobId": uploaded.BlobID, "mailboxId": inbox})
}

// blob uploads data, then downloads the whole of it and a range of it, and
// prints all three so that the workflow can download them again directly.
//
// JMAP does not define ranges on the download endpoint, and a server that
// ignores one answers with the whole blob, which jmapc refuses to return. The
// refusal is printed as rangeError rather than failing the command, so that
// the workflow can check it against what the server does with the same range.
func blob(ctx context.Context, c *jmapc.Client, data string, from, length int64) error {
	if data == "" || length <= 0 {
		return errors.New("blob needs -data and a -length above zero")
	}
	s, err := c.Session(ctx)
	if err != nil {
		return err
	}
	account, err := s.PrimaryAccountID(jmapc.CapabilityMail)
	if err != nil {
		return err
	}
	uploaded, err := c.Upload(ctx, account, "text/plain", strings.NewReader(data))
	if err != nil {
		return err
	}
	whole, err := download(ctx, c, account, uploaded.BlobID, nil)
	if err != nil {
		return err
	}
	out := map[string]any{
		"accountId": account,
		"blobId":    uploaded.BlobID,
		"size":      uploaded.Size,
		"whole":     whole.content,
	}
	part, err := download(ctx, c, account, uploaded.BlobID, &jmapc.DownloadOptions{From: from, Length: length})
	switch {
	case err != nil:
		out["rangeError"] = err.Error()
	case part.Range == nil:
		return errors.New("the ranged download reported no range")
	default:
		out["part"] = part.content
		out["rangeFrom"] = part.Range.From
		out["rangeTo"] = part.Range.To
		out["rangeTotal"] = part.Range.Total
	}
	return emit(out)
}

// downloaded is a blob read to the end.
type downloaded struct {
	content string
	Range   *jmapc.BlobRange
}

func download(ctx context.Context, c *jmapc.Client, account, id jmapc.ID, opts *jmapc.DownloadOptions) (*downloaded, error) {
	b, err := c.Download(ctx, account, id, opts)
	if err != nil {
		return nil, err
	}
	defer b.Close()
	content, err := io.ReadAll(b)
	if err != nil {
		return nil, err
	}
	return &downloaded{content: string(content), Range: b.Range}, nil
}

// emit writes v to stdout as one line of JSON.
func emit(v any) error {
	return json.NewEncoder(os.Stdout).Encode(v)
}
