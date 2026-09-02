package spec

// Capability URIs the catalogue knows about.
const (
	CapabilityCore = "urn:ietf:params:jmap:core"
	CapabilityMail = "urn:ietf:params:jmap:mail"
)

// registerCore adds the types RFC 8620 defines for every JMAP server: the
// building blocks that the standard methods refer to.
func registerCore(s *Spec) {
	s.AddObject(&Object{
		Name:       "FilterOperator",
		Capability: CapabilityCore,
		Doc:        "FilterOperator is a boolean node combining the conditions of a /query filter.",
		Fields: []*Field{
			{
				Name: "operator",
				Type: "String",
				Doc:  "How to combine the conditions: \"AND\", \"OR\", or \"NOT\".",
			},
			{
				Name: "conditions",
				Type: "Any[]",
				Doc:  "The conditions to combine, each either a FilterOperator or a filter condition for the type being queried.",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "Comparator",
		Capability: CapabilityCore,
		Doc:        "Comparator is one term of the sort order applied by a /query call.",
		Fields: []*Field{
			{
				Name: "property",
				Type: "String",
				Doc:  "The property of the record to compare.",
			},
			{
				Name:    "isAscending",
				Type:    "Boolean",
				Default: "true",
				Doc:     "Whether the comparison sorts ascending.",
			},
			{
				Name: "collation",
				Type: "String|null",
				Doc:  "The collation algorithm to compare strings with.",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "AddedItem",
		Capability: CapabilityCore,
		Doc:        "AddedItem is an id to insert into a cached query result, with the index to insert it at.",
		Fields: []*Field{
			{Name: "id", Type: "Id", Doc: "The id of the record that entered the result list."},
			{Name: "index", Type: "UnsignedInt", Doc: "The index in the result list to insert the id at."},
		},
	})

	s.AddObject(&Object{
		Name:       "SetError",
		Capability: CapabilityCore,
		Doc:        "SetError is the reason a single record in a /set call could not be created, updated, or destroyed.",
		Fields: []*Field{
			{Name: "type", Type: "String", Doc: "The type of error, such as \"invalidProperties\"."},
			{Name: "description", Type: "String|null", Doc: "A human-readable explanation of the error."},
			{
				Name: "properties",
				Type: "String[]|null",
				Doc:  "The properties that were invalid, when type is \"invalidProperties\".",
			},
			{
				Name: "existingId",
				Type: "Id|null",
				Doc:  "The id of the existing record, when type is \"alreadyExists\".",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "PatchObject",
		Capability: CapabilityCore,
		Doc:        "PatchObject is a set of changes to apply to a record, keyed by JSON pointer into it.",
		Fields:     nil,
	})
}
