package rust

import (
	"sort"
	"strings"

	"github.com/linyows/jmapc/internal/gen/shared"
	"github.com/linyows/jmapc/internal/request"
	"github.com/linyows/jmapc/internal/spec"
)

// RequestGenerator turns checked requests into Rust functions. It works on a whole
// module directory at once, because the modules it writes have to be declared
// together in the mod.rs beside them.
type RequestGenerator struct {
	// Spec is the catalogue the requests were checked against.
	Spec *spec.Spec
	// Requests are the requests to generate.
	Requests []*request.Request
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
}

// plan is the naming decided for one request before any code is written.
type plan struct {
	q                   *request.Request
	module              string
	funcName            string
	creations           []shared.Creation
	paramsType          string
	resultType          string
	returnType          string
	calls               map[*request.Call]*call
	sessionCapabilities []string
	// pagesType and pagesFunc are the walk over the paged call's windows: the
	// state it keeps, and the function that starts one. They are empty where
	// the request is not paged.
	pagesType string
	pagesFunc string
}

// Generate returns the source of one file per request, keyed by file name,
// together with the mod.rs that declares them.
func (g *RequestGenerator) Generate() (map[string][]byte, error) {
	plans, err := g.plan()
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(plans)+1)
	modules := make([]string, 0, len(plans))
	for _, p := range plans {
		out[p.module+".rs"] = g.file(p)
		modules = append(modules, p.module)
	}
	out["mod.rs"] = writeMod(modules)
	return out, nil
}

// FileName returns the file a request is generated into. Rust names a file after
// the module it holds, and a module name is snake_case.
func FileName(queryName string) string {
	return spec.RustName(queryName) + ".rs"
}

// plan settles every generated name up front.
func (g *RequestGenerator) plan() ([]*plan, error) {
	requests := append([]*request.Request(nil), g.Requests...)
	sort.Slice(requests, func(i, j int) bool { return requests[i].Name < requests[j].Name })

	plans := make([]*plan, 0, len(requests))
	for _, q := range requests {
		// Each request is its own module, so a name need only be unique within
		// one file rather than across the directory.
		taken := make(map[string]bool)
		prefix := spec.RustTypeName(q.Name)
		p := &plan{
			q:        q,
			module:   spec.RustName(q.Name),
			funcName: spec.RustName(q.Name),
			calls:    make(map[*request.Call]*call, len(q.Calls)),
		}
		if len(q.Params) > 0 {
			p.paramsType = shared.Unique(taken, prefix+"Params")
		}
		p.creations = shared.Creations(taken, spec.RustConstName(q.Name)+"_", q.Creations, spec.RustConstName)
		same := shared.SameNarrowing(q.Calls)
		for _, c := range q.Calls {
			info := &call{}
			switch {
			case same[c] != nil && same[c] != c:
				// Another call of this request reads the same records in the
				// same shape, and one shape is one type.
				*info = *p.calls[same[c]]
				info.writesTypes = false
			case c.Properties != nil || c.NestedProperties != nil:
				info.recordType = shared.Unique(taken, prefix+spec.RustTypeName(c.Field)+spec.RustTypeName(c.Method.DataType))
				info.responseType = shared.Unique(taken, prefix+spec.RustTypeName(c.Field)+"Response")
				info.writesTypes = true
			default:
				info.responseType = spec.RustTypeName(c.Method.Response)
			}
			if c.NestedProperties != nil && info.writesTypes {
				info.nestedType = shared.Unique(taken, prefix+spec.RustTypeName(c.Field)+spec.RustTypeName(c.Method.NestedType))
			}
			p.calls[c] = info
		}
		if q.Pages != nil {
			p.pagesType = shared.Unique(taken, prefix+"Pages")
			p.pagesFunc = spec.RustName(p.pagesType)
		}
		if q.Returns != nil {
			p.returnType = p.calls[q.Returns].responseType
		} else {
			p.resultType = shared.Unique(taken, prefix+"Result")
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
	return spec.RustName(short) + "_account_id"
}
