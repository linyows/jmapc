package spec

// registerContacts adds JMAP for Contacts, RFC 9610. A card is a JSContact Card
// from RFC 9553, which is why the bulk of the data model lives in jscontact.go:
// this file is only what JMAP adds around it.
func registerContacts(s *Spec) {
	registerJSContact(s)
	registerAddressBook(s)
	registerContactCard(s)
}

func registerAddressBook(s *Spec) {
	s.AddObject(&Object{
		Name:       "AddressBookRights",
		Capability: CapabilityContacts,
		Doc:        "AddressBookRights says what the authenticated user may do with an address book.",
		Fields: []*Field{
			{Name: "mayRead", Type: "Boolean", Doc: "Whether the user may read the cards in the address book."},
			{Name: "mayWrite", Type: "Boolean", Doc: "Whether the user may create, modify, and destroy cards in it."},
			{Name: "mayShare", Type: "Boolean", Doc: "Whether the user may change who else it is shared with."},
			{Name: "mayDelete", Type: "Boolean", Doc: "Whether the user may delete the address book itself."},
		},
	})

	s.AddObject(&Object{
		Name:       "AddressBook",
		Capability: CapabilityContacts,
		Doc:        "AddressBook is a named collection of contact cards.",
		Fields: []*Field{
			{Name: "id", Type: "Id", ServerSet: true, Immutable: true, Doc: "The id of the address book."},
			{Name: "name", Type: "String", Doc: "The user-visible name of the address book, at most 255 octets of UTF-8."},
			{Name: "description", Type: "String|null", Doc: "A longer description of what the address book holds."},
			{
				Name:    "sortOrder",
				Type:    "UnsignedInt",
				Default: "0",
				Doc:     "A hint for where to place the address book in a list of them.",
			},
			{
				Name:      "isDefault",
				Type:      "Boolean",
				ServerSet: true,
				Doc:       "Whether this is the address book a card goes into when the client does not say. Exactly one address book in an account has this set.",
			},
			{Name: "isSubscribed", Type: "Boolean", Doc: "Whether the user has subscribed to the address book."},
			{
				Name: "shareWith",
				Type: "Id[AddressBookRights]|null",
				Doc:  "Who else the address book is shared with, keyed by principal id, and what each may do.",
			},
			{
				Name:      "myRights",
				Type:      "AddressBookRights",
				ServerSet: true,
				Doc:       "What the authenticated user may do with the address book.",
			},
		},
	})

	s.RegisterStandard("AddressBook", CapabilityContacts, StandardMethods{
		Get: true, Changes: true, Set: true,
	})
	s.AppendArguments("AddressBook/set",
		&Field{
			Name:    "onDestroyRemoveContents",
			Type:    "Boolean",
			Default: "false",
			Doc:     "Whether destroying an address book may also destroy the cards in it. If false, destroying one that is not empty fails with an addressBookHasContents error.",
		},
		&Field{
			Name: "onSuccessSetIsDefault",
			Type: "Id|null",
			Doc:  "The id of the address book to make the default once the other changes succeed.",
		},
	)
}

func registerContactCard(s *Spec) {
	s.AddObject(&Object{
		Name:       "ContactCard",
		Capability: CapabilityContacts,
		Doc: "ContactCard is one entry in an address book: a person, an organization, or anything else a card can describe. " +
			"It is a JSContact Card, as defined by RFC 9553, with the id and addressBookIds that JMAP adds.",
		Fields: []*Field{
			// What JMAP adds to a JSContact Card.
			{Name: "id", Type: "Id", ServerSet: true, Immutable: true, Doc: "The id of the card."},
			{
				Name: "addressBookIds",
				Type: "Id[Boolean]",
				Doc:  "The address books the card is in, as a set of ids mapped to true.",
			},

			// JSContact metadata properties, RFC 9553, Section 2.1.
			atType("Card"),
			{Name: "version", Type: "String", Doc: "The version of JSContact the card is written in, which is \"1.0\"."},
			{Name: "created", Type: "UTCDate", Doc: "When the card was created."},
			{
				Name: "kind",
				Type: "String",
				Enum: []string{"individual", "group", "org", "location", "device", "application"},
				Doc:  "What the card describes: \"individual\", \"group\", \"org\", \"location\", \"device\", or \"application\". Absent means \"individual\".",
			},
			{Name: "language", Type: "String", Doc: "The language the card's text is written in, as an RFC 5646 tag."},
			{
				Name: "members",
				Type: "String[Boolean]",
				Doc:  "The uids of the cards belonging to this one, mapped to true, for a card whose kind is \"group\".",
			},
			{Name: "prodId", Type: "String", Doc: "The software that last modified the card."},
			{
				Name: "relatedTo",
				Type: "String[ContactRelation]",
				Doc:  "Other entities related to this one, keyed by their uid.",
			},
			{Name: "uid", Type: "String", Doc: "A globally unique identifier for the entity, which survives being copied between address books."},
			{Name: "updated", Type: "UTCDate", Doc: "When the card was last modified."},

			// Name and organization, Section 2.2.
			{Name: "name", Type: "ContactName", Doc: "The name of the entity."},
			{Name: "nicknames", Type: "Id[ContactNickname]", Doc: "Other names the entity goes by, keyed by an id local to the card."},
			{Name: "organizations", Type: "Id[ContactOrganization]", Doc: "The organizations the entity belongs to, keyed by an id local to the card."},
			{Name: "speakToAs", Type: "ContactSpeakToAs", Doc: "How to address the entity."},
			{Name: "titles", Type: "Id[ContactTitle]", Doc: "The positions and roles the entity holds, keyed by an id local to the card."},

			// Contact, Section 2.3.
			{Name: "emails", Type: "Id[ContactEmailAddress]", Doc: "The addresses the entity receives mail at, keyed by an id local to the card."},
			{Name: "onlineServices", Type: "Id[ContactOnlineService]", Doc: "The entity's accounts with online services, keyed by an id local to the card."},
			{Name: "phones", Type: "Id[ContactPhone]", Doc: "The numbers the entity can be reached on, keyed by an id local to the card."},
			{Name: "preferredLanguages", Type: "Id[ContactLanguagePref]", Doc: "The languages the entity prefers to be contacted in, keyed by an id local to the card."},

			// Calendaring and scheduling, Section 2.4.
			{Name: "calendars", Type: "Id[ContactCalendar]", Doc: "The entity's calendars and free-busy feeds, keyed by an id local to the card."},
			{Name: "schedulingAddresses", Type: "Id[ContactSchedulingAddress]", Doc: "Where to send the entity scheduling requests, keyed by an id local to the card."},

			// Address and location, Section 2.5.
			{Name: "addresses", Type: "Id[ContactAddress]", Doc: "The places associated with the entity, keyed by an id local to the card."},

			// Resources, Section 2.6.
			{Name: "cryptoKeys", Type: "Id[ContactCryptoKey]", Doc: "The entity's public keys and certificates, keyed by an id local to the card."},
			{Name: "directories", Type: "Id[ContactDirectory]", Doc: "The directories the entity is listed in, keyed by an id local to the card."},
			{Name: "links", Type: "Id[ContactLink]", Doc: "Other resources related to the entity, keyed by an id local to the card."},
			{Name: "media", Type: "Id[ContactMedia]", Doc: "Photographs, logos, and sound clips of the entity, keyed by an id local to the card."},

			// Multilingual, Section 2.7.
			{
				Name:        "localizations",
				Type:        "String[PatchObject]",
				PatchTarget: "ContactCard",
				Doc:         "Translations of the card's text, keyed by language tag. Each is a patch to apply to the card to render it in that language.",
			},

			// Additional, Section 2.8.
			{Name: "anniversaries", Type: "Id[ContactAnniversary]", Doc: "Dates of significance to the entity, keyed by an id local to the card."},
			{Name: "keywords", Type: "String[Boolean]", Doc: "Free-text keywords the user has put on the card, mapped to true."},
			{Name: "notes", Type: "Id[ContactNote]", Doc: "Free text about the entity, keyed by an id local to the card."},
			{Name: "personalInfo", Type: "Id[ContactPersonalInfo]", Doc: "What the entity is interested in or good at, keyed by an id local to the card."},
		},
	})

	s.AddObject(&Object{
		Name:       "ContactCardFilterCondition",
		Capability: CapabilityContacts,
		Doc:        "ContactCardFilterCondition is a condition a card must satisfy to match a ContactCard/query. Where a condition sets more than one property, a card must satisfy all of them.",
		Fields: []*Field{
			{Name: "inAddressBook", Type: "Id", Doc: "Matches cards in this address book."},
			{Name: "uid", Type: "String", Doc: "Matches the card with this uid."},
			{Name: "hasMember", Type: "String", Doc: "Matches group cards having a member with this uid."},
			{Name: "kind", Type: "String", Doc: "Matches cards of this kind."},
			{Name: "createdBefore", Type: "UTCDate", Doc: "Matches cards created before this time."},
			{Name: "createdAfter", Type: "UTCDate", Doc: "Matches cards created at or after this time."},
			{Name: "updatedBefore", Type: "UTCDate", Doc: "Matches cards last modified before this time."},
			{Name: "updatedAfter", Type: "UTCDate", Doc: "Matches cards last modified at or after this time."},
			{Name: "text", Type: "String", Doc: "Matches cards where this text appears in any of the properties the other conditions search individually."},
			{Name: "name", Type: "String", Doc: "Matches cards where this text appears in the name."},
			{Name: "name/given", Type: "String", Doc: "Matches cards where this text appears in a given name component."},
			{Name: "name/surname", Type: "String", Doc: "Matches cards where this text appears in a surname component."},
			{Name: "name/surname2", Type: "String", Doc: "Matches cards where this text appears in a secondary surname component."},
			{Name: "nickname", Type: "String", Doc: "Matches cards where this text appears in a nickname."},
			{Name: "organization", Type: "String", Doc: "Matches cards where this text appears in an organization name or unit."},
			{Name: "email", Type: "String", Doc: "Matches cards where this text appears in an email address."},
			{Name: "phone", Type: "String", Doc: "Matches cards where this text appears in a phone number."},
			{Name: "onlineService", Type: "String", Doc: "Matches cards where this text appears in an online service name, uri, or user."},
			{Name: "address", Type: "String", Doc: "Matches cards where this text appears in an address."},
			{Name: "note", Type: "String", Doc: "Matches cards where this text appears in a note."},
		},
	})

	card, _ := s.Object("ContactCard")
	card.Sort = []*SortProperty{
		{Name: "created", Doc: "Sorts by when the card was created."},
		{Name: "updated", Doc: "Sorts by when the card was last modified."},
		{Name: "name/given", Doc: "Sorts by the given name. A server need not support this."},
		{Name: "name/surname", Doc: "Sorts by the surname. A server need not support this."},
		{Name: "name/surname2", Doc: "Sorts by the secondary surname. A server need not support this."},
	}

	s.RegisterStandard("ContactCard", CapabilityContacts, StandardMethods{
		Get: true, Changes: true, Set: true, Copy: true, Query: true, QueryChanges: true,
	})
}
