<p align="right">English | <a href="https://github.com/linyows/jmapc/blob/main/README.ja.md">日本語</a></p>

<p align="center">
  <br><br><br>
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://github.com/linyows/jmapc/blob/main/misc/jmapc-dark-bg.svg?raw=true">
    <img alt="jmapc" src="https://github.com/linyows/jmapc/blob/main/misc/jmapc.svg?raw=true" width="280">
  </picture>
  <br><br><br>
</p>

<p align="center">
  <strong>jmapc</strong> is a JMAP compiler: you write the query, it writes the client.
</p>

<p align="center">
  <a href="https://github.com/linyows/jmapc/actions/workflows/test.yml">
    <img alt="GitHub Workflow Status" src="https://img.shields.io/github/actions/workflow/status/linyows/jmapc/test.yml?branch=main&style=for-the-badge&labelColor=666666">
  </a>
  <a href="https://github.com/linyows/jmapc/releases">
    <img alt="GitHub Release" src="http://img.shields.io/github/release/linyows/jmapc.svg?style=for-the-badge&labelColor=666666&color=DDDDDD">
  </a>
  <a href="https://pkg.go.dev/github.com/linyows/jmapc">
    <img alt="Go Documentation" src="http://img.shields.io/badge/go-docs-blue.svg?style=for-the-badge&labelColor=666666&color=DDDDDD">
  </a>
  <a href="https://deepwiki.com/linyows/jmapc">
    <img alt="Deepwiki Documentation" src="http://img.shields.io/badge/deepwiki-docs-purple.svg?style=for-the-badge&labelColor=666666&color=DDDDDD">
  </a>
</p>

jmapc generates **type-safe code** from JMAP, in Go, TypeScript or Rust. Here's
how it works:

1. You write queries in JMAP.
1. You run jmapc to generate code with type-safe interfaces to those queries.
1. You write application code that calls the generated code.

## Motivation

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
not. So write the query, and let jmapc write the client — an approach it
takes from [sqlc](https://sqlc.dev).

Writing the query is all you do; jmapc takes on the parts that are tedious by
hand and easy to get wrong.

- **A mistake in the query stops the build.** Result references are checked
  against the methods they point at, arguments against the data model, and
  property names against the type, so a misspelling never reaches the server.
- **The response is type-safe.** It decodes into a struct holding exactly the
  properties the query asked for, with no `map[string]any` to walk.
- **No error goes unchecked.** JMAP fails at three levels — request, method,
  and record — and the last of them arrives as HTTP 200, which is the one
  people miss. Generated code looks at it.

## Install

The generator is a Go tool, and what it generates is Go, so record it in the
module that uses it:

```
go get -tool github.com/linyows/jmapc/cmd/jmapc
```

That pins a version in `go.mod`, and `go tool jmapc` runs it. Everyone who
builds the project — and CI — then generates with the same version, which
matters for a tool whose output is committed.

```go
//go:generate go tool jmapc generate
```

To put it on your PATH instead:

```
go install github.com/linyows/jmapc/cmd/jmapc@latest
```

Or take a binary from the [releases](https://github.com/linyows/jmapc/releases),
which is the way in for a TypeScript or Rust project, where there is no Go
toolchain to run `go tool` with.

## Use

Write a query. The file name is the name of the function to generate.

```json
{
  "_doc": "ListInboxEmails returns the newest emails in one mailbox.",

  "methodCalls": [
    ["Email/query", {
      "_comment": "Find the ids of the matching emails.",
      "filter": {"inMailbox": "{{mailboxId}}"},
      "sort": [{"property": "receivedAt", "isAscending": false}],
      "limit": "{{limit}}"
    }, "search"],

    ["Email/get", {
      "_comment": "Fetch them in the same request, so the ids never make a round trip.",
      "#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"},
      "properties": ["id", "subject", "from", "receivedAt"]
    }, "fetch"]
  ],

  "_returns": "fetch"
}
```

(`queries/ListInboxEmails.jmap.json`)

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

`bodyProperties` narrows the parts of a message the same way, reaching through
their sub-parts, and a property naming a header field is typed by the form it
asks for: `header:List-Id:asText` is a `*string`, `header:To:asAddresses` a
`[]jmapc.EmailAddress`.

### Generated names

The file name settles every name in the generated file, so it has to be a Go
identifier: letters, digits and underscores, not starting with a digit.
`ListInboxEmails.jmap.json` gives:

| Generated | Name |
| --- | --- |
| The function | `ListInboxEmails` |
| Its parameters, where the query leaves any open | `ListInboxEmailsParams` |
| A record whose properties the query narrows | `ListInboxEmailsEmail`, and `ListInboxEmailsEmailBodyPart` for a narrowed body part |
| The response to a call returning that record | `ListInboxEmailsEmailGetResponse` |
| The result, where `_returns` names no call | `ListInboxEmailsResult` |
| The file | `listinboxemails_gen.go` |

A call the query does not narrow answers with the shared type instead, so
`SendEmail` returns `*jmapc.EmailSubmissionSetResponse`. Two queries in one
package cannot take the same name, and a generated type whose name is already
taken gains a number: `ListInboxEmailsEmail2`.

TypeScript lowercases the first letter of the function and of the file —
`listInboxEmails` in `listInboxEmails.ts` — and keeps the type names above.

Rust writes the function and its module in snake_case — `list_inbox_emails` in
`list_inbox_emails.rs` — and keeps the type names too, except that an initialism
becomes a word, since that is how Rust spells one: a `UTCDate` is a `UtcDate`.
Properties are snake_case, with a serde rename wherever that is not the name on
the wire.

### One request, several steps

The example below is what JMAP is for. Writing a message, submitting it, and
moving it out of Drafts are three operations that must not come apart, and here
they are one request:

```json
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
  "_returns": "send"
}
```

`#draft` refers to the message the first call creates, before the server has
given it an id. The pointers in the patch are checked against `Email`, so
`mailboxIds` misspelled is a build failure, and both mailbox parameters come out
as `jmapc.ID` because that is what the pointer selects by.

[`example/queries`](example/queries) holds twenty-five of these, over mail,
contacts, calendars, sharing and filtering: searching, syncing from a known state, sending,
creating a contact card, moving one occurrence of a recurring meeting without
touching the rest of the series.

## TypeScript

The same queries generate TypeScript:

```
jmapc generate -lang typescript -out src/jmapq
```

```typescript
import { Client } from "./jmapq/client.js"
import { listInboxEmails } from "./jmapq/listInboxEmails.js"

const client = new Client("https://example.com/.well-known/jmap", { auth: token })

const res = await listInboxEmails(client, { mailboxId: inbox, limit: 25 })
for (const email of res.list) {
  console.log(email.receivedAt, email.from?.[0].email, email.subject)
}
```

The runtime comes with it — `client.ts` and `types.ts` are generated alongside
the queries — so the output has **no dependencies**. What it asks of the
platform is `fetch`.

TypeScript says some things more precisely than Go can. A nullable property is
a union rather than a pointer, so `subject` is `string | null`. A union of
shapes stays a union: a filter is `FilterOperator | EmailFilterCondition | null`
rather than the `any` Go falls back on. And the primitives that carry a format
rather than a shape are named aliases of `string`, so an `Id` and a
`TimeZoneId` cannot be swapped by accident.

## Rust

The same queries generate Rust:

```
jmapc generate -lang rust -out src/jmapq
```

```rust
use jmapq::list_inbox_emails::{list_inbox_emails, ListInboxEmailsParams};
use jmapq::Client;

let client = Client::with_bearer_token("https://example.com/.well-known/jmap", http, token);

let res = list_inbox_emails(&client, ListInboxEmailsParams {
    mailbox_id: inbox,
    limit: 25,
})
.await?;
for email in &res.list {
    println!("{} {:?}", email.received_at, email.subject);
}
```

The runtime comes with it — `client.rs`, `types.rs`, and the `mod.rs` that
declares them beside the queries — so `mod jmapq;` is the whole of what a crate
has to add. What the generated code asks for is **serde and serde_json**, and
nothing else. How the bytes travel is a `Transport` you write over whichever
HTTP client the program already has, so no HTTP stack, no TLS backend and no
async runtime arrives with it:

```rust
struct Http(reqwest::Client);

impl Transport for Http {
    async fn send(&self, req: HttpRequest) -> Result<HttpResponse, TransportError> {
        let mut out = self.0.request(req.method.parse()?, &req.url);
        for (name, value) in req.headers {
            out = out.header(name, value);
        }
        if let Some(body) = req.body {
            out = out.body(body);
        }
        let res = out.send().await?;
        Ok(HttpResponse {
            status: res.status().as_u16(),
            content_type: res
                .headers()
                .get("content-type")
                .and_then(|v| v.to_str().ok())
                .unwrap_or("")
                .to_string(),
            body: res.bytes().await?.to_vec(),
        })
    }
}
```

That is also where authentication a bearer token does not cover belongs — a
signature over the request, a token refreshed on expiry — since the transport is
the last thing to see a request before it goes.

Rust says what TypeScript says, in its own words. A nullable property is an
`Option`, so `subject` is `Option<String>`. A union of shapes stays a union: a
filter is `Option<FilterOperatorOrEmailFilterCondition>`, an untagged enum,
rather than the `any` Go falls back on. The primitives that carry a format
rather than a shape are named aliases of `String`, so an `Id` and a `TimeZoneId`
read apart in a signature. And a record derives `Default`, which is what makes a
type with fifty optional properties bearable to build: name the two that matter
and leave the rest.

What is generated is already laid out the way rustfmt lays things out, so
`cargo fmt` over the crate leaves it alone.

## Writing a query

A query file is a JMAP Request object, exactly as
[RFC 8620](https://www.rfc-editor.org/rfc/rfc8620) defines it, plus four
members the generator reads and the server never sees.

A member beginning with an underscore is one the generator reads; everything
else is the request as RFC 8620 defines it.

| Member | |
|---|---|
| `methodCalls` | The calls, as `[name, arguments, callId]`. Required. |
| `using` | The capabilities the request declares. Optional: derived from the methods called. |
| `_doc` | The generated function's documentation. Optional. |
| `_returns` | The call whose response the function returns. Optional: without it, every response is returned. |
| `_createdIds` | Carry the creation ids of an earlier request in, and this request's out. Optional; see below. |
| `_comment` | Why a call is there. Goes in that call's arguments; see below. |

A query file is plain JSON, so `jq` reads it and an editor understands it. To
say why a call is there, give its arguments a `_comment`:

```json
["Email/get", {
  "_comment": "Fetch them in the same request, so the ids never make a round trip.",
  "#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"}
}, "fetch"]
```

The generator lifts it into the generated code and leaves it out of the request,
which it must: RFC 8620 requires a server to reject an argument it does not
know.

```go
// Fetch them in the same request, so the ids never make a round trip.
{Name: "Email/get", CallID: "fetch", Args: map[string]any{
```

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

### Creation ids across requests

Referring to `#draft` within one request needs nothing: the server resolves it.
Carrying a reference from one request into the next needs the ids to travel,
which `_createdIds` asks for.

```json
{
  "_createdIds": true,
  "methodCalls": [
    ["Mailbox/set", {"create": {"box": {"name": "{{name}}"}}}, "make"],
    ["Email/set", {"update": {"{{emailId}}": {"mailboxIds/#box": true}}}, "file"]
  ]
}
```

The generated function takes them and reports them:

```go
res, err := jmapq.FileIntoNewMailbox(ctx, c, params, carried)
// res.CreatedIDs goes to the next request.
```

RFC 8620 has this for proxies, which split one request across servers and need
the references to still resolve. A query using it returns every response rather
than one, since the ids belong to the request rather than to any call in it.

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
- `properties` names properties the type has, and `bodyProperties` names
  properties an `EmailBodyPart` has
- a property naming a header field asks for a parsed form the specification
  defines, so `header:List-Id:asText` is a string and `header:To:asAddresses` a
  list of addresses
- a `PatchObject` points at properties the record being patched actually has,
  and sets them to values of the right type
- `sort` names properties the type can actually be sorted by, and supplies the
  extra member a comparator like `hasKeyword` needs
- a property whose specification fixes the values it may take is given one of
  them, whether it is a string or the keys of a set like a participant's `roles`
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

## Sending a query

A query is worth trying before there is any code that calls it, so `jmapc run`
sends one and prints what came back.

```
jmapc run ListInboxEmails -p mailboxId=mbx1 -p limit=25
```

A value is written the way its type says. A `String` or an `Id` is the text
itself, so nothing has to be quoted past the shell, and anything with a shape is
JSON. A value the type does not accept is refused before anything is sent:

```
jmapc: parameter limit: "soon" is not a whole number
```

The server comes from `-session`, which takes the session URL or the host to
find it under, and the credentials from `-token` or `-user`. Each falls back to an
environment variable — `$JMAP_SESSION_URL`, `$JMAP_TOKEN`, `$JMAP_USER` — which
is what keeps a token out of shell history. The account id a query leaves out is looked up in the session, exactly
as the generated function looks it up, and `-account` overrides it.

`-dry-run` prints the request rather than sending it — the same request the
generated function builds, which is the thing to look at when a server answers
something unexpected:

```
jmapc run MarkEmailRead -dry-run -p emailId=m1
{
  "using": [
    "urn:ietf:params:jmap:core",
    "urn:ietf:params:jmap:mail"
  ],
  "methodCalls": [
    [
      "Email/set",
      {
        "accountId": "ACCOUNT_ID",
        "update": {
          "m1": {
            "keywords/$seen": true
          }
        }
      },
      "mark"
    ]
  ]
}
```

The account id is the one value a dry run cannot know, since it comes from a
session it never fetches, so `ACCOUNT_ID` stands in for it and the run says so
on standard error.

A run reads the response the way generated code does: a `/set` that answers 200
with a refusal in it is an error here too, printed after the response that
carries it.

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

There is a third level, and it is the one that gets missed. A `/set` answers
**200 with no error in it** and lists the records it would not act on:

```json
["Email/set", {"notCreated": {"draft": {"type": "invalidProperties",
                                        "properties": ["subject"]}}}, "write"]
```

Read only the transport error and this is a success where nothing happened.
Generated code checks it, so a refused record is a `*jmapc.SetErrors`:

```go
res, err := jmapq.SendEmail(ctx, c, params)
if err != nil {
    var refused *jmapc.SetErrors
    if errors.As(err, &refused) {
        for _, f := range refused.Failures {
            log.Printf("%s: %v", f.Key, f.Err) // draft: invalidProperties [subject]
        }
    }
    return err
}
```

`res` is returned alongside the error, since the part of the request the server
did carry out still happened. Calls the query does not name in `_returns` are
checked too — naming one call should not stop the others from being looked at.

In TypeScript the same failure is a thrown `SetErrors`, with the response on
`err.result`. In Rust it is an `Error::Set`, and the response is asked for by
the type the function would have returned, with `err.result::<T>()`.

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

A server offering `urn:ietf:params:jmap:blob` can also create and read blobs
through the API, which the endpoints cannot: `Blob/upload` puts a blob in the
same request as the call that uses it, so the id never comes back to the client
in between.

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

This is the event source form of push, which suits a client that can hold a
connection open. The other form registers a URL for the server to post to, which
is what an app on a phone needs: see `RegisterPush` and `ConfirmPush` in
[`example/queries`](example/queries). A subscription is not live when it comes
back — the server pushes a code to the URL, and the client writes it back with a
`PushSubscription/set` before anything else is sent. `jmapc.PushVerification`
decodes what arrives.

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

## Working on jmapc

```
go test ./...        # everything, including the end-to-end tests
go generate ./...    # regenerate the runtime types and every example client
```

The example is generated three times, once per language, into `example/jmapq`,
`example/ts` and `example/rust/src/jmapq`. Go's tests cannot say whether the
other two compile, so CI runs `tsc --strict` over the TypeScript and
`cargo fmt --check` and `cargo test` over the Rust. Each of the two has a
hand-written check beside the generated code, exercising the runtime against a
stub: that the headers go out, that auth wins over them, that the session is
cached, and that a `/set` answering 200 with a refusal in it is still an error.

The generator is run from source here, not through `go tool`, because the
repository is where it lives.

The runtime types and the example client are committed, and a test compares
them against what the catalogue produces now, so a change to the data model
that was not regenerated fails the build rather than going unnoticed. CI runs
the same checks, plus gofmt, go vet, and govulncheck.

## Coverage

### Capabilities

JMAP is a family of specifications: a server advertises capability URIs, and
each brings its own types and methods. These are the ones
[IANA lists](https://www.iana.org/assignments/jmap/jmap.xhtml), and where jmapc
stands on each.

| Capability | Specification | Supported |
|---|---|---|
| `urn:ietf:params:jmap:core` | [RFC 8620](https://www.rfc-editor.org/rfc/rfc8620) | ✅ |
| `urn:ietf:params:jmap:mail` | [RFC 8621](https://www.rfc-editor.org/rfc/rfc8621) | ✅ |
| `urn:ietf:params:jmap:submission` | [RFC 8621](https://www.rfc-editor.org/rfc/rfc8621) | ✅ |
| `urn:ietf:params:jmap:vacationresponse` | [RFC 8621](https://www.rfc-editor.org/rfc/rfc8621) | ✅ |
| `urn:ietf:params:jmap:contacts` | [RFC 9610](https://www.rfc-editor.org/rfc/rfc9610) | ✅ |
| `urn:ietf:params:jmap:calendars` | [draft-ietf-jmap-calendars](https://datatracker.ietf.org/doc/draft-ietf-jmap-calendars/) | ✅ |
| `urn:ietf:params:jmap:principals:availability` | [draft-ietf-jmap-calendars](https://datatracker.ietf.org/doc/draft-ietf-jmap-calendars/) | ✅ |
| `urn:ietf:params:jmap:principals` | [RFC 9670](https://www.rfc-editor.org/rfc/rfc9670) | ✅ |
| `urn:ietf:params:jmap:principals:owner` | [RFC 9670](https://www.rfc-editor.org/rfc/rfc9670) | ✅ |
| `urn:ietf:params:jmap:smimeverify` | [RFC 9219](https://www.rfc-editor.org/rfc/rfc9219) | ✅ |
| `urn:ietf:params:jmap:blob` | [RFC 9404](https://www.rfc-editor.org/rfc/rfc9404) | ✅ |
| `urn:ietf:params:jmap:quota` | [RFC 9425](https://www.rfc-editor.org/rfc/rfc9425) | ✅ |
| `urn:ietf:params:jmap:sieve` | [RFC 9661](https://www.rfc-editor.org/rfc/rfc9661) | ✅ |
| `urn:ietf:params:jmap:mdn` | [RFC 9007](https://www.rfc-editor.org/rfc/rfc9007) | ✅ |
| `urn:ietf:params:jmap:webpush-vapid` | [RFC 9749](https://www.rfc-editor.org/rfc/rfc9749) | ✅ |

Two of these store objects from specifications of their own: a contact card is
a [JSContact](https://www.rfc-editor.org/rfc/rfc9553) Card, and a calendar event
is a [JSCalendar](https://www.rfc-editor.org/rfc/rfc8984) JSEvent. Both name
types that JMAP also names, and each other's too — there are three different
`Link` types between them. So those carry a prefix: `ContactEmailAddress` is an
address on a card, `EmailAddress` is one in a header field, and `EventLink` is a
resource attached to a meeting. Each type's documentation gives the name its
specification uses.

JSCalendar also brings time types JMAP does not have. An event's `start` is a
`LocalDateTime` with no zone, and its `duration` is an ISO 8601 `Duration`,
because "P1D" across a daylight saving change is not always 24 hours. Both are
checked in a query, so a `start` written with a `Z` on the end, or a duration
written as `90m`, fails to build.

Not every capability brings types of its own. S/MIME verification adds four
properties to `Email` and nothing else, so a query needs it without any method
name saying so. jmapc works out which capabilities the properties a query
touches belong to, and declares them: ask for `smimeStatus` and
`urn:ietf:params:jmap:smimeverify` appears in `using` on its own.

Some bring neither types nor methods, only something to tell the client. VAPID
is one: what it has to say is a key. Those are read from the session, and
`Session.Capability` reads any of them, including one jmapc has never heard of.

```go
vapid, err := session.WebPushVAPID()
// vapid.ApplicationServerKey goes to the push service when subscribing there.

var limits struct{ MaxSizeScript int `json:"maxSizeScript"` }
err = session.Accounts[accountID].Capability(jmapc.CapabilitySieve, &limits)
```

A capability that is not built in is not out of reach: describe its types in a
[schema file](#vendor-extensions) and queries against them are checked like any
other. That is the same mechanism a vendor extension uses, and the work is
declarative — no Go to write.

### Methods

81 methods, all of them checked and generated the same way.

| Type | Methods |
|---|---|
| `Mailbox` | `get` `changes` `set` `query` `queryChanges` |
| `Thread` | `get` `changes` |
| `Email` | `get` `changes` `set` `copy` `query` `queryChanges` `import` `parse` |
| `SearchSnippet` | `get` |
| `Identity` | `get` `changes` `set` |
| `EmailSubmission` | `get` `changes` `set` `query` `queryChanges` |
| `VacationResponse` | `get` `set` |
| `AddressBook` | `get` `changes` `set` |
| `ContactCard` | `get` `changes` `set` `copy` `query` `queryChanges` |
| `Calendar` | `get` `changes` `set` |
| `CalendarEvent` | `get` `changes` `set` `copy` `query` `queryChanges` `parse` |
| `CalendarEventNotification` | `get` `changes` `set` `query` `queryChanges` |
| `ParticipantIdentity` | `get` `changes` `set` |
| `Principal` | `get` `changes` `set` `query` `queryChanges` `getAvailability` |
| `ShareNotification` | `get` `changes` `set` `query` `queryChanges` |
| `Quota` | `get` `changes` `query` `queryChanges` |
| `SieveScript` | `get` `set` `query` `validate` |
| `MDN` | `send` `parse` |
| `Blob` | `copy` `upload` `get` `lookup` |
| `PushSubscription` | `get` `set` |
| `Core` | `echo` |

### What is not checked

One thing, and it is on purpose.

**Open sets are not checked, deliberately.** Where a specification fixes the
values a property takes, jmapc checks them. Where it leaves the set open — a
mailbox `role`, an email keyword, a `Content-Disposition` — it does not, because
rejecting a value the server would have accepted is worse than letting a typo
through.

### Generation

`internal/spec` is a plain Go declaration of the data model, and the runtime
types in `types_gen.go` are generated from the same catalogue the queries are
checked against, so the two cannot drift apart.
