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
{
  "using": [
    "urn:ietf:params:jmap:core",
    "urn:ietf:params:jmap:mail"
  ],
  "methodCalls": [
    [
      "Email/query",
      {
        "filter": {
          "inMailbox": "mbx1"
        }
      },
      "search"
    ],
    [
      "Email/get",
      {
        "#ids": {
          "resultOf": "search",
          "name": "Email/query",
          "path": "/ids"
        }
      },
      "fetch"
    ]
  ]
}
```

The ids never come back to the client, which is why a JMAP client does not look
like a REST client, with a type per resource and a method per path.

Most clients expose this through a builder, which means learning JMAP *and*
learning the builder. But the query is the part you care about; the client is
not. So write the query, and let jmapc write the client — an approach it
takes from [sqlc](https://sqlc.dev).

Writing the query is all you do; jmapc takes on the parts that are tedious by
hand and easy to get wrong.

- Linting the query
- A type-safe response
- Exhaustive error handling

Result references are checked against the methods they point at, arguments
against the data model, and property names against the type, so a misspelling
fails the build before it ever reaches the server. The response decodes into a
struct holding exactly the properties the query asked for, with no
`map[string]any` to walk, so it is type-safe. JMAP fails at three levels —
request, method, and record. The record level arrives as HTTP 200, which is
easy to miss, but generated code checks it.

## Install

You generate the client with `go generate`, so record it in the module that
uses it:

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

The file name is the name of the function to generate.

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
      "_comment": "Fetch the message from its id.",
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
| The function that follows changes, where the query is watched | `SyncEmailsWatch` |
| The walk over the parts of an answer, where the query is paged | `SearchEmailsPages` |
| The file | `listinboxemails_gen.go` |

A call the query does not narrow answers with the shared type instead, so
`SendEmail` returns `*jmapc.EmailSubmissionSetResponse`. Two queries in one
package cannot take the same name, and a generated type whose name is already
taken gains a number: `ListInboxEmailsEmail2`.

Two calls of one query that read the same type through the same method and ask
for the same properties describe one record, so they share one type. Two names
for one shape would make a caller convert between them to hand a record from
one call to a function written for the other.

A query with no open parameters takes no `Params` argument at all —
`MailQuota(ctx, c)`, not `MailQuota(ctx, c, MailQuotaParams{})` — so adding
the first `{{param}}` to a query already in use changes the generated
function's arity and breaks every call site. That is a deliberate trade for
the common case of a query with no parameters reading like a plain function
call, not an oversight.

TypeScript lowercases the first letter of the function and of the file —
`listInboxEmails` in `listInboxEmails.ts` — and keeps the type names above.

Rust writes the function and its module in snake_case — `list_inbox_emails` in
`list_inbox_emails.rs` — and keeps the type names too, except that an initialism
becomes a word, since that is how Rust spells one: a `UTCDate` is a `UtcDate`.
Properties are snake_case, with a serde rename wherever that is not the name on
the wire.

### Examples

[`example/queries`](example/queries) holds twenty-five of these, over mail,
contacts, calendars, sharing and filtering: searching, syncing from a known state, sending,
creating a contact card, moving one occurrence of a recurring meeting without
touching the rest of the series.

## TypeScript

The same queries generate a TypeScript client:

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

The same queries generate a Rust client:

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

That is also where authentication that a bearer token does not cover belongs —
a signature over the request, a token refreshed on expiry — since the transport
is the last thing to see a request before it goes.

A nullable property is an `Option`, so `subject` is `Option<String>`. A union of
shapes stays a union: a filter is `Option<FilterOperatorOrEmailFilterCondition>`,
an untagged enum, rather than the `any` Go falls back on. The primitives that
carry a format rather than a shape are named aliases of `String`, so an `Id` and
a `TimeZoneId` read apart in a signature. And a record derives `Default`, which
is what makes a type with fifty optional properties bearable to build: name the
two that matter and leave the rest.

What is generated is already laid out the way rustfmt lays things out, so
`cargo fmt` over the crate leaves it alone.

## Writing a query

A query file is a JMAP Request object, exactly as
[RFC 8620](https://www.rfc-editor.org/rfc/rfc8620) defines it, plus four
members jmapc reads and the JMAP server never sees.

A member beginning with an underscore is one the generator reads; everything
else is the request as RFC 8620 defines it.

| Member | |
|---|---|
| `methodCalls` | The calls, as `[name, arguments, callId]`. Required. |
| `using` | The capabilities the request declares. Optional: derived from the methods called. |
| `_doc` | The generated function's documentation. Optional. |
| `_returns` | The call whose response the function returns. Optional: without it, every response is returned. |
| `_createdIds` | Carry the creation ids of an earlier request in, and this request's out. Optional; see below. |
| `_watches` | The call a generated client follows the changes of, so that it catches up whenever the server says there is something to catch up on. Optional; see [Push](#push). |
| `_pages` | The call a generated walk advances, so that what one request answers with part of can be read the whole way through. Optional; see [Walking an answer that does not fit in one request](#walking-an-answer-that-does-not-fit-in-one-request). |
| `_comment` | Why a call is there. Goes in that call's arguments; see below. |

A query file is plain JSON, so `jq` reads it and an editor understands it. To
say why a call is there, give its arguments a `_comment`.

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

#### An argument the caller may leave out

Write `{{name?}}` where the caller may leave the argument out altogether:

```json
["Email/changes", {"sinceState": "{{sinceState}}", "maxChanges": "{{maxChanges?}}"}, "changes"]
```

Nothing supplied means the argument is not in the request, which is not the
same as sending null. RFC 8620 makes the difference twice over: `maxChanges`
absent is no cap, while `maxChanges: 0` asks for nothing at all; and in a
PatchObject a pointer set to null removes the property while a pointer that is
not there leaves it alone. Without this, each argument that is only sometimes
sent would need a query of its own, and *n* of them would need 2ⁿ.

The caller says "left out" the way the language already does. Go takes a
pointer, or nothing where the type has a nil of its own:

```go
limit := jmapc.UnsignedInt(25)
jmapq.FindPeople(ctx, c, jmapq.FindPeopleParams{Phrase: "ada", Limit: &limit})
jmapq.FindPeople(ctx, c, jmapq.FindPeopleParams{Phrase: "ada"}) // no limit argument
```

TypeScript makes the member optional (`limit?: number`), Rust wraps it in an
`Option`, and `jmapc run` leaves the argument out when no `-p` names it.

Only a whole argument of a method call may be left out, and only where the
parameter standing for it is used nowhere else, so that leaving it out has one
meaning: this member is not there. A parameter inside a filter or an array is
part of a larger value, and dropping it would leave a question the query does
not answer, since an empty `AND` and no filter at all are different requests.
For a filter whose shape varies, hand the whole filter over as one parameter
instead.

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

## Verification

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
  and sets them to values of the right type, its keys written the way RFC 8620
  writes them: the leading `/` of the pointer is implicit, so a keyword is set
  at `keywords/$seen` rather than at `/keywords/$seen`
- `sort` names properties the type can actually be sorted by, and supplies the
  extra member a comparator like `hasKeyword` needs
- a property whose specification fixes the values it may take is given one of
  them, whether it is a string or the keys of a set like a participant's `roles`
- ids, dates, and integers are well formed
- the capabilities the request declares cover the methods it calls
- a watched call is one that reports what changed since a state, and the state
  it goes on from is left to the loop rather than written into the query
- a paged call is one that answers with part of a longer answer and says where
  the rest is, and where the next request starts is left to the walk

A misspelling is met with a suggestion:

```
queries/BadQuery.jmap.json: methodCalls[0].arguments.filter.hasAttachmnt: EmailFilterCondition has no property "hasAttachmnt"
	did you mean "hasAttachment"?
queries/BadQuery.jmap.json: methodCalls[1].arguments.#ids.name: the referenced call is Email/query, but the reference names Email/get
	call "c0" invokes Email/query
```

Two queries that differ only in what they call their parameters and their
calls are one query written twice, and jmapc says so rather than failing:

```
jmapc: ListArchiveEmails, ListInboxEmails are the same query under different names; one of them would do for all of them
```

Both are generated all the same, since a project may want two names for one
request. It is worth knowing about because each name brings a set of generated
types with it.

`jmapc check` runs the checks without writing anything.

### What only the server knows

Everything above is what the specifications say. What they leave to the server —
which capabilities it has, which accounts it holds, how much it will do in one
request — a build cannot know, and a query that is right about JMAP and wrong
about the server in front of it fails at run time. `-session` asks:

```
jmapc check -session jmap.example.com -token $JMAP_TOKEN
checked 25 queries against https://jmap.example.com/api/, as someone@example.com
```

What it reports:

- a capability the request declares and the server does not advertise
- an account the query names that the session does not hold, an account the
  session cannot fill in because it has no primary account for the capability,
  and an account that does not support what the call needs
- more calls than `maxCallsInRequest`, more records than `maxObjectsInGet`, more
  changes than `maxObjectsInSet`, a request already larger than `maxSizeRequest`
  before its parameters are filled in
- a `collation` the server does not compare strings with

What the query leaves to its caller is left alone: a parameter standing for a
list of ids may be any length, and guessing at it would report a query that is
fine.

The session URL is the one thing not read from the environment — `-token` and
`-user` fall back to `$JMAP_TOKEN` and `$JMAP_USER` — because a check that
reaches the network should say so on the command line rather than because of
what happens to be set around it.

## Editor support

The checks above run when jmapc does. Most of them can run while the query is
being typed instead, because they are checks on the file itself, and a JSON file
that names a schema is one an editor already knows how to check and complete.

```
jmapc schema -out jmapc.schema.json
```

That writes a JSON Schema for the catalogue, vendor extensions and all. Point a
query file at it:

```json
{
  "$schema": "../jmapc.schema.json",
  "methodCalls": [["Email/query", {"filter": {"inMailbox": "{{mailboxId}}"}}, "search"]]
}
```

or point the editor at every query at once, which in VS Code is:

```json
{
  "json.schemas": [
    {"fileMatch": ["*.jmap.json"], "url": "./jmapc.schema.json"}
  ]
}
```

Either way the editor completes a method name, offers the arguments that method
takes and the properties the type has, and underlines a misspelling where it was
written. A filter nested inside an `AND` is checked like one outside it, a
comparator offers the properties the type can actually be sorted by, and a
`{{parameter}}` is accepted anywhere a value goes.

What a schema cannot say is the part that depends on another call: that a back
reference names an earlier call and selects a value the argument accepts. That
stays jmapc's to check, which is why the editor is a first pass rather than a
replacement for the build.

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

A **request-level** failure, where the server rejected the request as a whole,
is a `*jmapc.RequestError` carrying the problem type from RFC 8620 §3.6.1. The client
catches some of these before sending: a capability the session does not
advertise, or more calls than the server accepts.

A **method-level** failure is a `jmapc.MethodErrors`. JMAP runs the calls it
can, so the response comes back alongside the error, and each error names the
method and call id that failed rather than the bare `"error"` the wire format
carries.

A generated function hands back the same thing: the calls the server answered
are decoded, the ones it would not run are left at their zero value, and the
result comes back with the error. A chained query fails this way as a matter
of course — the call that feeds another succeeds, the one fed by it cannot
resolve its reference — and what the first call answered is usually what says
why:

```go
res, err := jmapq.DestroyThread(ctx, c, params)
if err != nil {
    if len(res.ThreadGet.NotFound) > 0 {
        return fmt.Errorf("no such thread: %s", res.ThreadGet.NotFound[0])
    }
    return err
}
```

The exception is a query naming one call in `_returns`: that call is the whole
of the answer, so if it is the one that failed, there is nothing to hand back
and the result is nil.

TypeScript throws rather than returning, so what it read rides on the error.
`MethodErrors` carries the response it came from, and `result` holds as much of
what the query returns as the server answered. Read it as a `Partial`, since a
call the server would not run is not in it at all:

```ts
try {
  await destroyThread(client, params)
} catch (e) {
  if (e instanceof MethodErrors) {
    const partial = e.result as Partial<DestroyThreadResult>
    if (partial.threadGet?.notFound?.length) {
      throw new Error(`no such thread: ${partial.threadGet.notFound[0]}`)
    }
  }
  throw e
}
```

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

### Retries

`WithRetry` retries when the server answers with HTTP 429 or 503.

```go
c := jmapc.New(url, jmapc.WithBearerToken(token), jmapc.WithRetry(3))
```

The argument is how many attempts to make. The wait is what the server asked
for in `Retry-After`, or one that doubles from a fifth of a second to half a
minute where it asked for nothing in particular.

## Testing

Testing the code you write around a generated client means answering a request
that carries several method calls, some of which refer to the results of the
others. A stub written by hand for one test either ignores that — and stops
resembling a server — or grows into this:

```go
srv := jmaptest.New(t)
srv.Reply("Email/query", jmapc.EmailQueryResponse{
	AccountID: jmaptest.AccountID,
	IDs:       []jmapc.ID{"m1", "m2"},
})
srv.Handle("Email/get", func(c *jmaptest.Call) (any, error) {
	// The ids are the ones the query call answered with: the back reference
	// has already been resolved, the way a server resolves it.
	return emailsFor(c.IDs()), nil
})

res, err := jmapq.ListInboxEmails(ctx, srv.Client(), params)
```

What it takes off the test:

- **The back references.** They are resolved as RFC 8620 says, including the
  `*` that maps a path over a list, so a chained query reaches the handlers
  with real values in it.
- **The checking.** The request is held to the data model the way the build
  holds a query to it, so a call with an argument no method has fails the test
  rather than passing quietly. `jmaptest.WithoutChecks()` is the way out, for a
  method jmapc has never heard of.
- **The failures.** `srv.Fail` for a method-level error, `srv.FailRequest` for a
  request the server would not look at, and a `/set` response listing what it
  refused for the failure that answers 200.
- **What was asked.** `srv.Call("Email/query")` is the last call to a method,
  `srv.Calls()` all of them, and `srv.Requests()` how many requests they took —
  which is how to check that calls travelled together rather than one at a time.
- **The push.** `srv.Push` sends a state change to a client watching, which is
  what a watching query's loop waits for.

What it does not do is store anything. It is a server to test a client against
rather than an implementation of JMAP: nothing a `/set` creates comes back from
a later `/get` unless the test says it does.

A client is rarely converted all at once, and a half-converted one has two
halves to answer in the same test: the generated half, which reaches the
server through `srv.Client()`, and the half still written by hand, which posts
to paths of its own. `srv.Mux()` is where those paths go, and `srv.BaseURL()`
is what the other half is pointed at:

```go
srv := jmaptest.New(t)
srv.Mux().HandleFunc("/jmap", myOldAPIHandler)
srv.Mux().HandleFunc("/jmap/session", myOldSessionHandler)

old := myOldClient(srv.BaseURL())
```

So jmaptest is worth adopting on the first method converted rather than the
last.

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

## Walking an answer that does not fit in one request

A JMAP answer is often only part of an answer. A `/query` returns just the
window of results the caller asked for, and says where that window sits in the
full result. A `/changes` returns only as many changes as the server cares to,
and says whether there is more.

Either way, getting the rest means sending another request, and what goes in it
comes from the last answer: `position` for a `/query`, `sinceState` for a
`/changes`. Naming a call in `_pages` generates that loop — call it a walk —
so nobody has to write it by hand.

The call named in `_pages` is the one the walk resends on each step. Where the
next request should start — `position` for a `/query`, `sinceState` for a
`/changes` — is the walk's to manage rather than the caller's, so it is written
as a parameter in the query:

```json
{
  "_pages": "search",

  "methodCalls": [
    ["Email/query", {"filter": {"text": "{{phrase}}"}, "position": "{{position}}",
                     "limit": 50, "calculateTotal": true}, "search"],
    ["Email/get", {"#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"}}, "fetch"]
  ]
}
```

Go gets an iterator. Advancing it sends the next request behind the scenes:

```go
for page, err := range jmapq.SearchEmailsPages(ctx, c, params) {
	if err != nil {
		return err
	}
	for _, email := range page.EmailGet.List {
		fmt.Println(*email.Subject)
	}
}
```

TypeScript gets an async generator that does the same, one step at a time. A
failure throws as it does from the query itself:

```ts
for await (const page of searchEmailsPages(client, params)) {
  for (const email of page.emailGet.list) console.log(email.subject)
}
```

Rust gets a value that remembers where it is, and advances the same way. A
stream would mean a crate to define one, and what the generated code asks for
is serde and nothing else:

```rust
let mut pages = search_emails_pages(params);
while let Some(page) = pages.next(&client).await? {
    for email in &page.email_get.list {
        println!("{:?}", email.subject);
    }
}
```

When a walk stops depends on what it is walking.

A `/query` walk starts from the `position` the parameters carry, so it can pick
up where a previous walk left off. A window with nothing in it is not handed
back — it ends the walk instead — so everything the walk yields holds
something. Where the call asked for the total, the walk also stops asking once
it would be past that total.

A `/changes` walk hands back even an answer saying nothing changed, because
that answer still carries the `sinceState` to go on from. It ends only when the
server says there is no more to report.

A watching query already asks again on its own while the server says there is
more, so `_watches` and `_pages` are never written on the same query.

## Push

An event says which types in which accounts have moved on, not what changed. So
a client that wants the changes writes a loop: connect, ask what happened since
the state you hold, apply it, wait to be told again. That loop is the same every
time and every part of it is somewhere to be wrong, so a query can ask for it.

`_watches` names the call the loop reads the state from, which has to be one
that reports what changed since a state, as `Email/changes` does:

```json
{
  "_watches": "changes",

  "methodCalls": [
    ["Email/changes", {"sinceState": "{{sinceState}}", "maxChanges": 128}, "changes"],
    ["Email/get", {"#ids": {"resultOf": "changes", "name": "Email/changes", "path": "/created"}}, "created"]
  ]
}
```

`SyncEmails` is generated as it would be anyway, and `SyncEmailsWatch` alongside
it:

```go
err := jmapq.SyncEmailsWatch(ctx, c, jmapq.SyncEmailsParams{SinceState: state},
	func(ctx context.Context, res *jmapq.SyncEmailsResult) error {
		for _, email := range res.EmailGet.List {
			fmt.Println("new:", *email.Subject)
		}
		state = res.EmailChanges.NewState // keep it, and start there next time
		return nil
	})
```

It starts from the state the parameters carry, and goes on from the state each
answer reports. What it takes off the caller:

- **A stream is a connection, not a subscription.** When it drops, another is
  opened, resuming from the last event delivered, with a wait that doubles from
  a second to half a minute while the server is unreachable.
- **What changed while there was no connection was pushed to nobody**, so every
  connection is followed by a catch-up.
- **A server answers a `/changes` with as much as it cares to** and says
  `hasMoreChanges`, so the loop asks again until it stops saying it.
- **An event about another account, another type, or a state the loop has
  already reached** is not worth a request, and the last of those is the common
  one: a catch-up makes the server push what it has just been told.

The loop runs until the context ends, which is the error it returns. An error
from the callback stops it and comes back as it was. A server that refuses the
connection outright is returned rather than waited out, because waiting will not
change a 403. `jmapc.WithPing` and `jmapc.WithReconnect` are there for the
two things worth tuning.

Underneath is `Client.Watch`, which takes the catch-up as a function and is what
to call where the catching up is not one query:

```go
err := c.Watch(ctx, accountID, "Email", state,
	func(ctx context.Context, since string) (newState string, more bool, err error) {
		// ... /changes from since, then whatever the ids call for
	})
```

Only the Go client follows a watch. Holding a connection open is the runtime's
part rather than the generated code's, and the TypeScript and Rust runtimes do
not; generating either from a watching query writes the query without the loop
and says so.

Below `Watch` is `Client.EventSource`, which opens the push endpoint and hands
back the events:

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
		_ = state
	}
}
```

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

The schema is checked the same way, and for the same reason: whether a
validator accepts the example queries and refuses the mistakes the schema
claims to catch is not something Go's tests can say. `example/schema/check.mjs`
asks one, over a schema written from the catalogue as it stands.

The generator is run from source here, not through `go tool`, because the
repository is where it lives.

The runtime types and the example client are committed, and a test compares
them against what the catalogue produces now, so a change to the data model
that was not regenerated fails the build rather than going unnoticed. CI runs
the same checks, plus gofmt, go vet, and govulncheck.

## Coverage

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
