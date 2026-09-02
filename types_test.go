package jmapc

import (
	"encoding/json"
	"testing"
	"time"
)

func TestIDValid(t *testing.T) {
	valid := []ID{"a", "A1", "abc-def_123", ID(repeat("a", 255))}
	for _, id := range valid {
		if !id.Valid() {
			t.Errorf("%q.Valid() = false, want true", id)
		}
	}
	invalid := []ID{"", "-leading", "#creation", "has space", "slash/es", ID(repeat("a", 256))}
	for _, id := range invalid {
		if id.Valid() {
			t.Errorf("%q.Valid() = true, want false", id)
		}
	}
}

func repeat(s string, n int) string {
	out := make([]byte, 0, n*len(s))
	for range n {
		out = append(out, s...)
	}
	return string(out)
}

func TestUTCDateRoundTrip(t *testing.T) {
	in := NewUTCDate(time.Date(2024, 5, 1, 9, 0, 0, 500, time.FixedZone("JST", 9*3600)))
	encoded, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	if got, want := string(encoded), `"2024-05-01T00:00:00Z"`; got != want {
		t.Errorf("marshalled to %s, want %s", got, want)
	}
	var out UTCDate
	if err := json.Unmarshal(encoded, &out); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}
	if !out.Time.Equal(in.Time) {
		t.Errorf("round trip gave %v, want %v", out.Time, in.Time)
	}
}

// TestUTCDateAcceptsOffsets checks that a server which sends an offset where
// the specification calls for UTC is still understood, rather than failing the
// whole response over a timestamp.
func TestUTCDateAcceptsOffsets(t *testing.T) {
	var d UTCDate
	if err := json.Unmarshal([]byte(`"2024-05-01T09:00:00+09:00"`), &d); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}
	if want := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC); !d.Time.Equal(want) {
		t.Errorf("got %v, want %v", d.Time, want)
	}
}

func TestUTCDateRejectsNonsense(t *testing.T) {
	var d UTCDate
	if err := json.Unmarshal([]byte(`"yesterday"`), &d); err == nil {
		t.Error("expected an error")
	}
}

func TestDateKeepsOffset(t *testing.T) {
	in := NewDate(time.Date(2024, 5, 1, 9, 0, 0, 0, time.FixedZone("JST", 9*3600)))
	encoded, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	if got, want := string(encoded), `"2024-05-01T09:00:00+09:00"`; got != want {
		t.Errorf("marshalled to %s, want %s", got, want)
	}
}

func TestPatchObject(t *testing.T) {
	p := PatchObject{}.Set("keywords/$seen", true).Remove("keywords/$flagged")
	encoded, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}
	if got["keywords/$seen"] != true {
		t.Errorf("$seen = %v, want true", got["keywords/$seen"])
	}
	if v, present := got["keywords/$flagged"]; !present || v != nil {
		t.Errorf("$flagged = %v (present %v), want an explicit null", v, present)
	}
}

func TestWellKnownURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"example.com", "https://example.com/.well-known/jmap"},
		{"https://example.com", "https://example.com/.well-known/jmap"},
		{"https://example.com/", "https://example.com/.well-known/jmap"},
		{"https://mail.example.com/jmap", "https://mail.example.com/jmap/.well-known/jmap"},
	}
	for _, tt := range tests {
		if got := WellKnownURL(tt.in); got != tt.want {
			t.Errorf("WellKnownURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
