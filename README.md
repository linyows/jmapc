# jmapc

Write the JMAP request. Get a typed Go client.

`jmapc` is to JMAP what `sqlc` is to SQL: you write the query, it writes the
client.

## Why

JMAP is built around one idea. A request carries several method calls, and a
call may refer to the result of an earlier one, so a chain of dependent
operations costs a single round trip:

```json
["Email/query", {"filter": {"inMailbox": "mbx1"}}, "search"],
["Email/get",   {"#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"}}, "fetch"]
```

The ids never come back to the client. That is the whole point of the protocol,
and it is why a JMAP client does not look like a REST client, with a type per
resource and a method per path.

Most clients expose this through a builder, which means learning JMAP *and*
learning the builder. But the query is the part you care about; the client is
not. So write the query, and let the tool write the client.

What you get for it is the part that is tedious to do by hand and easy to get
wrong: the result references are checked against the methods they point at, the
arguments against the data model, the property names against the type, and the
response decodes into a struct holding exactly the properties you asked for.

## Install

```
go install github.com/linyows/jmapc/cmd/jmapc@latest
```

## Use

Write a query. The file name is the name of the function to generate.

```jsonc
// queries/ListInboxEmails.jmap.json
{
  "doc": "ListInboxEmails returns the newest emails in one mailbox.",

  "methodCalls": [
    // Find the ids of the matching emails.
    ["Email/query", {
      "filter": {"inMailbox": "{{mailboxId}}"},
      "sort": [{"property": "receivedAt", "isAscending": false}],
      "limit": "{{limit}}"
    }, "search"],

    // Fetch them in the same request, so the ids never make a round trip.
    ["Email/get", {
      "#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"},
      "properties": ["id", "subject", "from", "receivedAt"]
    }, "fetch"]
  ],

  "returns": "fetch"
}
```

Generate:

```
jmapc generate                 # or: go generate ./...
```

Use it:

```go
c := jmapc.New(jmapc.WellKnownURL("example.com"), jmapc.WithBearerToken(token))

res, err := jmapq.ListInboxEmails(ctx, c, jmapq.ListInboxEmailsParams{
	MailboxID: inbox,
	Limit:     25,
})
if err != nil {
	return err
}
for _, email := range res.List {
	fmt.Println(email.ReceivedAt, email.From[0].Email, *email.Subject)
}
```

`res.List` is `[]ListInboxEmailsEmail`, holding the four properties the query
asked for and nothing else. Ask for another property and the struct grows; ask
for one that does not exist and the build fails, with a suggestion.

### One request, several steps

The example below is what JMAP is for. Writing a message, submitting it, and
moving it out of Drafts are three operations that must not come apart, and here
they are one request:

```jsonc
// queries/SendEmail.jmap.json
{
  "methodCalls": [
    ["Email/set", {
      "create": {"draft": { /* ... */ }}
    }, "write"],

    ["EmailSubmission/set", {
      "create": {"send": {"emailId": "#draft", "identityId": "{{identityId}}"}},
      "onSuccessUpdateEmail": {
        "#send": {
          "mailboxIds/{{draftsMailboxId}}": null,
          "mailboxIds/{{sentMailboxId}}": true,
          "keywords/$draft": null
        }
      }
    }, "send"]
  ],
  "returns": "send"
}
```

`#draft` refers to the message the first call creates, before the server has
given it an id. The pointers in the patch are checked against `Email`, so
`mailboxIds` misspelled is a build failure, and both mailbox parameters come out
as `jmapc.ID` because that is what the pointer selects by.

## Writing a query

A query file is a JMAP Request object, exactly as
[RFC 8620](https://www.rfc-editor.org/rfc/rfc8620) defines it, plus three
members the generator reads and the server never sees.

| Member | |
|---|---|
| `methodCalls` | The calls, as `[name, arguments, callId]`. Required. |
| `using` | The capabilities the request declares. Optional: derived from the methods called. |
| `doc` | The generated function's documentation. Optional. |
| `returns` | The call whose response the function returns. Optional: without it, every response is returned. |

Comments are allowed, though JSON has none: a query is source code and deserves
annotating.

### Parameters

Write `{{name}}` where a value is left to the caller. Its Go type comes from the
argument it stands in for, so `{{limit}}` in `limit` is a `jmapc.UnsignedInt`
and `{{mailboxId}}` in `inMailbox` is a `jmapc.ID`. Use the same name twice and
it becomes one field, checked for agreeing on its type.

A map key may be a parameter too, which is how a `/set` names the record to
change:

```json
["Email/set", {"update": {"{{emailId}}": {"keywords/$seen": true}}}, "mark"]
```

The braces are used rather than a `$` prefix because JMAP keywords are
themselves written with one, as in `$seen`.

### Account ids

Leave `accountId` out and the generated function fills it in from the primary
account of the session, looking the session up once. Write `"{{accountId}}"` to
make it a parameter instead.

## What is checked

Everything below is a compile-time failure rather than a server round trip:

- the method exists, and is spelled the way the specification spells it
- every argument belongs to the method, with the type the method wants
- a back reference points at an *earlier* call, names that call's method
  correctly, and selects a value the target argument can accept
- filter conditions are checked against the type being queried, including the
  ones nested inside `AND`, `OR`, and `NOT` operators
- `properties` names properties the type has
- a `PatchObject` points at properties the record being patched actually has,
  and sets them to values of the right type
- `sort` names properties the type can actually be sorted by, and supplies the
  extra member a comparator like `hasKeyword` needs
- ids, dates, and integers are well formed
- the capabilities the request declares cover the methods it calls

A misspelling is met with a suggestion:

```
queries/BadQuery.jmap.json: methodCalls[0].arguments.filter.hasAttachmnt: EmailFilterCondition has no property "hasAttachmnt"
	did you mean "hasAttachment"?
queries/BadQuery.jmap.json: methodCalls[1].arguments.#ids.name: the referenced call is Email/query, but the reference names Email/get
	call "c0" invokes Email/query
```

`jmapc check` runs the checks without writing anything.

## Configuration

Flags, or a `jmapc.json` beside your module:

```json
{
  "queries": "queries",
  "out": "internal/jmapq",
  "package": "jmapq",
  "schemas": ["schema/notes.json"]
}
```

## Errors at run time

JMAP fails at two levels, and so does the runtime.

A **request-level** failure, where the server rejected the request whole, is a
`*jmapc.RequestError` carrying the problem type from RFC 8620 §3.6.1. The client
catches some of these before sending: a capability the session does not
advertise, or more calls than the server accepts.

A **method-level** failure is a `jmapc.MethodErrors`. JMAP runs the calls it
can, so the response comes back alongside the error, and each error names the
method and call id that failed rather than the bare `"error"` the wire format
carries.

## Blobs

Attachments do not travel through the API endpoint. They are uploaded and
downloaded over plain HTTP, at the URLs the session advertises, and the runtime
handles both:

```go
info, err := c.Upload(ctx, accountID, "application/pdf", file)
// info.BlobID now goes into an Email/set that attaches it.

blob, err := c.Download(ctx, accountID, part.BlobID, &jmapc.DownloadOptions{
	Name: *part.Name,
	Type: part.Type,
})
defer blob.Close()
```

An upload larger than the server said it accepts fails before it is sent.

## Push

`Client.EventSource` opens the server's push endpoint. An event says which types
in which accounts have moved on, not what changed, so the client follows up with
a `/changes` call:

```go
stream, err := c.EventSource(ctx, &jmapc.EventSourceOptions{
	Types: []string{"Email"},
	Ping:  30 * time.Second,
})
defer stream.Close()

for {
	change, err := stream.Next()
	if err != nil {
		break // reconnect, passing stream.LastEventID()
	}
	if state, ok := change.StateOf(accountID, "Email"); ok {
		// ... Email/changes since the state you hold
		_ = state
	}
}
```

A stream is a connection, not a subscription that outlives the network. An error
from `Next` means reconnect, and `LastEventID` is where to resume so nothing is
missed in between.

## Vendor extensions

JMAP is meant to be extended: a server advertises a capability URI of its own,
and with it come types and methods jmapc has never heard of. Describe them in a
schema file and queries against them are checked exactly as ones against `Email`
are — back references, property names, sort orders and all.

```json
{
  "capability": "urn:example:params:jmap:notes",
  "types": [
    {
      "name": "Note",
      "doc": "Note is a scrap of text the user keeps.",
      "properties": [
        {"name": "id", "type": "Id", "serverSet": true, "immutable": true, "doc": "The id of the note."},
        {"name": "title", "type": "String", "doc": "The note's title."}
      ],
      "methods": ["get", "changes", "set", "query"],
      "sort": [{"name": "createdAt", "doc": "Sorts by when the note was created."}]
    },
    {
      "name": "NoteFilterCondition",
      "doc": "NoteFilterCondition is a condition a note must satisfy to match a Note/query.",
      "properties": [{"name": "text", "type": "String", "doc": "Matches notes containing this text."}]
    }
  ]
}
```

Naming the six standard methods is enough to get them: their arguments and
responses follow the shapes RFC 8620 fixes. A method that does not follow one is
declared outright, with its arguments and response spelled out.

```
jmapc generate -schema schema/notes.json
```

Or list them in `jmapc.json` under `"schemas"`.

## Coverage

The data model covers:

- **[RFC 8620](https://www.rfc-editor.org/rfc/rfc8620), JMAP core** — the six
  standard methods (`/get`, `/changes`, `/set`, `/copy`, `/query`,
  `/queryChanges`), plus `Core/echo` and `Blob/copy`.
- **[RFC 8621](https://www.rfc-editor.org/rfc/rfc8621), JMAP for Mail** —
  `Mailbox`, `Thread`, `Email`, and `SearchSnippet`, with `Email/import` and
  `Email/parse`.
- **Submission** — `Identity` and `EmailSubmission`, including the
  `onSuccessUpdateEmail` side effect that files a message under Sent as part of
  sending it.
- **Vacation responses** — `VacationResponse`.

That is 28 methods in all. `internal/spec` is a plain Go declaration of the
data model, so adding a vendor type is a matter of registering it.

The runtime types in `types_gen.go` are generated from the same catalogue the
queries are checked against, so the two cannot drift apart.
