package spec

// registerSubmission adds the types and methods for sending mail, defined in
// RFC 8621, Sections 6 and 7. Submission is where JMAP stops describing a
// mailbox and starts handing messages to an SMTP server, so its errors and its
// state model are unlike the rest of the protocol.
func registerSubmission(s *Spec) {
	registerIdentity(s)
	registerEmailSubmission(s)
}

func registerIdentity(s *Spec) {
	s.AddObject(&Object{
		Name:       "Identity",
		Capability: CapabilitySubmission,
		Doc:        "Identity is an address the user may send mail from, together with the defaults to apply when they do.",
		Fields: []*Field{
			{Name: "id", Type: "Id", ServerSet: true, Immutable: true, Doc: "The id of the identity."},
			{
				Name:    "name",
				Type:    "String",
				Default: "\"\"",
				Doc:     "The display name to put in the From header field alongside the address.",
			},
			{
				Name:      "email",
				Type:      "String",
				Immutable: true,
				Doc:       "The address to send from. It may end in \"@domain\" to stand for any address at that domain.",
			},
			{Name: "replyTo", Type: "EmailAddress[]|null", Doc: "The Reply-To header field to set by default."},
			{Name: "bcc", Type: "EmailAddress[]|null", Doc: "The Bcc header field to set by default."},
			{
				Name:    "textSignature",
				Type:    "String",
				Default: "\"\"",
				Doc:     "The signature to append to the plain-text body of a message sent from this identity.",
			},
			{
				Name:    "htmlSignature",
				Type:    "String",
				Default: "\"\"",
				Doc:     "The signature to append to the HTML body of a message sent from this identity.",
			},
			{
				Name:      "mayDelete",
				Type:      "Boolean",
				ServerSet: true,
				Doc:       "Whether the user may delete the identity, which is false for one the server maintains itself.",
			},
		},
	})
	s.RegisterStandard("Identity", CapabilitySubmission, StandardMethods{Get: true, Changes: true, Set: true})
}

func registerEmailSubmission(s *Spec) {
	s.AddObject(&Object{
		Name:       "Address",
		Capability: CapabilitySubmission,
		Doc:        "Address is an SMTP envelope address, with the parameters to send alongside it.",
		Fields: []*Field{
			{Name: "email", Type: "String", Doc: "The addr-spec of the address, with no display name and no angle brackets."},
			{
				Name: "parameters",
				Type: "String[String|null]|null",
				Doc:  "The SMTP parameters to send with the address, keyed by parameter name, with null for one that takes no value.",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "Envelope",
		Capability: CapabilitySubmission,
		Doc:        "Envelope is the SMTP envelope of a submission: who the message is from and who it goes to, which need not match the message's own header fields.",
		Fields: []*Field{
			{Name: "mailFrom", Type: "Address", Doc: "The address to send the MAIL FROM command with, which is where bounces go."},
			{Name: "rcptTo", Type: "Address[]", Doc: "The addresses to send the message to, one RCPT TO command each."},
		},
	})

	s.AddObject(&Object{
		Name:       "DeliveryStatus",
		Capability: CapabilitySubmission,
		Doc:        "DeliveryStatus is what became of a message for one of its recipients.",
		Fields: []*Field{
			{
				Name: "smtpReply",
				Type: "String",
				Doc:  "The SMTP reply the recipient's server gave, kept verbatim.",
			},
			{
				Name: "delivered",
				Type: "String",
				Enum: []string{"queued", "yes", "no", "unknown"},
				Doc:  "How far the message got: \"queued\", \"yes\", \"no\", or \"unknown\".",
			},
			{
				Name: "displayed",
				Type: "String",
				Enum: []string{"unknown", "yes"},
				Doc:  "Whether the message was displayed to the recipient: \"unknown\" or \"yes\".",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "EmailSubmission",
		Capability: CapabilitySubmission,
		Doc:        "EmailSubmission is one attempt to send an email, and the record of what happened to it.",
		Fields: []*Field{
			{Name: "id", Type: "Id", ServerSet: true, Immutable: true, Doc: "The id of the submission."},
			{Name: "identityId", Type: "Id", Immutable: true, Doc: "The id of the identity to send from."},
			{Name: "emailId", Type: "Id", Immutable: true, Doc: "The id of the email to send."},
			{
				Name:      "threadId",
				Type:      "Id",
				ServerSet: true,
				Immutable: true,
				Doc:       "The id of the thread the sent email belongs to.",
			},
			{
				Name:      "envelope",
				Type:      "Envelope|null",
				Immutable: true,
				Doc:       "The SMTP envelope to send with, or null to have the server derive one from the message's header fields.",
			},
			{
				Name:      "sendAt",
				Type:      "UTCDate",
				ServerSet: true,
				Immutable: true,
				Doc:       "When the message was, or will be, released to the SMTP server.",
			},
			{
				Name: "undoStatus",
				Type: "String",
				Enum: []string{"pending", "final", "canceled"},
				Doc:  "Whether the submission can still be stopped: \"pending\", \"final\", or \"canceled\". Setting it to \"canceled\" is how a send is undone while it is still pending.",
			},
			{
				Name:      "deliveryStatus",
				Type:      "String[DeliveryStatus]|null",
				ServerSet: true,
				Doc:       "What became of the message for each recipient, keyed by address, or null if the server does not track it.",
			},
			{
				Name:      "dsnBlobIds",
				Type:      "Id[]",
				ServerSet: true,
				Doc:       "The blob ids of the delivery status notifications received for this submission.",
			},
			{
				Name:      "mdnBlobIds",
				Type:      "Id[]",
				ServerSet: true,
				Doc:       "The blob ids of the message disposition notifications received for this submission.",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "EmailSubmissionFilterCondition",
		Capability: CapabilitySubmission,
		Doc:        "EmailSubmissionFilterCondition is a condition a submission must satisfy to match an EmailSubmission/query.",
		Fields: []*Field{
			{Name: "identityIds", Type: "Id[]", Doc: "Matches submissions sent from one of these identities."},
			{Name: "emailIds", Type: "Id[]", Doc: "Matches submissions of one of these emails."},
			{Name: "threadIds", Type: "Id[]", Doc: "Matches submissions of an email in one of these threads."},
			{Name: "undoStatus", Type: "String", Doc: "Matches submissions with this undo status."},
			{Name: "before", Type: "UTCDate", Doc: "Matches submissions whose sendAt is before this time."},
			{Name: "after", Type: "UTCDate", Doc: "Matches submissions whose sendAt is at or after this time."},
		},
	})

	submission, _ := s.Object("EmailSubmission")
	submission.Sort = []*SortProperty{
		{Name: "emailId", Doc: "Sorts by the id of the email that was sent."},
		{Name: "threadId", Doc: "Sorts by the id of the thread the sent email belongs to."},
		{Name: "sentAt", Doc: "Sorts by when the message was released to the SMTP server."},
	}

	s.RegisterStandard("EmailSubmission", CapabilitySubmission, StandardMethods{
		Get: true, Changes: true, Set: true, Query: true, QueryChanges: true,
	})

	// A submission usually goes hand in hand with a change to the email being
	// sent, such as moving it out of the drafts mailbox. These arguments let
	// both happen in one call, so that neither can be left half done.
	s.AppendArguments("EmailSubmission/set",
		&Field{
			Name:        "onSuccessUpdateEmail",
			Type:        "Id[PatchObject]|null",
			PatchTarget: "Email",
			Doc:         "Patches to apply to the emails of the submissions that succeed, keyed by the submission's id or creation id.",
		},
		&Field{
			Name: "onSuccessDestroyEmail",
			Type: "Id[]|null",
			Doc:  "The ids or creation ids of the submissions whose emails should be destroyed once the submission succeeds.",
		},
	)
}
