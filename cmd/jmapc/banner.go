package main

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"strings"
)

// The banner is drawn from these rather than from strings written here, so
// that the drawing can be edited as the drawing it is.
var (
	// logo is the name of the project drawn in ASCII, one line per row.
	//
	//go:embed logo.txt
	logo string
	// description is what jmapc is, in one line, followed by where it lives.
	//
	//go:embed description.txt
	description string
)

// The escape sequences the banner is drawn with. Green reads on a light
// terminal and on a dark one, so the logo needs no background of its own and
// no guess about which the terminal is.
const (
	reset = "\x1b[0m"
	green = "\x1b[32m"
	dim   = "\x1b[2m"
)

// printUsage writes the banner and the usage text.
func printUsage(w io.Writer) {
	writeBanner(w)
	fmt.Fprint(w, usage)
}

// writeBanner writes the logo and, under it, what jmapc is.
func writeBanner(w io.Writer) {
	style := bannerStyle(w)
	writeLines(w, logo, style.logo, style.off)
	fmt.Fprintln(w)
	writeLines(w, description, style.description, style.off)
	fmt.Fprintln(w)
}

// writeLines writes each line of a block of text in a colour of its own, so
// that the colour ends where the line does and nothing after the banner is
// written in it.
func writeLines(w io.Writer, text, colour, off string) {
	for line := range strings.Lines(strings.TrimRight(text, "\n")) {
		fmt.Fprintf(w, "%s%s%s\n", colour, strings.TrimRight(line, " \n"), off)
	}
}

// style is how the parts of the banner are coloured, and holds empty strings
// where they are not coloured at all.
type style struct {
	logo        string
	description string
	off         string
}

// bannerStyle decides how to colour the banner for the writer it is going to.
// Anything that is not a terminal is written to without colour, since the
// escapes would be read as text: a help redirected to a file, or the buffer a
// test reads. NO_COLOR is honoured where it is set to anything at all, as
// https://no-color.org asks.
func bannerStyle(w io.Writer) style {
	return styleFor(isTerminal(w) && os.Getenv("NO_COLOR") == "")
}

// styleFor is the decision itself, with what the environment said already
// read.
func styleFor(colour bool) style {
	if !colour {
		return style{}
	}
	return style{logo: green, description: dim, off: reset}
}

// isTerminal reports whether a writer is a terminal.
func isTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
