// Package request turns a checked query into the JMAP request it stands for,
// with the parameters filled in. The generator writes that request as source;
// here the same request is built at run time, which is what lets a query be
// sent without generating a client for it first.
package request

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/linyows/jmapc"
	"github.com/linyows/jmapc/internal/query"
	"github.com/linyows/jmapc/internal/spec"
)

// Value is a parameter value as the caller gave it: the text they wrote, and
// the JSON it denotes for the type the parameter has. The text is kept because
// a parameter standing in for part of a member name goes into the name as
// text, not as JSON.
type Value struct {
	// Text is the value as written.
	Text string
	// JSON is the value as it goes on the wire.
	JSON json.RawMessage
}

// Accounts resolves the primary account id of a capability, for the calls
// whose accountId the query leaves out.
type Accounts func(capability string) (jmapc.ID, error)

// Build returns the request a query stands for, with values filling in the
// parameters it left open and accounts filling in the account ids it did not
// state.
func Build(s *spec.Spec, q *query.Query, values map[string]Value, accounts Accounts, createdIDs map[jmapc.ID]jmapc.ID) (*jmapc.Request, error) {
	if err := CheckValues(q, values); err != nil {
		return nil, err
	}
	b := &builder{spec: s, values: values, accounts: accounts, resolved: map[string]jmapc.ID{}}
	req := &jmapc.Request{Using: q.Using, CreatedIDs: createdIDs}
	for _, c := range q.Calls {
		args := make(map[string]any, len(c.Args.Fields)+1)
		if capability, ok := b.needsAccountID(c); ok {
			id, err := b.accountID(capability)
			if err != nil {
				return nil, err
			}
			args[query.AccountIDArgument] = id
		}
		for _, f := range c.Args.Fields {
			key, err := b.key(f)
			if err != nil {
				return nil, err
			}
			value, err := b.node(f.Value)
			if err != nil {
				return nil, err
			}
			args[key] = value
		}
		req.MethodCalls = append(req.MethodCalls, jmapc.Invocation{
			Name:   c.Method.Name,
			CallID: c.ID,
			Args:   args,
		})
	}
	return req, nil
}

// CheckValues reports the parameters the caller left out and the ones they
// supplied that the query does not have, both at once: a caller who mistyped
// one name should not have to run again to learn about the next. Build calls
// it, and a caller with something to do before building may call it first.
func CheckValues(q *query.Query, values map[string]Value) error {
	wanted := make(map[string]bool, len(q.Params))
	var missing []string
	for _, p := range q.Params {
		wanted[p.Name] = true
		if _, ok := values[p.Name]; !ok {
			missing = append(missing, fmt.Sprintf("%s (%s)", p.Name, p.ValueType()))
		}
	}
	var unknown []string
	for name := range values {
		if !wanted[name] {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	var problems []string
	if len(missing) > 0 {
		problems = append(problems, fmt.Sprintf("%s takes %s: %s",
			q.Name, noun(len(missing), "a parameter that was not given", "parameters that were not given"),
			strings.Join(missing, ", ")))
	}
	for _, name := range unknown {
		problems = append(problems, fmt.Sprintf("%s has no parameter %q%s", q.Name, name, hint(name, q.Params)))
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "\n"))
	}
	return nil
}

// hint suggests the parameter the caller probably meant.
func hint(name string, params []*query.Param) string {
	for _, p := range params {
		if strings.EqualFold(p.Name, name) {
			return fmt.Sprintf("; did you mean %q?", p.Name)
		}
	}
	return ""
}

// noun renders a count's noun in the right number.
func noun(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// builder holds what filling in one request needs.
type builder struct {
	spec     *spec.Spec
	values   map[string]Value
	accounts Accounts
	resolved map[string]jmapc.ID
}

// needsAccountID reports whether a call's accountId has to be filled in, and
// from which capability's primary account. A query that does not care which
// account it runs against does not have to say so in every call.
func (b *builder) needsAccountID(c *query.Call) (string, bool) {
	args, err := b.spec.ArgumentsOf(c.Method.Name)
	if err != nil {
		return "", false
	}
	if _, takes := args.Field(query.AccountIDArgument); !takes {
		return "", false
	}
	if _, given := c.Args.Find(query.AccountIDArgument); given {
		return "", false
	}
	if _, referenced := c.Args.Find("#" + query.AccountIDArgument); referenced {
		return "", false
	}
	if c.Method.Capability == "" {
		return spec.CapabilityCore, true
	}
	return c.Method.Capability, true
}

// accountID resolves a capability's primary account, once per capability.
func (b *builder) accountID(capability string) (jmapc.ID, error) {
	if id, ok := b.resolved[capability]; ok {
		return id, nil
	}
	if b.accounts == nil {
		return "", fmt.Errorf("the query leaves accountId to the session's primary account for %s, and there is no session to ask", capability)
	}
	id, err := b.accounts(capability)
	if err != nil {
		return "", err
	}
	b.resolved[capability] = id
	return id, nil
}

// node renders one argument value. A subtree depending on no parameter is the
// JSON it already is, which keeps the request the shape the query wrote.
func (b *builder) node(n query.Node) (any, error) {
	switch v := n.(type) {
	case *query.ResultRef:
		return v.Ref, nil

	case *query.ParamRef:
		return b.values[v.Param.Name].JSON, nil

	case *query.Literal:
		return v.JSON, nil

	case *query.Object:
		if !v.HasParam() {
			return v.Raw, nil
		}
		out := make(map[string]any, len(v.Fields))
		for _, f := range v.Fields {
			key, err := b.key(f)
			if err != nil {
				return nil, err
			}
			value, err := b.node(f.Value)
			if err != nil {
				return nil, err
			}
			out[key] = value
		}
		return out, nil

	case *query.Array:
		if !v.HasParam() {
			return v.Raw, nil
		}
		out := make([]any, 0, len(v.Items))
		for _, item := range v.Items {
			value, err := b.node(item)
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		return out, nil
	}
	return nil, fmt.Errorf("cannot render a %T", n)
}

// key renders an object member name. Most names are constants, but a query may
// build one from parameters, as a patch does when it points at a property keyed
// by an id the caller chooses.
func (b *builder) key(f query.ObjectField) (string, error) {
	if len(f.KeySegments) == 0 {
		return f.Key, nil
	}
	var name strings.Builder
	for _, seg := range f.KeySegments {
		if seg.Param == nil {
			name.WriteString(seg.Text)
			continue
		}
		name.WriteString(b.values[seg.Param.Name].Text)
	}
	return name.String(), nil
}
