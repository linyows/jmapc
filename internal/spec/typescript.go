package spec

import "strings"

// TSPrimitiveAliases returns the named string aliases the generated types need,
// in a stable order, each with what it is an alias for.
func TSPrimitiveAliases() []struct{ Name, Doc string } {
	return primitiveAliases(func(p primitive) string { return p.ts })
}

// TSType renders t as a TypeScript type. Nullability is a union with null, as
// TypeScript writes it, and a union of shapes is a union of types rather than
// the unknown that Go has to fall back on.
func (t *Type) TSType() string {
	if t == nil {
		return "unknown"
	}
	var base string
	switch {
	case t.IsArray():
		base = tsElement(t.Elem) + "[]"
	case t.IsMap():
		// A map keyed by an id is written with the alias, which TypeScript
		// allows as an index signature key since it is a string underneath.
		base = "{ [key: " + t.Key.TSType() + "]: " + t.Value.TSType() + " }"
	case t.IsUnion():
		parts := make([]string, len(t.Union))
		for i, m := range t.Union {
			parts[i] = m.TSType()
		}
		base = strings.Join(parts, " | ")
	default:
		if p, ok := primitives[t.Name]; ok {
			base = p.ts
		} else {
			base = ExportedName(t.Name)
		}
	}
	if t.Nullable {
		return base + " | null"
	}
	return base
}

// tsElement renders an array's element type, parenthesised where it is a union
// or nullable so that the brackets bind to the whole of it.
func tsElement(t *Type) string {
	s := t.TSType()
	if t.IsUnion() || t.Nullable {
		return "(" + s + ")"
	}
	return s
}

// TSBindingName returns name as the name of a function or a constant, which a
// word JavaScript reserves cannot be. Such a name has an underscore added, as
// there is no way to write the word itself as a name.
func TSBindingName(name string) string {
	if tsReserved[name] {
		return name + "_"
	}
	return name
}

// tsReserved are the words that cannot name a function or a constant in a
// module, which is strict mode code: the reserved words of ECMAScript, those
// strict mode adds, await, which a module reserves, and eval and arguments,
// which strict mode does not let a binding take.
var tsReserved = map[string]bool{
	"await": true, "break": true, "case": true, "catch": true, "class": true,
	"const": true, "continue": true, "debugger": true, "default": true, "delete": true,
	"do": true, "else": true, "enum": true, "export": true, "extends": true,
	"false": true, "finally": true, "for": true, "function": true, "if": true,
	"import": true, "in": true, "instanceof": true, "new": true, "null": true,
	"return": true, "super": true, "switch": true, "this": true, "throw": true,
	"true": true, "try": true, "typeof": true, "var": true, "void": true,
	"while": true, "with": true, "yield": true, "let": true, "static": true,
	"implements": true, "interface": true, "package": true, "private": true,
	"protected": true, "public": true, "eval": true, "arguments": true,
}

// TSNeedsQuoting reports whether a member name has to be quoted in a
// TypeScript type declaration.
func TSNeedsQuoting(name string) bool { return !isTSIdentifier(name) }

// isTSIdentifier reports whether a name can be written as a bare member name.
func isTSIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_', r == '$':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}
