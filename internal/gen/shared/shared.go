// Package shared holds the parts of query generation that do not depend on the
// language being generated: how a comment is wrapped, the prose the generated
// documentation is written in, which properties a record type holds, and how a
// name is kept unique.
package shared

import (
	"io"
	"strconv"
	"strings"
)

// commentWidth is the column the generated comments wrap at, counting the
// indent and the "// " prefix.
const commentWidth = 78

// WriteComment writes text as a comment, wrapped to commentWidth and prefixed
// with the given indent. Go and TypeScript spell a comment the same way, so one
// implementation serves both. It writes nothing for empty text.
func WriteComment(w io.Writer, indent, text string) {
	WriteCommentMarker(w, indent, "//", text)
}

// WriteCommentMarker is WriteComment for a language that spells a comment with
// something other than "//". Rust documents an item with "///", and wants the
// text wrapped to the same column all the same.
func WriteCommentMarker(w io.Writer, indent, marker, text string) {
	for _, line := range wrapComment(indent, marker, text) {
		io.WriteString(w, line)
		io.WriteString(w, "\n")
	}
}

// wrapComment returns the comment lines for text, each already carrying its
// indent and comment marker.
func wrapComment(indent, marker, text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	limit := commentWidth - len(indent) - len(marker) - 1
	if limit < 20 {
		limit = 20
	}
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			lines = append(lines, indent+marker)
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
				lines = append(lines, indent+marker+" "+cur.String())
				cur.Reset()
				cur.WriteString(word)
			}
		}
		if cur.Len() > 0 {
			lines = append(lines, indent+marker+" "+cur.String())
		}
	}
	return lines
}

// Unique returns name, or name with a number appended, so that nothing in
// taken already has it. It records what it returns, so the next caller sees the
// name as taken.
func Unique(taken map[string]bool, name string) string {
	candidate := name
	for i := 2; taken[candidate]; i++ {
		candidate = name + strconv.Itoa(i)
	}
	taken[candidate] = true
	return candidate
}

// RecordProperties returns the properties a record type holds. A /get response
// always carries the id, whether or not the query asked for it.
func RecordProperties(props []string) []string {
	for _, p := range props {
		if p == "id" {
			return props
		}
	}
	return append([]string{"id"}, props...)
}
