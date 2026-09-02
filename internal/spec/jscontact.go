package spec

// JSContact, RFC 9553, is the data model JMAP for Contacts stores: a Card and
// the two dozen object types it is built from. The types keep a "Contact"
// prefix here, because JSContact is a specification in its own right and names
// several types — EmailAddress, Address, Name, Media — that JMAP itself, or a
// future capability, also names. The prefix keeps them apart without renaming
// anything a query already refers to; each type's documentation gives the name
// the specification uses.

// atType is the "@type" member every JSContact object carries. It names the
// object's type, and is what lets a value that may take either of two shapes be
// told apart.
func atType(typeName string) *Field {
	return &Field{
		Name: "@type",
		Type: "String",
		Doc:  "The type of this object, which is \"" + typeName + "\".",
	}
}

// contexts is the member several JSContact objects share for saying where a
// piece of contact information applies.
func contextsField() *Field {
	return &Field{
		Name: "contexts",
		Type: "String[Boolean]",
		Doc:  "The contexts this applies in, mapped to true: \"private\", \"work\", or a context the object's own type defines.",
	}
}

// pref is the member that ranks alternatives of the same kind.
func prefField() *Field {
	return &Field{
		Name: "pref",
		Type: "UnsignedInt",
		Doc:  "How preferred this is among the alternatives, from 1 (most) to 100 (least).",
	}
}

// label is the free-text name a user may give one entry.
func labelField() *Field {
	return &Field{
		Name: "label",
		Type: "String",
		Doc:  "A human-readable label for this entry.",
	}
}

// resourceFields are the members of the Resource type of RFC 9553, Section
// 1.4.4, which CryptoKey, Directory, Link, and Media are each a kind of.
func resourceFields(typeName, kindDoc string) []*Field {
	return []*Field{
		atType(typeName),
		{Name: "kind", Type: "String", Doc: kindDoc},
		{Name: "uri", Type: "String", Doc: "The URI where the resource is found."},
		{Name: "mediaType", Type: "String", Doc: "The media type of the resource, as registered with IANA."},
		contextsField(),
		prefField(),
		labelField(),
	}
}

// registerJSContact adds the JSContact object types of RFC 9553.
func registerJSContact(s *Spec) {
	registerJSContactName(s)
	registerJSContactCommunication(s)
	registerJSContactAddress(s)
	registerJSContactResources(s)
	registerJSContactAdditional(s)
}

func registerJSContactName(s *Spec) {
	s.AddObject(&Object{
		Name:       "ContactNameComponent",
		Capability: CapabilityContacts,
		Doc:        "ContactNameComponent is one part of a name, such as a given name or a surname. RFC 9553 calls it NameComponent.",
		Fields: []*Field{
			atType("NameComponent"),
			{Name: "value", Type: "String", Doc: "The value of this part of the name."},
			{
				Name: "kind",
				Type: "String",
				Doc:  "What part of the name this is: \"title\", \"given\", \"given2\", \"surname\", \"surname2\", \"credential\", \"generation\", or \"separator\".",
			},
			{
				Name: "phonetic",
				Type: "String",
				Doc:  "How to pronounce this part, written in the script and system the enclosing name gives.",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "ContactName",
		Capability: CapabilityContacts,
		Doc:        "ContactName is the name of the entity a card describes, as an ordered set of parts rather than as one string. RFC 9553 calls it Name.",
		Fields: []*Field{
			atType("Name"),
			{Name: "components", Type: "ContactNameComponent[]", Doc: "The parts of the name."},
			{
				Name:    "isOrdered",
				Type:    "Boolean",
				Default: "false",
				Doc:     "Whether the components are already in the order they should be displayed in.",
			},
			{
				Name: "defaultSeparator",
				Type: "String",
				Doc:  "What to put between the components when joining them, where no separator component says otherwise.",
			},
			{Name: "full", Type: "String", Doc: "The name written out in full, for display."},
			{
				Name: "sortAs",
				Type: "String[String]",
				Doc:  "How to sort the name, keyed by the component kind the value sorts in place of.",
			},
			{
				Name: "phoneticScript",
				Type: "String",
				Doc:  "The script the phonetic members of the components are written in.",
			},
			{
				Name: "phoneticSystem",
				Type: "String",
				Doc:  "The system the phonetic members use: \"ipa\", \"jyut\", or \"piny\".",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "ContactNickname",
		Capability: CapabilityContacts,
		Doc:        "ContactNickname is a name the entity is also known by. RFC 9553 calls it Nickname.",
		Fields: []*Field{
			atType("Nickname"),
			{Name: "name", Type: "String", Doc: "The nickname."},
			contextsField(),
			prefField(),
		},
	})

	s.AddObject(&Object{
		Name:       "ContactOrgUnit",
		Capability: CapabilityContacts,
		Doc:        "ContactOrgUnit is a unit within an organization, such as a department. RFC 9553 calls it OrgUnit.",
		Fields: []*Field{
			atType("OrgUnit"),
			{Name: "name", Type: "String", Doc: "The name of the unit."},
			{Name: "sortAs", Type: "String", Doc: "The value to sort the unit by, in place of its name."},
		},
	})

	s.AddObject(&Object{
		Name:       "ContactOrganization",
		Capability: CapabilityContacts,
		Doc:        "ContactOrganization is a company or other body the entity is associated with. RFC 9553 calls it Organization; at least one of name and units is set.",
		Fields: []*Field{
			atType("Organization"),
			{Name: "name", Type: "String", Doc: "The name of the organization."},
			{Name: "units", Type: "ContactOrgUnit[]", Doc: "The units within the organization, from the largest to the smallest."},
			{Name: "sortAs", Type: "String", Doc: "The value to sort the organization by, in place of its name."},
			contextsField(),
		},
	})

	s.AddObject(&Object{
		Name:       "ContactPronouns",
		Capability: CapabilityContacts,
		Doc:        "ContactPronouns is a set of pronouns to use for the entity. RFC 9553 calls it Pronouns.",
		Fields: []*Field{
			atType("Pronouns"),
			{Name: "pronouns", Type: "String", Doc: "The pronouns, written as the entity would have them written."},
			contextsField(),
			prefField(),
		},
	})

	s.AddObject(&Object{
		Name:       "ContactSpeakToAs",
		Capability: CapabilityContacts,
		Doc:        "ContactSpeakToAs says how to address the entity in speech and writing. RFC 9553 calls it SpeakToAs; at least one of its members is set.",
		Fields: []*Field{
			atType("SpeakToAs"),
			{
				Name: "grammaticalGender",
				Type: "String",
				Doc:  "The grammatical gender to use in salutations: \"animate\", \"common\", \"feminine\", \"inanimate\", \"masculine\", or \"neuter\".",
			},
			{Name: "pronouns", Type: "Id[ContactPronouns]", Doc: "The pronouns to use, keyed by an id local to the card."},
		},
	})

	s.AddObject(&Object{
		Name:       "ContactTitle",
		Capability: CapabilityContacts,
		Doc:        "ContactTitle is a position or role the entity holds. RFC 9553 calls it Title.",
		Fields: []*Field{
			atType("Title"),
			{Name: "name", Type: "String", Doc: "The title or role."},
			{
				Name:    "kind",
				Type:    "String",
				Default: "\"title\"",
				Doc:     "Whether this is a \"title\" or a \"role\".",
			},
			{
				Name: "organizationId",
				Type: "Id",
				Doc:  "The id, within this card's organizations, of the organization the title is held in.",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "ContactRelation",
		Capability: CapabilityContacts,
		Doc:        "ContactRelation says how another entity is related to this one. RFC 9553 calls it Relation.",
		Fields: []*Field{
			atType("Relation"),
			{
				Name:    "relation",
				Type:    "String[Boolean]",
				Default: "an empty object",
				Doc:     "The kinds of relation, mapped to true, such as \"friend\", \"colleague\", or \"spouse\". An empty object means the entities are related in a way the card does not name.",
			},
		},
	})
}

func registerJSContactCommunication(s *Spec) {
	s.AddObject(&Object{
		Name:       "ContactEmailAddress",
		Capability: CapabilityContacts,
		Doc:        "ContactEmailAddress is an address the entity receives mail at. RFC 9553 calls it EmailAddress; it is unrelated to the EmailAddress of JMAP for Mail, which is a header field value.",
		Fields: []*Field{
			atType("EmailAddress"),
			{Name: "address", Type: "String", Doc: "The address itself, as an addr-spec."},
			contextsField(),
			prefField(),
			labelField(),
		},
	})

	s.AddObject(&Object{
		Name:       "ContactOnlineService",
		Capability: CapabilityContacts,
		Doc:        "ContactOnlineService is an account the entity has with an online service. RFC 9553 calls it OnlineService; at least one of uri and user is set.",
		Fields: []*Field{
			atType("OnlineService"),
			{Name: "service", Type: "String", Doc: "The name of the service, as the service itself spells it."},
			{Name: "uri", Type: "String", Doc: "The URI of the entity's presence on the service."},
			{Name: "user", Type: "String", Doc: "The entity's username on the service."},
			contextsField(),
			prefField(),
			labelField(),
		},
	})

	s.AddObject(&Object{
		Name:       "ContactPhone",
		Capability: CapabilityContacts,
		Doc:        "ContactPhone is a number the entity can be reached on. RFC 9553 calls it Phone.",
		Fields: []*Field{
			atType("Phone"),
			{Name: "number", Type: "String", Doc: "The number, either as a URI or as free text."},
			{
				Name: "features",
				Type: "String[Boolean]",
				Doc:  "What the number can be used for, mapped to true: \"mobile\", \"voice\", \"text\", \"video\", \"main-number\", \"textphone\", \"fax\", or \"pager\".",
			},
			contextsField(),
			prefField(),
			labelField(),
		},
	})

	s.AddObject(&Object{
		Name:       "ContactLanguagePref",
		Capability: CapabilityContacts,
		Doc:        "ContactLanguagePref is a language the entity prefers to be contacted in. RFC 9553 calls it LanguagePref.",
		Fields: []*Field{
			atType("LanguagePref"),
			{Name: "language", Type: "String", Doc: "The language tag, as defined by RFC 5646."},
			contextsField(),
			prefField(),
		},
	})

	s.AddObject(&Object{
		Name:       "ContactSchedulingAddress",
		Capability: CapabilityContacts,
		Doc:        "ContactSchedulingAddress is where to send scheduling requests for the entity. RFC 9553 calls it SchedulingAddress.",
		Fields: []*Field{
			atType("SchedulingAddress"),
			{Name: "uri", Type: "String", Doc: "The address to send scheduling requests to, such as a mailto: URI."},
			contextsField(),
			prefField(),
			labelField(),
		},
	})

	s.AddObject(&Object{
		Name:       "ContactCalendar",
		Capability: CapabilityContacts,
		Doc:        "ContactCalendar is a calendar or free-busy feed belonging to the entity. RFC 9553 calls it Calendar, and it is a kind of Resource.",
		Fields: []*Field{
			atType("Calendar"),
			{Name: "kind", Type: "String", Doc: "What the resource is: \"calendar\" or \"freeBusy\"."},
			{Name: "uri", Type: "String", Doc: "The URI where the calendar is found."},
			{Name: "mediaType", Type: "String", Doc: "The media type of the calendar, as registered with IANA."},
			contextsField(),
			prefField(),
			labelField(),
		},
	})
}

func registerJSContactAddress(s *Spec) {
	s.AddObject(&Object{
		Name:       "ContactAddressComponent",
		Capability: CapabilityContacts,
		Doc:        "ContactAddressComponent is one part of an address, such as a street name or a postcode. RFC 9553 calls it AddressComponent.",
		Fields: []*Field{
			atType("AddressComponent"),
			{Name: "value", Type: "String", Doc: "The value of this part of the address."},
			{
				Name: "kind",
				Type: "String",
				Doc:  "What part of the address this is: \"room\", \"apartment\", \"floor\", \"building\", \"number\", \"name\", \"block\", \"subdistrict\", \"district\", \"locality\", \"region\", \"postcode\", \"country\", \"direction\", \"landmark\", \"postOfficeBox\", or \"separator\".",
			},
			{Name: "phonetic", Type: "String", Doc: "How to pronounce this part, in the script and system the address gives."},
		},
	})

	s.AddObject(&Object{
		Name:       "ContactAddress",
		Capability: CapabilityContacts,
		Doc:        "ContactAddress is a place associated with the entity, as an ordered set of parts rather than as one string. RFC 9553 calls it Address; it is unrelated to the Address of JMAP for Mail, which is an SMTP envelope address.",
		Fields: []*Field{
			atType("Address"),
			{Name: "components", Type: "ContactAddressComponent[]", Doc: "The parts of the address."},
			{
				Name:    "isOrdered",
				Type:    "Boolean",
				Default: "false",
				Doc:     "Whether the components are already in the order they should be displayed in.",
			},
			{Name: "countryCode", Type: "String", Doc: "The ISO 3166-1 alpha-2 code of the country."},
			{Name: "coordinates", Type: "String", Doc: "The location as a geo: URI, as defined by RFC 5870."},
			{Name: "timeZone", Type: "String", Doc: "The time zone the address is in, named as in the IANA Time Zone Database."},
			contextsField(),
			{Name: "full", Type: "String", Doc: "The address written out in full, for display."},
			{
				Name: "defaultSeparator",
				Type: "String",
				Doc:  "What to put between the components when joining them, where no separator component says otherwise.",
			},
			prefField(),
			{Name: "phoneticScript", Type: "String", Doc: "The script the phonetic members of the components are written in."},
			{
				Name: "phoneticSystem",
				Type: "String",
				Doc:  "The system the phonetic members use: \"ipa\", \"jyut\", or \"piny\".",
			},
		},
	})
}

func registerJSContactResources(s *Spec) {
	s.AddObject(&Object{
		Name:       "ContactCryptoKey",
		Capability: CapabilityContacts,
		Doc:        "ContactCryptoKey is a public key or certificate belonging to the entity. RFC 9553 calls it CryptoKey, and it is a kind of Resource.",
		Fields:     resourceFields("CryptoKey", "The kind of key, if the card says."),
	})

	s.AddObject(&Object{
		Name:       "ContactDirectory",
		Capability: CapabilityContacts,
		Doc:        "ContactDirectory is a directory the entity is listed in, or the entry within one. RFC 9553 calls it Directory, and it is a kind of Resource.",
		Fields: append(
			resourceFields("Directory", "What the resource is: \"directory\" for a directory the entity is listed in, or \"entry\" for the entity's own entry."),
			&Field{
				Name: "listAs",
				Type: "UnsignedInt",
				Doc:  "Where the entity sorts among the entries of the directory, counting from 1.",
			},
		),
	})

	s.AddObject(&Object{
		Name:       "ContactLink",
		Capability: CapabilityContacts,
		Doc:        "ContactLink is a resource related to the entity that no other property covers. RFC 9553 calls it Link, and it is a kind of Resource.",
		Fields:     resourceFields("Link", "What the resource is: \"contact\" for a way of contacting the entity, or absent if the card does not say."),
	})

	s.AddObject(&Object{
		Name:       "ContactMedia",
		Capability: CapabilityContacts,
		Doc:        "ContactMedia is a photograph, logo, or sound clip of the entity. RFC 9553 calls it Media, and it is a kind of Resource. JMAP for Contacts adds blobId, so that the content can be fetched from the server rather than from the URI.",
		Fields: append(
			resourceFields("Media", "What the resource is: \"photo\", \"sound\", or \"logo\"."),
			&Field{
				Name: "blobId",
				Type: "Id",
				Doc:  "The id of the blob holding the content, for media the server stores itself.",
			},
		),
	})
}

func registerJSContactAdditional(s *Spec) {
	s.AddObject(&Object{
		Name:       "ContactPartialDate",
		Capability: CapabilityContacts,
		Doc:        "ContactPartialDate is a date some of whose parts are unknown, such as a birthday with no year. RFC 9553 calls it PartialDate.",
		Fields: []*Field{
			atType("PartialDate"),
			{Name: "year", Type: "UnsignedInt", Doc: "The year, if it is known."},
			{Name: "month", Type: "UnsignedInt", Doc: "The month, from 1 to 12, if it is known."},
			{Name: "day", Type: "UnsignedInt", Doc: "The day of the month, from 1 to 31, if it is known."},
			{
				Name: "calendarScale",
				Type: "String",
				Doc:  "The calendar system the date is given in, named as in CLDR. Absent means Gregorian.",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "ContactTimestamp",
		Capability: CapabilityContacts,
		Doc:        "ContactTimestamp is a date and time known exactly, which an anniversary may carry instead of a partial date. RFC 9553 calls it Timestamp.",
		Fields: []*Field{
			atType("Timestamp"),
			{Name: "utc", Type: "UTCDate", Doc: "The point in time, in UTC."},
		},
	})

	s.AddObject(&Object{
		Name:       "ContactAnniversary",
		Capability: CapabilityContacts,
		Doc:        "ContactAnniversary is a date of significance to the entity, such as a birthday. RFC 9553 calls it Anniversary.",
		Fields: []*Field{
			atType("Anniversary"),
			{Name: "kind", Type: "String", Doc: "What the date marks: \"birth\", \"death\", or \"wedding\"."},
			{
				Name: "date",
				Type: "ContactPartialDate|ContactTimestamp",
				Doc:  "The date, either partially known or exact. A value with no @type is a partial date.",
			},
			{Name: "place", Type: "ContactAddress", Doc: "Where the anniversary took place."},
		},
	})

	s.AddObject(&Object{
		Name:       "ContactAuthor",
		Capability: CapabilityContacts,
		Doc:        "ContactAuthor is who wrote a note. RFC 9553 calls it Author; at least one of its members besides @type is set.",
		Fields: []*Field{
			atType("Author"),
			{Name: "name", Type: "String", Doc: "The name of the author."},
			{Name: "uri", Type: "String", Doc: "A URI identifying the author."},
		},
	})

	s.AddObject(&Object{
		Name:       "ContactNote",
		Capability: CapabilityContacts,
		Doc:        "ContactNote is free text about the entity. RFC 9553 calls it Note.",
		Fields: []*Field{
			atType("Note"),
			{Name: "note", Type: "String", Doc: "The text of the note."},
			{Name: "created", Type: "UTCDate", Doc: "When the note was written."},
			{Name: "author", Type: "ContactAuthor", Doc: "Who wrote the note."},
		},
	})

	s.AddObject(&Object{
		Name:       "ContactPersonalInfo",
		Capability: CapabilityContacts,
		Doc:        "ContactPersonalInfo is something the entity is interested in or good at. RFC 9553 calls it PersonalInfo.",
		Fields: []*Field{
			atType("PersonalInfo"),
			{Name: "kind", Type: "String", Doc: "What sort of information this is: \"expertise\", \"hobby\", or \"interest\"."},
			{Name: "value", Type: "String", Doc: "The interest or field of expertise itself."},
			{
				Name: "level",
				Type: "String",
				Doc:  "How much: \"high\", \"medium\", or \"low\".",
			},
			{
				Name: "listAs",
				Type: "UnsignedInt",
				Doc:  "Where this sorts among the entity's other information of the same kind, counting from 1.",
			},
			labelField(),
		},
	})
}
