package spec

// registerMail adds the data types and methods of JMAP for Mail, RFC 8621.
func registerMail(s *Spec) {
	registerMailbox(s)
	registerThread(s)
	registerEmail(s)
	registerEmailImport(s)
	registerEmailParse(s)
	registerSearchSnippet(s)
}

// registerEmailImport adds Email/import, RFC 8621, Section 4.8. It creates
// emails from blobs that already hold a complete message, which is not
// something Email/set can do.
func registerEmailImport(s *Spec) {
	s.AddObject(&Object{
		Name:       "EmailImport",
		Capability: CapabilityMail,
		Doc:        "EmailImport is one message to import, naming the blob holding it and where it should land.",
		Fields: []*Field{
			{Name: "blobId", Type: "Id", Doc: "The id of the blob holding the raw RFC 5322 message."},
			{Name: "mailboxIds", Type: "Id[Boolean]", Doc: "The mailboxes to file the imported email in."},
			{Name: "keywords", Type: "String[Boolean]", Doc: "The keywords to set on the imported email."},
			{
				Name: "receivedAt",
				Type: "UTCDate",
				Doc:  "The time to record as the email's receivedAt, defaulting to when the import happens.",
			},
		},
	})
	args := s.AddObject(&Object{
		Name:       "EmailImportArguments",
		Capability: CapabilityMail,
		Kind:       KindArguments,
		Doc:        "EmailImportArguments holds the arguments of the Email/import method.",
		Fields: []*Field{
			accountIDField(),
			{
				Name: "ifInState",
				Type: "String|null",
				Doc:  "The state the emails are expected to be in. The call fails with a stateMismatch error if the server has moved on.",
			},
			{
				Name: "emails",
				Type: "Id[EmailImport]",
				Doc:  "The messages to import, keyed by creation id.",
			},
		},
	})
	resp := s.AddObject(&Object{
		Name:       "EmailImportResponse",
		Capability: CapabilityMail,
		Kind:       KindResponse,
		Doc:        "EmailImportResponse holds the response to the Email/import method.",
		Fields: []*Field{
			accountIDField(),
			{Name: "oldState", Type: "String|null", Doc: "The state before the import."},
			{Name: "newState", Type: "String", Doc: "The state after the import."},
			{
				Name: "created",
				Type: "Id[Email]|null",
				Doc:  "A map of creation id to the properties the server assigned to each imported email.",
			},
			{
				Name: "notCreated",
				Type: "Id[SetError]|null",
				Doc:  "A map of creation id to the reason the message could not be imported.",
			},
		},
	})
	s.AddMethod(&Method{
		Name:       "Email/import",
		Capability: CapabilityMail,
		Doc:        "Creates emails from blobs that already hold a complete RFC 5322 message, which is how mail is moved in from another system.",
		Arguments:  args.Name,
		Response:   resp.Name,
		DataType:   "Email",
	})
}

// registerEmailParse adds Email/parse, RFC 8621, Section 4.9. It reads a blob
// as a message without filing it anywhere, which is how an attached message is
// shown.
func registerEmailParse(s *Spec) {
	args := s.AddObject(&Object{
		Name:       "EmailParseArguments",
		Capability: CapabilityMail,
		Kind:       KindArguments,
		Doc:        "EmailParseArguments holds the arguments of the Email/parse method.",
		Fields: []*Field{
			accountIDField(),
			{Name: "blobIds", Type: "Id[]", Doc: "The ids of the blobs to parse as messages."},
			{
				Name: "properties",
				Type: "String[]|null",
				Doc:  "The properties to include in each parsed email, or null for the default set.",
			},
			{
				Name: "bodyProperties",
				Type: "String[]|null",
				Doc:  "The properties to include for each EmailBodyPart returned.",
			},
			{
				Name:    "fetchTextBodyValues",
				Type:    "Boolean",
				Default: "false",
				Doc:     "Whether to populate bodyValues for the parts listed in textBody.",
			},
			{
				Name:    "fetchHTMLBodyValues",
				Type:    "Boolean",
				Default: "false",
				Doc:     "Whether to populate bodyValues for the parts listed in htmlBody.",
			},
			{
				Name:    "fetchAllBodyValues",
				Type:    "Boolean",
				Default: "false",
				Doc:     "Whether to populate bodyValues for every textual body part.",
			},
			{
				Name: "maxBodyValueBytes",
				Type: "UnsignedInt",
				Doc:  "The maximum number of octets to return for each body value, truncating longer ones.",
			},
		},
	})
	resp := s.AddObject(&Object{
		Name:       "EmailParseResponse",
		Capability: CapabilityMail,
		Kind:       KindResponse,
		Doc:        "EmailParseResponse holds the response to the Email/parse method.",
		Fields: []*Field{
			accountIDField(),
			{
				Name: "parsed",
				Type: "Id[Email]|null",
				Doc:  "A map of blob id to the email parsed from it. The email has no id, mailboxIds, keywords, or receivedAt, because it is not a record in the account.",
			},
			{
				Name: "notParsable",
				Type: "Id[]|null",
				Doc:  "The ids of the blobs that exist but do not hold a message the server could parse.",
			},
			{Name: "notFound", Type: "Id[]|null", Doc: "The ids of the blobs that do not exist."},
		},
	})
	s.AddMethod(&Method{
		Name:                     "Email/parse",
		Capability:               CapabilityMail,
		Doc:                      "Reads blobs as RFC 5322 messages without filing them in the account, which is how a message sent as an attachment is displayed.",
		Arguments:                args.Name,
		Response:                 resp.Name,
		DataType:                 "Email",
		PropertiesArgument:       "properties",
		NestedPropertiesArgument: "bodyProperties",
		NestedType:               "EmailBodyPart",
	})
}

// registerSearchSnippet adds SearchSnippet/get, RFC 8621, Section 5. It takes a
// filter rather than only ids, because a snippet only means anything in the
// context of the search that produced it.
func registerSearchSnippet(s *Spec) {
	s.AddObject(&Object{
		Name:       "SearchSnippet",
		Capability: CapabilityMail,
		Doc:        "SearchSnippet is the part of an email that matched a search, with the matching words marked up for display.",
		Fields: []*Field{
			{Name: "emailId", Type: "Id", Doc: "The id of the email the snippet is from."},
			{
				Name: "subject",
				Type: "String|null",
				Doc:  "The email's subject with the matching words wrapped in <mark> tags, or null if nothing in it matched.",
			},
			{
				Name: "preview",
				Type: "String|null",
				Doc:  "An extract of the email's body with the matching words wrapped in <mark> tags, or null if nothing in it matched.",
			},
		},
	})
	args := s.AddObject(&Object{
		Name:       "SearchSnippetGetArguments",
		Capability: CapabilityMail,
		Kind:       KindArguments,
		Doc:        "SearchSnippetGetArguments holds the arguments of the SearchSnippet/get method.",
		Fields: []*Field{
			accountIDField(),
			{
				Name: "filter",
				Type: queryFilterType("Email"),
				Doc:  "The filter the search used, which is what the snippets are cut around.",
			},
			{Name: "emailIds", Type: "Id[]", Doc: "The ids of the emails to return snippets for."},
		},
	})
	resp := s.AddObject(&Object{
		Name:       "SearchSnippetGetResponse",
		Capability: CapabilityMail,
		Kind:       KindResponse,
		Doc:        "SearchSnippetGetResponse holds the response to the SearchSnippet/get method.",
		Fields: []*Field{
			accountIDField(),
			{Name: "list", Type: "SearchSnippet[]", Doc: "The snippets that were generated, one per email that was found."},
			{Name: "notFound", Type: "Id[]|null", Doc: "The ids that were requested but do not exist."},
		},
	})
	s.AddMethod(&Method{
		Name:           "SearchSnippet/get",
		Capability:     CapabilityMail,
		Doc:            "Returns the parts of the given emails that matched a search, marked up for display.",
		Arguments:      args.Name,
		Response:       resp.Name,
		DataType:       "SearchSnippet",
		ResultProperty: "list",
	})
}

func registerMailbox(s *Spec) {
	s.AddObject(&Object{
		Name:       "MailboxRights",
		Capability: CapabilityMail,
		Doc:        "MailboxRights says what the authenticated user may do with a mailbox.",
		Fields: []*Field{
			{Name: "mayReadItems", Type: "Boolean", Doc: "Whether the user may read the mailbox's emails."},
			{Name: "mayAddItems", Type: "Boolean", Doc: "Whether the user may add emails to the mailbox."},
			{Name: "mayRemoveItems", Type: "Boolean", Doc: "Whether the user may remove emails from the mailbox."},
			{Name: "maySetSeen", Type: "Boolean", Doc: "Whether the user may change the $seen keyword on the mailbox's emails."},
			{Name: "maySetKeywords", Type: "Boolean", Doc: "Whether the user may change keywords other than $seen."},
			{Name: "mayCreateChild", Type: "Boolean", Doc: "Whether the user may create a child of this mailbox."},
			{Name: "mayRename", Type: "Boolean", Doc: "Whether the user may rename or move the mailbox."},
			{Name: "mayDelete", Type: "Boolean", Doc: "Whether the user may delete the mailbox."},
			{Name: "maySubmit", Type: "Boolean", Doc: "Whether the user may submit the mailbox's emails for delivery."},
		},
	})

	s.AddObject(&Object{
		Name:       "Mailbox",
		Capability: CapabilityMail,
		Doc:        "Mailbox is a named set of emails, which is how JMAP models both folders and labels.",
		Fields: []*Field{
			{Name: "id", Type: "Id", ServerSet: true, Immutable: true, Doc: "The id of the mailbox."},
			{Name: "name", Type: "String", Doc: "The user-visible name of the mailbox, unique among its siblings."},
			{Name: "parentId", Type: "Id|null", Doc: "The id of the parent mailbox, or null if it is at the top level."},
			{
				Name:      "role",
				Type:      "String|null",
				Doc:       "The IANA-registered role of the mailbox, such as \"inbox\" or \"trash\", or null if it has none.",
				ServerSet: false,
			},
			{Name: "sortOrder", Type: "UnsignedInt", Default: "0", Doc: "A hint for where to place the mailbox in a list of its siblings."},
			{Name: "totalEmails", Type: "UnsignedInt", ServerSet: true, Doc: "The number of emails in the mailbox."},
			{Name: "unreadEmails", Type: "UnsignedInt", ServerSet: true, Doc: "The number of emails in the mailbox without the $seen keyword."},
			{Name: "totalThreads", Type: "UnsignedInt", ServerSet: true, Doc: "The number of threads with at least one email in the mailbox."},
			{
				Name:      "unreadThreads",
				Type:      "UnsignedInt",
				ServerSet: true,
				Doc:       "The number of threads with at least one unread email in the mailbox.",
			},
			{Name: "myRights", Type: "MailboxRights", ServerSet: true, Doc: "What the authenticated user may do with the mailbox."},
			{Name: "isSubscribed", Type: "Boolean", Doc: "Whether the user has subscribed to the mailbox."},
		},
	})

	s.AddObject(&Object{
		Name:       "MailboxFilterCondition",
		Capability: CapabilityMail,
		Doc:        "MailboxFilterCondition is a condition a mailbox must satisfy to match a Mailbox/query.",
		Fields: []*Field{
			{Name: "parentId", Type: "Id|null", Doc: "Matches mailboxes with this parent, or top-level mailboxes if null."},
			{Name: "name", Type: "String", Doc: "Matches mailboxes whose name contains this string."},
			{Name: "role", Type: "String|null", Doc: "Matches mailboxes with this role, or with no role if null."},
			{Name: "hasAnyRole", Type: "Boolean", Doc: "Matches mailboxes according to whether they have any role at all."},
			{Name: "isSubscribed", Type: "Boolean", Doc: "Matches mailboxes according to the user's subscription."},
		},
	})

	mailbox, _ := s.Object("Mailbox")
	mailbox.Sort = []*SortProperty{
		{Name: "sortOrder", Doc: "Sorts by the hint the server or user set for the mailbox's place among its siblings."},
		{Name: "name", Doc: "Sorts by the mailbox's name."},
	}

	s.RegisterStandard("Mailbox", CapabilityMail, StandardMethods{
		Get: true, Changes: true, Set: true, Query: true, QueryChanges: true,
	})
	s.AppendArguments("Mailbox/set", &Field{
		Name:    "onDestroyRemoveEmails",
		Type:    "Boolean",
		Default: "false",
		Doc:     "Whether destroying a mailbox may also remove the emails it holds. If false, destroying a non-empty mailbox fails.",
	})
	sortAsTree := func() []*Field {
		return []*Field{
			{
				Name:    "sortAsTree",
				Type:    "Boolean",
				Default: "false",
				Doc:     "Whether to order the results so that each mailbox follows its parent.",
			},
			{
				Name:    "filterAsTree",
				Type:    "Boolean",
				Default: "false",
				Doc:     "Whether a mailbox matches only if all of its ancestors match too.",
			},
		}
	}
	s.AppendArguments("Mailbox/query", sortAsTree()...)
	s.AppendArguments("Mailbox/queryChanges", sortAsTree()...)
}

func registerThread(s *Spec) {
	s.AddObject(&Object{
		Name:       "Thread",
		Capability: CapabilityMail,
		Doc:        "Thread is a set of emails the server considers to be one conversation.",
		Fields: []*Field{
			{Name: "id", Type: "Id", ServerSet: true, Immutable: true, Doc: "The id of the thread."},
			{
				Name:      "emailIds",
				Type:      "Id[]",
				ServerSet: true,
				Doc:       "The ids of the emails in the thread, sorted by receivedAt and then by id.",
			},
		},
	})
	s.RegisterStandard("Thread", CapabilityMail, StandardMethods{Get: true, Changes: true})
}

func registerEmail(s *Spec) {
	s.AddObject(&Object{
		Name:       "EmailAddress",
		Capability: CapabilityMail,
		Doc:        "EmailAddress is one address from a header field such as From or To.",
		Fields: []*Field{
			{Name: "name", Type: "String|null", Doc: "The display name of the addressee, or null if the header gave none."},
			{Name: "email", Type: "String", Doc: "The addr-spec of the address."},
		},
	})

	s.AddObject(&Object{
		Name:       "EmailAddressGroup",
		Capability: CapabilityMail,
		Doc:        "EmailAddressGroup is a named group of addresses from an address header field.",
		Fields: []*Field{
			{Name: "name", Type: "String|null", Doc: "The name of the group, or null for addresses outside any group."},
			{Name: "addresses", Type: "EmailAddress[]", Doc: "The addresses in the group."},
		},
	})

	s.AddObject(&Object{
		Name:       "EmailHeader",
		Capability: CapabilityMail,
		Doc:        "EmailHeader is one header field of an email or body part, as it appeared in the message.",
		Fields: []*Field{
			{Name: "name", Type: "String", Doc: "The header field name, as written in the message."},
			{
				Name: "value",
				Type: "String",
				Doc:  "The header field value, still in its raw form apart from unfolding.",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "EmailBodyValue",
		Capability: CapabilityMail,
		Doc:        "EmailBodyValue is the decoded content of one body part.",
		Fields: []*Field{
			{Name: "value", Type: "String", Doc: "The decoded content of the body part."},
			{
				Name: "isEncodingProblem",
				Type: "Boolean",
				Doc:  "Whether the part could not be decoded cleanly, so that the value contains replacement characters.",
			},
			{
				Name: "isTruncated",
				Type: "Boolean",
				Doc:  "Whether the value was cut short to satisfy maxBodyValueBytes.",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "EmailBodyPart",
		Capability: CapabilityMail,
		Doc:        "EmailBodyPart is one part of an email's MIME structure.",
		Fields: []*Field{
			{
				Name: "partId",
				Type: "String|null",
				Doc:  "Identifies the part's content within the bodyValues map, or null if the part has no body of its own.",
			},
			{Name: "blobId", Type: "Id|null", Doc: "The id of the blob holding the raw content, or null for a multipart part."},
			{Name: "size", Type: "UnsignedInt", Doc: "The size of the decoded content in octets."},
			{Name: "headers", Type: "EmailHeader[]", Doc: "The header fields of this part."},
			{Name: "name", Type: "String|null", Doc: "The filename the part should be saved as, if it names one."},
			{Name: "type", Type: "String", Doc: "The media type of the part."},
			{Name: "charset", Type: "String|null", Doc: "The character set of the part, for textual media types."},
			{
				Name: "disposition",
				Type: "String|null",
				Doc:  "The Content-Disposition of the part, such as \"inline\" or \"attachment\".",
			},
			{Name: "cid", Type: "String|null", Doc: "The Content-ID of the part, which inline images are referenced by."},
			{Name: "language", Type: "String[]|null", Doc: "The languages of the part's content."},
			{Name: "location", Type: "String|null", Doc: "The Content-Location of the part."},
			{Name: "subParts", Type: "EmailBodyPart[]|null", Doc: "The parts of a multipart part, or null if this part is not multipart."},
		},
	})

	s.AddObject(&Object{
		Name:       "Email",
		Capability: CapabilityMail,
		Doc:        "Email is a single message, presented as structured data rather than as raw RFC 5322 text.",
		Fields: []*Field{
			{Name: "id", Type: "Id", ServerSet: true, Immutable: true, Doc: "The id of the email."},
			{Name: "blobId", Type: "Id", ServerSet: true, Immutable: true, Doc: "The id of the blob holding the raw message."},
			{Name: "threadId", Type: "Id", ServerSet: true, Immutable: true, Doc: "The id of the thread the email belongs to."},
			{
				Name: "mailboxIds",
				Type: "Id[Boolean]",
				Doc:  "The mailboxes the email is in, as a set of ids mapped to true.",
			},
			{
				Name: "keywords",
				Type: "String[Boolean]",
				Doc:  "The keywords set on the email, such as \"$seen\" or \"$flagged\", mapped to true.",
			},
			{Name: "size", Type: "UnsignedInt", ServerSet: true, Immutable: true, Doc: "The size of the raw message in octets."},
			{
				Name:      "receivedAt",
				Type:      "UTCDate",
				Immutable: true,
				Doc:       "When the email was received, which is what the mailbox sorts on by default.",
			},
			{
				Name: "messageId",
				Type: "String[]|null",
				Doc:  "The Message-ID header field values, without the enclosing angle brackets.",
			},
			{Name: "inReplyTo", Type: "String[]|null", Doc: "The In-Reply-To header field values."},
			{Name: "references", Type: "String[]|null", Doc: "The References header field values."},
			{Name: "sender", Type: "EmailAddress[]|null", Doc: "The Sender header field value."},
			{Name: "from", Type: "EmailAddress[]|null", Doc: "The From header field value."},
			{Name: "to", Type: "EmailAddress[]|null", Doc: "The To header field value."},
			{Name: "cc", Type: "EmailAddress[]|null", Doc: "The Cc header field value."},
			{Name: "bcc", Type: "EmailAddress[]|null", Doc: "The Bcc header field value."},
			{Name: "replyTo", Type: "EmailAddress[]|null", Doc: "The Reply-To header field value."},
			{Name: "subject", Type: "String|null", Doc: "The Subject header field value."},
			{Name: "sentAt", Type: "Date|null", Doc: "The Date header field value."},
			{Name: "headers", Type: "EmailHeader[]", Doc: "Every header field of the message, in the order they appeared."},
			{Name: "bodyStructure", Type: "EmailBodyPart", Doc: "The full MIME structure of the message body."},
			{
				Name: "bodyValues",
				Type: "String[EmailBodyValue]",
				Doc:  "The decoded content of the body parts that were fetched, keyed by partId.",
			},
			{
				Name: "textBody",
				Type: "EmailBodyPart[]",
				Doc:  "The parts to display as the plain-text body of the message.",
			},
			{
				Name: "htmlBody",
				Type: "EmailBodyPart[]",
				Doc:  "The parts to display as the HTML body of the message.",
			},
			{
				Name: "attachments",
				Type: "EmailBodyPart[]",
				Doc:  "The parts to present as attachments rather than as body content.",
			},
			{
				Name:      "hasAttachment",
				Type:      "Boolean",
				ServerSet: true,
				Doc:       "Whether the message has at least one part the server considers an attachment.",
			},
			{
				Name:      "preview",
				Type:      "String",
				ServerSet: true,
				Doc:       "A short plain-text excerpt of the message body.",
			},

			// The S/MIME signature verification of RFC 9219. The server does
			// the cryptography, because a client cannot: the signature covers
			// the raw message, which the client never sees once the server has
			// taken it apart into this structure.
			{
				Name:       "smimeStatus",
				Type:       "String|null",
				Capability: CapabilitySMIMEVerify,
				ServerSet:  true,
				Enum: []string{"unknown", "signed", "signed/verified", "signed/failed",
					"encrypted+signed/verified", "encrypted+signed/failed"},
				Doc: "What the server made of the message's signature when it last looked: " +
					"\"unknown\", \"signed\", \"signed/verified\", \"signed/failed\", " +
					"\"encrypted+signed/verified\", or \"encrypted+signed/failed\". Null for a message with no signature.",
			},
			{
				Name:       "smimeStatusAtDelivery",
				Type:       "String|null",
				Capability: CapabilitySMIMEVerify,
				ServerSet:  true,
				Enum: []string{"unknown", "signed", "signed/verified", "signed/failed",
					"encrypted+signed/verified", "encrypted+signed/failed"},
				Doc: "What the server made of the signature when the message arrived, taking the same values as smimeStatus: " +
					"\"unknown\", \"signed\", \"signed/verified\", \"signed/failed\", " +
					"\"encrypted+signed/verified\", or \"encrypted+signed/failed\". " +
					"It can differ from smimeStatus: a certificate valid on delivery may have expired since, " +
					"and it is the state at delivery that says whether the message was trustworthy when it was sent.",
			},
			{
				Name:       "smimeErrors",
				Type:       "String[]|null",
				Capability: CapabilitySMIMEVerify,
				ServerSet:  true,
				Doc:        "What went wrong with the verification, in terms meant for a person to read.",
			},
			{
				Name:       "smimeVerifiedAt",
				Type:       "UTCDate|null",
				Capability: CapabilitySMIMEVerify,
				ServerSet:  true,
				Doc:        "When the server last checked the signature.",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "EmailFilterCondition",
		Capability: CapabilityMail,
		Doc:        "EmailFilterCondition is a condition an email must satisfy to match an Email/query.",
		Fields: []*Field{
			{Name: "inMailbox", Type: "Id", Doc: "Matches emails in this mailbox."},
			{Name: "inMailboxOtherThan", Type: "Id[]", Doc: "Matches emails that are in at least one mailbox outside this set."},
			{Name: "before", Type: "UTCDate", Doc: "Matches emails received before this time."},
			{Name: "after", Type: "UTCDate", Doc: "Matches emails received at or after this time."},
			{Name: "minSize", Type: "UnsignedInt", Doc: "Matches emails of at least this size in octets."},
			{Name: "maxSize", Type: "UnsignedInt", Doc: "Matches emails smaller than this size in octets."},
			{
				Name: "allInThreadHaveKeyword",
				Type: "String",
				Doc:  "Matches emails whose thread has this keyword on every email.",
			},
			{
				Name: "someInThreadHaveKeyword",
				Type: "String",
				Doc:  "Matches emails whose thread has this keyword on at least one email.",
			},
			{
				Name: "noneInThreadHaveKeyword",
				Type: "String",
				Doc:  "Matches emails whose thread has this keyword on no email.",
			},
			{Name: "hasKeyword", Type: "String", Doc: "Matches emails with this keyword set."},
			{Name: "notKeyword", Type: "String", Doc: "Matches emails without this keyword set."},
			{Name: "hasAttachment", Type: "Boolean", Doc: "Matches emails according to whether they have an attachment."},
			{
				Name: "text",
				Type: "String",
				Doc:  "Matches emails where this text appears in the body, subject, or any address header field.",
			},
			{Name: "from", Type: "String", Doc: "Matches emails where this text appears in the From header field."},
			{Name: "to", Type: "String", Doc: "Matches emails where this text appears in the To header field."},
			{Name: "cc", Type: "String", Doc: "Matches emails where this text appears in the Cc header field."},
			{Name: "bcc", Type: "String", Doc: "Matches emails where this text appears in the Bcc header field."},
			{Name: "subject", Type: "String", Doc: "Matches emails where this text appears in the Subject header field."},
			{Name: "body", Type: "String", Doc: "Matches emails where this text appears in the message body."},
			{
				Name: "header",
				Type: "String[]",
				Doc:  "Matches emails carrying the named header field, optionally with the given value: [name] or [name, value].",
			},

			// RFC 9219.
			{
				Name:       "hasSmime",
				Type:       "Boolean",
				Capability: CapabilitySMIMEVerify,
				Doc:        "Matches emails according to whether they carry an S/MIME signature at all.",
			},
			{
				Name:       "hasVerifiedSmime",
				Type:       "Boolean",
				Capability: CapabilitySMIMEVerify,
				Doc:        "Matches emails according to whether their signature verifies now.",
			},
			{
				Name:       "hasVerifiedSmimeAtDelivery",
				Type:       "Boolean",
				Capability: CapabilitySMIMEVerify,
				Doc:        "Matches emails according to whether their signature verified when the message arrived.",
			},
		},
	})

	keywordSort := func(name, doc string) *SortProperty {
		return &SortProperty{
			Name: name,
			Doc:  doc,
			Extra: []*Field{{
				Name: "keyword",
				Type: "String",
				Doc:  "The keyword to sort on.",
			}},
		}
	}
	email, _ := s.Object("Email")
	email.Sort = []*SortProperty{
		{Name: "receivedAt", Doc: "Sorts by when the email was received, which is the usual order for a mailbox."},
		{Name: "size", Doc: "Sorts by the size of the raw message."},
		{Name: "from", Doc: "Sorts by the From header field."},
		{Name: "to", Doc: "Sorts by the To header field."},
		{Name: "subject", Doc: "Sorts by the Subject header field, ignoring any reply or forward prefix."},
		{Name: "sentAt", Doc: "Sorts by the Date header field."},
		keywordSort("hasKeyword", "Sorts the emails carrying the keyword apart from those that do not."),
		keywordSort("allInThreadHaveKeyword", "Sorts by whether every email in the thread carries the keyword."),
		keywordSort("someInThreadHaveKeyword", "Sorts by whether any email in the thread carries the keyword."),
	}

	s.RegisterStandard("Email", CapabilityMail, StandardMethods{
		Get: true, Changes: true, Set: true, Copy: true, Query: true, QueryChanges: true,
	})

	// Email/get narrows the body parts as well as the records themselves.
	if m, ok := s.Method("Email/get"); ok {
		m.NestedPropertiesArgument = "bodyProperties"
		m.NestedType = "EmailBodyPart"
	}

	s.AppendArguments("Email/get",
		&Field{
			Name: "bodyProperties",
			Type: "String[]|null",
			Doc:  "The properties to include for each EmailBodyPart returned.",
		},
		&Field{
			Name:    "fetchTextBodyValues",
			Type:    "Boolean",
			Default: "false",
			Doc:     "Whether to populate bodyValues for the parts listed in textBody.",
		},
		&Field{
			Name:    "fetchHTMLBodyValues",
			Type:    "Boolean",
			Default: "false",
			Doc:     "Whether to populate bodyValues for the parts listed in htmlBody.",
		},
		&Field{
			Name:    "fetchAllBodyValues",
			Type:    "Boolean",
			Default: "false",
			Doc:     "Whether to populate bodyValues for every textual body part.",
		},
		&Field{
			Name: "maxBodyValueBytes",
			Type: "UnsignedInt",
			Doc:  "The maximum number of octets to return for each body value, truncating longer ones.",
		},
	)

	collapseThreads := func() *Field {
		return &Field{
			Name:    "collapseThreads",
			Type:    "Boolean",
			Default: "false",
			Doc:     "Whether to return only one email per thread, the one that sorts highest among those matching the filter.",
		}
	}
	s.AppendArguments("Email/query", collapseThreads())
	s.AppendArguments("Email/queryChanges", collapseThreads())
}
