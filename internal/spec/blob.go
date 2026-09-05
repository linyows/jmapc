package spec

// registerBlobExtension adds the blob management extension, RFC 9404. RFC 8620
// already lets a client upload and download blobs over HTTP, and copy one
// between accounts; this brings the rest into the API itself, where it can take
// part in a request alongside everything else.
//
// The types keep a "Blob" prefix, because the runtime already has a Blob (an
// open download) and a BlobInfo (what the upload endpoint returns), and neither
// is what this specification means by those words.
func registerBlobExtension(s *Spec) {
	registerBlobUpload(s)
	registerBlobGet(s)
	registerBlobLookup(s)
}

func registerBlobUpload(s *Spec) {
	s.AddObject(&Object{
		Name:       "BlobDataSource",
		Capability: CapabilityBlob,
		Doc: "BlobDataSource is one run of octets to put into a blob. Exactly one of its forms is used: " +
			"text, base64, or a range of a blob that already exists. RFC 9404 calls it DataSourceObject.",
		Fields: []*Field{
			{
				Name: "data:asText",
				Type: "String|null",
				Doc:  "The octets as text, which the server encodes as UTF-8.",
			},
			{
				Name: "data:asBase64",
				Type: "String|null",
				Doc:  "The octets as base64, for content that is not text.",
			},
			{
				Name: "blobId",
				Type: "Id",
				Doc:  "The id of a blob to take the octets from, so that a new blob can be assembled out of ones the server already holds.",
			},
			{
				Name: "offset",
				Type: "UnsignedInt|null",
				Doc:  "Where in that blob to start, defaulting to the beginning.",
			},
			{
				Name: "length",
				Type: "UnsignedInt|null",
				Doc:  "How many octets to take, defaulting to the rest of the blob.",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "BlobUploadObject",
		Capability: CapabilityBlob,
		Doc:        "BlobUploadObject is one blob to create, given as the sources whose octets make it up. RFC 9404 calls it UploadObject.",
		Fields: []*Field{
			{
				Name: "data",
				Type: "BlobDataSource[]",
				Doc:  "The sources of the blob's octets, concatenated in order.",
			},
			{
				Name:    "type",
				Type:    "String|null",
				Default: "null",
				Doc:     "The media type to record for the blob. The server may disregard it.",
			},
		},
	})

	s.AddObject(&Object{
		Name:       "BlobUploadResult",
		Capability: CapabilityBlob,
		Kind:       KindResponse,
		Doc:        "BlobUploadResult is what the server made of one blob it was asked to create.",
		Fields: []*Field{
			{Name: "id", Type: "Id", Doc: "The id of the blob, to refer to it by from here on."},
			{Name: "type", Type: "String|null", Doc: "The media type the server recorded for it."},
			{Name: "size", Type: "UnsignedInt", Doc: "The size of the blob in octets."},
		},
	})

	args := s.AddObject(&Object{
		Name:       "BlobUploadArguments",
		Capability: CapabilityBlob,
		Kind:       KindArguments,
		Doc:        "BlobUploadArguments holds the arguments of the Blob/upload method.",
		Fields: []*Field{
			accountIDField(),
			{
				Name:        "create",
				Type:        "Id[BlobUploadObject]",
				Doc:         "The blobs to create, keyed by creation id, which the rest of the request may refer to as \"#\" followed by that id.",
				CreationIDs: true,
			},
		},
	})
	resp := s.AddObject(&Object{
		Name:       "BlobUploadResponse",
		Capability: CapabilityBlob,
		Kind:       KindResponse,
		Doc:        "BlobUploadResponse holds the response to the Blob/upload method.",
		Fields: []*Field{
			accountIDField(),
			{
				Name: "created",
				Type: "Id[BlobUploadResult]|null",
				Doc:  "The blobs that were created, keyed by creation id.",
			},
			{
				Name: "notCreated",
				Type: "Id[SetError]|null",
				Doc:  "A map of creation id to the reason the blob could not be created.",
			},
		},
	})
	s.AddMethod(&Method{
		Name:       "Blob/upload",
		Capability: CapabilityBlob,
		Doc: "Creates blobs from data given in the request itself, or assembled out of blobs the server already holds. " +
			"Unlike the upload endpoint of RFC 8620, this takes part in a request, so a blob can be created and used by a later call without a round trip in between.",
		Arguments: args.Name,
		Response:  resp.Name,
		DataType:  "BlobData",
	})
}

func registerBlobGet(s *Spec) {
	s.AddObject(&Object{
		Name:       "BlobData",
		Capability: CapabilityBlob,
		Doc: "BlobData is the content of a blob as the API returns it, rather than as a download. " +
			"RFC 9404 calls it a blob; the name is qualified here because the runtime's Blob is an open download.",
		Fields: []*Field{
			{Name: "id", Type: "Id", Doc: "The id of the blob."},
			{
				Name: "data:asText",
				Type: "String|null",
				Doc:  "The octets as text, or null where they are not valid UTF-8.",
			},
			{
				Name: "data:asBase64",
				Type: "String",
				Doc:  "The octets as base64, which works whatever they hold.",
			},
			{
				Name:    "isEncodingProblem",
				Type:    "Boolean",
				Default: "false",
				Doc:     "Whether the octets asked for as text were not valid UTF-8.",
			},
			{
				Name:    "isTruncated",
				Type:    "Boolean",
				Default: "false",
				Doc:     "Whether the range asked for ran past the end of the blob.",
			},
			{
				Name: "size",
				Type: "UnsignedInt",
				Doc:  "The size of the whole blob in octets, whatever range was asked for.",
			},
		},
	})

	args := s.AddObject(&Object{
		Name:       "BlobGetArguments",
		Capability: CapabilityBlob,
		Kind:       KindArguments,
		Doc:        "BlobGetArguments holds the arguments of the Blob/get method.",
		Fields: []*Field{
			accountIDField(),
			{Name: "ids", Type: "Id[]", Doc: "The ids of the blobs to fetch."},
			{
				Name: "properties",
				Type: "String[]|null",
				Doc: "What to return for each blob: \"data:asText\", \"data:asBase64\", \"size\", " +
					"\"data\" to let the server pick whichever encoding fits, " +
					"or \"digest:\" followed by an algorithm the session says it supports.",
			},
			{
				Name:    "offset",
				Type:    "UnsignedInt|null",
				Default: "0",
				Doc:     "Where in each blob to start, so that a large blob can be read a piece at a time.",
			},
			{
				Name: "length",
				Type: "UnsignedInt|null",
				Doc:  "How many octets to return, defaulting to the rest of the blob.",
			},
		},
	})
	resp := s.AddObject(&Object{
		Name:       "BlobGetResponse",
		Capability: CapabilityBlob,
		Kind:       KindResponse,
		Doc:        "BlobGetResponse holds the response to the Blob/get method.",
		Fields: []*Field{
			accountIDField(),
			{Name: "list", Type: "BlobData[]", Doc: "The blobs that were found."},
			{Name: "notFound", Type: "Id[]", Doc: "The ids that were requested but do not exist."},
		},
	})
	s.AddMethod(&Method{
		Name:       "Blob/get",
		Capability: CapabilityBlob,
		Doc: "Returns the content of blobs through the API rather than over the download endpoint, " +
			"which suits something small enough to want alongside the rest of a response.",
		Arguments:          args.Name,
		Response:           resp.Name,
		DataType:           "BlobData",
		PropertiesArgument: "properties",
		ResultProperty:     "list",
	})
}

func registerBlobLookup(s *Spec) {
	s.AddObject(&Object{
		Name:       "BlobLookupInfo",
		Capability: CapabilityBlob,
		Doc: "BlobLookupInfo says which records refer to a blob. RFC 9404 calls it BlobInfo; " +
			"the name is qualified here because the runtime's BlobInfo describes an upload.",
		Fields: []*Field{
			{Name: "id", Type: "Id", Doc: "The id of the blob."},
			{
				Name: "matchedIds",
				Type: "String[Id[]]",
				Doc:  "The records that refer to the blob, keyed by data type name.",
			},
		},
	})

	args := s.AddObject(&Object{
		Name:       "BlobLookupArguments",
		Capability: CapabilityBlob,
		Kind:       KindArguments,
		Doc:        "BlobLookupArguments holds the arguments of the Blob/lookup method.",
		Fields: []*Field{
			accountIDField(),
			{
				Name: "typeNames",
				Type: "String[]",
				Doc:  "The data types to look in, such as \"Email\" or \"Mailbox\". The session says which ones the server supports.",
			},
			{Name: "ids", Type: "Id[]", Doc: "The ids of the blobs to look for."},
		},
	})
	resp := s.AddObject(&Object{
		Name:       "BlobLookupResponse",
		Capability: CapabilityBlob,
		Kind:       KindResponse,
		Doc:        "BlobLookupResponse holds the response to the Blob/lookup method.",
		Fields: []*Field{
			accountIDField(),
			{Name: "list", Type: "BlobLookupInfo[]", Doc: "What was found for each blob."},
			{Name: "notFound", Type: "Id[]", Doc: "The ids that were requested but do not exist."},
		},
	})
	s.AddMethod(&Method{
		Name:       "Blob/lookup",
		Capability: CapabilityBlob,
		Doc: "Reports which records refer to a blob, which is how a client finds out whether deleting something " +
			"would take an attachment with it, or which message an attachment came from.",
		Arguments:      args.Name,
		Response:       resp.Name,
		DataType:       "BlobLookupInfo",
		ResultProperty: "list",
	})
}
