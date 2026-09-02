package spec

// registerSieve adds JMAP for Sieve Scripts, RFC 9661. A Sieve script is a
// filter the server runs on incoming mail, and this is the way to manage the
// ones an account has: what they are called, which one is running, and whether
// a script would even parse.
//
// The script itself is not a property. It is a blob, uploaded before the script
// record refers to it, which is what lets a script of any size be stored without
// putting it through the API.
func registerSieve(s *Spec) {
	s.AddObject(&Object{
		Name:       "SieveScript",
		Capability: CapabilitySieve,
		Doc:        "SieveScript is one stored filtering script. An account may have several, of which at most one is running.",
		Fields: []*Field{
			{Name: "id", Type: "Id", ServerSet: true, Immutable: true, Doc: "The id of the script."},
			{
				Name: "name",
				Type: "String|null",
				Doc:  "The user-visible name of the script, which is unique within the account. Null asks the server to choose one.",
			},
			{
				Name: "blobId",
				Type: "Id",
				Doc:  "The id of the blob holding the script's text, which is uploaded before the script refers to it.",
			},
			{
				Name:      "isActive",
				Type:      "Boolean",
				ServerSet: true,
				Default:   "false",
				Doc: "Whether this is the script the server runs. At most one script in an account is active, " +
					"and it is activated through the arguments of SieveScript/set rather than by setting this.",
			},
		},
		Sort: []*SortProperty{
			{Name: "name", Doc: "Sorts by the script's name."},
			{Name: "isActive", Doc: "Sorts the active script apart from the rest."},
		},
	})

	s.AddObject(&Object{
		Name:       "SieveScriptFilterCondition",
		Capability: CapabilitySieve,
		Doc:        "SieveScriptFilterCondition is a condition a script must satisfy to match a SieveScript/query.",
		Fields: []*Field{
			{Name: "name", Type: "String", Doc: "Matches scripts whose name contains this string."},
			{Name: "isActive", Type: "Boolean", Doc: "Matches scripts according to whether they are the one running."},
		},
	})

	s.RegisterStandard("SieveScript", CapabilitySieve, StandardMethods{
		Get: true, Set: true, Query: true,
	})

	// Activation belongs to /set rather than to the isActive property, so that
	// creating a script and putting it into service is one call: a script that
	// fails to store is never activated, and the one it replaces keeps running.
	s.AppendArguments("SieveScript/set",
		&Field{
			Name: "onSuccessActivateScript",
			Type: "Id|null",
			Doc: "The id of the script to activate once the other changes succeed, " +
				"which may be a creation id written as \"#\" followed by the name it was created under.",
		},
		&Field{
			Name:    "onSuccessDeactivateScript",
			Type:    "Boolean",
			Default: "false",
			Doc: "Whether to stop running whichever script is active. " +
				"Where both this and onSuccessActivateScript are given, the deactivation happens first.",
		},
	)

	// Validation is a method of its own because a script may be worth checking
	// without being worth storing.
	args := s.AddObject(&Object{
		Name:       "SieveScriptValidateArguments",
		Capability: CapabilitySieve,
		Kind:       KindArguments,
		Doc:        "SieveScriptValidateArguments holds the arguments of the SieveScript/validate method.",
		Fields: []*Field{
			accountIDField(),
			{Name: "blobId", Type: "Id", Doc: "The id of the blob holding the script to check."},
		},
	})
	resp := s.AddObject(&Object{
		Name:       "SieveScriptValidateResponse",
		Capability: CapabilitySieve,
		Kind:       KindResponse,
		Doc:        "SieveScriptValidateResponse holds the response to the SieveScript/validate method.",
		Fields: []*Field{
			accountIDField(),
			{
				Name: "error",
				Type: "SetError|null",
				Doc:  "What is wrong with the script, as an invalidSieve error, or null if it is valid.",
			},
		},
	})
	s.AddMethod(&Method{
		Name:       "SieveScript/validate",
		Capability: CapabilitySieve,
		Doc: "Checks whether a script would parse and whether the server supports the extensions it requires, " +
			"without storing it. It is how an editor tells the user about a mistake before they commit to it.",
		Arguments: args.Name,
		Response:  resp.Name,
		DataType:  "SieveScript",
	})
}
