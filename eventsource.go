package jmapc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// StateChange is the event a JMAP server pushes when something in an account
// changes, as defined in RFC 8620, Section 7.1. It says only that a type has
// moved on, not what changed; the client follows up with a /changes call.
type StateChange struct {
	// Type is the object type, always "StateChange".
	Type string `json:"@type"`
	// Changed maps an account id to the new state string of each type that
	// has changed within it, keyed by type name such as "Email".
	Changed map[ID]map[string]string `json:"changed"`
}

// StateOf returns the new state of a type in an account, and whether the event
// mentioned it at all.
func (s *StateChange) StateOf(accountID ID, typeName string) (string, bool) {
	states, ok := s.Changed[accountID]
	if !ok {
		return "", false
	}
	state, ok := states[typeName]
	return state, ok
}

// PushVerification is what the server posts to a push subscription's URL as
// soon as it is created, before it will send anything else. The client writes
// the code back with a PushSubscription/set, which is what proves it controls
// the URL: without that step a subscription could be pointed at a third party
// and used to flood them.
//
// It arrives at the URL the client registered, not through the API, which is
// why it is here rather than in the generated types.
type PushVerification struct {
	// Type is the object type, always "PushVerification".
	Type string `json:"@type"`
	// PushSubscriptionID is the id of the subscription that was created.
	PushSubscriptionID ID `json:"pushSubscriptionId"`
	// VerificationCode is the code to write back to that subscription.
	VerificationCode string `json:"verificationCode"`
}

// EventSourceOptions say what to ask the push endpoint for.
type EventSourceOptions struct {
	// Types are the object types to be told about, such as "Email". Leave it
	// empty to hear about every type.
	Types []string
	// Ping asks the server to send a comment every so often, so that a
	// connection dropped by something in the middle is noticed rather than
	// hanging. Servers clamp it to a range of their own choosing. Zero asks
	// for no pings.
	Ping time.Duration
	// CloseAfterState asks the server to close the connection after the first
	// event, which suits a client that only wants to know it has fallen
	// behind.
	CloseAfterState bool
	// LastEventID resumes from a known point: the server sends the events
	// since that one, so that a reconnection does not miss anything. Pass the
	// LastEventID of the stream that dropped.
	LastEventID string
}

// EventStream is an open connection to a server's push endpoint. It is not safe
// for concurrent use, and the caller must close it.
type EventStream struct {
	body io.ReadCloser
	scan *bufio.Scanner
	// lastEventID is the id of the most recent event, which a reconnection
	// resumes from.
	lastEventID string
}

// EventSource opens a connection to the server's push endpoint, as described in
// RFC 8620, Section 7.3.
//
// The connection stays open until it is closed or the server drops it, which it
// will: a stream is not a subscription that outlives the network. Treat an error
// from Next as a signal to reconnect, passing LastEventID so that nothing is
// missed in between.
func (c *Client) EventSource(ctx context.Context, opts *EventSourceOptions) (*EventStream, error) {
	if opts == nil {
		opts = &EventSourceOptions{}
	}
	s, err := c.Session(ctx)
	if err != nil {
		return nil, err
	}
	if s.EventSourceURL == "" {
		return nil, fmt.Errorf("jmapc: the session advertises no eventSourceUrl")
	}

	types := "*"
	if len(opts.Types) > 0 {
		types = strings.Join(opts.Types, ",")
	}
	closeAfter := "no"
	if opts.CloseAfterState {
		closeAfter = "state"
	}
	url, err := expandURITemplate(s.EventSourceURL, map[string]string{
		"types":      types,
		"closeafter": closeAfter,
		"ping":       strconv.Itoa(int(opts.Ping / time.Second)),
	})
	if err != nil {
		return nil, fmt.Errorf("jmapc: expanding eventSourceUrl: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("jmapc: building event source request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	if opts.LastEventID != "" {
		req.Header.Set("Last-Event-ID", opts.LastEventID)
	}

	resp, err := c.send(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, c.requestError(resp)
	}
	scan := bufio.NewScanner(resp.Body)
	scan.Buffer(make([]byte, 0, 64<<10), maxEventBytes)
	return &EventStream{
		body:        resp.Body,
		scan:        scan,
		lastEventID: opts.LastEventID,
	}, nil
}

// maxEventBytes caps how long a single line of an event may be, so that a
// server that never sends a newline cannot exhaust memory.
const maxEventBytes = 1 << 20

// Next blocks until the server pushes the next state change and returns it.
// Pings and any other event types the server sends are consumed and skipped.
//
// It returns io.EOF when the server closes the stream, which it does after the
// first event when CloseAfterState was set, and may do at any time otherwise.
func (s *EventStream) Next() (*StateChange, error) {
	var event, data strings.Builder
	flushable := false

	for {
		if !s.scan.Scan() {
			if err := s.scan.Err(); err != nil {
				return nil, fmt.Errorf("jmapc: reading the event stream: %w", err)
			}
			return nil, io.EOF
		}
		line := strings.TrimSuffix(s.scan.Text(), "\r")

		// A blank line ends an event. An event carrying no data is a comment
		// or a keep-alive, and there is nothing to hand back.
		if line == "" {
			if !flushable || data.Len() == 0 {
				event.Reset()
				data.Reset()
				flushable = false
				continue
			}
			name := event.String()
			payload := data.String()
			event.Reset()
			data.Reset()
			flushable = false
			if name != "" && name != "state" {
				// Something the specification does not define, such as a ping.
				continue
			}
			var change StateChange
			if err := json.Unmarshal([]byte(payload), &change); err != nil {
				return nil, fmt.Errorf("jmapc: decoding a %q event: %w", name, err)
			}
			return &change, nil
		}

		// A line beginning with a colon is a comment, which is how a server
		// keeps the connection warm without sending an event.
		if strings.HasPrefix(line, ":") {
			continue
		}

		field, value, found := strings.Cut(line, ":")
		if !found {
			field, value = line, ""
		}
		value = strings.TrimPrefix(value, " ")
		flushable = true
		switch field {
		case "event":
			event.Reset()
			event.WriteString(value)
		case "data":
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(value)
		case "id":
			s.lastEventID = value
		}
	}
}

// LastEventID returns the id of the most recent event, to resume from after the
// stream drops.
func (s *EventStream) LastEventID() string { return s.lastEventID }

// Close ends the connection.
func (s *EventStream) Close() error { return s.body.Close() }
