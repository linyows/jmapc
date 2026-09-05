package spec

import "fmt"

// StandardMethods selects which of the standard methods defined in RFC 8620,
// Section 5, a data type supports. Their arguments and responses follow a fixed
// shape, so the catalogue derives them from the data type name rather than
// spelling each one out.
type StandardMethods struct {
	Get          bool
	Changes      bool
	Set          bool
	Copy         bool
	Query        bool
	QueryChanges bool
}

// RegisterStandard adds the selected standard methods for the given data type,
// along with the argument and response objects they need. The data type itself,
// and the "<DataType>FilterCondition" type that /query filters on, must already
// be registered.
func (s *Spec) RegisterStandard(dataType, capability string, m StandardMethods) {
	if _, ok := s.Object(dataType); !ok {
		panic(fmt.Sprintf("spec: standard methods for unregistered type %q", dataType))
	}
	if m.Get {
		s.registerGet(dataType, capability)
	}
	if m.Changes {
		s.registerChanges(dataType, capability)
	}
	if m.Set {
		s.registerSet(dataType, capability)
	}
	if m.Copy {
		s.registerCopy(dataType, capability)
	}
	if m.Query {
		s.registerQuery(dataType, capability)
	}
	if m.QueryChanges {
		s.registerQueryChanges(dataType, capability)
	}
}

// accountIDField is the argument every standard method begins with.
func accountIDField() *Field {
	return &Field{
		Name: "accountId",
		Type: "Id",
		Doc:  "The id of the account to operate on.",
	}
}

func (s *Spec) registerGet(dataType, capability string) {
	name := dataType + "/get"
	const kindWord = "Get"
	args := s.AddObject(&Object{
		Name:       dataType + "GetArguments",
		Capability: capability,
		Kind:       KindArguments,
		Doc:        dataType + kindWord + "Arguments holds the arguments of the " + name + " method.",
		Fields: []*Field{
			accountIDField(),
			{
				Name: "ids",
				Type: "Id[]|null",
				Doc:  "The ids of the records to fetch, or null to fetch all of them.",
			},
			{
				Name: "properties",
				Type: "String[]|null",
				Doc:  "The properties to include in each returned record, or null for all of them. The id property is always returned.",
			},
		},
	})
	resp := s.AddObject(&Object{
		Name:       dataType + "GetResponse",
		Capability: capability,
		Kind:       KindResponse,
		Doc:        dataType + kindWord + "Response holds the response to the " + name + " method.",
		Fields: []*Field{
			accountIDField(),
			{
				Name: "state",
				Type: "String",
				Doc:  "A string encoding the current state of the type on the server, for use with " + dataType + "/changes.",
			},
			{
				Name: "list",
				Type: dataType + "[]",
				Doc:  "The records that were found, in an undefined order.",
			},
			{
				Name: "notFound",
				Type: "Id[]",
				Doc:  "The ids that were requested but do not exist.",
			},
		},
	})
	s.AddMethod(&Method{
		Name:               name,
		Capability:         capability,
		Doc:                "Fetches " + dataType + " records by id.",
		Arguments:          args.Name,
		Response:           resp.Name,
		DataType:           dataType,
		PropertiesArgument: "properties",
		ResultProperty:     "list",
	})
}

func (s *Spec) registerChanges(dataType, capability string) {
	name := dataType + "/changes"
	const kindWord = "Changes"
	args := s.AddObject(&Object{
		Name:       dataType + "ChangesArguments",
		Capability: capability,
		Kind:       KindArguments,
		Doc:        dataType + kindWord + "Arguments holds the arguments of the " + name + " method.",
		Fields: []*Field{
			accountIDField(),
			{
				Name: "sinceState",
				Type: "String",
				Doc:  "The state string the client already has, as returned by an earlier " + dataType + "/get or " + dataType + "/changes.",
			},
			{
				Name: "maxChanges",
				Type: "UnsignedInt|null",
				Doc:  "The maximum number of ids to return across the three change lists.",
			},
		},
	})
	resp := s.AddObject(&Object{
		Name:       dataType + "ChangesResponse",
		Capability: capability,
		Kind:       KindResponse,
		Doc:        dataType + kindWord + "Response holds the response to the " + name + " method.",
		Fields: []*Field{
			accountIDField(),
			{Name: "oldState", Type: "String", Doc: "The state the changes are calculated from."},
			{Name: "newState", Type: "String", Doc: "The state the client reaches by applying these changes."},
			{
				Name: "hasMoreChanges",
				Type: "Boolean",
				Doc:  "Whether further changes remain, in which case the call should be repeated from newState.",
			},
			{Name: "created", Type: "Id[]", Doc: "The ids of records created since oldState."},
			{Name: "updated", Type: "Id[]", Doc: "The ids of records updated since oldState."},
			{Name: "destroyed", Type: "Id[]", Doc: "The ids of records destroyed since oldState."},
		},
	})
	s.AddMethod(&Method{
		Name:       name,
		Capability: capability,
		Doc:        "Reports which " + dataType + " records have changed since a given state.",
		Arguments:  args.Name,
		Response:   resp.Name,
		DataType:   dataType,
	})
}

func (s *Spec) registerSet(dataType, capability string) {
	name := dataType + "/set"
	const kindWord = "Set"
	args := s.AddObject(&Object{
		Name:       dataType + "SetArguments",
		Capability: capability,
		Kind:       KindArguments,
		Doc:        dataType + kindWord + "Arguments holds the arguments of the " + name + " method.",
		Fields: []*Field{
			accountIDField(),
			{
				Name: "ifInState",
				Type: "String|null",
				Doc:  "The state the changes are expected to apply to. The call fails with a stateMismatch error if the server has moved on.",
			},
			{
				Name:        "create",
				Type:        "Id[" + dataType + "]|null",
				Doc:         "A map of creation id to the record to create. A creation id may be referenced elsewhere in the same request as \"#\" followed by the id.",
				CreationIDs: true,
			},
			{
				Name:        "update",
				Type:        "Id[PatchObject]|null",
				PatchTarget: dataType,
				Doc:         "A map of record id to the patch to apply to it.",
			},
			{
				Name: "destroy",
				Type: "Id[]|null",
				Doc:  "The ids of the records to destroy.",
			},
		},
	})
	resp := s.AddObject(&Object{
		Name:       dataType + "SetResponse",
		Capability: capability,
		Kind:       KindResponse,
		Doc:        dataType + kindWord + "Response holds the response to the " + name + " method.",
		Fields: []*Field{
			accountIDField(),
			{
				Name: "oldState",
				Type: "String|null",
				Doc:  "The state before these changes were applied, if the server tracks it.",
			},
			{Name: "newState", Type: "String", Doc: "The state after these changes were applied."},
			{
				Name: "created",
				Type: "Id[" + dataType + "|null]|null",
				Doc:  "A map of creation id to the properties the server assigned to each created record.",
			},
			{
				Name: "updated",
				Type: "Id[" + dataType + "|null]|null",
				Doc:  "A map of record id to any properties the server changed beyond those the patch set.",
			},
			{Name: "destroyed", Type: "Id[]|null", Doc: "The ids of the records that were destroyed."},
			{
				Name: "notCreated",
				Type: "Id[SetError]|null",
				Doc:  "A map of creation id to the reason the record could not be created.",
			},
			{
				Name: "notUpdated",
				Type: "Id[SetError]|null",
				Doc:  "A map of record id to the reason the record could not be updated.",
			},
			{
				Name: "notDestroyed",
				Type: "Id[SetError]|null",
				Doc:  "A map of record id to the reason the record could not be destroyed.",
			},
		},
	})
	s.AddMethod(&Method{
		Name:       name,
		Capability: capability,
		Doc:        "Creates, updates, and destroys " + dataType + " records in one atomic call.",
		Arguments:  args.Name,
		Response:   resp.Name,
		DataType:   dataType,
	})
}

func (s *Spec) registerCopy(dataType, capability string) {
	name := dataType + "/copy"
	const kindWord = "Copy"
	args := s.AddObject(&Object{
		Name:       dataType + "CopyArguments",
		Capability: capability,
		Kind:       KindArguments,
		Doc:        dataType + kindWord + "Arguments holds the arguments of the " + name + " method.",
		Fields: []*Field{
			{Name: "fromAccountId", Type: "Id", Doc: "The id of the account to copy records from."},
			{Name: "ifFromInState", Type: "String|null", Doc: "The state the source account is expected to be in."},
			accountIDField(),
			{Name: "ifInState", Type: "String|null", Doc: "The state the destination account is expected to be in."},
			{
				Name:        "create",
				Type:        "Id[" + dataType + "]",
				Doc:         "A map of creation id to the record to copy, each of which must have an id property naming the record in the source account.",
				CreationIDs: true,
			},
			{
				Name:    "onSuccessDestroyOriginal",
				Type:    "Boolean",
				Default: "false",
				Doc:     "Whether to destroy the originals once the copy succeeds.",
			},
			{
				Name: "destroyFromIfInState",
				Type: "String|null",
				Doc:  "The state the source account must be in for the originals to be destroyed.",
			},
		},
	})
	resp := s.AddObject(&Object{
		Name:       dataType + "CopyResponse",
		Capability: capability,
		Kind:       KindResponse,
		Doc:        dataType + kindWord + "Response holds the response to the " + name + " method.",
		Fields: []*Field{
			{Name: "fromAccountId", Type: "Id", Doc: "The id of the account the records were copied from."},
			accountIDField(),
			{Name: "oldState", Type: "String|null", Doc: "The state of the destination account before the copy."},
			{Name: "newState", Type: "String", Doc: "The state of the destination account after the copy."},
			{
				Name: "created",
				Type: "Id[" + dataType + "]|null",
				Doc:  "A map of creation id to the record created in the destination account.",
			},
			{
				Name: "notCreated",
				Type: "Id[SetError]|null",
				Doc:  "A map of creation id to the reason the record could not be copied.",
			},
		},
	})
	s.AddMethod(&Method{
		Name:       name,
		Capability: capability,
		Doc:        "Copies " + dataType + " records from one account to another.",
		Arguments:  args.Name,
		Response:   resp.Name,
		DataType:   dataType,
	})
}

// queryFilterType is the type expression for the filter argument of a /query
// call: either a boolean operator node or a condition specific to the data type.
func queryFilterType(dataType string) string {
	return "FilterOperator|" + dataType + "FilterCondition|null"
}

func (s *Spec) registerQuery(dataType, capability string) {
	name := dataType + "/query"
	const kindWord = "Query"
	args := s.AddObject(&Object{
		Name:       dataType + "QueryArguments",
		Capability: capability,
		Kind:       KindArguments,
		Doc:        dataType + kindWord + "Arguments holds the arguments of the " + name + " method.",
		Fields: []*Field{
			accountIDField(),
			{
				Name: "filter",
				Type: queryFilterType(dataType),
				Doc:  "The condition records must match to be included in the results.",
			},
			{
				Name:       "sort",
				Type:       "Comparator[]|null",
				SortTarget: dataType,
				Doc:        "The comparators to sort the results by, in order of precedence.",
			},
			{
				Name:    "position",
				Type:    "Int",
				Default: "0",
				Doc:     "The zero-based index of the first result to return. A negative value counts back from the end.",
			},
			{
				Name: "anchor",
				Type: "Id|null",
				Doc:  "The id of a record to position the returned window relative to, instead of using position.",
			},
			{
				Name:    "anchorOffset",
				Type:    "Int",
				Default: "0",
				Doc:     "The offset from the anchor at which the returned window starts.",
			},
			{
				Name: "limit",
				Type: "UnsignedInt|null",
				Doc:  "The maximum number of ids to return.",
			},
			{
				Name:    "calculateTotal",
				Type:    "Boolean",
				Default: "false",
				Doc:     "Whether the server should compute the total number of matching records.",
			},
		},
	})
	resp := s.AddObject(&Object{
		Name:       dataType + "QueryResponse",
		Capability: capability,
		Kind:       KindResponse,
		Doc:        dataType + kindWord + "Response holds the response to the " + name + " method.",
		Fields: []*Field{
			accountIDField(),
			{
				Name: "queryState",
				Type: "String",
				Doc:  "A string encoding the current state of the query on the server, for use with " + dataType + "/queryChanges.",
			},
			{
				Name: "canCalculateChanges",
				Type: "Boolean",
				Doc:  "Whether the server can calculate changes for this query.",
			},
			{Name: "position", Type: "UnsignedInt", Doc: "The zero-based index of the first returned id in the full result list."},
			{Name: "ids", Type: "Id[]", Doc: "The ids of the matching records, in sorted order."},
			{
				Name: "total",
				Type: "UnsignedInt",
				Doc:  "The total number of matching records, present only if calculateTotal was true.",
			},
			{
				Name: "limit",
				Type: "UnsignedInt",
				Doc:  "The limit the server applied, present only if it is lower than the one requested.",
			},
		},
	})
	s.AddMethod(&Method{
		Name:       name,
		Capability: capability,
		Doc:        "Returns the ids of the " + dataType + " records matching a filter, in sorted order.",
		Arguments:  args.Name,
		Response:   resp.Name,
		DataType:   dataType,
	})
}

func (s *Spec) registerQueryChanges(dataType, capability string) {
	name := dataType + "/queryChanges"
	const kindWord = "QueryChanges"
	args := s.AddObject(&Object{
		Name:       dataType + "QueryChangesArguments",
		Capability: capability,
		Kind:       KindArguments,
		Doc:        dataType + kindWord + "Arguments holds the arguments of the " + name + " method.",
		Fields: []*Field{
			accountIDField(),
			{Name: "filter", Type: queryFilterType(dataType), Doc: "The filter the original query used."},
			{
				Name:       "sort",
				Type:       "Comparator[]|null",
				SortTarget: dataType,
				Doc:        "The sort the original query used.",
			},
			{
				Name: "sinceQueryState",
				Type: "String",
				Doc:  "The queryState the client already has, as returned by an earlier " + dataType + "/query.",
			},
			{Name: "maxChanges", Type: "UnsignedInt|null", Doc: "The maximum number of changes to return."},
			{
				Name: "upToId",
				Type: "Id|null",
				Doc:  "The id of the last record in the client's cached window, beyond which changes may be omitted.",
			},
			{
				Name:    "calculateTotal",
				Type:    "Boolean",
				Default: "false",
				Doc:     "Whether the server should compute the total number of matching records.",
			},
		},
	})
	resp := s.AddObject(&Object{
		Name:       dataType + "QueryChangesResponse",
		Capability: capability,
		Kind:       KindResponse,
		Doc:        dataType + kindWord + "Response holds the response to the " + name + " method.",
		Fields: []*Field{
			accountIDField(),
			{Name: "oldQueryState", Type: "String", Doc: "The query state the changes are calculated from."},
			{Name: "newQueryState", Type: "String", Doc: "The query state the client reaches by applying these changes."},
			{
				Name: "total",
				Type: "UnsignedInt",
				Doc:  "The total number of matching records, present only if calculateTotal was true.",
			},
			{
				Name: "removed",
				Type: "Id[]",
				Doc:  "The ids to remove from the cached result list.",
			},
			{
				Name: "added",
				Type: "AddedItem[]",
				Doc:  "The ids to add to the cached result list, each with the index to insert it at.",
			},
		},
	})
	s.AddMethod(&Method{
		Name:       name,
		Capability: capability,
		Doc:        "Reports how the result of a " + dataType + "/query has changed since a given query state.",
		Arguments:  args.Name,
		Response:   resp.Name,
		DataType:   dataType,
	})
}

// AppendArguments adds extra fields to the argument object of an already
// registered method, for the arguments a specification adds on top of the
// standard shape.
func (s *Spec) AppendArguments(method string, fields ...*Field) {
	o, err := s.ArgumentsOf(method)
	if err != nil {
		panic("spec: " + err.Error())
	}
	o.Fields = append(o.Fields, fields...)
}

// AppendResponse adds extra fields to the response object of an already
// registered method.
func (s *Spec) AppendResponse(method string, fields ...*Field) {
	o, err := s.ResponseOf(method)
	if err != nil {
		panic("spec: " + err.Error())
	}
	o.Fields = append(o.Fields, fields...)
}
