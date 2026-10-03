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
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
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
		emails = append(emails, map[string]any{
			"id":         e.ID,
			"subject":    e.Subject,
			"from":       from,
			"to":         to,
			"mailboxes":  len(e.MailboxIDs),
			"receivedAt": e.ReceivedAt,
		})
	}
	return emit(map[string]any{"count": len(emails), "emails": emails})
}

// emit writes v to stdout as one line of JSON.
func emit(v any) error {
	return json.NewEncoder(os.Stdout).Encode(v)
}
