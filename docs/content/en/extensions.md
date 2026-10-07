# Vendor extensions

JMAP is meant to be extended: a server advertises a capability URI of its own,
bringing types and methods jmapc does not know. Describe them in a
schema file and requests against them are checked exactly as ones against `Email`
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
      "methods": ["get", "changes", "set", "request"],
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

A method declared outright that narrows the properties of the records it
returns, as a /get does, names the argument that narrows them in `"properties"`
and the response property holding them in `"resultProperty"`. If it returns the
id of every record whatever it is asked for, `"returnsId": true` says so, and a
back reference to the ids under that property, `/list/*/id` where it is `list`,
holds without asking for the id. Left out, jmapc assumes the id comes back only
where the call asks for it. A data type with no id cannot take `"returnsId"`.

Three more lists say what such a method answers with, as Email/parse has them.
`"defaultProperties"` names the properties it returns where a call names none,
if not every property; `"nullProperties"` those it returns as null whatever it
is asked for, which a call may not ask for or refer to; and
`"nullableProperties"` those it may return as null though their type does not
say so, which are generated as nullable.

A type is named as the specifications name theirs, with a capital and letters
and digits after it, and not as a type JMAP already has, however it is
capitalised: a generator writes `email` and `Email` as one name. A schema that
gets a name or a reference wrong is refused when it is read, with what is wrong
and where.

A type whose /get takes properties beyond its fields, as an Email takes a header
field, lists them under `"dynamic"`: `["meta:"]` takes every property beginning
`meta:`, and an entry without the colon takes that one name.

```
jmapc generate -schema schema/notes.json
```

Or list them in `jmapc.json` under `"schemas"`.
