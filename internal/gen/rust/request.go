package rust

import (
	"fmt"
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
	// Properties are the named sets of properties the requests ask for, whose
	// types go into a module of their own so that every request asking for one
	// answers with the same type. It is nil where the project names no set.
	Properties *request.PropertySets
}

// call is what the Rust generator writes for one method call.
type call struct {
	shared.CallPlan
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
	out := make(map[string][]byte, len(plans)+3)
	// from records what each module was generated from, so that two things
	// generated into one are refused rather than one silently replacing the
	// other. A module is the request's name in snake_case, so "FooBar" and
	// "fooBar" are two requests and one module, and the runtime's modules are
	// taken before any request is.
	from := make(map[string]string, len(plans)+5)
	for _, m := range RuntimeModules {
		from[m] = "the runtime"
	}
	add := func(module, what string, src []byte) error {
		if earlier, taken := from[module]; taken {
			return fmt.Errorf("gen: %s and %s would both be generated into %s.rs; rename one of them", earlier, what, module)
		}
		from[module] = what
		out[module+".rs"] = src
		return nil
	}
	modules := make([]string, 0, len(plans)+1)
	properties := len(g.Properties.Names()) > 0
	if properties {
		if err := add(PropertiesModule, "the sets of properties", g.propertiesFile()); err != nil {
			return nil, err
		}
	}
	if len(plans) > 0 {
		if err := add(VerifyModule, "the function that verifies the requests", g.verifyFile(plans)); err != nil {
			return nil, err
		}
		modules = append(modules, VerifyModule)
	}
	for _, p := range plans {
		// A module named after a keyword is declared as a raw identifier, and
		// its file is named without the r#, as rustc looks for it.
		if err := add(strings.TrimPrefix(p.module, "r#"), "the request "+p.q.Name, g.file(p)); err != nil {
			return nil, err
		}
		modules = append(modules, p.module)
	}
	sort.Strings(modules)
	out["mod.rs"] = writeMod(modules, properties)
	return out, nil
}

// plan settles every generated name up front.
func (g *RequestGenerator) plan() ([]*plan, error) {
	requests := append([]*request.Request(nil), g.Requests...)
	sort.Slice(requests, func(i, j int) bool { return requests[i].Name < requests[j].Name })

	plans := make([]*plan, 0, len(requests))
	for _, q := range requests {
		// Each request is its own module, so a name need only be unique within
		// one file rather than across the directory. The names of the sets are
		// taken all the same: they are brought into every module that asks for
		// one, and a local type of the same name would clash with the use.
		taken := make(map[string]bool)
		for _, name := range g.Properties.Names() {
			taken[spec.RustTypeName(name)] = true
		}
		prefix := spec.RustTypeName(q.Name)
		p := &plan{
			q:        q,
			module:   spec.RustFieldName(q.Name),
			funcName: spec.RustFieldName(q.Name),
			calls:    make(map[*request.Call]*call, len(q.Calls)),
		}
		if len(q.Params) > 0 {
			p.paramsType = shared.Unique(taken, prefix+"Params")
		}
		p.creations = shared.Creations(taken, spec.RustConstName(q.Name)+"_", q.Creations, spec.RustConstName)
		calls, capabilities := shared.PlanCalls(g.Spec, q, taken, shared.Namer{
			Prefix:    prefix,
			Field:     spec.RustTypeName,
			Part:      spec.RustTypeName,
			Set:       spec.RustTypeName,
			Runtime:   spec.RustTypeName,
			AccountID: accountIDVar,
		})
		for _, c := range q.Calls {
			p.calls[c] = &call{CallPlan: *calls[c]}
		}
		p.sessionCapabilities = capabilities
		if q.Pages != nil {
			p.pagesType = shared.Unique(taken, prefix+"Pages")
			p.pagesFunc = spec.RustName(p.pagesType)
		}
		if q.Returns != nil {
			p.returnType = p.calls[q.Returns].ResponseType
		} else {
			p.resultType = shared.Unique(taken, prefix+"Result")
			p.returnType = p.resultType
		}
		plans = append(plans, p)
	}
	return plans, nil
}

// accountIDVar names the local holding the primary account id for a capability.
func accountIDVar(capability string) string {
	short := capability
	if i := strings.LastIndex(short, ":"); i >= 0 {
		short = short[i+1:]
	}
	return spec.RustName(short) + "_account_id"
}
