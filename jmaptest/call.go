package jmaptest

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/linyows/jmapc"
)

// Call is one method call as it arrived, with its back references already
// resolved: an argument the client left to the server is in Args with the
// value the earlier call answered with, under the name it fills in.
type Call struct {
	// Method is the method name, such as "Email/get".
	Method string
	// ID is the call id, which back references and the response use to refer
	// to this call.
	ID string
	// Args are the arguments, each still as the JSON it arrived as.
	Args map[string]json.RawMessage

	// refs are the back references, kept so that a test can see what the
	// client asked for as well as what it was given.
	refs map[string]jmapc.ResultReference
}

// decodeCall reads one method call of a request.
func decodeCall(raw json.RawMessage) (*Call, error) {
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil || len(parts) != 3 {
		return nil, fmt.Errorf("a method call is written as [name, arguments, callId], and this is %s", raw)
	}
	c := &Call{Args: map[string]json.RawMessage{}, refs: map[string]jmapc.ResultReference{}}
	if err := json.Unmarshal(parts[0], &c.Method); err != nil {
		return nil, fmt.Errorf("the method name is not a string: %s", parts[0])
	}
	if err := json.Unmarshal(parts[2], &c.ID); err != nil {
		return nil, fmt.Errorf("the call id is not a string: %s", parts[2])
	}
	if err := json.Unmarshal(parts[1], &c.Args); err != nil {
		return nil, fmt.Errorf("the arguments of %s are not an object: %s", c.Method, parts[1])
	}
	return c, nil
}

// resolve fills in the arguments the client left to the server, from the
// responses to the calls before this one. RFC 8620, Section 3.7: the argument
// is named with a leading "#", and its value says which call to read, which
// method that call had to be, and where in its response to look.
func (c *Call) resolve(answered map[string]json.RawMessage, names map[string]string) error {
	for key, raw := range c.Args {
		name, isRef := strings.CutPrefix(key, "#")
		if !isRef {
			continue
		}
		var ref jmapc.ResultReference
		if err := json.Unmarshal(raw, &ref); err != nil {
			return fmt.Errorf("the back reference %s is not one: %s", key, raw)
		}
		response, ok := answered[ref.ResultOf]
		if !ok {
			return fmt.Errorf("%s refers to the call %q, which is not one this request has run", key, ref.ResultOf)
		}
		if names[ref.ResultOf] != ref.Name {
			return fmt.Errorf("%s says the call %q invoked %s, and it invoked %s",
				key, ref.ResultOf, ref.Name, names[ref.ResultOf])
		}
		var value any
		if err := json.Unmarshal(response, &value); err != nil {
			return fmt.Errorf("the response to %q is not JSON: %v", ref.ResultOf, err)
		}
		selected, err := pointer(value, ref.Path)
		if err != nil {
			return fmt.Errorf("%s: %v", key, err)
		}
		filled, err := json.Marshal(selected)
		if err != nil {
			return fmt.Errorf("%s: encoding what it selected: %v", key, err)
		}
		delete(c.Args, key)
		c.Args[name] = filled
		c.refs[name] = ref
	}
	return nil
}

// Arg reads one argument into dest, and reports an argument that is not there
// as an error rather than as a zero value, since the two mean different things
// to a JMAP method.
func (c *Call) Arg(name string, dest any) error {
	raw, ok := c.Args[name]
	if !ok {
		return fmt.Errorf("jmaptest: %s was called without %s", c.Method, name)
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("jmaptest: reading the %s of %s: %w", name, c.Method, err)
	}
	return nil
}

// AccountID is the account the call names, which generated code fills in from
// the session where the query leaves it out.
func (c *Call) AccountID() jmapc.ID {
	var id jmapc.ID
	_ = c.Arg("accountId", &id)
	return id
}

// IDs are the records the call names, whether the client listed them or a back
// reference filled them in.
func (c *Call) IDs() []jmapc.ID {
	var ids []jmapc.ID
	_ = c.Arg("ids", &ids)
	return ids
}

// Reference returns the back reference that filled an argument in, and whether
// one did.
func (c *Call) Reference(argument string) (jmapc.ResultReference, bool) {
	ref, ok := c.refs[argument]
	return ref, ok
}

// String renders the call the way a request holds it, for a message about what
// was sent.
func (c *Call) String() string {
	names := make([]string, 0, len(c.Args))
	for name := range c.Args {
		names = append(names, name)
	}
	sort.Strings(names)
	return fmt.Sprintf("%s(%s) as %q", c.Method, strings.Join(names, ", "), c.ID)
}

// pointer selects a value from a response, following the JMAP pointer of
// RFC 8620, Section 3.7. It is a JSON pointer with one addition: "*" maps the
// rest of the path over an array and flattens the result by one level, which
// is what turns a list of records into a list of the ids in them.
func pointer(value any, path string) (any, error) {
	if path == "" || path == "/" {
		return value, nil
	}
	rest := strings.TrimPrefix(path, "/")
	for rest != "" {
		var token string
		token, rest, _ = strings.Cut(rest, "/")
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")

		if token == "*" {
			items, ok := value.([]any)
			if !ok {
				return nil, fmt.Errorf("the path %q maps over something that is not an array", path)
			}
			out := make([]any, 0, len(items))
			for _, item := range items {
				selected, err := pointer(item, "/"+rest)
				if err != nil {
					return nil, err
				}
				// Mapping flattens by one level, so that a path through a list
				// of lists comes back as one list.
				if nested, isList := selected.([]any); isList {
					out = append(out, nested...)
					continue
				}
				out = append(out, selected)
			}
			return out, nil
		}

		switch holder := value.(type) {
		case map[string]any:
			next, ok := holder[token]
			if !ok {
				return nil, fmt.Errorf("the path %q asks for %q, which the response does not have", path, token)
			}
			value = next
		case []any:
			i, err := strconv.Atoi(token)
			if err != nil || i < 0 || i >= len(holder) {
				return nil, fmt.Errorf("the path %q asks for %q of an array of %d", path, token, len(holder))
			}
			value = holder[i]
		default:
			return nil, fmt.Errorf("the path %q asks for %q of something that has no members", path, token)
		}
	}
	return value, nil
}
