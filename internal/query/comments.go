package query

// stripComments removes // and /* */ comments from a JSON document, replacing
// them with spaces so that byte offsets into the document stay put. JMAP
// requests are JSON, and JSON has no comments, but a query is source code that
// deserves to be annotated.
func stripComments(src []byte) []byte {
	out := make([]byte, len(src))
	copy(out, src)

	const (
		code = iota
		inString
		inLine
		inBlock
	)
	state := code
	for i := 0; i < len(out); i++ {
		c := out[i]
		switch state {
		case code:
			switch {
			case c == '"':
				state = inString
			case c == '/' && i+1 < len(out) && out[i+1] == '/':
				out[i], out[i+1] = ' ', ' '
				i++
				state = inLine
			case c == '/' && i+1 < len(out) && out[i+1] == '*':
				out[i], out[i+1] = ' ', ' '
				i++
				state = inBlock
			}
		case inString:
			switch c {
			case '\\':
				i++
			case '"':
				state = code
			}
		case inLine:
			if c == '\n' {
				state = code
				continue
			}
			out[i] = ' '
		case inBlock:
			if c == '*' && i+1 < len(out) && out[i+1] == '/' {
				out[i], out[i+1] = ' ', ' '
				i++
				state = code
				continue
			}
			if c != '\n' {
				out[i] = ' '
			}
		}
	}
	return out
}
