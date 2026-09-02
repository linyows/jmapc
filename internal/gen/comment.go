// Package gen turns the JMAP data model, and the queries written against it,
// into Go source.
package gen

import (
	"io"
	"strings"
)

// commentWidth is the column the generated comments wrap at, counting the
// indent and the "// " prefix.
const commentWidth = 78

// writeComment writes text as a Go comment, wrapped to commentWidth and
// prefixed with the given indent. It writes nothing for empty text.
func writeComment(w io.Writer, indent, text string) {
	for _, line := range wrapComment(indent, text) {
		io.WriteString(w, line)
		io.WriteString(w, "\n")
	}
}

// wrapComment returns the comment lines for text, each already carrying its
// indent and "//" prefix.
func wrapComment(indent, text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	limit := commentWidth - len(indent) - len("// ")
	if limit < 20 {
		limit = 20
	}
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			lines = append(lines, indent+"//")
			continue
		}
		var cur strings.Builder
		for _, word := range strings.Fields(para) {
			switch {
			case cur.Len() == 0:
				cur.WriteString(word)
			case cur.Len()+1+len(word) <= limit:
				cur.WriteByte(' ')
				cur.WriteString(word)
			default:
				lines = append(lines, indent+"// "+cur.String())
				cur.Reset()
				cur.WriteString(word)
			}
		}
		if cur.Len() > 0 {
			lines = append(lines, indent+"// "+cur.String())
		}
	}
	return lines
}
