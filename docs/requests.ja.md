<p align="right"><a href="requests.md">English</a> | 日本語</p>

# リクエストの書き方

リクエストファイルは [RFC 8620](https://www.rfc-editor.org/rfc/rfc8620) が定義する JMAP の Request オブジェクトそのものです。
これに、jmapcが読みjmapサーバが見ることのない四つのメンバが加わります。

アンダースコアで始まるメンバはジェネレータが読むもので、それ以外は RFC 8620 が定義するリクエストそのものです。

| メンバ | |
|---|---|
| `methodCalls` | 呼び出しを `[name, arguments, callId]` の形で並べます。必須です。 |
| `using` | リクエストが宣言するケイパビリティです。省略でき、その場合は呼び出すメソッドから導出されます。 |
| `_doc` | 生成される関数のドキュメントです。省略できます。 |
| `_returns` | どの呼び出しのレスポンスを関数の戻り値にするかを指定します。省略すると全てのレスポンスが返ります。 |
| `_createdIds` | 先行するリクエストの creation id を受け取り、このリクエストのものを返します。省略可。次項を参照してください。 |
| `_watches` | 生成されたクライアントが変更を追う呼び出しを指定します。サーバが変更を報告するたびに追いつきます。省略可。[プッシュ](push.ja.md)を参照してください。 |
| `_pages` | 生成されたループが進める呼び出しを指定します。一度のリクエストが一部だけ返す答えを、最後まで読み通せます。省略可。[一度のリクエストに収まらない答えを読み通す](paging.ja.md)を参照してください。 |
| `_comment` | その呼び出しが何のためにあるかを書きます。呼び出しの引数の中に置きます。次項を参照してください。 |

リクエストファイルは素の JSON です。
`jq` で読めますし、エディタも理解します。
呼び出しの意図を書くには、その引数に `_comment` を置きます。

## パラメータ

呼び出し側に委ねる値の位置に `{{name}}` と書きます。
Go の型は、その値が埋まる引数から決まります。
`limit` に置いた `{{limit}}` は `jmapc.UnsignedInt` になり、`inMailbox` に置いた `{{mailboxId}}` は `jmapc.ID` になります。
同じ名前を二箇所で使えば一つのフィールドにまとまり、型が一致しているかが検証されます。

二つの形のどちらかを取りうる引数は、形ごとにフィールドを持つ構造体になります。
設定するフィールドはそのうちの一つだけです。
これが効くのはフィルタで、フィルタは論理演算子か、問い合わせ対象の型に対する条件か、そのどちらかだからです。
`filter` に置いた `{{filter}}` は `jmapc.FilterOperatorOrEmailFilterCondition` になります。

```go
client.Search(ctx, c, client.SearchParams{
	Filter: jmapc.FilterOperatorOrEmailFilterCondition{
		EmailFilterCondition: &jmapc.EmailFilterCondition{Text: "invoice"},
	},
})
```

どのフィールドも設定しなかった場合と、二つ以上設定した場合は、リクエストを符号化する時点でエラーになります。
サーバに拒否させるまでもありません。
`FilterOperator` がまとめる条件は `[]any` のままです。
どの条件型が入るかは問い合わせ対象の型によって変わるのに対し、演算子自体はすべての型に共通で一度だけ定義されているからです。

マップのキーもパラメータにできます。
`/set` が変更対象のレコードを指定する方法がこれです。

```json
["Email/set", {"update": {"{{emailId}}": {"keywords/$seen": true}}}, "mark"]
```

### 呼び出し側が省略できる引数

呼び出し側が引数ごと省略してよい位置には `{{name?}}` と書きます。

```json
["Email/changes", {"sinceState": "{{sinceState}}", "maxChanges": "{{maxChanges?}}"}, "changes"]
```

値を渡さなければ、その引数はリクエストに入りません。
これは null を送るのとは別のことです。
RFC 8620 でも違いは二度出てきます。
`maxChanges` が無いことは上限なしを意味しますが、`maxChanges: 0` は何も要求しないことになります。
PatchObject では、ポインタに null を入れるとそのプロパティが消え、ポインタ自体が無いときだけ触らずに残ります。
この記法がなければ、ときどきしか送らない引数ごとに別のリクエストが必要になり、n 個あれば 2^n 個要ることになります。

省略の伝え方は、その言語がすでに持っている形に従います。
Go はポインタを取り、型自身に nil があるならそのままです。

```go
limit := jmapc.UnsignedInt(25)
client.FindPeople(ctx, c, client.FindPeopleParams{Phrase: "ada", Limit: &limit})
client.FindPeople(ctx, c, client.FindPeopleParams{Phrase: "ada"}) // limit 引数は送られません
```

Rust では `Option` に包まれ、TypeScript ではメンバー自体が省略可能になります (`limit?: number`)。
`jmapc run` では、`-p` で指定しなかった引数がそのまま省略されます。

省略できるのはメソッド呼び出しの引数そのものだけで、しかもそのパラメータが他の場所で使われていない場合に限ります。
こうすることで「省略された」の意味が一つに定まります。つまり、そのメンバーが無い、ということです。
フィルタや配列の中にあるパラメータは、より大きな値の一部です。
そこだけ落とすと、空の `AND` と、フィルタが無いことのどちらなのかという、リクエストが答えていない問いが残ります。
形の変わるフィルタは、フィルタ全体を一つのパラメータとして渡してください。

## プロパティの共通化

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
継承元に対して書いた関数に、二つの構造体の間で値を詰め替えることなく、継承先のレコードを渡せます。

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
ある呼び出しでは絞り込み、別の呼び出しでは絞り込まないとなると、一つの名前が二つの形を指すことになります。
その呼び出しではプロパティを書き下すか、ボディパートにも集合を付けてください。

```json
["Email/get", {"ids": "{{ids}}",
               "properties": ["id", "subject", "textBody"],
               "bodyProperties": "@BodyPartSummary"}, "fetch"]
```

## リクエストを跨ぐ creation id

一つのリクエストの中で `#draft` を参照するのに準備は要りません。
サーバが解決します。
あるリクエストから次のリクエストへ参照を持ち越すには id 自体が移動する必要があり、それを求めるのが `_createdIds` です。

```json
{
  "_createdIds": true,
  "methodCalls": [
    ["Mailbox/set", {"create": {"box": {"name": "{{name}}"}}}, "make"],
    ["Email/set", {"update": {"{{emailId}}": {"mailboxIds/#box": true}}}, "file"]
  ]
}
```

生成される関数は、それを受け取って返します。

```go
res, err := client.FileIntoNewMailbox(ctx, c, params, carried)
// res.CreatedIDs を次のリクエストへ渡す。
```

RFC 8620 がこれを用意しているのはプロキシのためです。
一つのリクエストを複数のサーバに分割しても、参照が解決できるようにするものです。
これを使うリクエストは、単一のレスポンスではなく全てのレスポンスを返します。
creation id はリクエスト全体のものであって、その中のどの呼び出しのものでもないからです。

## アカウント id

`accountId` を省略すると、生成された関数がセッションのプライマリアカウントから補います。
セッションの取得は一度だけです。
パラメータにしたい場合は `"{{accountId}}"` と書きます。


## 生成される名前

生成される名前はすべてファイル名から決まります。
そのためファイル名は Go の識別子でなければなりません。
英数字とアンダースコアからなり、数字で始まらない名前です。

`ListInboxEmails.jmap.json` からは次の名前が生成されます。

| 生成物 | 名前 |
|---|---|
| 関数 | `ListInboxEmails` |
| パラメータ。リクエストが値を開けている場合に生成されます | `ListInboxEmailsParams` |
| プロパティを絞り込んだレコード | call id `fetch` から `ListInboxEmailsFetchEmail`。ボディパートを絞り込めば `ListInboxEmailsFetchEmailBodyPart` も生成されます。名前付きの集合を指定した呼び出しでは、代わりにその集合の名前が型の名前になります。[プロパティの共通化](#プロパティの共通化)を参照してください |
| そのレコードを返す呼び出しのレスポンス | `ListInboxEmailsFetchResponse`（call id の `fetch` から） |
| 結果。`_returns` が呼び出しを指定しない場合に生成されます | `ListInboxEmailsResult` |
| 変更を追う関数。リクエストが `_watches` を持つ場合に生成されます | `SyncEmailsWatch` |
| 答えを最後まで読み通すもの。リクエストが `_pages` を持つ場合に生成されます | `SearchEmailsPages` |
| ファイル | `listinboxemails_gen.go` |

生成されるコードの名前は call id から決まります。
結果は呼び出しごとに一つのフィールドを持ち、その名前はリクエストが付けた id です。絞り込みのある呼び出しのレコード型とレスポンス型も同じ名前から作られます。
位置による連番は使いません。call id はリクエスト内で一意だと決まっており、連番にすると呼び出しを前に挿したときに名前が別の形を指してしまうからです。

```json
["Email/query", {...}, "search"],
["Email/get",   {...}, "fetch"]
```

```go
res.Search.IDs      // Email/query のレスポンス
res.Fetch.List      // Email/get のレスポンス
```

RFC 8620 は call id に任意の文字列を許しているので、識別子にならない call id の場合は、呼び出すメソッド名にフォールバックします。

絞り込みのない呼び出しは、リクエストごとの型ではなく共有の型で返ります。
`SendEmail` の戻り値が `*jmapc.EmailSubmissionSetResponse` なのはそのためです。
同じパッケージに同名のリクエストを二つ置くことはできません。
生成される型の名前がすでに使われている場合は、`ListInboxEmailsFetchEmail2` のように連番が付きます。

一つのリクエストの中で、同じメソッドで同じ型を読み、同じプロパティを要求している呼び出しどうしは、同じレコードを表しています。
そのため型は一つになり、名前は最初の呼び出しの call id から作られます。
同じ形に二つの名前が付くと、片方の呼び出しが返したレコードを、もう片方のために書いた関数に渡すのに変換が要ることになります。
同じ形を読む呼び出しを前に挿すと名前は新しい呼び出しに移りますが、これはビルドが報告します。名前が指す形が変わることはありません。
同じ名前付き集合を指定した呼び出しどうしは、パッケージのどこにあってもその集合の型を共有します。
同じプロパティを書き下しただけの呼び出しはここに含まれません。集合の名前は書き手が選んだものだからです。

レコードを作る `/set` には、そのレコードに付けた名前ごとに定数が生成されます。
レスポンスはその名前でレコードを返してくるからです。

```json
["Mailbox/set", {"create": {"newMailbox": {"name": "{{name}}"}}}, "make"]
```

```go
res, err := client.CreateMailbox(ctx, c, client.CreateMailboxParams{Name: name})
...
created := res.Created[client.CreateMailboxNewMailbox]
```

これがないと、同じ名前が二つのファイルに何の繋がりもなく存在することになります。
リクエスト側で改名してもビルドは通り、実行時にルックアップが外れるだけです。
Rust では `CREATE_MAILBOX_NEW_MAILBOX`、TypeScript では `createMailboxNewMailbox` になります。
`{"{{creationId}}": ...}` のように呼び出し側に名前を委ねた場合は、呼び出し側がすでに名前を持っているので定数は作られません。

パラメータを持たないリクエストは、Params 引数自体を取りません。
`MailQuota(ctx, c, MailQuotaParams{})` ではなく `MailQuota(ctx, c)` になります。
そのため、すでに使われているリクエストに最初の `{{param}}` を追加すると、生成される関数の引数の数が変わり、呼び出し箇所がすべて壊れます。
これは意図的なトレードオフです。パラメータを持たないリクエストが、ただの関数呼び出しのように読めることを優先しています。

TypeScript では関数名とファイル名の先頭が小文字になり、`listInboxEmails.ts` の `listInboxEmails` になります。
型名は上の表のままです。

Rust では関数名とモジュール名が snake_case になり、`list_inbox_emails.rs` の `list_inbox_emails` になります。
型名も上の表のままですが、頭字語は一語として綴られます。
これは Rust の綴り方に倣ったもので、`UTCDate` は `UtcDate` になります。
プロパティは snake_case になり、それがワイヤ上の名前と違う場合には serde の rename が付きます。

## 用例

[`example/requests`](../example/requests) には、メール、連絡先、カレンダー、共有、フィルタにまたがる 25 個のリクエストがあります。
検索、既知の状態からの同期、送信、連絡先カードの作成、繰り返し予定のうち一回だけを他に触れずに動かす操作などです。
