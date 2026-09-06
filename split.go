package jmapc

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// errorMethod is the name a response gives an invocation carrying a
// method-level error, in place of the method that was called.
const errorMethod = "error"

// WithSplitGets sends a /get holding more ids than the server's
// maxObjectsInGet in several requests rather than letting the server refuse
// it, and joins what comes back into the one response the caller asked for.
//
// It is off by default because one call to Do then costs several round trips,
// and because the records no longer arrive as one snapshot: each request is
// answered separately, and the account may change between them. Where the
// state a /get reports differs between requests, the joined response is
// returned together with a *StateChanged, which errors.As reaches.
//
// Only the ids written into the query are counted. A call whose ids come from
// a back reference is sent as it is, since their number is known to the server
// alone, and so is a call another call refers to, since a reference resolves
// within one request and splitting the call it names would leave nothing to
// resolve against.
func WithSplitGets() Option {
	return func(c *Client) { c.splitGets = true }
}

// StateChanged reports that a /get answered in several requests reported more
// than one state, so the records it returned are not one snapshot of the
// account. The response is returned with it, since the records are the ones
// the server held at the time each request reached it.
type StateChanged struct {
	// Method is the method that was split, such as "Email/get".
	Method string
	// CallID is the id the request gave that call.
	CallID string
	// From is the state the first request reported, and To the state of the
	// request that differed from it.
	From, To string
}

func (e *StateChanged) Error() string {
	return fmt.Sprintf("jmapc: %s was sent in several requests and the state changed from %q to %q between them; "+
		"the records it returned are not one snapshot", e.Method, e.From, e.To)
}

// splitPart is one of the further requests a split needs, and the calls in it
// whose answers are joined onto the answer of the request the caller made.
type splitPart struct {
	request *Request
	chunks  []splitChunk
}

// splitChunk names one call of a further request and the call of the original
// request its answer belongs to.
type splitChunk struct {
	callID string
	into   string
}

// planSplit returns the request to send in place of r, and the further
// requests carrying the ids that did not fit. Where nothing is split, it
// returns r and no further requests.
func (c *Client) planSplit(r *Request) (*Request, []splitPart) {
	if !c.splitGets || r == nil {
		return r, nil
	}
	limit := c.limit(func(core *CoreCapability) UnsignedInt { return core.MaxObjectsInGet })
	if limit <= 0 {
		return r, nil
	}

	referenced := referencedCalls(r)
	taken := make(map[string]bool, len(r.MethodCalls))
	for _, call := range r.MethodCalls {
		taken[call.CallID] = true
	}

	var out *Request
	var extra []Invocation
	var chunks []splitChunk
	for i, call := range r.MethodCalls {
		args, ids, ok := splittableGet(call, limit, referenced)
		if !ok {
			continue
		}
		if out == nil {
			copied := *r
			copied.MethodCalls = append([]Invocation(nil), r.MethodCalls...)
			out = &copied
		}
		out.MethodCalls[i].Args = argsWithIDs(args, ids[:limit])
		for rest := ids[limit:]; len(rest) > 0; {
			size := min(limit, len(rest))
			id := unusedCallID(taken, call.CallID)
			extra = append(extra, Invocation{
				Name:   call.Name,
				CallID: id,
				Args:   argsWithIDs(args, rest[:size]),
			})
			chunks = append(chunks, splitChunk{callID: id, into: call.CallID})
			rest = rest[size:]
		}
	}
	if out == nil {
		return r, nil
	}
	return out, c.groupParts(r, extra, chunks)
}

// groupParts puts the calls that did not fit into requests of their own, no
// more of them in one than the server takes.
func (c *Client) groupParts(r *Request, calls []Invocation, chunks []splitChunk) []splitPart {
	perRequest := len(calls)
	if max := c.limit(func(core *CoreCapability) UnsignedInt { return core.MaxCallsInRequest }); max > 0 && max < perRequest {
		perRequest = max
	}
	var parts []splitPart
	for start := 0; start < len(calls); start += perRequest {
		end := min(start+perRequest, len(calls))
		parts = append(parts, splitPart{
			request: &Request{Using: r.Using, MethodCalls: calls[start:end]},
			chunks:  chunks[start:end],
		})
	}
	return parts
}

// splittableGet reports whether a call is a /get holding more ids than the
// server takes, and returns its arguments and those ids.
func splittableGet(call Invocation, limit int, referenced map[string]bool) (map[string]json.RawMessage, []json.RawMessage, bool) {
	if !strings.HasSuffix(call.Name, "/get") || call.CallID == "" || referenced[call.CallID] {
		return nil, nil, false
	}
	raw, err := json.Marshal(call.Args)
	if err != nil {
		return nil, nil, false
	}
	var args map[string]json.RawMessage
	if err := json.Unmarshal(raw, &args); err != nil || args == nil {
		return nil, nil, false
	}
	var ids []json.RawMessage
	if err := json.Unmarshal(args["ids"], &ids); err != nil || len(ids) <= limit {
		return nil, nil, false
	}
	return args, ids, true
}

// argsWithIDs copies a call's arguments with a different set of ids.
func argsWithIDs(args map[string]json.RawMessage, ids []json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(args))
	for name, value := range args {
		out[name] = value
	}
	list, err := json.Marshal(ids)
	if err != nil {
		// ids came out of a decoded array, so re-encoding them cannot fail.
		panic("jmapc: encoding the ids of a split call: " + err.Error())
	}
	out["ids"] = list
	return out
}

// referencedCalls reports which calls another call refers to by result
// reference.
func referencedCalls(r *Request) map[string]bool {
	out := map[string]bool{}
	for _, call := range r.MethodCalls {
		raw, err := json.Marshal(call.Args)
		if err != nil {
			continue
		}
		var args map[string]json.RawMessage
		if err := json.Unmarshal(raw, &args); err != nil {
			continue
		}
		for _, value := range args {
			var ref ResultReference
			if err := json.Unmarshal(value, &ref); err == nil && ref.ResultOf != "" {
				out[ref.ResultOf] = true
			}
		}
	}
	return out
}

// unusedCallID returns a call id for one part of a split call, not already in
// use by another call.
func unusedCallID(taken map[string]bool, base string) string {
	for n := 1; ; n++ {
		id := fmt.Sprintf("%s~%d", base, n)
		if !taken[id] {
			taken[id] = true
			return id
		}
	}
}

// joinSplit joins the answers of one further request onto the answers of the
// request the caller made.
func joinSplit(into *Response, part *Response, chunks []splitChunk) error {
	var errs []error
	for _, chunk := range chunks {
		answer, ok := part.find(chunk.callID)
		if !ok {
			errs = append(errs, fmt.Errorf("jmapc: the server answered nothing to %q, one part of the split call %q",
				chunk.callID, chunk.into))
			continue
		}
		target, ok := into.find(chunk.into)
		if !ok {
			errs = append(errs, fmt.Errorf("jmapc: the server answered nothing to the split call %q", chunk.into))
			continue
		}
		if err := joinAnswer(target, answer); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// joinAnswer joins one answer onto another. A part the server refused replaces
// the answer being built, since the call as a whole did not return every
// record the caller asked for.
func joinAnswer(target, answer *Invocation) error {
	if answer.Name == errorMethod {
		target.Name, target.Args = answer.Name, answer.Args
		return nil
	}
	if target.Name == errorMethod {
		return nil
	}
	into, err := rawArgsMap(target)
	if err != nil {
		return err
	}
	from, err := rawArgsMap(answer)
	if err != nil {
		return err
	}
	for _, name := range []string{"list", "notFound"} {
		joined, err := joinJSONArrays(into[name], from[name])
		if err != nil {
			return fmt.Errorf("jmapc: joining the %s of %s: %w", name, target.Name, err)
		}
		if joined != nil {
			into[name] = joined
		}
	}
	args, err := json.Marshal(into)
	if err != nil {
		return fmt.Errorf("jmapc: encoding the joined answer to %s: %w", target.Name, err)
	}
	target.Args = json.RawMessage(args)

	var first, next string
	_ = json.Unmarshal(into["state"], &first)
	_ = json.Unmarshal(from["state"], &next)
	if first != next {
		return &StateChanged{Method: target.Name, CallID: target.CallID, From: first, To: next}
	}
	return nil
}

// rawArgsMap decodes the arguments of an answer.
func rawArgsMap(in *Invocation) (map[string]json.RawMessage, error) {
	raw, ok := in.RawArgs()
	if !ok {
		return nil, fmt.Errorf("jmapc: the answer to %s was not read from a response", in.Name)
	}
	var args map[string]json.RawMessage
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("jmapc: the answer to %s is not an object: %w", in.Name, err)
	}
	return args, nil
}

// joinJSONArrays appends one JSON array to another, treating a member that is
// absent or null as an empty array. It returns nil where neither is there.
func joinJSONArrays(first, second json.RawMessage) (json.RawMessage, error) {
	if first == nil && second == nil {
		return nil, nil
	}
	values, err := jsonArray(first)
	if err != nil {
		return nil, err
	}
	rest, err := jsonArray(second)
	if err != nil {
		return nil, err
	}
	joined, err := json.Marshal(append(values, rest...))
	if err != nil {
		return nil, err
	}
	return joined, nil
}

// jsonArray decodes a JSON array, and reads an absent or null value as one
// with nothing in it.
func jsonArray(raw json.RawMessage) ([]json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	return values, nil
}
