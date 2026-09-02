package ts

import (
	"io"
	"strings"
)

// commentWidth is the column the generated comments wrap at, counting the
// indent and the "// " prefix.
const commentWidth = 78

// writeComment writes text as a TypeScript comment, wrapped and indented.
func writeComment(w io.Writer, indent, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	limit := commentWidth - len(indent) - len("// ")
	if limit < 20 {
		limit = 20
	}
	for _, para := range strings.Split(text, "\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			io.WriteString(w, indent+"//\n")
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
				io.WriteString(w, indent+"// "+cur.String()+"\n")
				cur.Reset()
				cur.WriteString(word)
			}
		}
		if cur.Len() > 0 {
			io.WriteString(w, indent+"// "+cur.String()+"\n")
		}
	}
}
