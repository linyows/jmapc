package spec

// registerPushSubscription adds the push subscriptions of RFC 8620,
// Section 7.2: the other half of push, where the server posts to a URL the
// client registers rather than holding a connection open. It is what a client
// uses when it cannot keep a connection — an app on a phone, or anything that
// is not running most of the time.
//
// Neither method takes an accountId. A subscription belongs to the credentials
// that created it, not to an account, so these are the only methods in the
// catalogue with no account to name.
func registerPushSubscription(s *Spec) {
	s.AddObject(&Object{
		Name:       "PushSubscriptionKeys",
		Capability: CapabilityCore,
		Doc: "PushSubscriptionKeys are the client's encryption keys, which the server uses to encrypt " +
			"everything it pushes, as RFC 8291 describes. RFC 8620 leaves this object unnamed.",
		Fields: []*Field{
			{
				Name: "p256dh",
				Type: "String",
				Doc:  "The P-256 ECDH public key, in URL-safe base64.",
			},
			{
				Name: "auth",
				Type: "String",
				Doc:  "The authentication secret, in URL-safe base64.",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "PushSubscription",
		Capability: CapabilityCore,
		Doc: "PushSubscription is a URL the server posts to when something changes. " +
			"It is tied to the credentials that created it rather than to an account, and the server destroys it " +
			"when those credentials expire.",
		Fields: []*Field{
			{Name: "id", Type: "Id", ServerSet: true, Immutable: true, Doc: "The id of the subscription."},
			{
				Name:      "deviceClientId",
				Type:      "String",
				Immutable: true,
				Doc: "An id identifying the client and the device it runs on, so that a client which has lost its " +
					"local state can still find the subscriptions it made. It must not carry an unobfuscated device id: " +
					"the recommendation is a hash of the device's identifier together with the vendor's own.",
			},
			{
				Name:      "url",
				Type:      "String",
				Immutable: true,
				Doc: "The absolute URL the server posts to, which must begin with \"https://\". " +
					"A /get never returns it, since it may be private to one device.",
			},
			{
				Name:      "keys",
				Type:      "PushSubscriptionKeys|null",
				Immutable: true,
				Doc: "The keys to encrypt pushed data with. A /get never returns them, for the same reason it " +
					"withholds the url.",
			},
			{
				Name: "verificationCode",
				Type: "String|null",
				Doc: "The code proving the client controls the URL. It must be null when the subscription is created; " +
					"the server then pushes a code to the URL, and the client writes it back here. " +
					"Until that happens the server makes no further requests to the URL, which is what stops a " +
					"subscription being used to attack a third party.",
			},
			{
				Name: "expires",
				Type: "UTCDate|null",
				Doc: "When the subscription lapses. The server may impose one where the client gives none, or shorten " +
					"the one it gives, and a client extends the lifetime by writing a later time here.",
			},
			{
				Name: "types",
				Type: "String[]|null",
				Doc: "The data types worth being told about, named as the TypeState object names them. " +
					"Null means every type.",
			},
		},
	})

	// Not the standard /get: there is no accountId, because a subscription
	// belongs to the credentials rather than to an account, and no state,
	// because there is nothing to compare it against.
	getArgs := s.AddObject(&Object{
		Name:       "PushSubscriptionGetArguments",
		Capability: CapabilityCore,
		Kind:       KindArguments,
		Doc:        "PushSubscriptionGetArguments holds the arguments of the PushSubscription/get method.",
		Fields: []*Field{
			{
				Name: "ids",
				Type: "Id[]|null",
				Doc:  "The ids of the subscriptions to fetch, or null for all of them.",
			},
			{
				Name: "properties",
				Type: "String[]|null",
				Doc: "The properties to return. Asking for url or keys is refused with a forbidden error, " +
					"and leaving this out returns everything except those two.",
			},
		},
	})
	getResp := s.AddObject(&Object{
		Name:       "PushSubscriptionGetResponse",
		Capability: CapabilityCore,
		Kind:       KindResponse,
		Doc:        "PushSubscriptionGetResponse holds the response to the PushSubscription/get method.",
		Fields: []*Field{
			{
				Name: "list",
				Type: "PushSubscription[]",
				Doc:  "The subscriptions that were found, which are only those the current credentials created.",
			},
			{Name: "notFound", Type: "Id[]", Doc: "The ids that were requested but do not exist."},
		},
	})
	s.AddMethod(&Method{
		Name:       "PushSubscription/get",
		Capability: CapabilityCore,
		Doc: "Fetches the push subscriptions the current credentials created. " +
			"It never returns the url or the keys, which may be private to one device.",
		Arguments:          getArgs.Name,
		Response:           getResp.Name,
		DataType:           "PushSubscription",
		PropertiesArgument: "properties",
		ResultProperty:     "list",
	})

	// Not the standard /set either: no accountId, and no ifInState or state
	// strings, since there is no shared state to be out of step with.
	setArgs := s.AddObject(&Object{
		Name:       "PushSubscriptionSetArguments",
		Capability: CapabilityCore,
		Kind:       KindArguments,
		Doc:        "PushSubscriptionSetArguments holds the arguments of the PushSubscription/set method.",
		Fields: []*Field{
			{
				Name:        "create",
				Type:        "Id[PushSubscription]|null",
				Doc:         "The subscriptions to create, keyed by creation id.",
				CreationIDs: true,
			},
			{
				Name:        "update",
				Type:        "Id[PatchObject]|null",
				PatchTarget: "PushSubscription",
				Doc: "Patches to apply, keyed by subscription id. This is how the verification code is written back " +
					"and how the expiry is extended; the url and keys cannot be changed, only replaced by destroying " +
					"the subscription and creating another.",
			},
			{
				Name: "destroy",
				Type: "Id[]|null",
				Doc:  "The ids of the subscriptions to destroy.",
			},
		},
	})
	setResp := s.AddObject(&Object{
		Name:       "PushSubscriptionSetResponse",
		Capability: CapabilityCore,
		Kind:       KindResponse,
		Doc:        "PushSubscriptionSetResponse holds the response to the PushSubscription/set method.",
		Fields: []*Field{
			{
				Name: "created",
				Type: "Id[PushSubscription|null]|null",
				Doc:  "A map of creation id to the properties the server assigned to each subscription it created.",
			},
			{
				Name: "updated",
				Type: "Id[PushSubscription|null]|null",
				Doc:  "A map of subscription id to any properties the server changed beyond those the patch set.",
			},
			{Name: "destroyed", Type: "Id[]|null", Doc: "The ids of the subscriptions that were destroyed."},
			{
				Name: "notCreated",
				Type: "Id[SetError]|null",
				Doc:  "A map of creation id to the reason the subscription could not be created.",
			},
			{
				Name: "notUpdated",
				Type: "Id[SetError]|null",
				Doc: "A map of subscription id to the reason it could not be updated. " +
					"A wrong verification code is refused here, as an invalidProperties error.",
			},
			{
				Name: "notDestroyed",
				Type: "Id[SetError]|null",
				Doc:  "A map of subscription id to the reason it could not be destroyed.",
			},
		},
	})
	s.AddMethod(&Method{
		Name:       "PushSubscription/set",
		Capability: CapabilityCore,
		Doc: "Creates, updates, and destroys push subscriptions. Creating one starts the verification exchange: " +
			"the server pushes a code to the URL, and until the client writes that code back the server sends nothing else.",
		Arguments: setArgs.Name,
		Response:  setResp.Name,
		DataType:  "PushSubscription",
	})
}
