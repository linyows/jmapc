# 生成される名前

生成される名前はすべてファイル名から決まります。
そのためファイル名は Go の識別子でなければなりません。
英数字とアンダースコアからなり、数字で始まらない名前です。

`ListInboxEmails.jmap.json` からは次の名前が生成されます。

| 生成物 | 名前 |
|---|---|
| 関数 | `ListInboxEmails` |
| パラメータ。リクエストが値を開けている場合に生成されます | `ListInboxEmailsParams` |
| プロパティを絞り込んだレコード | call id `fetch` から `ListInboxEmailsFetchEmail`。ボディパートを絞り込めば `ListInboxEmailsFetchEmailBodyPart` も生成されます。名前付きの集合を指定した呼び出しでは、代わりにその集合の名前が型の名前になります。[プロパティ集合](properties.md)を参照してください |
| そのレコードを返す呼び出しのレスポンス | `ListInboxEmailsFetchResponse`（call id の `fetch` から） |
| 結果。`_returns` が呼び出しを指定しない場合に生成されます | `ListInboxEmailsResult` |
| 変更を追う関数。リクエストが `_watches` を持つ場合に生成されます | `SyncEmailsWatch` |
| 答えを最後まで読み通すもの。リクエストが `_pages` を持つ場合に生成されます | `SearchEmailsPages` |
| ファイル | `listinboxemails_gen.go` |

生成されるコードの名前は call id から決まります。
結果は呼び出しごとに1つのフィールドを持ち、その名前はリクエストが付けた id です。絞り込みのある呼び出しのレコード型とレスポンス型も同じ名前から作られます。
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
同じパッケージに同名のリクエストを2つ置くことはできません。
生成される型の名前がすでに使われている場合は、`ListInboxEmailsFetchEmail2` のように連番が付きます。

1つのリクエストの中で、同じメソッドで同じ型を読み、同じプロパティを要求している呼び出しどうしは、同じレコードを表しています。
そのため型は1つになり、名前は最初の呼び出しの call id から作られます。
同じ形に2つの名前が付くと、片方の呼び出しが返したレコードを、もう片方のために書いた関数に渡すのに変換が要ることになります。
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

これがないと、同じ名前が2つのファイルに何の繋がりもなく存在することになります。
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
型名も上の表のままですが、頭字語は1語として綴られます。
これは Rust の綴り方に倣ったもので、`UTCDate` は `UtcDate` になります。
プロパティは snake_case になり、それがワイヤ上の名前と違う場合には serde の rename が付きます。
