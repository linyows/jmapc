package ts

import (
	"sort"
	"strings"

	"github.com/linyows/jmapc/internal/gen/shared"
	"github.com/linyows/jmapc/internal/request"
	"github.com/linyows/jmapc/internal/spec"
)

// RequestGenerator turns checked requests into TypeScript functions. It works on a
// whole package at once, because the names it invents have to be unique across
// every request written into the same place.
type RequestGenerator struct {
	// Spec is the catalogue the requests were checked against.
	Spec *spec.Spec
	// Requests are the requests to generate.
	Requests []*request.Request
	// Properties are the named sets of properties the requests ask for, whose
	// types are written into a module of their own so that every request
	// asking for one answers with the same type. It is nil where the project
	// names no set.
	Properties *request.PropertySets
}

// call holds the names settled for one method call.
type call struct {
	responseType string
	recordType   string
	nestedType   string
	accountIDVar string
	// writesTypes says this call is the one that writes the types it names. A
	// call reading the same records in the same shape as an earlier one shares
	// its types rather than declaring them again.
	writesTypes bool
	// sharedRecord says the record type is one of the named sets, which the
	// properties module declares rather than this one.
	sharedRecord bool
	// sharedNested says the same of the nested type.
	sharedNested bool
}

// plan is the naming decided for one request before any code is written.
type plan struct {
	q                   *request.Request
	funcName            string
	creations           []shared.Creation
	paramsType          string
	resultType          string
	returnType          string
	calls               map[*request.Call]*call
	sessionCapabilities []string
	// pagesName is the generator that walks the paged call's windows, empty
	// where the request is not paged.
	pagesName string
}

// Generate returns the source of one file per request, keyed by file name.
func (g *RequestGenerator) Generate() (map[string][]byte, error) {
	plans, err := g.plan()
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(plans)+1)
	for _, p := range plans {
		out[FileName(p.q.Name)] = g.file(p)
	}
	if len(g.Properties.Names()) > 0 {
		out[PropertiesFileName] = g.propertiesFile()
	}
	return out, nil
}

// FileName returns the file a request is generated into. TypeScript names a file
// after what it exports, so this is the function's own name.
func FileName(queryName string) string {
	return lowerFirst(queryName) + ".ts"
}

// plan settles every generated name up front.
func (g *RequestGenerator) plan() ([]*plan, error) {
	requests := append([]*request.Request(nil), g.Requests...)
	sort.Slice(requests, func(i, j int) bool { return requests[i].Name < requests[j].Name })

	plans := make([]*plan, 0, len(requests))
	for _, q := range requests {
		// Each request is its own module, so a name need only be unique within
		// one file rather than across the package. The names of the sets are
		// taken all the same: they are imported into every module that asks
		// for one, and a local type of the same name would shadow the import.
		taken := make(map[string]bool)
		for _, name := range g.Properties.Names() {
			taken[name] = true
		}
		p := &plan{
			q:        q,
			funcName: lowerFirst(q.Name),
			calls:    make(map[*request.Call]*call, len(q.Calls)),
		}
		if len(q.Params) > 0 {
			p.paramsType = shared.Unique(taken, q.Name+"Params")
		}
		p.creations = shared.Creations(taken, p.funcName, q.Creations, spec.ExportedName)
		same := shared.SameNarrowing(q.Calls)
		for _, c := range q.Calls {
			info := &call{}
			switch {
			case same[c] != nil && same[c] != c:
				// Another call of this request reads the same records in the
				// same shape, and one shape is one type.
				*info = *p.calls[same[c]]
				info.writesTypes = false
			case c.PropertySet != nil:
				// The set names the type, so every call asking for it answers
				// with the one the properties module declares.
				info.recordType = c.PropertySet.Name
				info.responseType = shared.Unique(taken, q.Name+c.Field+"Response")
				info.writesTypes = true
				info.sharedRecord = true
			case c.Properties != nil || c.NestedProperties != nil:
				info.recordType = shared.Unique(taken, q.Name+c.Field+spec.ExportedName(c.Method.DataType))
				info.responseType = shared.Unique(taken, q.Name+c.Field+"Response")
				info.writesTypes = true
			default:
				info.responseType = spec.ExportedName(c.Method.Response)
			}
			switch {
			case c.NestedPropertySet != nil:
				info.nestedType = c.NestedPropertySet.Name
				info.sharedNested = true
			case c.NestedProperties != nil && info.writesTypes:
				info.nestedType = shared.Unique(taken, q.Name+c.Field+spec.ExportedName(c.Method.NestedType))
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
func (g *RequestGenerator) planAccountIDs(p *plan) {
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
