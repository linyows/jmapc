# Writing a request

A request file is a JMAP Request object, exactly as
[RFC 8620](https://www.rfc-editor.org/rfc/rfc8620) defines it, plus
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
| `_pages` | The call a generated walk advances, so that a result returned one part at a time can be read in full. Optional; see [Paging](paging.md). |
| `_comment` | Why a call is there. Goes in that call's arguments; see below. |

A request file is plain JSON, so `jq` can read it and an editor can check it. To
record why a call is there, give its arguments a `_comment`.

A call may name a set of properties declared once for several requests, in
place of the list; see [Property sets](properties.md). What the generated code
is called, from the function down to the fields of its result, is in
[Generated names](generated-code.md).

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

## Examples

[`example/requests`](https://github.com/linyows/jmapc/tree/main/example/requests) holds twenty-five of these, over mail,
contacts, calendars, sharing and filtering: searching, syncing from a known state, sending,
creating a contact card, moving one occurrence of a recurring meeting without
touching the rest of the series.
