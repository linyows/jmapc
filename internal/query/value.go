package query

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/linyows/jmapc"
	"github.com/linyows/jmapc/internal/spec"
)

// paramPattern matches a value the query leaves open. The braces are used
// rather than a "$" prefix because JMAP keywords are themselves written with a
// leading "$", as in "$seen" and "$flagged".
var paramPattern = regexp.MustCompile(`^\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}$`)

// embeddedParamPattern finds a parameter anywhere within a member name. A value
// has to be a parameter outright, because its type comes from the argument it
// fills, but a name is text and may be built from pieces: a patch pointing at
// "mailboxIds/{{mailboxId}}" names a property the caller chooses.
var embeddedParamPattern = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// optionalParamPattern matches a parameter the caller may leave out. The
// question mark is asked at the point the value would go, because that is
// where leaving it out shows: the argument is not in the request at all.
var optionalParamPattern = regexp.MustCompile(`^\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\?\s*\}\}$`)

// optionalParam records a use of a parameter the caller may leave out. Leaving
// one out takes the argument it stands for out of the request, so it is only
// meaningful where the parameter is the whole of an argument.
func (c *checker) optionalParam(name string, t *spec.Type, where, doc string, allowed bool) *Param {
	p := c.params.use(c, name, t, where, doc, false)
	if !allowed {
		c.errorf(where, fmt.Sprintf("write it as {{%s}}, since this value is always there", name),
			"only an argument of a method call may be left out, and %q stands for part of one", name)
		return p
	}
	p.Optional = true
	return p
}

// value checks one JSON value against the type the JMAP data model says it must
// have, and returns the node the generator will emit for it.
func (c *checker) value(t *spec.Type, raw json.RawMessage, where, doc string) Node {
	raw = json.RawMessage(bytes.TrimSpace(raw))

	// Whether a value may be left out is settled by where it sits, and nothing
	// under it sits there, so the permission does not travel down.
	mayBeOptional := c.argumentValue
	c.argumentValue = false

	if s, isString := stringValue(raw); isString {
		if m := paramPattern.FindStringSubmatch(s); m != nil {
			return &ParamRef{Param: c.params.use(c, m[1], t, where, doc, false)}
		}
		if m := optionalParamPattern.FindStringSubmatch(s); m != nil {
			return &ParamRef{Param: c.optionalParam(m[1], t, where, doc, mayBeOptional)}
		}
		if strings.Contains(s, "{{") {
			c.errorf(where, "write the whole value as {{name}}",
				"a parameter cannot be embedded in a larger string")
			return &Literal{JSON: raw}
		}
	}

	if isNull(raw) {
		if !t.Nullable && t.Name != spec.Any {
			c.errorf(where, "", "%s may not be null", t)
		}
		return &Literal{JSON: raw}
	}

	switch {
	case t.Name == spec.Any:
		return c.anyValue(raw)
	case t.IsUnion():
		return c.union(t, raw, where, doc)
	case t.IsArray():
		return c.array(t, raw, where, doc)
	case t.IsMap():
		return c.mapValue(t, raw, where, doc)
	case t.IsObject():
		return c.object(t, raw, where)
	default:
		return c.primitive(t, raw, where)
	}
}

// union checks a value that may take either of several shapes, which is how
// JMAP spells a /query filter. It reports the failure of the closest-fitting
// alternative rather than of all of them, because a filter that is nearly a
// valid condition is more usefully described as that condition with one thing
// wrong.
func (c *checker) union(t *spec.Type, raw json.RawMessage, where, doc string) Node {
	saved := c.filterUnion
	if unionMentions(t, "FilterOperator") {
		c.filterUnion = t
	}
	defer func() { c.filterUnion = saved }()

	var best Node
	var bestErrs ErrorList
	for _, member := range c.rankUnion(t, raw) {
		node, errs := c.try(func() Node { return c.value(member, raw, where, doc) })
		if len(errs) == 0 {
			return node
		}
		if best == nil || len(errs) < len(bestErrs) {
			best, bestErrs = node, errs
		}
	}
	c.errs = append(c.errs, bestErrs...)
	return best
}

// rankUnion orders the alternatives of a union by how well each one fits the
// value at hand, so that a value that is nearly one shape is reported as that
// shape with something wrong, rather than as the wrong shape entirely. Without
// this, a filter condition with one bad property would be reported as a
// FilterOperator missing its operator.
func (c *checker) rankUnion(t *spec.Type, raw json.RawMessage) []*spec.Type {
	members := append([]*spec.Type(nil), t.Union...)
	keys, err := objectKeys(raw)
	if err != nil || len(keys) == 0 {
		return members
	}
	present := make(map[string]bool, len(keys))
	for _, key := range keys {
		present[key] = true
	}
	score := make(map[*spec.Type]int, len(members))
	for _, m := range members {
		o, ok := c.spec.Object(m.Name)
		if !ok {
			continue
		}
		if missingRequired(o, present) {
			// The value cannot be of this type at all, whatever else it holds.
			// Without this, a filter condition with a misspelled property and
			// nothing else would be reported as a malformed FilterOperator.
			score[m] = -1
			continue
		}
		for _, key := range keys {
			if _, known := o.Field(key); known {
				score[m]++
			}
		}
	}
	sort.SliceStable(members, func(i, j int) bool {
		return score[members[i]] > score[members[j]]
	})
	return members
}

// checkEnum reports a value the property's specification does not allow. It
// does nothing where the property has no fixed set, which is most of them.
func (c *checker) checkEnum(value, where string) {
	if len(c.enum) == 0 {
		return
	}
	for _, allowed := range c.enum {
		if value == allowed {
			return
		}
	}
	c.errorf(where, hintFor(value, c.enum), "%q is not one of the values this property takes (%s)",
		value, strings.Join(c.enum, ", "))
}

// propertyHint suggests what an unknown property in a path may have meant.
func propertyHint(err error) string {
	var unknown *spec.UnknownPropertyError
	if !errors.As(err, &unknown) {
		return ""
	}
	return hintFor(unknown.Property, unknown.Known)
}

// missingRequired reports whether an object type declares a property that the
// value does not have.
func missingRequired(o *spec.Object, present map[string]bool) bool {
	for _, f := range o.Fields {
		if f.Required && !present[f.Name] {
			return true
		}
	}
	return false
}

// try runs f and, if it reports any problem, undoes everything f recorded and
// returns the problems instead. It lets a union try each alternative without
// leaving the parameters or errors of a failed attempt behind.
func (c *checker) try(f func() Node) (Node, ErrorList) {
	errMark, paramMark := len(c.errs), c.params.mark()
	node := f()
	if len(c.errs) > errMark {
		errs := append(ErrorList(nil), c.errs[errMark:]...)
		c.errs = c.errs[:errMark]
		c.params.rollback(paramMark)
		return node, errs
	}
	return node, nil
}

// array checks a JSON array against an array type.
func (c *checker) array(t *spec.Type, raw json.RawMessage, where, doc string) Node {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		c.errorf(where, "", "expected %s, found %s", t, jsonKind(raw))
		return &Literal{JSON: raw}
	}
	out := &Array{Items: make([]Node, 0, len(items)), Raw: raw}
	elem := elemDoc(t.Elem, doc)
	for i, item := range items {
		out.Items = append(out.Items, c.value(t.Elem, item, fmt.Sprintf("%s[%d]", where, i), elem))
	}
	return out
}

// mapValue checks a JSON object against a map type, such as the mailboxIds of
// an Email or the create map of a /set call.
func (c *checker) mapValue(t *spec.Type, raw json.RawMessage, where, doc string) Node {
	members, keys, ok := c.objectMembers(t, raw, where)
	if !ok {
		return &Literal{JSON: raw}
	}
	// The keys of a create argument are the names the query gives the records
	// it makes, and nothing under them is one, so the permission does not
	// travel down.
	creationIDs := c.creationIDs
	c.creationIDs = false

	out := &Object{Raw: raw}
	for _, key := range keys {
		keyType := t.Key
		if keyType == nil {
			keyType = &spec.Type{Name: spec.String}
		}
		if creationIDs && !embeddedParamPattern.MatchString(key) {
			c.creations = append(c.creations, key)
		}
		field := ObjectField{Key: key, KeySegments: c.keySegments(key, keyType, where, doc)}
		if field.KeySegments == nil && keyType.Name == spec.IdType &&
			!isCreationID(key) && !jmapc.ID(key).Valid() {
			c.errorf(where+"."+key, "", "%q is not a valid id", key)
		}
		if field.KeySegments == nil && keyType.Name == spec.String {
			// A set such as a participant's roles fixes its keys, not the
			// booleans they map to.
			c.checkEnum(key, where+"."+key)
		}
		field.Value = c.value(t.Value, members[key], where+"."+key, doc)
		out.Fields = append(out.Fields, field)
	}
	return out
}

// object checks a JSON object against a named object type.
func (c *checker) object(t *spec.Type, raw json.RawMessage, where string) Node {
	o, known := c.spec.Object(t.Name)
	if !known {
		c.errorf(where, "", "unknown type %q in the data model", t.Name)
		return &Literal{JSON: raw}
	}
	members, keys, ok := c.objectMembers(t, raw, where)
	if !ok {
		return &Literal{JSON: raw}
	}

	// A Comparator's members depend on the property it sorts by, so it too is
	// resolved against the type being queried.
	if o.Name == "Comparator" && c.sortTarget != "" {
		return c.comparator(members, keys, raw, where)
	}

	// A PatchObject is keyed by JSON pointer rather than by property name, so
	// its members are resolved against the type being patched instead of
	// against a fixed set of properties.
	if len(o.Fields) == 0 {
		return c.patchObject(members, keys, raw, where)
	}

	out := &Object{Raw: raw}
	for _, key := range keys {
		field, isKnown := o.Field(key)
		if !isKnown {
			c.errorf(where+"."+key, hintFor(key, o.PropertyNames()), "%s has no property %q", o.Name, key)
			continue
		}
		elemType := field.ParsedType()
		if o.Name == "FilterOperator" && key == "conditions" && c.filterUnion != nil {
			// The conditions of a filter operator are filters of the same kind
			// as the one being built, which the data model can only describe
			// as Any.
			elemType = &spec.Type{Elem: c.filterUnion}
		}
		// A property may itself carry patches or comparators, as the
		// localizations of a contact card carry patches to the card. Their
		// target travels with them, wherever in the arguments they turn up.
		savedPatch, savedSort, savedEnum := c.patchTarget, c.sortTarget, c.enum
		if field.PatchTarget != "" {
			c.patchTarget = field.PatchTarget
		}
		if field.SortTarget != "" {
			c.sortTarget = field.SortTarget
		}
		c.enum = field.Enum
		c.useCapability(field)
		value := c.value(elemType, members[key], where+"."+key, field.Doc)
		c.patchTarget, c.sortTarget, c.enum = savedPatch, savedSort, savedEnum
		out.Fields = append(out.Fields, ObjectField{Key: key, Value: value})
	}
	return out
}

// objectMembers decodes a JSON object, preserving the order its members were
// written in.
func (c *checker) objectMembers(t *spec.Type, raw json.RawMessage, where string) (map[string]json.RawMessage, []string, bool) {
	keys, err := objectKeys(raw)
	if err != nil {
		c.errorf(where, "", "expected %s, found %s", t, jsonKind(raw))
		return nil, nil, false
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		c.errorf(where, "", "expected %s, found %s", t, jsonKind(raw))
		return nil, nil, false
	}
	return members, keys, true
}

// anyValue accepts a value the data model does not constrain, keeping it as it
// was written.
func (c *checker) anyValue(raw json.RawMessage) Node {
	return &Literal{JSON: raw}
}

// primitive checks a value against one of the primitive JMAP types.
func (c *checker) primitive(t *spec.Type, raw json.RawMessage, where string) Node {
	kind := jsonKind(raw)
	fail := func() Node {
		c.errorf(where, "", "expected %s, found %s", t, kind)
		return &Literal{JSON: raw}
	}

	switch t.Name {
	case spec.String:
		if kind != "a string" {
			return fail()
		}
		if s, ok := stringValue(raw); ok {
			c.checkEnum(s, where)
		}
	case spec.Boolean:
		if kind != "a boolean" {
			return fail()
		}
	case spec.Number:
		if kind != "a number" {
			return fail()
		}
	case spec.Int, spec.UnsignedInt:
		if kind != "a number" {
			return fail()
		}
		var n json.Number
		if err := json.Unmarshal(raw, &n); err != nil {
			return fail()
		}
		i, err := n.Int64()
		if err != nil {
			c.errorf(where, "", "expected %s, found the fractional number %s", t, n)
			return &Literal{JSON: raw}
		}
		if t.Name == spec.UnsignedInt && i < 0 {
			c.errorf(where, "", "expected %s, found the negative number %s", t, n)
			return &Literal{JSON: raw}
		}
		if i > int64(jmapc.MaxInt) || i < int64(jmapc.MinInt) {
			c.errorf(where, "JMAP integers are limited to the range a double can hold exactly",
				"%s is outside the range of %s", n, t)
			return &Literal{JSON: raw}
		}
	case spec.IdType:
		s, ok := stringValue(raw)
		if !ok {
			return fail()
		}
		if !isCreationID(s) && !jmapc.ID(s).Valid() {
			c.errorf(where, "an id is 1 to 255 characters from A-Z, a-z, 0-9, _ and -, and a creation id is written as \"#\" followed by such a name",
				"%q is not a valid id", s)
		}
	case spec.UTCDateType:
		s, ok := stringValue(raw)
		if !ok {
			return fail()
		}
		if _, err := time.Parse("2006-01-02T15:04:05Z", s); err != nil {
			c.errorf(where, "a UTCDate is written as 2006-01-02T15:04:05Z", "%q is not a UTCDate", s)
		}
	case spec.DateType:
		s, ok := stringValue(raw)
		if !ok {
			return fail()
		}
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			c.errorf(where, "a Date is written as 2006-01-02T15:04:05Z07:00", "%q is not a Date", s)
		}
	case spec.LocalDateTimeType:
		s, ok := stringValue(raw)
		if !ok {
			return fail()
		}
		if !jmapc.LocalDateTime(s).Valid() {
			c.errorf(where, "a LocalDateTime is written as 2006-01-02T15:04:05, with no time zone",
				"%q is not a LocalDateTime", s)
		}
	case spec.DurationType:
		s, ok := stringValue(raw)
		if !ok {
			return fail()
		}
		if !jmapc.Duration(s).Valid() {
			c.errorf(where, "a Duration is written as PT1H30M or P1D, with no years or months",
				"%q is not a Duration", s)
		}
	case spec.SignedDurationType:
		s, ok := stringValue(raw)
		if !ok {
			return fail()
		}
		if !jmapc.SignedDuration(s).Valid() {
			c.errorf(where, "a SignedDuration is a Duration, optionally prefixed with - or +",
				"%q is not a SignedDuration", s)
		}
	case spec.TimeZoneIDType:
		if _, ok := stringValue(raw); !ok {
			return fail()
		}
	default:
		c.errorf(where, "", "unknown type %q in the data model", t.Name)
	}
	return &Literal{JSON: raw}
}

// keyDoc describes what a parameter standing in for a map key selects. The
// context is the documentation of the property holding the map, which says
// which map this is a key into; on its own, "the id of the record" could be a
// key into anything.
func keyDoc(keyType *spec.Type, context string) string {
	lead := "The key this entry is stored under."
	if keyType.Name == spec.IdType {
		lead = "The id of the record this entry applies to."
	}
	if context == "" {
		return lead
	}
	return lead + "\n\n" + context
}

// elemDoc describes a value inside a list the way keyDoc describes the key of
// a map. The documentation of the argument holding the list is about all of
// them and reads as a plural, which a parameter standing for one of them is
// not: an "ids" that may be null describes the argument, not the single id
// written in it.
func elemDoc(elemType *spec.Type, context string) string {
	lead := "One of the values in the list."
	if elemType != nil && elemType.Name == spec.IdType {
		lead = "One of the ids in the list."
	}
	if context == "" {
		return lead
	}
	return lead + "\n\n" + context
}

// keySegments splits a member name into its literal and parameter parts, and
// returns nil when the name holds no parameter at all.
//
// A keyType of nil means the name itself says nothing about what the parameter
// is, so it is recorded weakly: another use of the same parameter, somewhere
// that does say, settles its type.
func (c *checker) keySegments(key string, keyType *spec.Type, where, doc string) []KeySegment {
	matches := embeddedParamPattern.FindAllStringSubmatchIndex(key, -1)
	if len(matches) == 0 {
		return nil
	}
	weak := keyType == nil
	if weak {
		keyType = &spec.Type{Name: spec.String}
		doc = "The name of the property this patch applies to."
	} else {
		doc = keyDoc(keyType, doc)
	}
	var segments []KeySegment
	last := 0
	for _, m := range matches {
		if m[0] > last {
			segments = append(segments, KeySegment{Text: key[last:m[0]]})
		}
		segments = append(segments, KeySegment{
			Param: c.params.use(c, key[m[2]:m[3]], keyType, where+"."+key, doc, weak),
		})
		last = m[1]
	}
	if last < len(key) {
		segments = append(segments, KeySegment{Text: key[last:]})
	}
	return segments
}

// isCreationID reports whether s is a reference to a record created earlier in
// the same request, which RFC 8620, Section 5.3, writes as "#" followed by the
// creation id.
func isCreationID(s string) bool {
	return strings.HasPrefix(s, "#") && jmapc.ID(s[1:]).Valid()
}

// unionMentions reports whether a union type has a member with the given name.
func unionMentions(t *spec.Type, name string) bool {
	for _, m := range t.Union {
		if m.Name == name {
			return true
		}
	}
	return false
}

// assignable reports whether a value of type got may stand in where want is
// expected. It is what decides whether a back reference feeds the argument it
// points at.
func assignable(got, want *spec.Type) bool {
	if got == nil || want == nil {
		return false
	}
	if want.Name == spec.Any || got.Name == spec.Any {
		return true
	}
	if want.IsUnion() {
		for _, m := range want.Union {
			if assignable(got, m) {
				return true
			}
		}
		return false
	}
	if got.IsUnion() {
		for _, m := range got.Union {
			if !assignable(m, want) {
				return false
			}
		}
		return true
	}
	switch {
	case want.IsArray():
		return got.IsArray() && assignable(got.Elem, want.Elem)
	case want.IsMap():
		return got.IsMap() && assignable(got.Key, want.Key) && assignable(got.Value, want.Value)
	default:
		return got.Name == want.Name
	}
}

// objectKeys returns the member names of a JSON object in the order they were
// written, which decoding into a map would lose.
func objectKeys(raw json.RawMessage) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("not an object")
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("object key is not a string")
		}
		keys = append(keys, key)
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

// jsonKind names the kind of a JSON value, for use in error messages.
func jsonKind(raw json.RawMessage) string {
	raw = json.RawMessage(bytes.TrimSpace(raw))
	if len(raw) == 0 {
		return "nothing"
	}
	switch raw[0] {
	case '{':
		return "an object"
	case '[':
		return "an array"
	case '"':
		return "a string"
	case 't', 'f':
		return "a boolean"
	case 'n':
		return "null"
	default:
		return "a number"
	}
}

// isNull reports whether raw is the JSON null literal.
func isNull(raw json.RawMessage) bool {
	return string(bytes.TrimSpace(raw)) == "null"
}

// stringValue decodes raw as a JSON string.
func stringValue(raw json.RawMessage) (string, bool) {
	raw = json.RawMessage(bytes.TrimSpace(raw))
	if len(raw) == 0 || raw[0] != '"' {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

// sortStrings sorts a slice of strings in place.
func sortStrings(s []string) { sort.Strings(s) }

// patchObject checks the members of a PatchObject: each key is a JSON pointer
// into the record being patched, and each value is what that pointer should be
// set to, or null to remove it.
//
// Without knowing what is being patched there is nothing to check, and a typo
// like "mailboxIds/xxx" spelled "mailboxIDs/xxx" would go to the server as a
// property nothing reads. The catalogue records the target on the argument that
// carries the patch, and c.patchTarget carries it down to here.
func (c *checker) patchObject(members map[string]json.RawMessage, keys []string, raw json.RawMessage, where string) Node {
	out := &Object{Raw: raw}
	for _, key := range keys {
		if strings.HasPrefix(key, "/") {
			c.errorf(where+"."+key, fmt.Sprintf("write it as %q", strings.TrimPrefix(key, "/")),
				"the leading %q of a patch key is there already, so %q asks for a property with no name",
				"/", key)
			continue
		}
		field := ObjectField{Key: key}
		segments := strings.Split(key, "/")
		unknown := make([]bool, len(segments))
		for i, seg := range segments {
			unknown[i] = embeddedParamPattern.MatchString(seg)
		}

		valueType := &spec.Type{Name: spec.Any}
		var keyTypes []*spec.Type
		var target *spec.Field
		if c.patchTarget != "" {
			resolved, value, resolvedField, err := c.spec.ResolvePatch(c.patchTarget, segments, unknown)
			if err != nil {
				c.errorf(where+"."+key, propertyHint(err), "%v", err)
				continue
			}
			keyTypes, valueType, target = resolved, value, resolvedField
		}

		field.KeySegments = c.patchKeySegments(segments, keyTypes, where+"."+key)
		// null in a patch means "remove this", so it is allowed wherever a value
		// is, whether or not the property itself may hold null.
		removable := *valueType
		removable.Nullable = true

		var valueDoc string
		savedEnum := c.enum
		c.enum = nil
		if target != nil {
			valueDoc, c.enum = target.Doc, target.Enum
		}
		field.Value = c.value(&removable, members[key], where+"."+key, valueDoc)
		c.enum = savedEnum
		out.Fields = append(out.Fields, field)
	}
	return out
}

// patchKeySegments turns the segments of a patch pointer into the pieces the
// generator joins back together, recording a parameter for each one a query
// left open. A segment's type comes from what the pointer selects by at that
// depth, so a parameter naming a mailbox in "mailboxIds/{{id}}" is an Id, the
// same as it would be anywhere else.
func (c *checker) patchKeySegments(segments []string, keyTypes []*spec.Type, where string) []KeySegment {
	var out []KeySegment
	var found bool
	for i, seg := range segments {
		if i > 0 {
			out = append(out, KeySegment{Text: "/"})
		}
		matches := embeddedParamPattern.FindAllStringSubmatchIndex(seg, -1)
		if len(matches) == 0 {
			out = append(out, KeySegment{Text: seg})
			continue
		}
		found = true
		segType, weak := &spec.Type{Name: spec.String}, true
		if i < len(keyTypes) && keyTypes[i] != nil && keyTypes[i].Name != spec.Any {
			segType, weak = keyTypes[i], false
		}
		segDoc := "The name of the property this patch applies to."
		if !weak {
			segDoc = keyDoc(segType, "")
		}
		last := 0
		for _, m := range matches {
			if m[0] > last {
				out = append(out, KeySegment{Text: seg[last:m[0]]})
			}
			out = append(out, KeySegment{
				Param: c.params.use(c, seg[m[2]:m[3]], segType, where, segDoc, weak),
			})
			last = m[1]
		}
		if last < len(seg) {
			out = append(out, KeySegment{Text: seg[last:]})
		}
	}
	if !found {
		return nil
	}
	return mergeTextSegments(out)
}

// mergeTextSegments joins the literal pieces that ended up next to each other,
// so that a pointer emits as "mailboxIds/" + id rather than as three pieces
// concatenated.
func mergeTextSegments(segments []KeySegment) []KeySegment {
	out := segments[:0]
	for _, seg := range segments {
		if seg.Param == nil && len(out) > 0 && out[len(out)-1].Param == nil {
			out[len(out)-1].Text += seg.Text
			continue
		}
		out = append(out, seg)
	}
	return out
}

// comparator checks one term of a /query sort order. A server is only obliged
// to sort on the properties its specification names, so sorting on anything
// else is a query that will fail at run time on a conforming server. Some of
// those properties take an extra member of their own, such as the keyword that
// "hasKeyword" sorts on, and the comparator is incomplete without it.
func (c *checker) comparator(members map[string]json.RawMessage, keys []string, raw json.RawMessage, where string) Node {
	dataType, ok := c.spec.Object(c.sortTarget)
	if !ok || len(dataType.Sort) == 0 {
		// Some specifications leave the sortable properties to the server. With
		// nothing to check against, a comparator naming any property has to be
		// allowed through.
		return c.anyValue(raw)
	}
	base, ok := c.spec.Object("Comparator")
	if !ok {
		return c.anyValue(raw)
	}

	// The property decides what else the comparator may carry, so resolve it
	// before checking the other members.
	sortProperty, extra := c.sortProperty(dataType, members, where)

	out := &Object{Raw: raw}
	for _, key := range keys {
		field, isKnown := base.Field(key)
		if !isKnown {
			if f, isExtra := extra[key]; isExtra {
				field = f
			} else {
				c.errorf(where+"."+key, hintFor(key, comparatorNames(base, extra)),
					"a comparator has no member %q", key)
				continue
			}
		}
		out.Fields = append(out.Fields, ObjectField{
			Key:   key,
			Value: c.value(field.ParsedType(), members[key], where+"."+key, field.Doc),
		})
	}

	if sortProperty != nil {
		for _, f := range sortProperty.Extra {
			if _, given := members[f.Name]; !given {
				c.errorf(where, "", "sorting by %q needs the comparator to also set %q",
					sortProperty.Name, f.Name)
			}
		}
	}
	return out
}

// sortProperty resolves the property a comparator sorts by, and returns the
// extra members that property allows. It reports a property the type cannot be
// sorted on, and stays quiet when the property is a parameter, because then
// there is nothing to check against.
func (c *checker) sortProperty(dataType *spec.Object, members map[string]json.RawMessage, where string) (*spec.SortProperty, map[string]*spec.Field) {
	extra := map[string]*spec.Field{}
	raw, given := members["property"]
	if !given {
		c.errorf(where, "", "a comparator must say which property to sort by")
		return nil, extra
	}
	name, isString := stringValue(raw)
	if !isString || paramPattern.MatchString(name) {
		// The property is left to the caller, so which members the comparator
		// needs cannot be known here. Allow the extras of every sortable
		// property rather than rejecting a query that may well be right.
		for _, p := range dataType.Sort {
			for _, f := range p.Extra {
				extra[f.Name] = f
			}
		}
		return nil, extra
	}
	p, sortable := dataType.SortProperty(name)
	if !sortable {
		c.errorf(where+".property", hintFor(name, dataType.SortNames()),
			"%s cannot be sorted by %q", dataType.Name, name)
		return nil, extra
	}
	for _, f := range p.Extra {
		extra[f.Name] = f
	}
	return p, extra
}

// comparatorNames returns every member a comparator may have here, for a
// suggestion.
func comparatorNames(base *spec.Object, extra map[string]*spec.Field) []string {
	names := base.PropertyNames()
	for name := range extra {
		names = append(names, name)
	}
	return names
}
