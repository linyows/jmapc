<p align="right">English | <a href="requests.ja.md">日本語</a></p>

# Writing a request

A request file is a JMAP Request object, exactly as
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
| `_watches` | The call a generated client follows the changes of, so that it catches up whenever the server reports a change. Optional; see [Push](push.md). |
| `_pages` | The call a generated walk advances, so that a result returned one part at a time can be read in full. Optional; see [Walking an answer that does not fit in one request](paging.md). |
| `_comment` | Why a call is there. Goes in that call's arguments; see below. |

A request file is plain JSON, so `jq` can read it and an editor can check it. To
record why a call is there, give its arguments a `_comment`.

## Parameters

Write `{{name}}` where a value is left to the caller. Its Go type comes from the
argument it stands in for, so `{{limit}}` in `limit` is a `jmapc.UnsignedInt`
and `{{mailboxId}}` in `inMailbox` is a `jmapc.ID`. Use the same name twice and
it becomes one field, checked for agreeing on its type.

An argument that may take either of two shapes becomes a struct with a field
per shape, of which exactly one is set. A filter is where that matters, since
it is either a boolean operator or a condition on the type being queried:
`{{filter}}` in `filter` is a `jmapc.FilterOperatorOrEmailFilterCondition`.

```go
client.Search(ctx, c, client.SearchParams{
	Filter: jmapc.FilterOperatorOrEmailFilterCondition{
		EmailFilterCondition: &jmapc.EmailFilterCondition{Text: "invoice"},
	},
})
```

Setting no field, or more than one, is an error where the request is encoded
rather than something the server has to refuse. The conditions a
`FilterOperator` combines stay `[]any`: which condition type they hold depends
on the type being queried, and the operator itself is written once for all of
them.

A map key may be a parameter too, which is how a `/set` names the record to
change:

```json
["Email/set", {"update": {"{{emailId}}": {"keywords/$seen": true}}}, "mark"]
```

The braces are used rather than a `$` prefix because JMAP keywords are
themselves written with one, as in `$seen`.

### An argument the caller may leave out

Write `{{name?}}` where the caller may leave the argument out altogether:

```json
["Email/changes", {"sinceState": "{{sinceState}}", "maxChanges": "{{maxChanges?}}"}, "changes"]
```

Nothing supplied means the argument is not in the request, which is not the
same as sending null. RFC 8620 makes the difference twice over: `maxChanges`
absent is no cap, while `maxChanges: 0` asks for nothing at all; and in a
PatchObject a pointer set to null removes the property while a pointer that is
not there leaves it alone. Without this, each argument that is only sometimes
sent would need a request of its own, and *n* of them would need 2ⁿ.

The caller says "left out" the way the language already does. Go takes a
pointer, or nothing where the type has a nil of its own:

```go
limit := jmapc.UnsignedInt(25)
client.FindPeople(ctx, c, client.FindPeopleParams{Phrase: "ada", Limit: &limit})
client.FindPeople(ctx, c, client.FindPeopleParams{Phrase: "ada"}) // no limit argument
```

Rust wraps it in an `Option`, TypeScript makes the member optional
(`limit?: number`), and `jmapc run` leaves the argument out when no `-p` names
it.

Only a whole argument of a method call may be left out, and only where the
parameter standing for it is used nowhere else, so that leaving it out has one
meaning: this member is not there. A parameter inside a filter or an array is
part of a larger value, and dropping it would leave a question the request does
not answer, since an empty `AND` and no filter at all are different requests.
For a filter whose shape varies, hand the whole filter over as one parameter
instead.

## Sharing properties across requests

Six requests reading the same fourteen properties of an `Email` get six record
types, one per call, and a function that renders a message is written for one of
them and converts from the other five. Name the shape instead, and the six
answer with one type.

The sets sit in `properties.json`, beside the requests. It is not a JMAP
request, so it does not carry the `.jmap.json` extension, and a project that
names no set does not need the file at all.

```json
{
  "EmailSummary": {
    "doc": "EmailSummary is what a list of messages shows.",
    "type": "Email",
    "properties": ["id", "threadId", "subject", "from", "receivedAt", "preview", "hasAttachment"]
  },

  "EmailWithFlags": {
    "extends": "EmailSummary",
    "properties": ["mailboxIds", "keywords"]
  }
}
```

| Member | |
|---|---|
| `type` | The data type the properties are selected from. Required, unless `extends` fixes it already. |
| `properties` | The property names, in the order the generated fields are written in. Required. |
| `extends` | The set this one adds to. Optional. |
| `doc` | What the set is for, carried into the generated type's comment. Optional. |

A request asks for a set by name, in place of the list:

```json
["Email/get", {"#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"},
               "properties": "@EmailSummary"}, "fetch"]
```

What reaches the server is the list the set holds. A set is resolved while the
code is generated and a `{{name}}` is filled in by the caller at run time, which
is why the two are written differently.

The generated type takes the set's name — `EmailSummary`, not
`ListInboxEmailsFetchEmail` — so every request asking for the set answers with
that one type, and a function written for it takes what any of them returned.
The property list is written once as well: adding a property to what the
application reads is one edit rather than one edit per request, and no request
can be left behind still asking for less.

A set that extends another embeds it, so a function written for the base takes a
record of the derived set without anything being copied between two structs:

```go
func subjectOf(e client.EmailSummary) string { ... }

subjectOf(changed.EmailSummary)
```

Rust flattens the base into the derived struct and TypeScript extends the
interface, so the same holds in each.

A set's name is the name of a type a caller writes, so a request cannot take it:
a request file named `EmailSummary.jmap.json` beside a set of that name is
reported rather than quietly renamed.

A call that asks for a set narrows nothing else about the records it reads, so a
set and `bodyProperties` in one call is reported. Narrowing the body parts
changes the record type with it, since the fields referring to a body part refer
to the narrowed type instead, and a set narrowed in one call and not in another
would be two shapes under one name. Write the properties out in that call, or
name a set for the body parts as well:

```json
["Email/get", {"ids": "{{ids}}",
               "properties": ["id", "subject", "textBody"],
               "bodyProperties": "@BodyPartSummary"}, "fetch"]
```

## Creation ids across requests

Referring to `#draft` within one request needs nothing: the server resolves it.
Carrying a reference from one request into the next needs the ids to be carried
between them, which `_createdIds` does.

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
res, err := client.FileIntoNewMailbox(ctx, c, params, carried)
// res.CreatedIDs goes to the next request.
```

RFC 8620 has this for proxies, which split one request across servers and need
the references to still resolve. A request using it returns every response rather
than one, since the ids belong to the request rather than to any call in it.

## Account ids

Leave `accountId` out and the generated function fills it in from the primary
account of the session, looking the session up once. Write `"{{accountId}}"` to
make it a parameter instead.


## Generated names

The file name determines every name in the generated file, so it has to be a Go
identifier: letters, digits and underscores, not starting with a digit.
`ListInboxEmails.jmap.json` gives:

| Generated | Name |
| --- | --- |
| The function | `ListInboxEmails` |
| Its parameters, where the request leaves any open | `ListInboxEmailsParams` |
| A record whose properties the request narrows | `ListInboxEmailsFetchEmail`, after the call id `fetch`, and `ListInboxEmailsFetchEmailBodyPart` for a narrowed body part. A call asking for a named set answers with the set's own name instead; see [Sharing properties across requests](#sharing-properties-across-requests) |
| The response to a call returning that record | `ListInboxEmailsFetchResponse`, after the call id `fetch` |
| The result, where `_returns` names no call | `ListInboxEmailsResult` |
| The function that follows changes, where the request is watched | `SyncEmailsWatch` |
| The walk over the parts of an answer, where the request is paged | `SearchEmailsPages` |
| The file | `listinboxemails_gen.go` |

The call ids are the names in the generated code: a result holds one field per
call, named after the id the request gave it, and the record and response types
of a call that narrows are named the same way. Nothing is numbered by position,
since a call id is unique within a request already and inserting a call ahead
of another would otherwise move a name onto a different shape:

```json
["Email/query", {...}, "search"],
["Email/get",   {...}, "fetch"]
```

```go
res.Search.IDs      // the Email/query response
res.Fetch.List      // the Email/get response
```

A call id that is not an identifier — RFC 8620 allows any string — falls back
to the method it invokes.

A call the request does not narrow answers with the shared type instead, so
`SendEmail` returns `*jmapc.EmailSubmissionSetResponse`. Two requests in one
package cannot take the same name, and a generated type whose name is already
taken gains a number: `ListInboxEmailsFetchEmail2`.

Two calls of one request that read the same type through the same method and ask
for the same properties describe one record, so they share one type, named
after the first of them. Two calls asking for the same named set share the type
that set names, wherever in the package they are; a call spelling the same
properties out is not one of them, since the name a set stands for is the one
the author chose. Two names for one shape would make a caller convert
between them to hand a record from one call to a function written for the
other. Inserting a call that reads that same shape ahead of both moves the name
to the new call, which the build reports; the shape a name stands for does not
change.

A `/set` that creates gets a constant for each name it gives a record, since
the response reports the record back under that name:

```json
["Mailbox/set", {"create": {"newMailbox": {"name": "{{name}}"}}}, "make"]
```

```go
res, err := client.CreateMailbox(ctx, c, client.CreateMailboxParams{Name: name})
...
created := res.Created[client.CreateMailboxNewMailbox]
```

Without it the name appears in two files with no link between them, and
renaming it in the request still builds: the lookup misses at run time instead.
Rust writes `CREATE_MAILBOX_NEW_MAILBOX` and TypeScript
`createMailboxNewMailbox`. A creation id the request leaves to the caller —
`{"{{creationId}}": ...}` — has no constant, since the caller already has the
name.

A request with no open parameters takes no `Params` argument at all —
`MailQuota(ctx, c)`, not `MailQuota(ctx, c, MailQuotaParams{})` — so adding
the first `{{param}}` to a request already in use changes the generated
function's arity and breaks every call site. That is a deliberate trade for
the common case of a request with no parameters reading like a plain function
call, not an oversight.

Rust writes the function and its module in snake_case — `list_inbox_emails` in
`list_inbox_emails.rs` — and keeps the type names above, except that an
initialism becomes a word, since that is how Rust spells one: a `UTCDate` is a
`UtcDate`. Properties are snake_case, with a serde rename wherever that is not
the name on the wire.

TypeScript lowercases the first letter of the function and of the file —
`listInboxEmails` in `listInboxEmails.ts` — and keeps the type names too.

## Examples

[`example/requests`](../example/requests) holds twenty-five of these, over mail,
contacts, calendars, sharing and filtering: searching, syncing from a known state, sending,
creating a contact card, moving one occurrence of a recurring meeting without
touching the rest of the series.
