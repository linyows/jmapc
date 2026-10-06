package request

import (
	"strings"
	"testing"
)

// TestEveryFormatSaysHowItIsWritten checks the table both a request and a
// parameter on a command line are held to: each type describes itself, and
// refuses what is not written in its format.
func TestEveryFormatSaysHowItIsWritten(t *testing.T) {
	for name, f := range Formats {
		if !strings.HasPrefix(f.Doc, "a "+name+" ") {
			t.Errorf("%s is described as %q", name, f.Doc)
		}
		if f.Valid("soon") {
			t.Errorf("%s takes %q", name, "soon")
		}
	}
}
