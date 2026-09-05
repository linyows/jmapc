package query

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
)

// shapeParamPattern finds a parameter in a string, keeping the question mark
// that says the caller may leave the argument out, since a query that lets one
// be left out is not the same query as one that does not.
var shapeParamPattern = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*(\?)?\s*\}\}`)

// shapeOf returns the query with its names taken off: the parameters numbered
// by where they first appear, the call ids numbered by position, and the
// documentation dropped. Two queries with the same shape are one query written
// twice, whatever the two call things.
//
// It works from the source rather than from the checked query, because what is
// being compared is what was written: two files that differ only in how they
// spell a parameter make the same request.
func shapeOf(src []byte) string {
	var doc map[string]any
	if err := json.Unmarshal(src, &doc); err != nil {
		return ""
	}
	// The documentation says nothing about what is sent.
	delete(doc, DocMember)
	delete(doc, SchemaMember)

	calls, _ := doc["methodCalls"].([]any)
	ids := make(map[string]string, len(calls))
	for i, raw := range calls {
		call, ok := raw.([]any)
		if !ok || len(call) != 3 {
			continue
		}
		if id, ok := call[2].(string); ok {
			ids[id] = "#" + strconv.Itoa(i)
		}
	}

	s := &shaper{ids: ids, params: map[string]int{}}
	for _, raw := range calls {
		call, ok := raw.([]any)
		if !ok || len(call) != 3 {
			continue
		}
		if args, ok := call[1].(map[string]any); ok {
			delete(args, CommentArgument)
			call[1] = s.value(args)
		}
		if id, ok := call[2].(string); ok {
			call[2] = s.callID(id)
		}
	}
	for _, member := range []string{ReturnsMember, WatchesMember, PagesMember} {
		if id, ok := doc[member].(string); ok {
			doc[member] = s.callID(id)
		}
	}

	out, err := json.Marshal(doc)
	if err != nil {
		return ""
	}
	return string(out)
}

// shaper carries the numbering while a query is walked.
type shaper struct {
	ids    map[string]string
	params map[string]int
}

// callID is the call id under the number its position gives it, since the id a
// query chooses reaches neither the generated names nor the meaning of the
// request.
func (s *shaper) callID(id string) string {
	if numbered, ok := s.ids[id]; ok {
		return numbered
	}
	return id
}

// value rewrites a value, and everything under it, into the shape it has once
// the names are taken off. Members are visited in order of their names so that
// the numbering does not depend on how Go happened to walk a map.
func (s *shaper) value(v any) any {
	switch value := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(value))
		for _, key := range keys {
			// A back reference names the call it reads, and that name is a
			// call id like any other.
			if key == "resultOf" {
				if id, ok := value[key].(string); ok {
					out[key] = s.callID(id)
					continue
				}
			}
			out[s.text(key)] = s.value(value[key])
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = s.value(item)
		}
		return out
	case string:
		return s.text(value)
	}
	return v
}

// text numbers the parameters a string holds, leaving the rest of it alone.
func (s *shaper) text(text string) string {
	return shapeParamPattern.ReplaceAllStringFunc(text, func(match string) string {
		m := shapeParamPattern.FindStringSubmatch(match)
		name, optional := m[1], m[2]
		n, seen := s.params[name]
		if !seen {
			n = len(s.params) + 1
			s.params[name] = n
		}
		return fmt.Sprintf("{{%d%s}}", n, optional)
	})
}
