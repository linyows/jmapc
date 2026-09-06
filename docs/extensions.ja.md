<p align="right"><a href="extensions.md">English</a> | 日本語</p>

# ベンダ拡張

JMAP は拡張される前提の設計です。
サーバは独自のケイパビリティ URI を広告し、それとともに jmapc の知らない型とメソッドが現れます。
スキーマファイルに記述すれば、それに対するリクエストも `Email` に対するものとまったく同じように検証されます。
結果参照、プロパティ名、ソート順、すべてが対象です。

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

標準の六つのメソッドは、名前を挙げるだけで手に入ります。
引数とレスポンスの形は RFC 8620 が固定しているからです。
その形に従わないメソッドは、引数とレスポンスを書き下して宣言します。

```
jmapc generate -schema schema/notes.json
```

`jmapc.json` の `"schemas"` に列挙することもできます。
