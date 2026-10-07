# ベンダ拡張

JMAPは拡張される前提の設計です。
サーバは独自のケイパビリティURIを広告し、それとともにjmapcの知らない型とメソッドが現れます。
スキーマファイルに記述すれば、それに対するリクエストも`Email`に対するものとまったく同じように検証されます。
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

標準の6つのメソッドは、名前を挙げるだけで手に入ります。
引数とレスポンスの形はRFC 8620が固定しているからです。
その形に従わないメソッドは、引数とレスポンスを書き下して宣言します。

書き下したメソッドが/getのように返すレコードのプロパティを絞り込むなら、絞り込む引数を`"properties"`に、レコードを持つレスポンスのプロパティを`"resultProperty"`に書きます。
何を求められてもすべてのレコードのidを返すなら、`"returnsId": true`と書きます。
すると、idを求めなくても、そのプロパティの下のidへの後方参照（`resultProperty`が`list`なら`/list/*/id`）が通ります。
書かなければ、jmapcはidを呼び出しが求めたときにだけ返るものとみなします。
idを持たないデータ型のメソッドには`"returnsId"`を書けません。

Email/parseのように、こうしたメソッドが何を返すかを3つのリストで書けます。
`"defaultProperties"`には、呼び出しがプロパティを指定しないときに返すものを、すべてのプロパティではない場合に書きます。
`"nullProperties"`には、何を求められてもnullで返すものを書きます。呼び出しはそれを求めることも参照することもできません。
`"nullableProperties"`には、型はnullを許さないのにnullで返ることがあるものを書きます。生成するコードではnullを許す型になります。

型の名前は、仕様の型と同じく大文字で始め、英字と数字だけで付けます。
JMAPがすでに持つ型と大文字小文字だけが違う名前も付けられません。
生成器は`email`と`Email`を同じ名前として書き出すからです。
名前や参照を誤ったスキーマは、読み込んだ時点で何がどこで誤っているかを示して拒否します。

Emailがヘッダフィールドをとるようにフィールド以外のプロパティを/getでとる型は、それを`"dynamic"`に並べます。
`["meta:"]`は`meta:`で始まるすべてのプロパティを、コロンで終わらない項目はその名前1つをとります。

```
jmapc generate -schema schema/notes.json
```

`jmapc.json`の`"schemas"`に列挙することもできます。
