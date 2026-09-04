package request

import (
	"strings"
	"testing"

	"github.com/linyows/jmapc/internal/query"
	"github.com/linyows/jmapc/internal/spec"
)

// param stands in for a parameter of the given type, which is all ParseValue
// looks at.
func param(t *testing.T, typ string) *query.Param {
	t.Helper()
	return &query.Param{Name: "p", Type: spec.MustParseType(typ)}
}

// TestParseValue checks that a value written at a terminal takes the shape its
// type calls for. A string is the text itself, so nothing has to be quoted
// against the shell, and anything with a shape is written as JSON.
func TestParseValue(t *testing.T) {
	cases := []struct{ typ, text, want string }{
		{"String", "Work", `"Work"`},
		{"String", "25", `"25"`},
		{"Id", "mbx1", `"mbx1"`},
		{"UnsignedInt", "25", `25`},
		{"Int", "-3", `-3`},
		{"Number", "1.5", `1.5`},
		{"Boolean", "true", `true`},
		{"UTCDate", "2026-09-04T09:00:00Z", `"2026-09-04T09:00:00Z"`},
		{"Duration", "PT1H30M", `"PT1H30M"`},
		{"String[]", `["a", "b"]`, `["a","b"]`},
		{"EmailFilterCondition", `{"hasAttachment": true}`, `{"hasAttachment":true}`},
		// An accepted value the query leaves nullable is still a value: a
		// caller who means null writes it in the query.
		{"String|null", "Work", `"Work"`},
		// Any takes JSON as JSON and everything else as the string it looks like.
		{"Any", `{"a": 1}`, `{"a":1}`},
		{"Any", "hello", `"hello"`},
	}
	for _, c := range cases {
		v, err := ParseValue(param(t, c.typ), c.text)
		if err != nil {
			t.Errorf("%s %q: %v", c.typ, c.text, err)
			continue
		}
		if string(v.JSON) != c.want {
			t.Errorf("%s %q = %s, want %s", c.typ, c.text, v.JSON, c.want)
		}
		if v.Text != c.text {
			t.Errorf("%s %q kept the text as %q", c.typ, c.text, v.Text)
		}
	}
}

// TestParseValueRejects checks the values a server would refuse, which are
// worth refusing here instead: the round trip says less than the type does.
func TestParseValueRejects(t *testing.T) {
	cases := []struct{ typ, text, want string }{
		{"UnsignedInt", "-1", "is negative"},
		{"UnsignedInt", "many", "not a whole number"},
		{"Int", "1.5", "not a whole number"},
		{"Number", "soon", "not a number"},
		{"Boolean", "yes", "write true or false"},
		{"Id", "not an id", "not a valid id"},
		{"UTCDate", "2026-09-04", "not a UTCDate"},
		{"Date", "today", "not a Date"},
		{"LocalDateTime", "2026-09-04T09:00:00Z", "not a LocalDateTime"},
		{"Duration", "90m", "not a Duration"},
		{"SignedDuration", "90m", "not a SignedDuration"},
		{"String[]", "a, b", "written as JSON"},
	}
	for _, c := range cases {
		_, err := ParseValue(param(t, c.typ), c.text)
		if err == nil {
			t.Errorf("%s %q was accepted", c.typ, c.text)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s %q: %v, want %s", c.typ, c.text, err, c.want)
		}
		if !strings.Contains(err.Error(), "parameter p") {
			t.Errorf("%s %q: %v, want the parameter named", c.typ, c.text, err)
		}
	}
}
