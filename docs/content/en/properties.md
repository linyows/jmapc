# Property sets

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
