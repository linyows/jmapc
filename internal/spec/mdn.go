package spec

// registerMDN adds message disposition notifications, RFC 9007: the receipt
// that tells a sender their message was displayed, deleted, or filed away.
//
// An MDN is not stored. It is a message the server composes and sends, so there
// is no /get and no /set, only a method to send one and a method to read one
// that arrived.
func registerMDN(s *Spec) {
	s.AddObject(&Object{
		Name:       "MDNDisposition",
		Capability: CapabilityMDN,
		Doc: "MDNDisposition says what became of the message and how much of that the user chose. " +
			"RFC 9007 calls it Disposition; the name is qualified here because it is too general to claim on its own.",
		Fields: []*Field{
			{
				Name: "actionMode",
				Type: "String",
				Enum: []string{"manual-action", "automatic-action"},
				Doc:  "Whether the user did this themselves or the client did it for them: \"manual-action\" or \"automatic-action\".",
			},
			{
				Name: "sendingMode",
				Type: "String",
				Enum: []string{"mdn-sent-manually", "mdn-sent-automatically"},
				Doc: "Whether the user chose to send the notification: \"mdn-sent-manually\" if they were asked, " +
					"\"mdn-sent-automatically\" if the client sent it without asking.",
			},
			{
				Name: "type",
				Type: "String",
				Enum: []string{"deleted", "dispatched", "displayed", "processed"},
				Doc: "What happened to the message: \"deleted\" without being read, \"dispatched\" onwards, " +
					"\"displayed\" to the user, or \"processed\" in some way the other three do not cover.",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "MDN",
		Capability: CapabilityMDN,
		Doc: "MDN is a message disposition notification: a receipt saying what became of a message the user received. " +
			"Sending one is a courtesy the recipient decides on, not something the sender can require.",
		Fields: []*Field{
			{
				Name: "forEmailId",
				Type: "Id|null",
				Doc:  "The id of the email this is about. It is required when sending, and may be null in one that was parsed, where the message it refers to may not be in the account.",
			},
			{Name: "subject", Type: "String|null", Doc: "The Subject header field for the notification itself."},
			{Name: "textBody", Type: "String|null", Doc: "A human-readable explanation, which the notification carries alongside the machine-readable part."},
			{
				Name:    "includeOriginalMessage",
				Type:    "Boolean",
				Default: "false",
				Doc:     "Whether to send the original message back with the notification.",
			},
			{Name: "reportingUA", Type: "String|null", Doc: "The name of the software that produced the notification."},
			{Name: "disposition", Type: "MDNDisposition", Doc: "What became of the message."},
			{
				Name:      "mdnGateway",
				Type:      "String|null",
				ServerSet: true,
				Doc:       "The gateway that translated the notification, for one that crossed from another mail system.",
			},
			{
				Name:      "originalRecipient",
				Type:      "String|null",
				ServerSet: true,
				Doc:       "The address the original message was addressed to, which may differ from where it ended up.",
			},
			{
				Name: "finalRecipient",
				Type: "String|null",
				Doc:  "The address the notification is sent on behalf of. The server fills it in from the identity when it is not given.",
			},
			{
				Name:      "originalMessageId",
				Type:      "String|null",
				ServerSet: true,
				Doc:       "The Message-ID of the message this is about.",
			},
			{
				Name:      "error",
				Type:      "String[]|null",
				ServerSet: true,
				Doc:       "What went wrong, for a notification whose disposition reports a failure.",
			},
			{
				Name: "extensionFields",
				Type: "String[String]|null",
				Doc:  "Fields beyond those the specification defines, keyed by field name.",
			},
		},
	})

	sendArgs := s.AddObject(&Object{
		Name:       "MDNSendArguments",
		Capability: CapabilityMDN,
		Kind:       KindArguments,
		Doc:        "MDNSendArguments holds the arguments of the MDN/send method.",
		Fields: []*Field{
			accountIDField(),
			{
				Name: "identityId",
				Type: "Id",
				Doc:  "The identity to send from, which decides both the sender and the address the notification is issued for.",
			},
			{
				Name: "send",
				Type: "Id[MDN]",
				Doc:  "The notifications to send, keyed by creation id.",
			},
			{
				Name:        "onSuccessUpdateEmail",
				Type:        "Id[PatchObject]|null",
				PatchTarget: "Email",
				Doc: "Patches to apply to the emails of the notifications that were sent, keyed by creation id. " +
					"This is where the $mdnsent keyword is set, so that a receipt is not sent for the same message twice.",
			},
		},
	})
	sendResp := s.AddObject(&Object{
		Name:       "MDNSendResponse",
		Capability: CapabilityMDN,
		Kind:       KindResponse,
		Doc:        "MDNSendResponse holds the response to the MDN/send method.",
		Fields: []*Field{
			accountIDField(),
			{
				Name: "sent",
				Type: "Id[MDN]|null",
				Doc:  "The notifications that were sent, with the properties the server filled in, keyed by creation id.",
			},
			{
				Name: "notSent",
				Type: "Id[SetError]|null",
				Doc:  "A map of creation id to the reason the notification could not be sent.",
			},
		},
	})
	s.AddMethod(&Method{
		Name:       "MDN/send",
		Capability: CapabilityMDN,
		Doc:        "Sends a receipt telling the sender of a message what became of it.",
		Arguments:  sendArgs.Name,
		Response:   sendResp.Name,
		DataType:   "MDN",
	})

	parseArgs := s.AddObject(&Object{
		Name:       "MDNParseArguments",
		Capability: CapabilityMDN,
		Kind:       KindArguments,
		Doc:        "MDNParseArguments holds the arguments of the MDN/parse method.",
		Fields: []*Field{
			accountIDField(),
			{Name: "blobIds", Type: "Id[]", Doc: "The ids of the blobs to read as notifications."},
		},
	})
	parseResp := s.AddObject(&Object{
		Name:       "MDNParseResponse",
		Capability: CapabilityMDN,
		Kind:       KindResponse,
		Doc:        "MDNParseResponse holds the response to the MDN/parse method.",
		Fields: []*Field{
			accountIDField(),
			{
				Name: "parsed",
				Type: "Id[MDN]|null",
				Doc:  "The notification found in each blob, keyed by blob id.",
			},
			{Name: "notParsable", Type: "Id[]|null", Doc: "The ids of the blobs that do not hold a notification the server could read."},
			{Name: "notFound", Type: "Id[]|null", Doc: "The ids of the blobs that do not exist."},
		},
	})
	s.AddMethod(&Method{
		Name:       "MDN/parse",
		Capability: CapabilityMDN,
		Doc: "Reads a notification that arrived as a message, so that a client can tell the user what became of something they sent. " +
			"A notification is a message like any other, and this is what turns it back into the report it carries.",
		Arguments: parseArgs.Name,
		Response:  parseResp.Name,
		DataType:  "MDN",
	})
}
