package spec

// registerQuota adds JMAP Quotas, RFC 9425. A quota is a limit the server
// imposes and the client can only read: how much of something the account is
// allowed, and how much of it is gone. There is no /set, because nothing here
// is the client's to decide.
func registerQuota(s *Spec) {
	s.AddObject(&Object{
		Name:       "Quota",
		Capability: CapabilityQuota,
		Doc: "Quota is one limit on what an account may hold, and how much of that limit is used. " +
			"An account may be under several at once: a count of messages, a number of octets, " +
			"one imposed on the account and another on the whole domain.",
		Fields: []*Field{
			{Name: "id", Type: "Id", ServerSet: true, Immutable: true, Doc: "The id of the quota."},
			{
				Name: "resourceType",
				Type: "String",
				Enum: []string{"count", "octets"},
				Doc:  "What is being counted: \"count\" for a number of objects, or \"octets\" for their size.",
			},
			{
				Name:      "used",
				Type:      "UnsignedInt",
				ServerSet: true,
				Doc:       "How much of the resource is in use, in whatever the resourceType counts.",
			},
			{
				Name: "hardLimit",
				Type: "UnsignedInt",
				Doc:  "The point beyond which the server refuses to store more.",
			},
			{
				Name: "scope",
				Type: "String",
				Enum: []string{"account", "domain", "global"},
				Doc: "Who the limit applies to: \"account\" for this account alone, " +
					"\"domain\" for everyone in the domain, or \"global\" for the whole server.",
			},
			{
				Name: "name",
				Type: "String",
				Doc:  "The name of the quota, which is unique within its scope and resourceType.",
			},
			{
				Name: "types",
				Type: "String[]",
				Doc:  "The data types the quota applies to, such as \"Mail\" or \"Calendar\". The names are those of the capabilities, not of individual record types.",
			},
			{
				Name:    "warnLimit",
				Type:    "UnsignedInt|null",
				Default: "null",
				Doc:     "The point at which the server would like the client to warn the user, which it sets below the hard limit so that there is time to do something about it.",
			},
			{
				Name:    "softLimit",
				Type:    "UnsignedInt|null",
				Default: "null",
				Doc:     "The point beyond which the server starts refusing some operations while still allowing others, such as accepting mail but not letting the user send any.",
			},
			{
				Name:    "description",
				Type:    "String|null",
				Default: "null",
				Doc:     "A description of the quota, meant to be shown to the user.",
			},
		},
		Sort: []*SortProperty{
			{Name: "name", Doc: "Sorts by the quota's name."},
			{Name: "used", Doc: "Sorts by how much of the quota is in use."},
		},
	})

	s.AddObject(&Object{
		Name:       "QuotaFilterCondition",
		Capability: CapabilityQuota,
		Doc:        "QuotaFilterCondition is a condition a quota must satisfy to match a Quota/query.",
		Fields: []*Field{
			{Name: "name", Type: "String", Doc: "Matches quotas whose name contains this string."},
			{
				Name: "scope",
				Type: "String",
				Enum: []string{"account", "domain", "global"},
				Doc:  "Matches quotas of this scope: \"account\", \"domain\", or \"global\".",
			},
			{
				Name: "resourceType",
				Type: "String",
				Enum: []string{"count", "octets"},
				Doc:  "Matches quotas counting this resource: \"count\" or \"octets\".",
			},
			{
				Name: "type",
				Type: "String",
				Doc:  "Matches quotas that apply to this data type.",
			},
		},
	})

	// No /set: a quota is the server's to decide, not the client's.
	s.RegisterStandard("Quota", CapabilityQuota, StandardMethods{
		Get: true, Changes: true, Query: true, QueryChanges: true,
	})

	// A quota's used value moves constantly while the rest of it rarely does,
	// so the server may say which properties actually changed and spare the
	// client fetching the whole record.
	s.AppendResponse("Quota/changes", &Field{
		Name: "updatedProperties",
		Type: "String[]|null",
		Doc: "The properties that changed on every quota in the updated list, " +
			"or null if the client should assume anything may have. " +
			"A server that tracks this can say \"used\" alone, which is usually all that moved.",
	})
}
