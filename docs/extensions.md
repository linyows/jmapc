<p align="right">English | <a href="extensions.ja.md">日本語</a></p>

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

```
jmapc generate -schema schema/notes.json
```

Or list them in `jmapc.json` under `"schemas"`.
