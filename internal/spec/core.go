package spec

// Capability URIs the catalogue knows about.
const (
	CapabilityCore       = "urn:ietf:params:jmap:core"
	CapabilityMail       = "urn:ietf:params:jmap:mail"
	CapabilitySubmission = "urn:ietf:params:jmap:submission"
	CapabilityVacation   = "urn:ietf:params:jmap:vacationresponse"
	CapabilityContacts   = "urn:ietf:params:jmap:contacts"
	CapabilityCalendars  = "urn:ietf:params:jmap:calendars"
	// CapabilityCalendarsParse covers CalendarEvent/parse, which a server may
	// support without supporting the rest of the calendar model.
	CapabilityCalendarsParse = "urn:ietf:params:jmap:calendars:parse"
	// CapabilityAvailability covers Principal/getAvailability.
	CapabilityAvailability = "urn:ietf:params:jmap:principals:availability"
	CapabilityPrincipals   = "urn:ietf:params:jmap:principals"
	// CapabilitySMIMEVerify adds the S/MIME verification properties to an
	// Email; it defines no types or methods of its own.
	CapabilitySMIMEVerify = "urn:ietf:params:jmap:smimeverify"
	// CapabilityBlob brings blob creation, reading and lookup into the API,
	// alongside the upload and download endpoints of the core specification.
	CapabilityBlob = "urn:ietf:params:jmap:blob"
	// CapabilityQuota reports the limits an account is under and how much of
	// each is used.
	CapabilityQuota = "urn:ietf:params:jmap:quota"
	// CapabilitySieve manages the filtering scripts the server runs on
	// incoming mail.
	CapabilitySieve = "urn:ietf:params:jmap:sieve"
	// CapabilityPrincipalsOwner appears only in an account's capabilities,
	// where it names the principal that owns the account. It brings no methods
	// of its own.
	CapabilityPrincipalsOwner = "urn:ietf:params:jmap:principals:owner"
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
				Name:     "operator",
				Type:     "String",
				Required: true,
				Enum:     []string{"AND", "OR", "NOT"},
				Doc:      "How to combine the conditions: \"AND\", \"OR\", or \"NOT\".",
			},
			{
				Name:     "conditions",
				Type:     "Any[]",
				Required: true,
				Doc:      "The conditions to combine, each either a FilterOperator or a filter condition for the type being queried.",
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

	s.AddObject(&Object{
		Name:       "Account",
		Capability: CapabilityCore,
		Doc:        "Account is one account the authenticated user has access to, as described in the session object.",
		Fields: []*Field{
			{Name: "name", Type: "String", Doc: "A user-facing label for the account."},
			{
				Name: "isPersonal",
				Type: "Boolean",
				Doc:  "Whether the account belongs to the authenticated user rather than being shared with them.",
			},
			{Name: "isReadOnly", Type: "Boolean", Doc: "Whether the user may only read from this account."},
			{
				Name: "accountCapabilities",
				Type: "String[Any]",
				Doc:  "The capabilities this account supports, and their per-account limits, keyed by capability URI.",
			},
		},
	})

	registerEcho(s)
	registerBlobCopy(s)
	registerBlobExtension(s)
}

// registerEcho adds Core/echo, which returns its arguments unchanged and so is
// the one method whose arguments the data model cannot describe.
func registerEcho(s *Spec) {
	args := s.AddObject(&Object{
		Name:       "CoreEchoArguments",
		Capability: CapabilityCore,
		Kind:       KindArguments,
		Doc:        "CoreEchoArguments holds the arguments of the Core/echo method, which may be anything at all.",
	})
	resp := s.AddObject(&Object{
		Name:       "CoreEchoResponse",
		Capability: CapabilityCore,
		Kind:       KindResponse,
		Doc:        "CoreEchoResponse holds the response to the Core/echo method, which is the arguments it was given.",
	})
	s.AddMethod(&Method{
		Name:       "Core/echo",
		Capability: CapabilityCore,
		Doc:        "Returns its arguments unchanged, which is how a client checks that it can reach and authenticate with the server.",
		Arguments:  args.Name,
		Response:   resp.Name,
	})
}

// registerBlobCopy adds Blob/copy, which moves raw blobs between accounts and
// does not follow the shape of the standard /copy method.
func registerBlobCopy(s *Spec) {
	args := s.AddObject(&Object{
		Name:       "BlobCopyArguments",
		Capability: CapabilityCore,
		Kind:       KindArguments,
		Doc:        "BlobCopyArguments holds the arguments of the Blob/copy method.",
		Fields: []*Field{
			{Name: "fromAccountId", Type: "Id", Doc: "The id of the account to copy blobs from."},
			accountIDField(),
			{Name: "blobIds", Type: "Id[]", Doc: "The ids of the blobs to copy."},
		},
	})
	resp := s.AddObject(&Object{
		Name:       "BlobCopyResponse",
		Capability: CapabilityCore,
		Kind:       KindResponse,
		Doc:        "BlobCopyResponse holds the response to the Blob/copy method.",
		Fields: []*Field{
			{Name: "fromAccountId", Type: "Id", Doc: "The id of the account the blobs were copied from."},
			accountIDField(),
			{
				Name: "copied",
				Type: "Id[Id]|null",
				Doc:  "A map of the blob id in the source account to the id the blob has in the destination account.",
			},
			{
				Name: "notCopied",
				Type: "Id[SetError]|null",
				Doc:  "A map of blob id to the reason it could not be copied.",
			},
		},
	})
	s.AddMethod(&Method{
		Name:       "Blob/copy",
		Capability: CapabilityCore,
		Doc:        "Copies blobs from one account to another, which is how an attachment is reused without downloading and uploading it again.",
		Arguments:  args.Name,
		Response:   resp.Name,
		DataType:   "Blob",
	})
}
