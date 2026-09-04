package ts

import (
	"sort"
	"strings"

	"github.com/linyows/jmapc/internal/gen/shared"
	"github.com/linyows/jmapc/internal/query"
	"github.com/linyows/jmapc/internal/spec"
)

// QueryGenerator turns checked queries into TypeScript functions. It works on a
// whole package at once, because the names it invents have to be unique across
// every query written into the same place.
type QueryGenerator struct {
	// Spec is the catalogue the queries were checked against.
	Spec *spec.Spec
	// Queries are the queries to generate.
	Queries []*query.Query
}

// call holds the names settled for one method call.
type call struct {
	responseType string
	recordType   string
	nestedType   string
	accountIDVar string
}

// plan is the naming decided for one query before any code is written.
type plan struct {
	q                   *query.Query
	funcName            string
	paramsType          string
	resultType          string
	returnType          string
	calls               map[*query.Call]*call
	sessionCapabilities []string
	// pagesName is the generator that walks the paged call's windows, empty
	// where the query is not paged.
	pagesName string
}

// Generate returns the source of one file per query, keyed by file name.
func (g *QueryGenerator) Generate() (map[string][]byte, error) {
	plans, err := g.plan()
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(plans))
	for _, p := range plans {
		out[FileName(p.q.Name)] = g.file(p)
	}
	return out, nil
}

// FileName returns the file a query is generated into. TypeScript names a file
// after what it exports, so this is the function's own name.
func FileName(queryName string) string {
	return lowerFirst(queryName) + ".ts"
}

// plan settles every generated name up front.
func (g *QueryGenerator) plan() ([]*plan, error) {
	queries := append([]*query.Query(nil), g.Queries...)
	sort.Slice(queries, func(i, j int) bool { return queries[i].Name < queries[j].Name })

	plans := make([]*plan, 0, len(queries))
	for _, q := range queries {
		// Each query is its own module, so a name need only be unique within
		// one file rather than across the package.
		taken := make(map[string]bool)
		p := &plan{
			q:        q,
			funcName: lowerFirst(q.Name),
			calls:    make(map[*query.Call]*call, len(q.Calls)),
		}
		if len(q.Params) > 0 {
			p.paramsType = shared.Unique(taken, q.Name+"Params")
		}
		for _, c := range q.Calls {
			info := &call{}
			if c.Properties != nil || c.NestedProperties != nil {
				info.recordType = shared.Unique(taken, q.Name+spec.ExportedName(c.Method.DataType))
				info.responseType = shared.Unique(taken, q.Name+c.Field+"Response")
			} else {
				info.responseType = spec.ExportedName(c.Method.Response)
			}
			if c.NestedProperties != nil {
				info.nestedType = shared.Unique(taken, q.Name+spec.ExportedName(c.Method.NestedType))
			}
			p.calls[c] = info
		}
		if q.Pages != nil {
			p.pagesName = lowerFirst(shared.Unique(taken, q.Name+"Pages"))
		}
		if q.Returns != nil {
			p.returnType = p.calls[q.Returns].responseType
		} else {
			p.resultType = shared.Unique(taken, q.Name+"Result")
			p.returnType = p.resultType
		}
		g.planAccountIDs(p)
		plans = append(plans, p)
	}
	return plans, nil
}

// planAccountIDs works out which calls need an accountId filling in, and from
// which capability's primary account.
func (g *QueryGenerator) planAccountIDs(p *plan) {
	seen := make(map[string]bool)
	for _, c := range p.q.Calls {
		capability, needed := c.AccountIDCapability(g.Spec)
		if !needed {
			continue
		}
		if !seen[capability] {
			seen[capability] = true
			p.sessionCapabilities = append(p.sessionCapabilities, capability)
		}
		p.calls[c].accountIDVar = accountIDVar(capability)
	}
	sort.Strings(p.sessionCapabilities)
}

// accountIDVar names the local holding the primary account id for a capability.
func accountIDVar(capability string) string {
	short := capability
	if i := strings.LastIndex(short, ":"); i >= 0 {
		short = short[i+1:]
	}
	return lowerFirst(spec.ExportedName(short)) + "AccountId"
}

// lowerFirst lowers a name's first letter, which is how TypeScript spells a
// function or a variable.
//
// A leading run of capitals is an initialism and lowers as a whole, so an
// MDNSendResponse becomes mdnSendResponse rather than mDNSendResponse.
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	i := 0
	for i < len(r) && r[i] >= 'A' && r[i] <= 'Z' {
		i++
	}
	if i > 1 && i < len(r) {
		i--
	}
	if i == 0 {
		return s
	}
	return strings.ToLower(string(r[:i])) + string(r[i:])
}

// tsMemberName spells a name as TypeScript does. The names settled by the
// checker are in Go's shape, where an initialism is capitalised throughout —
// MailboxID, HeaderListIDAsText — and TypeScript writes those as JMAP does, so
// the capitals come back down: mailboxId, headerListIdAsText.
func tsMemberName(name string) string {
	var b strings.Builder
	runes := []rune(name)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r < 'A' || r > 'Z' {
			b.WriteRune(r)
			continue
		}
		// The first letter of a run of capitals keeps its case unless it
		// begins the name; the rest of the run lowers, except where a capital
		// starts a new word.
		start := i
		for i+1 < len(runes) && runes[i+1] >= 'A' && runes[i+1] <= 'Z' {
			i++
		}
		run := runes[start : i+1]
		rest := runes[i+1:]
		// The last capital of a run usually begins the next word — the S of
		// SMIMEStatus — but not where what follows is a lone "s", which makes
		// the run itself plural: the IDs of EmailIDs is one word, not ID and Ds.
		plural := len(rest) == 1 && rest[0] == 's'
		if len(rest) > 0 && len(run) > 1 && !plural {
			i--
			run = runes[start : i+1]
		}
		b.WriteString(string(run[0]))
		b.WriteString(strings.ToLower(string(run[1:])))
	}
	return lowerFirst(b.String())
}
