# Introduction

jmapc generates a client for JMAP, so this documentation starts with the
protocol: what JMAP is, and which of its parts shape the requests and the code
the rest of these pages describe.

## JMAP

[JMAP](https://jmap.io), the JSON Meta Application Protocol, is an open IETF
standard for reaching a user's mail, contacts and calendars. It is meant to
replace IMAP, CardDAV and CalDAV with one protocol, sent as JSON over HTTP.
[RFC 8620](https://www.rfc-editor.org/rfc/rfc8620) defines the core, and
further specifications add a data model each: mail in
[RFC 8621](https://www.rfc-editor.org/rfc/rfc8621), contacts in
[RFC 9610](https://www.rfc-editor.org/rfc/rfc9610), and more; the ones jmapc
supports are listed in [Coverage](coverage.md).

A client starts from the **session**, a JSON document the server publishes,
usually found at `/.well-known/jmap`. It lists the capabilities the server
supports, the accounts the user can reach, the limits a request must stay
within, and the URLs to send requests, upload and download blobs, and receive
push events at.

A **request** is a JSON object sent to the API URL. It holds a list of method
calls, and the server runs them in order and answers each in one response:

```json
{
  "using": ["urn:ietf:params:jmap:core", "urn:ietf:params:jmap:mail"],
  "methodCalls": [
    ["Email/query", {"accountId": "a1", "filter": {"inMailbox": "mbx1"}, "limit": 10}, "search"],
    ["Email/get", {"accountId": "a1",
                   "#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"},
                   "properties": ["subject", "from"]}, "fetch"]
  ]
}
```

A call is named after a data type and a method, as in `Email/query`. RFC 8620
defines six standard methods that every data type offers in the same shape
where it offers them: `/get`, `/set`, `/changes`, `/query`, `/queryChanges` and
`/copy`. The argument prefixed with `#` is a **back reference**: the server
takes the ids for `Email/get` from the result of the `Email/query` before it,
so the two calls cost one round trip, and the ids never come back to the client
in between.

Every data type also reports a **state**, a string that changes whenever its
records do. A client that keeps the state from its last read asks `/changes`
for what happened since, rather than reading everything again, and the server
can push a notice that a state has changed, so that the client knows when to
ask.

## Where jmapc comes in

jmapc takes the request object above as its input. You write the requests your
program sends as files, in JMAP itself, and jmapc checks each against the
specifications and generates a function that sends it and returns a response
typed by the properties the request asked for. The session, the account id, the
errors JMAP reports and the loops for push and paging are handled by the
generated code and the runtime it calls.

[Why jmapc](why.md) explains why jmapc compiles requests rather than offering a
library to build them with, and [Getting started](getting-started.md) takes a
module from nothing to a working call.

## Learning more about JMAP

[jmap.io](https://jmap.io) is the protocol's own site. It has
[Why JMAP?](https://jmap.io/why-jmap/), comparing JMAP with IMAP,
[the specifications](https://jmap.io/spec/),
[guides](https://jmap.io/guides/) for client and server developers, and
[a list of software](https://jmap.io/software/), servers and clients, that
implements it.
