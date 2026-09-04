// Package limits checks a query against what a server says it will accept.
//
// The checks a build can make are checks against the specifications: that a
// method exists, that an argument belongs to it, that a value has the right
// type. What the specifications leave to the server is what it supports and
// how much of it — the capabilities it advertises, the accounts it holds, how
// many calls it takes in one request, how many objects in one /get. A query
// that is right about JMAP and wrong about the server in front of it fails at
// run time, and this is how to find that out sooner.
package limits

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/linyows/jmapc"
	"github.com/linyows/jmapc/internal/query"
	"github.com/linyows/jmapc/internal/request"
	"github.com/linyows/jmapc/internal/spec"
)

// Check reports what a server would refuse about a query, as the checks
// against the specifications report what JMAP would.
func Check(catalogue *spec.Spec, session *jmapc.Session, q *query.Query) error {
	c := &checker{catalogue: catalogue, session: session, q: q}
	core, err := session.Core()
	if err != nil {
		// A server has to advertise the core capability, and one that does
		// not is beyond checking against.
		c.errorf("using", "", "%v", err)
		return c.errs.Err()
	}
	c.capabilities()
	c.accounts()
	c.calls(core)
	c.objects(core)
	c.collations(core)
	c.size(core)
	return c.errs.Err()
}

// checker collects what one query and one session disagree about.
type checker struct {
	catalogue *spec.Spec
	session   *jmapc.Session
	q         *query.Query
	errs      query.ErrorList
}

func (c *checker) errorf(where, hint, format string, args ...any) {
	c.errs = append(c.errs, &query.Error{
		File:  c.q.Path,
		Where: where,
		Msg:   fmt.Sprintf(format, args...),
		Hint:  hint,
	})
}

// capabilities checks that the server offers what the request declares. A
// capability jmapc knows and the server does not is the difference between a
// query that is right about JMAP and one that will work here.
func (c *checker) capabilities() {
	for _, uri := range c.q.Using {
		if c.session.HasCapability(uri) {
			continue
		}
		c.errorf("using", "the server advertises "+list(capabilityURIs(c.session)),
			"the server does not advertise %s", uri)
	}
}

// accounts checks the account each call runs against: that the session has one
// to fill in where the query leaves it out, that an account the query names is
// one the session holds, and that the account supports what the call needs.
func (c *checker) accounts() {
	for i, call := range c.q.Calls {
		where := fmt.Sprintf("methodCalls[%d]", i)
		capability, needed := call.AccountIDCapability(c.catalogue)
		if needed {
			id, err := c.session.PrimaryAccountID(capability)
			if err != nil {
				c.errorf(where, `write "accountId" to name the account to use`,
					"the query leaves the account to the session, and %v", err)
				continue
			}
			c.accountSupports(where, id, capability)
			continue
		}
		stated, ok := call.Args.Find(query.AccountIDArgument)
		if !ok {
			continue
		}
		literal, ok := stated.(*query.Literal)
		if !ok {
			// A parameter, so the account is not known until the caller
			// supplies one.
			continue
		}
		var id jmapc.ID
		if err := json.Unmarshal(literal.JSON, &id); err != nil {
			continue
		}
		if _, held := c.session.Accounts[id]; !held {
			c.errorf(where+".arguments."+query.AccountIDArgument,
				"the session holds "+list(accountIDs(c.session)),
				"the session has no account %q", id)
			continue
		}
		if call.Method.Capability != "" {
			c.accountSupports(where, id, call.Method.Capability)
		}
	}
}

// accountSupports checks that an account offers a capability, where the
// session says what its accounts offer. A server that lists nothing for an
// account is not saying the account supports nothing.
func (c *checker) accountSupports(where string, id jmapc.ID, capability string) {
	account, ok := c.session.Accounts[id]
	if !ok || len(account.AccountCapabilities) == 0 {
		return
	}
	if _, offered := account.AccountCapabilities[capability]; offered {
		return
	}
	c.errorf(where, "", "the account %q does not support %s", id, capability)
}

// calls checks the length of the request against the number of calls the
// server takes in one.
func (c *checker) calls(core *jmapc.CoreCapability) {
	if core.MaxCallsInRequest == 0 || len(c.q.Calls) <= int(core.MaxCallsInRequest) {
		return
	}
	c.errorf("methodCalls", "split the query in two, carrying what the first created with "+query.CreatedIDsMember,
		"the query makes %d calls, and the server takes %d in one request",
		len(c.q.Calls), core.MaxCallsInRequest)
}

// objects checks the arguments that name records against the number of records
// the server acts on in one call.
func (c *checker) objects(core *jmapc.CoreCapability) {
	for i, call := range c.q.Calls {
		where := fmt.Sprintf("methodCalls[%d].arguments", i)
		if core.MaxObjectsInGet > 0 {
			if ids, ok := call.Args.Find("ids"); ok {
				if n, counted := length(ids); counted && n > int(core.MaxObjectsInGet) {
					c.errorf(where+".ids", "",
						"the call asks for %d records, and the server returns %d from one call",
						n, core.MaxObjectsInGet)
				}
			}
		}
		if core.MaxObjectsInSet == 0 {
			continue
		}
		total := 0
		for _, name := range []string{"create", "update", "destroy"} {
			if arg, ok := call.Args.Find(name); ok {
				if n, counted := length(arg); counted {
					total += n
				}
			}
		}
		if total > int(core.MaxObjectsInSet) {
			c.errorf(where, "",
				"the call changes %d records, and the server changes %d in one call",
				total, core.MaxObjectsInSet)
		}
	}
}

// collations checks the sort orders against the collations the server can
// compare strings with, which is the one thing about a comparator that the
// specifications leave entirely to the server.
func (c *checker) collations(core *jmapc.CoreCapability) {
	if len(core.CollationAlgorithms) == 0 {
		return
	}
	offered := make(map[string]bool, len(core.CollationAlgorithms))
	for _, name := range core.CollationAlgorithms {
		offered[name] = true
	}
	for i, call := range c.q.Calls {
		sortArg, ok := call.Args.Find("sort")
		if !ok {
			continue
		}
		comparators, ok := sortArg.(*query.Array)
		if !ok {
			continue
		}
		for j, item := range comparators.Items {
			object, ok := item.(*query.Object)
			if !ok {
				continue
			}
			value, ok := object.Find("collation")
			if !ok {
				continue
			}
			literal, ok := value.(*query.Literal)
			if !ok {
				continue
			}
			var collation string
			if err := json.Unmarshal(literal.JSON, &collation); err != nil || collation == "" {
				continue
			}
			if offered[collation] {
				continue
			}
			c.errorf(fmt.Sprintf("methodCalls[%d].arguments.sort[%d].collation", i, j),
				"the server compares with "+list(core.CollationAlgorithms),
				"the server does not offer the collation %q", collation)
		}
	}
}

// size checks the request against the largest the server accepts. What the
// caller passes for a parameter is not known here, so a parameter counts as
// the name it was written under: the request that goes out is this size at the
// least.
func (c *checker) size(core *jmapc.CoreCapability) {
	if core.MaxSizeRequest == 0 {
		return
	}
	values := make(map[string]request.Value, len(c.q.Params))
	for _, p := range c.q.Params {
		text := "{{" + p.Name + "}}"
		values[p.Name] = request.Value{Text: text, JSON: json.RawMessage(strconv.Quote(text))}
	}
	req, err := request.Build(c.catalogue, c.q, values,
		func(string) (jmapc.ID, error) { return "accountId", nil }, nil)
	if err != nil {
		return
	}
	body, err := json.Marshal(req)
	if err != nil {
		return
	}
	if len(body) <= int(core.MaxSizeRequest) {
		return
	}
	c.errorf("methodCalls", "the values the caller passes are not counted here, so what goes out is larger still",
		"the request is %d bytes before its parameters are filled in, and the server accepts %d",
		len(body), core.MaxSizeRequest)
}

// length returns how many entries an argument holds, for the arguments that
// hold a list or a map of records. It counts nothing for a value that depends
// on the caller, since how many they pass is not known here.
func length(n query.Node) (int, bool) {
	switch v := n.(type) {
	case *query.Array:
		return len(v.Items), true
	case *query.Object:
		return len(v.Fields), true
	}
	return 0, false
}

// capabilityURIs returns what the session advertises, sorted.
func capabilityURIs(s *jmapc.Session) []string {
	out := make([]string, 0, len(s.Capabilities))
	for uri := range s.Capabilities {
		out = append(out, uri)
	}
	sort.Strings(out)
	return out
}

// accountIDs returns the accounts the session holds, sorted.
func accountIDs(s *jmapc.Session) []string {
	out := make([]string, 0, len(s.Accounts))
	for id := range s.Accounts {
		out = append(out, string(id))
	}
	sort.Strings(out)
	return out
}

// list renders names for a hint, as prose rather than as a dump.
func list(names []string) string {
	switch len(names) {
	case 0:
		return "nothing"
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
