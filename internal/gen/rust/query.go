package rust

import (
	"sort"
	"strings"

	"github.com/linyows/jmapc/internal/gen/shared"
	"github.com/linyows/jmapc/internal/query"
	"github.com/linyows/jmapc/internal/spec"
)

// QueryGenerator turns checked queries into Rust functions. It works on a whole
// module directory at once, because the modules it writes have to be declared
// together in the mod.rs beside them.
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
	module              string
	funcName            string
	paramsType          string
	resultType          string
	returnType          string
	calls               map[*query.Call]*call
	sessionCapabilities []string
}

// Generate returns the source of one file per query, keyed by file name,
// together with the mod.rs that declares them.
func (g *QueryGenerator) Generate() (map[string][]byte, error) {
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

// FileName returns the file a query is generated into. Rust names a file after
// the module it holds, and a module name is snake_case.
func FileName(queryName string) string {
	return spec.RustName(queryName) + ".rs"
}

// plan settles every generated name up front.
func (g *QueryGenerator) plan() ([]*plan, error) {
	queries := append([]*query.Query(nil), g.Queries...)
	sort.Slice(queries, func(i, j int) bool { return queries[i].Name < queries[j].Name })

	plans := make([]*plan, 0, len(queries))
	for _, q := range queries {
		// Each query is its own module, so a name need only be unique within
		// one file rather than across the directory.
		taken := make(map[string]bool)
		prefix := spec.RustTypeName(q.Name)
		p := &plan{
			q:        q,
			module:   spec.RustName(q.Name),
			funcName: spec.RustName(q.Name),
			calls:    make(map[*query.Call]*call, len(q.Calls)),
		}
		if len(q.Params) > 0 {
			p.paramsType = shared.Unique(taken, prefix+"Params")
		}
		for _, c := range q.Calls {
			info := &call{}
			if c.Properties != nil || c.NestedProperties != nil {
				info.recordType = shared.Unique(taken, prefix+spec.RustTypeName(c.Method.DataType))
				info.responseType = shared.Unique(taken, prefix+spec.RustTypeName(c.Field)+"Response")
			} else {
				info.responseType = spec.RustTypeName(c.Method.Response)
			}
			if c.NestedProperties != nil {
				info.nestedType = shared.Unique(taken, prefix+spec.RustTypeName(c.Method.NestedType))
			}
			p.calls[c] = info
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
func (g *QueryGenerator) planAccountIDs(p *plan) {
	seen := make(map[string]bool)
	for _, c := range p.q.Calls {
		args, err := g.Spec.ArgumentsOf(c.Method.Name)
		if err != nil {
			continue
		}
		if _, takes := args.Field(query.AccountIDArgument); !takes {
			continue
		}
		if _, given := c.Args.Find(query.AccountIDArgument); given {
			continue
		}
		if _, referenced := c.Args.Find("#" + query.AccountIDArgument); referenced {
			continue
		}
		capability := c.Method.Capability
		if capability == "" {
			capability = spec.CapabilityCore
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
