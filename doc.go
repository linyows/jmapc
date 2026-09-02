// Package jmapc is the runtime for clients generated from JMAP queries.
//
// JMAP is built around one idea: a request carries several method calls, and a
// call may refer to the result of an earlier one, so that a chain of dependent
// operations costs a single round trip. Fetching the newest messages in a
// mailbox is one request holding an Email/query and an Email/get, with the ids
// passing between them on the server.
//
// Most clients expose that through a builder, which means learning both JMAP
// and the builder. jmapc takes the other route. You write the JMAP request
// itself, in a file next to your code; the jmapc command checks it against the
// JMAP data model and writes a Go function that sends it and decodes the
// reply into types that hold exactly the properties you asked for. What you
// learn is JMAP.
//
// This package is what the generated code calls: it holds the client, the
// request and response types, the errors JMAP defines, and the Go form of the
// JMAP data types.
//
// # Getting started
//
// Write a query in queries/ListInboxEmails.jmap.json:
//
//	{
//	  "doc": "ListInboxEmails returns the newest emails in one mailbox.",
//	  "methodCalls": [
//	    ["Email/query", {
//	      "filter": {"inMailbox": "{{mailboxId}}"},
//	      "sort": [{"property": "receivedAt", "isAscending": false}],
//	      "limit": "{{limit}}"
//	    }, "search"],
//	    ["Email/get", {
//	      "#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"},
//	      "properties": ["id", "subject", "from", "receivedAt"]
//	    }, "fetch"]
//	  ],
//	  "returns": "fetch"
//	}
//
// Generate the client:
//
//	go run github.com/linyows/jmapc/cmd/jmapc generate
//
// Then call it:
//
//	c := jmapc.New(jmapc.WellKnownURL("example.com"), jmapc.WithBearerToken(token))
//	res, err := jmapq.ListInboxEmails(ctx, c, jmapq.ListInboxEmailsParams{
//		MailboxID: inbox,
//		Limit:     25,
//	})
//
// # Errors
//
// JMAP fails at two levels, and so does this package. A request-level failure,
// where the server rejected the request as a whole, is a [RequestError]. A
// method-level failure, where some calls ran and others did not, is a
// [MethodErrors]; the response is returned alongside it, because the calls that
// did run still have results worth reading.
package jmapc
