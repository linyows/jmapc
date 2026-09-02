package query

import (
	"fmt"
	"sort"
	"strings"
)

// Error is one problem found in a query file, located by the path through the
// JSON document rather than by line, so that the message points at the argument
// that is wrong.
type Error struct {
	// File is the query file the problem was found in.
	File string
	// Where is the path through the query document, such as
	// "methodCalls[1].arguments.filter.inMailbox".
	Where string
	// Msg describes the problem.
	Msg string
	// Hint suggests what the author may have meant.
	Hint string
}

func (e *Error) Error() string {
	var b strings.Builder
	if e.File != "" {
		b.WriteString(e.File)
		b.WriteString(": ")
	}
	if e.Where != "" {
		b.WriteString(e.Where)
		b.WriteString(": ")
	}
	b.WriteString(e.Msg)
	if e.Hint != "" {
		b.WriteString("\n\t")
		b.WriteString(e.Hint)
	}
	return b.String()
}

// ErrorList is every problem found in one query file. Parsing reports all of
// them at once, so that a query can be fixed in one pass.
type ErrorList []*Error

func (l ErrorList) Error() string {
	switch len(l) {
	case 0:
		return "no errors"
	case 1:
		return l[0].Error()
	}
	msgs := make([]string, len(l))
	for i, e := range l {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "\n")
}

// Err returns the list as an error, or nil when it is empty.
func (l ErrorList) Err() error {
	if len(l) == 0 {
		return nil
	}
	return l
}

// nearest returns the candidate closest to name, for a "did you mean" hint, or
// an empty string when nothing is close enough to be worth suggesting.
func nearest(name string, candidates []string) string {
	best, bestDist := "", 0
	// A suggestion is worth making only when it is close. Allowing a distance
	// of half the name lets a long mistake match a short candidate that means
	// something else entirely — "organiser" suggesting "owner" — and a
	// confident wrong suggestion is worse than none, since the alternatives are
	// listed either way.
	limit := min(len(name)/3+1, 4)
	for _, c := range candidates {
		d := editDistance(strings.ToLower(name), strings.ToLower(c))
		if d > limit {
			continue
		}
		if best == "" || d < bestDist {
			best, bestDist = c, d
		}
	}
	return best
}

// editDistance returns the Levenshtein distance between a and b.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// hintFor builds a "did you mean" hint, listing the alternatives when none is
// close enough to single out.
func hintFor(name string, candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	if guess := nearest(name, candidates); guess != "" {
		return fmt.Sprintf("did you mean %q?", guess)
	}
	sorted := append([]string(nil), candidates...)
	sort.Strings(sorted)
	const maxListed = 12
	if len(sorted) > maxListed {
		return fmt.Sprintf("known names include %s, and %d others",
			strings.Join(sorted[:maxListed], ", "), len(sorted)-maxListed)
	}
	return "known names are " + strings.Join(sorted, ", ")
}
