# プロパティ集合

同じ 14 個の `Email` のプロパティを読む 6 本のリクエストからは、呼び出しごとに 1 つずつ、6 つのレコード型が生成されます。
メールを描画する関数はそのうち 1 つに対して書かれ、残りの 5 つからは詰め替えることになります。
形そのものに名前を付ければ、生成される型は 1 つになります。

集合はリクエストの隣の `properties.json` に書きます。
これは JMAP のリクエストではないので、拡張子は `.jmap.json` ではありません。
集合を使わないプロジェクトにこのファイルは要りません。

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

| メンバ | |
|---|---|
| `type` | プロパティを選ぶ対象のデータ型です。必須ですが、`extends` で型が定まる場合は省略できます。 |
| `properties` | プロパティ名です。並び順が、生成されるフィールドの並び順になります。必須です。 |
| `extends` | この集合が追加する先の集合です。省略できます。 |
| `doc` | その集合が何のためにあるかを書きます。生成される型のコメントになります。省略できます。 |

リクエストには、プロパティの列挙の代わりに集合の名前を書きます。

```json
["Email/get", {"#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"},
               "properties": "@EmailSummary"}, "fetch"]
```

サーバに届くのは集合に並べたプロパティの列挙です。
集合はコードを生成するときに解決され、`{{name}}` は実行時に呼び出し側が埋めます。書き方が違うのはそのためです。

生成される型の名前は集合の名前です。
`ListInboxEmailsFetchEmail` ではなく `EmailSummary` になります。
その集合を指定したリクエストの結果はすべてこの 1 つの型なので、この型に対して書いた関数を、どのリクエストの結果にも使えます。
プロパティの列挙も 1 箇所になります。
アプリケーションが読むプロパティを 1 つ増やすときの編集はリクエストごとではなく 1 回で済み、どれか 1 本だけ古いまま少ないプロパティで送られ続けることもありません。

他の集合を継承した集合では、生成される型に継承元が埋め込まれます。
継承元に対して書いた関数に、2つの構造体の間で値を詰め替えることなく、継承先のレコードを渡せます。

```go
func subjectOf(e client.EmailSummary) string { ... }

subjectOf(changed.EmailSummary)
```

Rust では継承元が継承先の構造体に flatten され、TypeScript ではインタフェースの継承になります。どちらでも同じことが成り立ちます。

集合の名前は、呼び出し側が書く型の名前です。
そのため、リクエストに同じ名前は使えません。
`EmailSummary` という集合の隣に `EmailSummary.jmap.json` があれば、黙って改名されるのではなくエラーとして報告されます。

集合を指定した呼び出しでは、レコードをそれ以上絞り込めません。
集合と `bodyProperties` を同じ呼び出しに書くとエラーになります。
ボディパートを絞り込むとレコード型も一緒に変わるからです。ボディパートを指すフィールドが、絞り込んだ型を指すようになります。
ある呼び出しでは絞り込み、別の呼び出しでは絞り込まないとなると、1つの名前が2つの形を指すことになります。
その呼び出しではプロパティを書き下すか、ボディパートにも集合を付けてください。

```json
["Email/get", {"ids": "{{ids}}",
               "properties": ["id", "subject", "textBody"],
               "bodyProperties": "@BodyPartSummary"}, "fetch"]
```
