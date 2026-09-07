// Package jmapc is the runtime for clients generated from JMAP requests.
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
// Write a request in requests/ListInboxEmails.jmap.json:
//
//	{
//	  "_doc": "ListInboxEmails returns the newest emails in one mailbox.",
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
//	  "_returns": "fetch"
//	}
//
// Generate the client:
//
//	go run github.com/linyows/jmapc/cmd/jmapc generate
//
// Then call it:
//
//	c := jmapc.New(jmapc.WellKnownURL("example.com"), jmapc.WithBearerToken(token))
//	res, err := client.ListInboxEmails(ctx, c, client.ListInboxEmailsParams{
//		MailboxID: inbox,
//		Limit:     25,
//	})
//
// # Authentication
//
// [WithBearerToken] sends a fixed token. [WithTokenSource] takes a function
// instead, for a token that expires: the client calls it when it has no token,
// shortly before the one it holds expires, and when a server answers 401, and
// requests arriving together share one call.
//
// # The session
//
// The session is the Session object of RFC 8620, Section 2: what a server
// answers with at its session resource, stating where a request is sent, what
// the server supports and what its limits are. It is not a login session and
// carries no credentials; authentication is the Authorization header, above.
//
// [Client.Session] fetches it when something first needs it and holds it
// afterwards, and fetches it again once a response reports a
// sessionState other than the one held, which is how an account added, a limit
// changed or an endpoint moved reaches a client that outlives it.
// [WithoutSessionRefresh] turns that off, and [Client.RefreshSession] fetches
// the session whether or not a response reported a change.
//
// # Splitting a large /get
//
// [WithSplitGets] sends a /get naming more ids than the server's
// maxObjectsInGet in several requests and joins the answers, at the cost of
// several round trips and of the records no longer arriving as one snapshot.
// Where the state differs between those requests, the joined response is
// returned together with a [StateChanged].
//
// # Blobs
//
// Attachments do not go through the API endpoint. [Client.Upload] and
// [Client.Download] exchange them over plain HTTP at the URLs the session
// advertises, and an upload larger than the server accepts fails before it is
// sent. Both stream, so an attachment larger than memory is never held in it.
//
// [DownloadOptions] From and Length fetch part of a blob, as an HTTP Range
// header, which is how a download interrupted part way is resumed.
//
// # Push
//
// [Client.EventSource] opens the server's push endpoint and reports which types
// in which accounts have changed. An event reports only that, not what changed,
// so the client follows up with a /changes call. A stream is a connection, not a
// subscription that outlives the network: treat an error from
// [EventStream.Next] as a signal to reconnect, passing the stream's
// [EventStream.LastEventID] so that nothing is missed in between.
//
// [Client.Watch] is that loop written out: it reconnects, catches up after
// every connection, and asks again while the server reports more.
// [WithResync] gives it a way back from a server that can no longer say what
// changed since the state the watch holds, which is what a server answers when
// a watch is resumed after a long pause.
//
// # Observability
//
// [WithObserver] takes an [Observer], which receives a report of what the
// client does and affects none of it: the JMAP calls of each request and their
// outcome, each HTTP request under it, and each delay for a slot or before a
// retry. [SlogObserver] writes those records to a [log/slog.Logger]. The hooks
// return the context used for the operation they cover, so a tracer can start
// a span in one and have the spans under it become its children.
//
// # Errors
//
// JMAP fails at two levels, and so does this package. A request-level failure,
// where the server rejected the request as a whole, is a [RequestError]. A
// method-level failure, where some calls ran and others did not, is a
// [MethodErrors]; the response is returned alongside it, because the calls that
// did run still have results worth reading.
//
// [IsTemporary] says whether a failure is one time may resolve, [IsRateLimited]
// whether the server is asking for fewer requests, and [RetryAfter] how long it
// asked the caller to wait. Together they are what a caller needs to decide
// between sending the request again later and reporting it as wrong, without
// reading status codes and error types itself.
package jmapc
