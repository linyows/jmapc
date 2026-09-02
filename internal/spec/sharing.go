package spec

// registerSharing adds JMAP Sharing, RFC 9670. It answers two questions that
// come up once an account can be shared: who are the people and things an
// account may be shared with, and what has recently been shared with me.
func registerSharing(s *Spec) {
	registerPrincipal(s)
	registerShareNotification(s)
}

func registerPrincipal(s *Spec) {
	s.AddObject(&Object{
		Name:       "Principal",
		Capability: CapabilityPrincipals,
		Doc: "Principal is an entity that data can be shared with and that may own accounts: " +
			"a person, a group, or something bookable such as a room or a projector.",
		Fields: []*Field{
			{Name: "id", Type: "Id", ServerSet: true, Immutable: true, Doc: "The id of the principal."},
			{
				Name:      "type",
				Type:      "String",
				ServerSet: true,
				Immutable: true,
				Enum:      []string{"individual", "group", "resource", "location", "other"},
				Doc:       "What the principal is: \"individual\", \"group\", \"resource\", \"location\", or \"other\".",
			},
			{Name: "name", Type: "String", Doc: "The user-visible name of the principal."},
			{Name: "description", Type: "String|null", Doc: "A longer description of the principal."},
			{
				Name: "email",
				Type: "String|null",
				Doc:  "An email address for the principal, or null for one that has none.",
			},
			{
				Name: "timeZone",
				Type: "String|null",
				Doc:  "The time zone the principal is normally in, named as in the IANA Time Zone Database.",
			},
			{
				Name:      "capabilities",
				Type:      "String[Any]",
				ServerSet: true,
				Doc:       "What the principal supports, keyed by capability URI, with the details each capability defines.",
			},
			{
				Name:      "accounts",
				Type:      "Id[Account]|null",
				ServerSet: true,
				Doc:       "The accounts the principal shares with the authenticated user, keyed by account id, or null if none are visible.",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "PrincipalFilterCondition",
		Capability: CapabilityPrincipals,
		Doc:        "PrincipalFilterCondition is a condition a principal must satisfy to match a Principal/query.",
		Fields: []*Field{
			{Name: "accountIds", Type: "String[]", Doc: "Matches principals that share one of these accounts with the user."},
			{Name: "email", Type: "String", Doc: "Matches principals whose email address contains this text."},
			{Name: "name", Type: "String", Doc: "Matches principals whose name contains this text."},
			{Name: "text", Type: "String", Doc: "Matches principals where this text appears in the name, email, or description."},
			{
				Name: "type",
				Type: "String",
				Enum: []string{"individual", "group", "resource", "location", "other"},
				Doc:  "Matches principals of this type: \"individual\", \"group\", \"resource\", \"location\", or \"other\".",
			},
			{Name: "timeZone", Type: "String", Doc: "Matches principals in this time zone."},
		},
	})

	// RFC 9670 leaves the sortable properties of a principal to the server, so
	// nothing is recorded and a comparator is not checked against a list.
	s.RegisterStandard("Principal", CapabilityPrincipals, StandardMethods{
		Get: true, Changes: true, Set: true, Query: true, QueryChanges: true,
	})
}

func registerShareNotification(s *Spec) {
	s.AddObject(&Object{
		Name:       "ShareNotificationEntity",
		Capability: CapabilityPrincipals,
		Doc:        "ShareNotificationEntity identifies whoever changed what was shared. RFC 9670 calls it Entity.",
		Fields: []*Field{
			{Name: "name", Type: "String", Doc: "The name of whoever made the change."},
			{Name: "email", Type: "String|null", Doc: "Their email address."},
			{Name: "principalId", Type: "Id|null", Doc: "Their principal id, for someone the server knows as a principal."},
		},
	})

	s.AddObject(&Object{
		Name:       "ShareNotification",
		Capability: CapabilityPrincipals,
		Doc: "ShareNotification records that someone changed what is shared with the user. " +
			"Nothing else tells them: the object simply appears in, or disappears from, an account they can see.",
		Fields: []*Field{
			{Name: "id", Type: "Id", ServerSet: true, Immutable: true, Doc: "The id of the notification."},
			{Name: "created", Type: "UTCDate", ServerSet: true, Immutable: true, Doc: "When the change was made."},
			{
				Name:      "changedBy",
				Type:      "ShareNotificationEntity",
				ServerSet: true,
				Immutable: true,
				Doc:       "Who made the change.",
			},
			{
				Name:      "objectType",
				Type:      "String",
				ServerSet: true,
				Immutable: true,
				Doc:       "The type of the object whose sharing changed, such as \"Mailbox\" or \"Calendar\".",
			},
			{
				Name:      "objectAccountId",
				Type:      "Id",
				ServerSet: true,
				Immutable: true,
				Doc:       "The id of the account the object belongs to.",
			},
			{
				Name:      "objectId",
				Type:      "Id",
				ServerSet: true,
				Immutable: true,
				Doc:       "The id of the object itself.",
			},
			{
				Name:      "oldRights",
				Type:      "String[Boolean]|null",
				ServerSet: true,
				Immutable: true,
				Doc:       "What the user could do with the object before the change, or null if they could not see it at all.",
			},
			{
				Name:      "newRights",
				Type:      "String[Boolean]|null",
				ServerSet: true,
				Immutable: true,
				Doc:       "What the user can do with the object now, or null if it is no longer shared with them.",
			},
			{
				Name:      "name",
				Type:      "String",
				ServerSet: true,
				Immutable: true,
				Doc:       "The name the object had when the change was made, so that a notification about something since renamed still reads sensibly.",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "ShareNotificationFilterCondition",
		Capability: CapabilityPrincipals,
		Doc:        "ShareNotificationFilterCondition is a condition a notification must satisfy to match a ShareNotification/query.",
		Fields: []*Field{
			{Name: "after", Type: "UTCDate|null", Doc: "Matches notifications created at or after this time."},
			{Name: "before", Type: "UTCDate|null", Doc: "Matches notifications created before this time."},
			{Name: "objectType", Type: "String", Doc: "Matches notifications about objects of this type."},
			{Name: "objectAccountId", Type: "Id", Doc: "Matches notifications about objects in this account."},
		},
	})

	notification, _ := s.Object("ShareNotification")
	notification.Sort = []*SortProperty{
		{Name: "created", Doc: "Sorts by when the change was made."},
	}

	// Only destroy is useful here: a notification is created by the server, and
	// creating or updating one is rejected with a forbidden error.
	s.RegisterStandard("ShareNotification", CapabilityPrincipals, StandardMethods{
		Get: true, Changes: true, Set: true, Query: true, QueryChanges: true,
	})
}
