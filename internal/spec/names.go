package spec

import (
	"strings"
	"unicode"
)

// initialisms are the words Go convention capitalises wholly, so that a JMAP
// property such as "htmlBody" becomes HTMLBody rather than HtmlBody.
var initialisms = map[string]string{
	"acl":   "ACL",
	"api":   "API",
	"ascii": "ASCII",
	"cc":    "Cc",
	"dns":   "DNS",
	"eml":   "EML",
	"html":  "HTML",
	"http":  "HTTP",
	"https": "HTTPS",
	"id":    "ID",
	"imap":  "IMAP",
	"json":  "JSON",
	"mdn":   "MDN",
	"mime":  "MIME",
	"rfc":   "RFC",
	"smime": "SMIME",
	"smtp":  "SMTP",
	"tls":   "TLS",
	"uid":   "UID",
	"uri":   "URI",
	"url":   "URL",
	"utc":   "UTC",
	"utf":   "UTF",
	"uuid":  "UUID",
	"vcard": "VCard",
}

// exportedName converts a JMAP name to an exported Go identifier.
func exportedName(name string) string {
	words := splitWords(name)
	var b strings.Builder
	for _, w := range words {
		b.WriteString(capitalizeWord(w))
	}
	return b.String()
}

// unexportedName converts a JMAP name to an unexported Go identifier, keeping
// the initialism convention: "accountId" becomes accountID.
func unexportedName(name string) string {
	words := splitWords(name)
	var b strings.Builder
	for i, w := range words {
		if i == 0 {
			b.WriteString(strings.ToLower(w))
			continue
		}
		b.WriteString(capitalizeWord(w))
	}
	s := b.String()
	if isGoKeyword(s) {
		return s + "_"
	}
	return s
}

// capitalizeWord uppercases a word wholly if it is a known initialism, and
// otherwise capitalises only its first letter. A trailing plural "s" is kept
// lowercase so that "ids" becomes IDs.
func capitalizeWord(w string) string {
	if w == "" {
		return ""
	}
	lower := strings.ToLower(w)
	if up, ok := initialisms[lower]; ok {
		return up
	}
	if strings.HasSuffix(lower, "s") {
		if up, ok := initialisms[strings.TrimSuffix(lower, "s")]; ok {
			return up + "s"
		}
	}
	r := []rune(w)
	return string(unicode.ToUpper(r[0])) + string(r[1:])
}

// splitWords breaks a camelCase, snake_case, or kebab-case name into words.
func splitWords(name string) []string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, string(cur))
			cur = nil
		}
	}
	runes := []rune(name)
	for i, r := range runes {
		switch {
		case r == '_' || r == '-' || r == ' ' || r == '.' || r == ':' || r == '@' || r == '/':
			flush()
		case unicode.IsUpper(r):
			// A run of capitals stays together until the last one, which
			// begins the next word: "HTMLBody" splits as HTML and Body.
			if i > 0 && (!unicode.IsUpper(runes[i-1]) ||
				(i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
				flush()
			}
			cur = append(cur, r)
		case unicode.IsDigit(r):
			if i > 0 && !unicode.IsDigit(runes[i-1]) {
				flush()
			}
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	return words
}

// goKeywords are the reserved words an identifier derived from a JMAP name may
// collide with.
var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

func isGoKeyword(s string) bool { return goKeywords[s] }

// ExportedName converts a JMAP name to an exported Go identifier.
func ExportedName(name string) string { return exportedName(name) }

// UnexportedName converts a JMAP name to an unexported Go identifier.
func UnexportedName(name string) string { return unexportedName(name) }
