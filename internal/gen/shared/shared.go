// Package shared holds the parts of query generation that do not depend on the
// language being generated: how a comment is wrapped, the prose the generated
// documentation is written in, which properties a record type holds, and how a
// name is kept unique.
package shared

import (
	"io"
	"strconv"
	"strings"

	"github.com/linyows/jmapc/internal/query"
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

// SameNarrowing maps each call that narrows what it fetches to the first call
// of the query that narrows it the same way, and a call that is the first to
// itself. A call that narrows nothing is not in the map at all.
//
// Two calls of one query reading the same type through the same method and
// asking for the same properties describe one record, so generating a type for
// each would be two names for one shape, and a caller passing a record from one
// to a function written for the other would have to convert between them. The
// property lists have to agree in order as well as in content, since the order
// is the order the generated fields are written in.
func SameNarrowing(calls []*query.Call) map[*query.Call]*query.Call {
	first := make(map[string]*query.Call, len(calls))
	out := make(map[*query.Call]*query.Call, len(calls))
	for _, c := range calls {
		if c.Properties == nil && c.NestedProperties == nil {
			continue
		}
		// The method is part of the key because the response type is the
		// method's, whatever the records inside it are.
		key := strings.Join([]string{
			c.Method.Name,
			c.Method.DataType,
			strings.Join(c.Properties, "\x00"),
			c.Method.NestedType,
			strings.Join(c.NestedProperties, "\x00"),
		}, "\x01")
		if seen, dup := first[key]; dup {
			out[c] = seen
			continue
		}
		first[key] = c
		out[c] = c
	}
	return out
}

// Creation is the name a generated file gives one creation id, so that a
// caller reading back what a /set created spells it once rather than twice.
type Creation struct {
	// Name is the identifier the constant goes by.
	Name string
	// ID is the creation id as the query wrote it.
	ID string
}

// Creations names the creation ids a query invents, keeping the names unique
// among everything else the file declares. Renaming one in the query renames
// the constant with it, which is what turns a rename into a compile error
// rather than a lookup that quietly misses.
func Creations(taken map[string]bool, prefix string, ids []string, name func(string) string) []Creation {
	out := make([]Creation, 0, len(ids))
	for _, id := range ids {
		out = append(out, Creation{Name: Unique(taken, prefix+name(id)), ID: id})
	}
	return out
}
