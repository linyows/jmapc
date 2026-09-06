package main

import (
	"bytes"
	"strings"
	"testing"
)

// A help redirected to a file, or read by a test, holds the escapes as text
// unless they are left out.
func TestTheBannerIsPlainWhereItIsNotATerminal(t *testing.T) {
	var buf bytes.Buffer
	printUsage(&buf)
	out := buf.String()

	if strings.Contains(out, "\x1b") {
		t.Errorf("the banner carries escape sequences where it is not written to a terminal:\n%q", out)
	}
	for _, want := range []string{"|__/", "jmapc: Generates type-safe code", "https://github.com/linyows/jmapc"} {
		if !strings.Contains(out, want) {
			t.Errorf("the banner does not hold %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("the usage does not follow the banner:\n%s", out)
	}
	// The logo comes first, then what jmapc is.
	if strings.Index(out, "|__/") > strings.Index(out, "jmapc: Generates") {
		t.Errorf("the description is printed before the logo:\n%s", out)
	}
}

func TestTheLogoIsGreenAndTheDescriptionDim(t *testing.T) {
	coloured := styleFor(true)
	if coloured.logo != green {
		t.Errorf("the logo is drawn with %q, want %q", coloured.logo, green)
	}
	if coloured.description != dim {
		t.Errorf("the description is drawn with %q, want %q", coloured.description, dim)
	}
	if plain := styleFor(false); plain != (style{}) {
		t.Errorf("a writer that takes no colour was given %+v", plain)
	}
}

// The colour ends where each line does, so that what follows the banner is
// written in none of it.
func TestEachLineOfTheBannerEndsItsColour(t *testing.T) {
	var buf bytes.Buffer
	writeLines(&buf, "one\ntwo\n", green, reset)
	want := green + "one" + reset + "\n" + green + "two" + reset + "\n"
	if buf.String() != want {
		t.Errorf("writeLines wrote %q, want %q", buf.String(), want)
	}
}

// The files are named after what they hold, and the banner reads them in the
// order it shows them.
func TestTheAssetsHoldWhatTheirNamesSay(t *testing.T) {
	if !strings.Contains(logo, "|__/") {
		t.Errorf("the logo does not look like the name drawn in ASCII:\n%s", logo)
	}
	if !strings.HasPrefix(description, "jmapc: ") {
		t.Errorf("the description does not start by naming jmapc:\n%s", description)
	}
}
