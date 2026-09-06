<p align="right"><a href="paging.md">English</a> | 日本語</p>

# 一度のリクエストに収まらない答えを読み通す

JMAP の答えは、しばしば答え全体の一部にすぎません。
`/query` は呼び出し側が求めた件数の窓だけを返し、その窓が結果全体のどこに位置するかを報告します。
`/changes` はサーバが返すと決めた数だけの変更を返し、続きがあるかどうかを報告します。

どちらの場合も、答えの残りを得るには次のリクエストを送り直す必要があります。
次のリクエストの中身は直前の答えから決まり、それは `/query` なら次に求める `position`、`/changes` なら次に渡す `sinceState` です。
この「答えを見て、次のリクエストを組み立てて、また送る」というループは、リクエストの `_pages` に呼び出し名を書くだけで生成できます。

`_pages` に指定した呼び出しが、ループがそのたびに送り直す呼び出しです。
次のリクエストがどこから始まるかを言う値（`/query` の `position`、`/changes` の `sinceState`）は、呼び出し側ではなくループが管理するので、リクエストの中ではパラメータとして書いておくだけで構いません。

```json
{
  "_pages": "search",

  "methodCalls": [
    ["Email/query", {"filter": {"text": "{{phrase}}"}, "position": "{{position}}",
                     "limit": 50, "calculateTotal": true}, "search"],
    ["Email/get", {"#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"}}, "fetch"]
  ]
}
```

Go にはイテレータが生成されます。
読み進めるたびに、裏で次のリクエストが送られます。

```go
for page, err := range client.SearchEmailsPages(ctx, c, params) {
	if err != nil {
		return err
	}
	for _, email := range page.EmailGet.List {
		fmt.Println(*email.Subject)
	}
}
```

TypeScript には非同期ジェネレータが生成され、同じように読み進めるたびにリクエストを送ります。
失敗は、リクエスト自体と同じように throw されます。

```ts
for await (const page of searchEmailsPages(client, params)) {
  for (const email of page.emailGet.list) console.log(email.subject)
}
```

Rust には、現在の位置を保持する値が生成されます。
ストリームを返すにはそれを定義するクレートが要り、生成されるコードが求めるのは serde だけだからです。

```rust
let mut pages = search_emails_pages(params);
while let Some(page) = pages.next(&client).await? {
    for email in &page.email_get.list {
        println!("{:?}", email.subject);
    }
}
```

読み通しが終わるタイミングは、`/query` と `/changes` とで違います。

`/query` の場合、パラメータが持つ `position` から読み始めるので、前回の続きから読み進められます。
中身のない窓は呼び出し側には渡さず、そこで読み通しを終えます。
ですから呼び出し側が受け取る窓には必ず中身があります。
呼び出しが `calculateTotal` で総数を求めていれば、その総数を超えて窓を求めることもありません。

`/changes` の場合は、何も変わっていないという答えも呼び出し側に渡します。
その答えが、次に進むための `sinceState` を運んでいるからです。
終わるのは、サーバが変更の続きはないと報告したときです。

watch するリクエスト（`_watches` を持つリクエスト）は、サーバが変更の続きを報告する間はもともとリクエストを送り直しています。
つまり同じ仕組みをすでに内蔵しているので、`_watches` と `_pages` を同じリクエストに書くことはありません。
