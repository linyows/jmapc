// Package jsonschema describes the query files themselves. It turns the JMAP
// data model into a JSON Schema, so that an editor completes a query while it
// is being written and reports a mistake where it was made, rather than at the
// next build.
//
// What it describes is what jmapc checks, as far as a schema can say it: the
// methods that exist, the arguments each one takes, the types of their values,
// the properties a record has, and the values a property whose specification
// fixes them may take. What it cannot say is the part that depends on another
// call — that a back reference names an earlier call and selects a value the
// argument accepts — which stays jmapc's to check.
package jsonschema

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/linyows/jmapc/internal/query"
	"github.com/linyows/jmapc/internal/spec"
)

// Draft is the JSON Schema dialect the output is written in. Draft 7 is what
// the editors that read a schema for completion support in full.
const Draft = "http://json-schema.org/draft-07/schema#"

// The definitions the schema declares for itself, rather than for a type in
// the catalogue.
const (
	parameterDef         = "parameter"
	optionalParameterDef = "optionalParameter"
	referenceDef         = "resultReference"
	callDef              = "methodCall"
	methodDef            = "methodName"
)

// Generator turns a catalogue into a JSON Schema for the query files written
// against it.
type Generator struct {
	// Spec is the catalogue to describe, vendor extensions included.
	Spec *spec.Spec
}

// Generate returns the schema, as indented JSON.
func (g *Generator) Generate() ([]byte, error) {
	b := &builder{spec: g.Spec, defs: map[string]any{}}
	b.declare()
	root := map[string]any{
		"$schema":              Draft,
		"title":                "JMAP query",
		"description":          "A JMAP request, plus the members jmapc reads and the server never sees. The file is named after the function to generate.",
		"type":                 "object",
		"required":             []any{"methodCalls"},
		"additionalProperties": false,
		"properties": map[string]any{
			"$schema": map[string]any{
				"type":        "string",
				"description": "The schema this query is written against, which jmapc ignores.",
			},
			query.DocMember: map[string]any{
				"type":        "string",
				"description": "The generated function's documentation.",
			},
			query.ReturnsMember: map[string]any{
				"type":        "string",
				"description": "The call id whose response the generated function returns. Without it, every response is returned.",
			},
			query.WatchesMember: map[string]any{
				"type":        "string",
				"description": "The call id whose state a watching client follows. It has to name a call that reports what changed since a state, and the generated client calls the query whenever the server says that type has moved on.",
			},
			query.PagesMember: map[string]any{
				"type":        "string",
				"description": "The call id a generated walk advances. It has to name a call that answers with part of a longer answer and says where the rest is — a /query, or a /changes — and the argument saying where the next request starts is left to the walk.",
			},
			query.CreatedIDsMember: map[string]any{
				"type":        "boolean",
				"description": "Whether the generated function carries the creation ids of an earlier request in and reports its own, so that a reference to something created there still resolves here.",
			},
			"using": map[string]any{
				"type":        "array",
				"description": "The capabilities the request declares, by URI or by the short name jmapc knows one under. Left out, they are derived from the methods called.",
				"items": map[string]any{
					"type":     "string",
					"examples": b.capabilities(),
				},
			},
			"methodCalls": map[string]any{
				"type":        "array",
				"description": "The calls the server runs, in order.",
				"minItems":    1,
				"items":       ref(callDef),
			},
		},
		"definitions": b.defs,
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// builder collects the definitions a schema needs while writing it, so that a
// type is described once however many methods refer to it.
type builder struct {
	spec *spec.Spec
	defs map[string]any
}

// fieldContext carries what a property says about its own value beyond its
// type: the values it may take, and the type a patch or a comparator it holds
// applies to. All three travel with the value, since none of them can be read
// off the type expression.
type fieldContext struct {
	enum        []string
	patchTarget string
	sortTarget  string
}

// declare writes the definitions that describe the query language rather than
// the data model, and the ones for every method the catalogue knows.
func (b *builder) declare() {
	b.defs[parameterDef] = map[string]any{
		"type":        "string",
		"pattern":     `^\{\{\s*[A-Za-z_][A-Za-z0-9_]*\s*\}\}$`,
		"description": "A value the caller supplies, written as {{name}}. Its type comes from the argument it stands in for.",
	}
	b.defs[optionalParameterDef] = map[string]any{
		"type":        "string",
		"pattern":     `^\{\{\s*[A-Za-z_][A-Za-z0-9_]*\s*\?\s*\}\}$`,
		"description": "An argument the caller may leave out, written {{name?}}. Leaving it out leaves the argument out of the request, which is not the same as sending null. Only a whole argument may be written this way.",
	}
	b.defs[referenceDef] = map[string]any{
		"type":                 "object",
		"description":          "A value filled in by the server from the result of an earlier call in the same request.",
		"required":             []any{"resultOf", "name", "path"},
		"additionalProperties": false,
		"properties": map[string]any{
			"resultOf": map[string]any{"type": "string", "description": "The call id of the earlier call."},
			"name":     map[string]any{"$ref": "#/definitions/" + methodDef, "description": "The method that call invoked, which the server checks."},
			"path":     map[string]any{"type": "string", "description": "A JMAP JSON pointer into that call's response."},
		},
	}

	methods := b.spec.Methods()
	names := make([]any, 0, len(methods))
	for _, m := range methods {
		names = append(names, m.Name)
	}
	b.defs[methodDef] = map[string]any{
		"type":        "string",
		"description": "The method to call.",
		"enum":        names,
	}

	branches := make([]any, 0, len(methods))
	for _, m := range methods {
		args, err := b.spec.ArgumentsOf(m.Name)
		if err != nil {
			continue
		}
		b.arguments(m, args)
		// The arguments a call takes depend on the method it names, which a
		// schema says by asking about the first element of the triple.
		branches = append(branches, map[string]any{
			"if":   map[string]any{"items": []any{map[string]any{"const": m.Name}}},
			"then": map[string]any{"items": []any{true, ref(m.Arguments), true}},
		})
	}
	b.defs[callDef] = map[string]any{
		"type":        "array",
		"description": "One method call, as [name, arguments, callId].",
		"minItems":    3,
		"maxItems":    3,
		"items": []any{
			ref(methodDef),
			map[string]any{"type": "object", "description": "The arguments to the method."},
			map[string]any{"type": "string", "description": "The call id back references and responses use to refer to this call."},
		},
		"allOf": branches,
	}
}

// capabilities lists what a "using" may name — the capability URIs the
// catalogue knows, and the short names jmapc accepts for them — as the examples
// an editor offers for a query that states its capabilities itself.
func (b *builder) capabilities() []any {
	seen := map[string]bool{}
	for _, m := range b.spec.Methods() {
		if m.Capability != "" {
			seen[m.Capability] = true
		}
	}
	uris := append([]string(nil), query.CapabilityAliases()...)
	for uri := range seen {
		uris = append(uris, uri)
	}
	sort.Strings(uris)
	out := make([]any, len(uris))
	for i, uri := range uris {
		out[i] = uri
	}
	return out
}

// arguments defines the argument object of one method. It is the one object
// that takes members the data model does not describe: the comment a query
// leaves for the generator, and the back references that stand in for
// arguments the server fills in itself.
func (b *builder) arguments(m *spec.Method, o *spec.Object) {
	if _, done := b.defs[o.Name]; done {
		return
	}
	b.defs[o.Name] = true // claimed, so that a cycle through this type stops here
	props := b.properties(m, o, true)
	props[query.CommentArgument] = map[string]any{
		"type":        "string",
		"description": "Why this call is there. It goes into the generated code and never into the request, since a server must reject an argument it does not know.",
	}
	b.defs[o.Name] = map[string]any{
		"type":                 "object",
		"description":          o.Doc,
		"properties":           props,
		"patternProperties":    map[string]any{`^#`: ref(referenceDef)},
		"additionalProperties": false,
	}
}

// object defines a type from the data model, once however many places refer to
// it.
func (b *builder) object(o *spec.Object) {
	if _, done := b.defs[o.Name]; done {
		return
	}
	b.defs[o.Name] = true
	// A type with no properties is one whose members the data model cannot
	// name: a PatchObject is keyed by JSON pointer into the record it patches.
	if len(o.Fields) == 0 {
		b.defs[o.Name] = map[string]any{"type": "object", "description": o.Doc}
		return
	}
	b.defs[o.Name] = map[string]any{
		"type":                 "object",
		"description":          o.Doc,
		"properties":           b.properties(nil, o, false),
		"additionalProperties": false,
	}
}

// properties renders the properties of an object. The method is given for an
// argument object, since two of its arguments select property names of the
// type the method operates on, and those names are worth completing.
func (b *builder) properties(m *spec.Method, o *spec.Object, argument bool) map[string]any {
	props := make(map[string]any, len(o.Fields))
	for _, f := range o.Fields {
		ctx := fieldContext{enum: f.Enum, patchTarget: f.PatchTarget, sortTarget: f.SortTarget}
		var s any
		switch {
		case m != nil && f.Name == m.PropertiesArgument && m.DataType != "":
			s = b.propertyNames(f, m.DataType, true)
		case m != nil && f.Name == m.NestedPropertiesArgument && m.NestedType != "":
			s = b.propertyNames(f, m.NestedType, false)
		default:
			s = b.value(f.ParsedType(), ctx)
		}
		if argument {
			s = orOptionalParameter(s)
		}
		props[f.Name] = describe(s, f.Doc)
	}
	return props
}

// orOptionalParameter lets a value also be written as a parameter the caller
// may leave out. Only an argument may be written that way, since leaving one
// out takes the argument itself out of the request.
func orOptionalParameter(s any) any {
	if m, isMap := s.(map[string]any); isMap {
		if alternatives, ok := m["anyOf"].([]any); ok {
			m["anyOf"] = append(alternatives, ref(optionalParameterDef))
			return m
		}
	}
	return map[string]any{"anyOf": []any{s, ref(optionalParameterDef)}}
}

// propertyNames renders an argument that selects property names of a type,
// which is the one place a plain array of strings can be completed from the
// data model. A name the data model does not fix is still accepted where the
// specifications leave room for one: a property naming a header field, or one
// the server gives meaning to.
func (b *builder) propertyNames(f *spec.Field, typeName string, dynamic bool) any {
	o, ok := b.spec.Object(typeName)
	if !ok {
		return b.value(f.ParsedType(), fieldContext{})
	}
	names := make([]any, 0, len(o.Fields))
	for _, p := range o.Fields {
		names = append(names, p.Name)
	}
	alternatives := []any{map[string]any{"enum": names}}
	if dynamic {
		alternatives = append(alternatives, map[string]any{
			"type":        "string",
			"pattern":     `^(header:|digest:|data$)`,
			"description": "A property the server gives meaning to rather than one the data model fixes: a header field of the message, or a digest of a blob.",
		})
	}
	alternatives = append(alternatives, ref(parameterDef))
	return map[string]any{
		"anyOf": []any{
			map[string]any{"type": "array", "items": map[string]any{"anyOf": alternatives}},
			map[string]any{"type": "null"},
			ref(parameterDef),
		},
	}
}

// value renders the schema for a value of the given type. Anywhere a value may
// go, a parameter may go instead, since a query leaves to its caller whatever
// it does not state.
func (b *builder) value(t *spec.Type, ctx fieldContext) any {
	core := b.core(t, ctx)
	if core == any(true) {
		return true
	}
	alternatives := []any{core}
	if t.Nullable {
		alternatives = append(alternatives, map[string]any{"type": "null"})
	}
	alternatives = append(alternatives, ref(parameterDef))
	return map[string]any{"anyOf": alternatives}
}

// core renders a type without the null and the parameter that value adds.
func (b *builder) core(t *spec.Type, ctx fieldContext) any {
	switch {
	case t.Name == spec.Any:
		return true

	case t.IsUnion():
		if s, ok := b.filter(t); ok {
			return s
		}
		members := make([]any, 0, len(t.Union))
		for _, member := range t.Union {
			members = append(members, b.core(member, ctx))
		}
		return map[string]any{"anyOf": members}

	case t.IsArray():
		return map[string]any{"type": "array", "items": b.value(t.Elem, ctx)}

	case t.IsMap():
		s := map[string]any{"type": "object", "additionalProperties": b.value(t.Value, fieldContext{})}
		// A set such as a participant's roles fixes its keys, not the values
		// they map to.
		if len(ctx.enum) > 0 {
			s["propertyNames"] = map[string]any{"enum": stringList(ctx.enum)}
		}
		return s

	case t.IsObject():
		return b.named(t.Name, ctx)

	default:
		return b.primitive(t, ctx)
	}
}

// named renders a reference to a type in the catalogue, defining it if this is
// the first mention. Two of them are not the fixed shape their name suggests:
// a comparator's members depend on the property it sorts by, and a patch is
// keyed by pointer into the record it applies to.
func (b *builder) named(name string, ctx fieldContext) any {
	o, ok := b.spec.Object(name)
	if !ok {
		return true
	}
	if name == "Comparator" && ctx.sortTarget != "" {
		return b.comparator(o, ctx.sortTarget)
	}
	b.object(o)
	return ref(name)
}

// comparator renders the sort order of a /query call against the type being
// queried, so that the property a comparator names is one that type can
// actually be sorted by, together with whatever else that property asks for.
func (b *builder) comparator(o *spec.Object, target string) any {
	data, ok := b.spec.Object(target)
	if !ok || len(data.Sort) == 0 {
		b.object(o)
		return ref(o.Name)
	}
	name := target + "Comparator"
	if _, done := b.defs[name]; done {
		return ref(name)
	}
	b.defs[name] = true

	names := make([]any, 0, len(data.Sort))
	props := map[string]any{}
	for _, s := range data.Sort {
		names = append(names, s.Name)
		// A comparator like hasKeyword takes a member of its own, which no
		// other property has any use for.
		for _, extra := range s.Extra {
			props[extra.Name] = describe(b.value(extra.ParsedType(), fieldContext{enum: extra.Enum}), extra.Doc)
		}
	}
	for _, f := range o.Fields {
		props[f.Name] = describe(b.value(f.ParsedType(), fieldContext{enum: f.Enum}), f.Doc)
	}
	props["property"] = map[string]any{
		"description": fmt.Sprintf("The property to sort by, which is one %s can be sorted by.", target),
		"anyOf":       []any{map[string]any{"enum": names}, ref(parameterDef)},
	}
	b.defs[name] = map[string]any{
		"type":                 "object",
		"description":          fmt.Sprintf("%s, for a %s/query.", o.Doc, target),
		"properties":           props,
		"additionalProperties": false,
	}
	return ref(name)
}

// filter renders the filter of a /query call. The data model can only say that
// the conditions of an AND, OR or NOT are Any, since what they may hold depends
// on the type being queried; here the schema says it, by naming itself.
func (b *builder) filter(t *spec.Type) (any, bool) {
	var operator, condition string
	for _, member := range t.Union {
		switch {
		case member.Name == "FilterOperator":
			operator = member.Name
		case strings.HasSuffix(member.Name, "FilterCondition"):
			condition = member.Name
		default:
			return nil, false
		}
	}
	if operator == "" || condition == "" {
		return nil, false
	}
	name := strings.TrimSuffix(condition, "Condition")
	if _, done := b.defs[name]; done {
		return ref(name), true
	}
	b.defs[name] = true

	conditionType, ok := b.spec.Object(condition)
	if !ok {
		return nil, false
	}
	b.object(conditionType)

	operatorType, ok := b.spec.Object(operator)
	if !ok {
		return nil, false
	}
	props := map[string]any{}
	for _, f := range operatorType.Fields {
		if f.Name == "conditions" {
			props[f.Name] = describe(map[string]any{
				"type":  "array",
				"items": ref(name),
			}, f.Doc)
			continue
		}
		props[f.Name] = describe(b.value(f.ParsedType(), fieldContext{enum: f.Enum}), f.Doc)
	}
	b.defs[name+"Operator"] = map[string]any{
		"type":                 "object",
		"description":          operatorType.Doc,
		"properties":           props,
		"additionalProperties": false,
	}
	b.defs[name] = map[string]any{
		"description": fmt.Sprintf("A filter for a %s: either a condition or a boolean node combining several.", strings.TrimSuffix(name, "Filter")),
		"anyOf":       []any{ref(name + "Operator"), ref(condition), ref(parameterDef)},
	}
	return ref(name), true
}

// primitive renders one of the types the specifications spell out, with what
// they say about its form. A form checked here is a mistake caught while the
// query is being written rather than when it is built.
func (b *builder) primitive(t *spec.Type, ctx fieldContext) any {
	var s map[string]any
	switch t.Name {
	case spec.String, spec.TimeZoneIDType:
		s = map[string]any{"type": "string"}
	case spec.Boolean:
		s = map[string]any{"type": "boolean"}
	case spec.Number:
		s = map[string]any{"type": "number"}
	case spec.Int:
		s = map[string]any{"type": "integer"}
	case spec.UnsignedInt:
		s = map[string]any{"type": "integer", "minimum": 0}
	case spec.IdType:
		// A creation id stands in for a record the same request creates, and
		// goes wherever an id goes.
		s = map[string]any{
			"type":    "string",
			"pattern": `^#?[A-Za-z0-9_-]{1,255}$`,
		}
	case spec.DateType:
		s = map[string]any{
			"type":    "string",
			"pattern": `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[-+]\d{2}:\d{2})$`,
		}
	case spec.UTCDateType:
		s = map[string]any{
			"type":    "string",
			"pattern": `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`,
		}
	case spec.LocalDateTimeType:
		s = map[string]any{
			"type":    "string",
			"pattern": `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$`,
		}
	case spec.DurationType:
		s = map[string]any{"type": "string", "pattern": durationPattern}
	case spec.SignedDurationType:
		s = map[string]any{"type": "string", "pattern": `^[-+]?` + strings.TrimPrefix(durationPattern, "^")}
	default:
		return true
	}
	if len(ctx.enum) > 0 && s["type"] == "string" {
		return map[string]any{"enum": stringList(ctx.enum)}
	}
	return s
}

// durationPattern matches the ISO 8601 durations JMAP allows, which are the
// ones with no years and no months: "P1D" across a daylight saving change is
// not always 24 hours, but a month is not always a fixed number of days at all.
const durationPattern = `^P(\d+D)?(T(\d+H)?(\d+M)?(\d+(\.\d+)?S)?)?$`

// describe attaches a property's documentation to its schema, where there is
// somewhere to put it.
func describe(s any, doc string) any {
	m, ok := s.(map[string]any)
	if !ok || doc == "" {
		return s
	}
	if _, taken := m["description"]; !taken {
		m["description"] = doc
	}
	return m
}

// ref names a definition.
func ref(name string) map[string]any {
	return map[string]any{"$ref": "#/definitions/" + name}
}

// stringList renders a list of strings as the JSON values they become.
func stringList(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}
