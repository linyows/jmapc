package spec

// JSCalendar, RFC 8984, is the data model JMAP for Calendars stores. As with
// JSContact, the types take a prefix here — "Event", since they are the parts
// an event is built from — because JSCalendar names several types that another
// specification in this catalogue also names: its Link and Relation are not the
// Link and Relation of JSContact. Each type's documentation gives the name the
// specification uses.

// registerJSCalendar adds the object types of RFC 8984 that an event is built
// from.
func registerJSCalendar(s *Spec) {
	s.AddObject(&Object{
		Name:       "EventRelation",
		Capability: CapabilityCalendars,
		Doc:        "EventRelation says how another object is related to this one. RFC 8984 calls it Relation.",
		Fields: []*Field{
			atType("Relation"),
			{
				Name:    "relation",
				Type:    "String[Boolean]",
				Default: "an empty object",
				Doc:     "The kinds of relation, mapped to true: \"first\", \"next\", \"child\", or \"parent\". An empty object means the objects are related in a way this does not name.",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "EventLink",
		Capability: CapabilityCalendars,
		Doc:        "EventLink is an external resource associated with an event, such as an agenda or a conference recording. RFC 8984 calls it Link.",
		Fields: []*Field{
			atType("Link"),
			{Name: "href", Type: "String", Doc: "The URI of the resource."},
			{Name: "cid", Type: "String", Doc: "The Content-ID of the resource, for one carried alongside the event."},
			{Name: "contentType", Type: "String", Doc: "The media type of the resource."},
			{Name: "size", Type: "UnsignedInt", Doc: "The size of the resource in octets."},
			{
				Name: "rel",
				Type: "String",
				Doc:  "How the resource relates to the event, as an IANA-registered link relation such as \"describedby\" or \"enclosure\".",
			},
			{
				Name: "display",
				Type: "String",
				Enum: []string{"badge", "graphic", "fullsize", "thumbnail"},
				Doc:  "How to display the resource, for one that is an image: \"badge\", \"graphic\", \"fullsize\", or \"thumbnail\".",
			},
			{Name: "title", Type: "String", Doc: "A human-readable description of the resource."},
		},
	})

	s.AddObject(&Object{
		Name:       "EventLocation",
		Capability: CapabilityCalendars,
		Doc:        "EventLocation is a physical place an event happens at. RFC 8984 calls it Location.",
		Fields: []*Field{
			atType("Location"),
			{Name: "name", Type: "String", Doc: "The name of the location."},
			{Name: "description", Type: "String", Doc: "Directions or other detail about the location."},
			{
				Name: "locationTypes",
				Type: "String[Boolean]",
				Doc:  "What sort of place this is, mapped to true, using the values registered for RFC 4589.",
			},
			{
				Name: "relativeTo",
				Type: "String",
				Enum: []string{"start", "end"},
				Doc:  "What part of the event happens here: \"start\" or \"end\".",
			},
			{
				Name: "timeZone",
				Type: "TimeZoneId",
				Doc:  "The time zone of the location, where it differs from the event's own.",
			},
			{Name: "coordinates", Type: "String", Doc: "The location as a geo: URI, as defined by RFC 5870."},
			{Name: "links", Type: "Id[EventLink]", Doc: "Resources about the location, keyed by an id local to the event."},
		},
	})

	s.AddObject(&Object{
		Name:       "EventVirtualLocation",
		Capability: CapabilityCalendars,
		Doc:        "EventVirtualLocation is somewhere online an event happens, such as a video call. RFC 8984 calls it VirtualLocation.",
		Fields: []*Field{
			atType("VirtualLocation"),
			{Name: "name", Type: "String", Default: "\"\"", Doc: "The name of the virtual location."},
			{Name: "description", Type: "String", Doc: "Instructions for joining, or other detail."},
			{Name: "uri", Type: "String", Doc: "The URI to join at, such as a tel: or https: address."},
			{
				Name: "features",
				Type: "String[Boolean]",
				Enum: []string{"audio", "chat", "feed", "moderator", "phone", "screen", "video"},
				Doc:  "What the location supports, mapped to true: \"audio\", \"chat\", \"feed\", \"moderator\", \"phone\", \"screen\", or \"video\".",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "EventParticipant",
		Capability: CapabilityCalendars,
		Doc:        "EventParticipant is someone or something taking part in an event. RFC 8984 calls it Participant.",
		Fields: []*Field{
			atType("Participant"),
			{Name: "name", Type: "String", Doc: "The participant's name."},
			{Name: "email", Type: "String", Doc: "The participant's email address."},
			{Name: "description", Type: "String", Doc: "A note about the participant."},
			{
				Name: "sendTo",
				Type: "String[String]",
				Doc:  "Where to send scheduling messages, keyed by method, such as \"imip\" mapped to a mailto: URI.",
			},
			{
				Name: "kind",
				Type: "String",
				Enum: []string{"individual", "group", "location", "resource"},
				Doc:  "What the participant is: \"individual\", \"group\", \"location\", or \"resource\".",
			},
			{
				Name:     "roles",
				Type:     "String[Boolean]",
				Required: true,
				Enum:     []string{"owner", "attendee", "optional", "informational", "chair", "contact"},
				Doc:      "What the participant is there for, mapped to true: \"owner\", \"attendee\", \"optional\", \"informational\", \"chair\", or \"contact\".",
			},
			{Name: "locationId", Type: "Id", Doc: "The id, within the event's locations, of where the participant will be."},
			{Name: "language", Type: "String", Doc: "The language to send scheduling messages in, as an RFC 5646 tag."},
			{
				Name:    "participationStatus",
				Type:    "String",
				Default: "\"needs-action\"",
				Enum:    []string{"needs-action", "accepted", "declined", "tentative", "delegated"},
				Doc:     "Whether the participant is coming: \"needs-action\", \"accepted\", \"declined\", \"tentative\", or \"delegated\".",
			},
			{Name: "participationComment", Type: "String", Doc: "A note the participant sent with their reply."},
			{
				Name:    "expectReply",
				Type:    "Boolean",
				Default: "false",
				Doc:     "Whether the participant is expected to reply.",
			},
			{
				Name:    "scheduleAgent",
				Type:    "String",
				Default: "\"server\"",
				Enum:    []string{"server", "client", "none"},
				Doc:     "Who sends the scheduling messages: \"server\", \"client\", or \"none\".",
			},
			{
				Name:    "scheduleForceSend",
				Type:    "Boolean",
				Default: "false",
				Doc:     "Whether to send a scheduling message even though nothing the participant cares about has changed.",
			},
			{
				Name:    "scheduleSequence",
				Type:    "UnsignedInt",
				Default: "0",
				Doc:     "The sequence number of the last scheduling message sent to this participant.",
			},
			{Name: "scheduleStatus", Type: "String[]", Doc: "The status codes returned by the last scheduling attempt."},
			{Name: "scheduleUpdated", Type: "UTCDate", Doc: "When the participant's own copy of the event was last updated."},
			{Name: "sentBy", Type: "String", Doc: "The address of whoever acted on the participant's behalf."},
			{Name: "invitedBy", Type: "Id", Doc: "The id, within the event's participants, of whoever invited this one."},
			{Name: "delegatedTo", Type: "Id[Boolean]", Doc: "The participants this one has delegated to, mapped to true."},
			{Name: "delegatedFrom", Type: "Id[Boolean]", Doc: "The participants who delegated to this one, mapped to true."},
			{Name: "memberOf", Type: "Id[Boolean]", Doc: "The group participants this one belongs to, mapped to true."},
			{Name: "links", Type: "Id[EventLink]", Doc: "Resources about the participant, keyed by an id local to the event."},
		},
	})

	s.AddObject(&Object{
		Name:       "EventOffsetTrigger",
		Capability: CapabilityCalendars,
		Doc:        "EventOffsetTrigger fires an alert a set time before or after the event. RFC 8984 calls it OffsetTrigger.",
		Fields: []*Field{
			atType("OffsetTrigger"),
			{
				Name:     "offset",
				Type:     "SignedDuration",
				Required: true,
				Doc:      "How long before or after the reference point to fire, negative for before.",
			},
			{
				Name:    "relativeTo",
				Type:    "String",
				Default: "\"start\"",
				Enum:    []string{"start", "end"},
				Doc:     "What the offset is measured from: \"start\" or \"end\".",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "EventAbsoluteTrigger",
		Capability: CapabilityCalendars,
		Doc:        "EventAbsoluteTrigger fires an alert at a fixed time, whatever the event does. RFC 8984 calls it AbsoluteTrigger.",
		Fields: []*Field{
			atType("AbsoluteTrigger"),
			{Name: "when", Type: "UTCDate", Required: true, Doc: "When to fire the alert."},
		},
	})

	s.AddObject(&Object{
		Name:       "EventAlert",
		Capability: CapabilityCalendars,
		Doc:        "EventAlert is a reminder attached to an event. RFC 8984 calls it Alert.",
		Fields: []*Field{
			atType("Alert"),
			{
				Name: "trigger",
				Type: "EventOffsetTrigger|EventAbsoluteTrigger",
				Doc:  "When to fire the alert, either relative to the event or at a fixed time. A trigger of a kind this catalogue does not know is left as it was written.",
			},
			{Name: "acknowledged", Type: "UTCDate", Doc: "When the user dismissed the alert."},
			{Name: "relatedTo", Type: "String[EventRelation]", Doc: "Other alerts this one relates to, keyed by their uid."},
			{
				Name:    "action",
				Type:    "String",
				Default: "\"display\"",
				Enum:    []string{"display", "email"},
				Doc:     "What to do when the alert fires: \"display\" or \"email\".",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "EventNDay",
		Capability: CapabilityCalendars,
		Doc:        "EventNDay names a day of the week within a recurrence, optionally counting from the start or end of the period. RFC 8984 calls it NDay.",
		Fields: []*Field{
			atType("NDay"),
			{
				Name: "day",
				Type: "String",
				Enum: []string{"mo", "tu", "we", "th", "fr", "sa", "su"},
				Doc:  "The day of the week: \"mo\", \"tu\", \"we\", \"th\", \"fr\", \"sa\", or \"su\".",
			},
			{
				Name: "nthOfPeriod",
				Type: "Int",
				Doc:  "Which occurrence of that day within the period, counting back from the end when negative. Absent means every one.",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "EventRecurrenceRule",
		Capability: CapabilityCalendars,
		Doc:        "EventRecurrenceRule says how an event repeats. RFC 8984 calls it RecurrenceRule.",
		Fields: []*Field{
			atType("RecurrenceRule"),
			{
				Name:     "frequency",
				Type:     "String",
				Required: true,
				Enum: []string{"yearly", "monthly", "weekly", "daily",
					"hourly", "minutely", "secondly"},
				Doc: "How often the event repeats: \"yearly\", \"monthly\", \"weekly\", \"daily\", \"hourly\", \"minutely\", or \"secondly\".",
			},
			{
				Name:    "interval",
				Type:    "UnsignedInt",
				Default: "1",
				Doc:     "How many periods to skip between occurrences, so 2 with a weekly frequency means every other week.",
			},
			{
				Name:    "rscale",
				Type:    "String",
				Default: "\"gregorian\"",
				Doc:     "The calendar system the rule is expressed in, named as in CLDR.",
			},
			{
				Name:    "skip",
				Type:    "String",
				Default: "\"omit\"",
				Enum:    []string{"omit", "backward", "forward"},
				Doc:     "What to do when a rule lands on a date that does not exist, such as the 31st of a short month: \"omit\", \"backward\", or \"forward\".",
			},
			{
				Name:    "firstDayOfWeek",
				Type:    "String",
				Default: "\"mo\"",
				Enum:    []string{"mo", "tu", "we", "th", "fr", "sa", "su"},
				Doc:     "Which day a week starts on, which decides where weekly intervals fall: \"mo\", \"tu\", \"we\", \"th\", \"fr\", \"sa\", or \"su\".",
			},
			{Name: "byDay", Type: "EventNDay[]", Doc: "The days of the week the event falls on."},
			{Name: "byMonthDay", Type: "Int[]", Doc: "The days of the month, counting back from the end when negative."},
			{Name: "byMonth", Type: "String[]", Doc: "The months, as \"1\" through \"12\", with an \"L\" suffix for a leap month."},
			{Name: "byYearDay", Type: "Int[]", Doc: "The days of the year, counting back from the end when negative."},
			{Name: "byWeekNo", Type: "Int[]", Doc: "The weeks of the year, counting back from the end when negative."},
			{Name: "byHour", Type: "UnsignedInt[]", Doc: "The hours of the day."},
			{Name: "byMinute", Type: "UnsignedInt[]", Doc: "The minutes of the hour."},
			{Name: "bySecond", Type: "UnsignedInt[]", Doc: "The seconds of the minute."},
			{
				Name: "bySetPosition",
				Type: "Int[]",
				Doc:  "Which of the occurrences the rest of the rule generates to keep, counting back from the end when negative.",
			},
			{Name: "count", Type: "UnsignedInt", Doc: "How many occurrences to generate. It cannot be given together with until."},
			{Name: "until", Type: "LocalDateTime", Doc: "The last date-time an occurrence may start at. It cannot be given together with count."},
		},
	})

	s.AddObject(&Object{
		Name:       "EventTimeZoneRule",
		Capability: CapabilityCalendars,
		Doc:        "EventTimeZoneRule is one rule of a custom time zone: when an offset starts to apply, and what it is. RFC 8984 calls it TimeZoneRule.",
		Fields: []*Field{
			atType("TimeZoneRule"),
			{Name: "start", Type: "LocalDateTime", Doc: "When this rule first applies, in local time."},
			{Name: "offsetFrom", Type: "String", Doc: "The UTC offset before the rule applies, such as \"+0100\"."},
			{Name: "offsetTo", Type: "String", Doc: "The UTC offset once the rule applies."},
			{Name: "recurrenceRules", Type: "EventRecurrenceRule[]", Doc: "How the rule repeats."},
			{
				Name:        "recurrenceOverrides",
				Type:        "LocalDateTime[PatchObject]",
				PatchTarget: "EventTimeZoneRule",
				Doc:         "Changes to particular occurrences of the rule, keyed by the start of the occurrence.",
			},
			{Name: "names", Type: "String[Boolean]", Doc: "The names of the zone while this rule applies, such as \"GMT\", mapped to true."},
			{Name: "comments", Type: "String[]", Doc: "Comments about the rule."},
		},
	})

	s.AddObject(&Object{
		Name:       "EventTimeZone",
		Capability: CapabilityCalendars,
		Doc:        "EventTimeZone is a time zone the event defines itself, for a zone the IANA database does not have. RFC 8984 calls it TimeZone.",
		Fields: []*Field{
			atType("TimeZone"),
			{Name: "tzId", Type: "String", Doc: "The identifier of the zone within the event, which begins with \"/\"."},
			{Name: "updated", Type: "UTCDate", Doc: "When the definition was last updated."},
			{Name: "url", Type: "String", Doc: "Where the authoritative definition of the zone is published."},
			{Name: "validUntil", Type: "UTCDate", Doc: "The point beyond which the rules given here are not known to hold."},
			{Name: "aliases", Type: "String[Boolean]", Doc: "Other names for this zone, mapped to true."},
			{Name: "standard", Type: "EventTimeZoneRule[]", Doc: "The rules for standard time."},
			{Name: "daylight", Type: "EventTimeZoneRule[]", Doc: "The rules for daylight saving time."},
		},
	})
}
