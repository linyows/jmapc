<p align="right"><a href="verification.md">English</a> | 日本語</p>

# 検証

以下はすべて、サーバへの往復ではなくビルド時の失敗になります。

- メソッドが存在し、仕様どおりに綴られていること
- 引数がそのメソッドのものであり、メソッドが要求する型であること
- 結果参照が**先行する**呼び出しを指し、その呼び出しのメソッド名を正しく名指し、参照先の引数が受け取れる値を選んでいること
- フィルタ条件が、クエリ対象の型に照らして検証されること。`AND`、`OR`、`NOT` の中に入れ子になったものも含みます
- `properties` がその型の持つプロパティを、`bodyProperties` が `EmailBodyPart` の持つプロパティを指していること
- ヘッダフィールドを指すプロパティが、仕様の定めるパース形式を要求していること。`header:List-Id:asText` は文字列に、`header:To:asAddresses` はアドレスのリストになります
- `PatchObject` が、パッチ対象のレコードが実際に持つプロパティを指し、正しい型の値を設定していること。キーの書き方は RFC 8620 に従います。ポインタ先頭の `/` は暗黙なので、キーワードを立てる位置は `/keywords/$seen` ではなく `keywords/$seen` です
- `sort` がその型で実際にソートできるプロパティを指し、`hasKeyword` のような比較子が要求する追加メンバを与えていること
- 仕様が値を固定しているプロパティに、その値のいずれかが与えられていること。文字列の値と、参加者の `roles` のような集合のキーの両方が対象です
- id、日付、整数の形式が正しいこと
- リクエストが宣言するケイパビリティが、呼び出すメソッドを網羅していること
- `_watches` が指す呼び出しが、ある状態からの変更を報告するものであり、そこから進む状態がリクエストに書き込まれずループに委ねられていること
- `_pages` が指す呼び出しが、より長い答えの一部を返して残りの位置を報告するものであり、次のリクエストの開始位置がループに委ねられていること

綴り間違いには候補が提示されます。

```
requests/BadQuery.jmap.json: methodCalls[0].arguments.filter.hasAttachmnt: EmailFilterCondition has no property "hasAttachmnt"
	did you mean "hasAttachment"?
requests/BadQuery.jmap.json: methodCalls[1].arguments.#ids.name: the referenced call is Email/query, but the reference names Email/get
	call "c0" invokes Email/query
```

パラメータや call id の名前だけが違う二つのリクエストは、同じリクエストを二度書いたものです。
jmapc はこれを失敗ではなく通知として伝えます。

```
jmapc: ListArchiveEmails, ListInboxEmails are the same query under different names; one of them would do for all of them
```

両方とも生成はされます。一つのリクエストに二つの名前を付けたいこともあるからです。
知らせるのは、名前の数だけ生成される型が増えるためです。

`jmapc check` は、何も書き出さずに検証だけを実行します。

## サーバ側にしかない情報

ここまではすべて、仕様が定めていることです。
仕様がサーバに委ねていること —— どのケイパビリティを持つか、どのアカウントを持つか、一度のリクエストでどれだけ受け付けるか —— は、ビルド時には分かりません。
JMAP については正しく、実行対象のサーバについては間違っているリクエストは、実行時に失敗します。
`-session` は実行中のサーバに対して検査します。

```
jmapc check -session jmap.example.com -token $JMAP_TOKEN
checked 25 queries against https://jmap.example.com/api/, as someone@example.com
```

報告するのは次のものです。

- リクエストが宣言していて、サーバが広告していないケイパビリティ
- リクエストが名指していてセッションが持たないアカウント、そのケイパビリティの primary account がなくセッションが埋められないアカウント、呼び出しに必要なものをサポートしないアカウント
- `maxCallsInRequest` を超える呼び出し数、`maxObjectsInGet` を超えるレコード数、`maxObjectsInSet` を超える変更数、パラメータを埋める前から `maxSizeRequest` を超えるリクエスト
- サーバが文字列の比較に使えない `collation`

リクエストが呼び出し側に委ねているものには触れません。
id のリストを表すパラメータは何個にでもなり得るので、そこを推測すれば、問題のないリクエストを問題ありと報告することになります。

セッションの URL だけは環境変数から読みません。
`-token` と `-user` は `$JMAP_TOKEN` と `$JMAP_USER` にフォールバックします。
ネットワークに出る検証は、周囲にたまたま設定されているものによってではなく、コマンドラインでそう言われて出るべきだからです。

## エディタ対応

上の検証は jmapc を走らせたときに実行されます。
その多くは、リクエストを書いている最中に走らせることもできます。
ファイルそのものに対する検証だからです。
そしてスキーマを名指しした JSON ファイルなら、エディタは検証も補完もすでに知っています。

```
jmapc schema -out jmapc.schema.json
```

これがカタログの JSON Schema を、ベンダ拡張も含めて書き出します。
リクエストファイルからそれを指すか、

```json
{
  "$schema": "../jmapc.schema.json",
  "methodCalls": [["Email/query", {"filter": {"inMailbox": "{{mailboxId}}"}}, "search"]]
}
```

エディタ側からまとめて指します。
VS Code なら次のとおりです。

```json
{
  "json.schemas": [
    {"fileMatch": ["*.jmap.json"], "url": "./jmapc.schema.json"}
  ]
}
```

どちらでも、エディタはメソッド名を補完し、そのメソッドが取る引数とその型が持つプロパティを提示し、綴り間違いをその場に下線で示します。
`AND` の中に入れ子になったフィルタも外側と同じように検証され、比較子はその型が実際にソートできるプロパティを提示し、`{{パラメータ}}` は値が置ける場所ならどこでも受け付けられます。

スキーマに言えないのは、他の呼び出しに依存する部分です。
結果参照が先行する呼び出しを名指し、引数が受け取れる値を選んでいるか、そこは jmapc の仕事のままです。
エディタはビルドの代わりではなく、その手前の一段だと考えてください。
