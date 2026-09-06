<p align="right"><a href="testing.md">English</a> | 日本語</p>

# テスト

生成されたクライアントの周りに書いたコードをテストするということは、複数のメソッド呼び出しを載せたリクエストに答えるということです。
しかもその一部は、他の呼び出しの結果を参照しています。
一つのテストのために手で書いたスタブは、そこを無視してサーバらしさを失うか、さもなければこれへと育っていきます。

```go
srv := jmaptest.New(t)
srv.Reply("Email/query", jmapc.EmailQueryResponse{
	AccountID: jmaptest.AccountID,
	IDs:       []jmapc.ID{"m1", "m2"},
})
srv.Handle("Email/get", func(c *jmaptest.Call) (any, error) {
	// id は query の呼び出しが答えたものです。結果参照は、
	// サーバが解決するのと同じように、すでに解決されています。
	return emailsFor(c.IDs()), nil
})

res, err := client.ListInboxEmails(ctx, srv.Client(), params)
```

テストから引き受けるのは次のことです。

- **結果参照。** RFC 8620 の言うとおりに解決します。パスをリストに写像する `*` も含みます。ですから連鎖したリクエストは、実際の値が入った状態でハンドラに届きます。
- **検証。** リクエストは、ビルドがリクエストを照らすのと同じデータモデルに照らされます。どのメソッドも持たない引数を送る呼び出しは、黙って通るのではなくテストを失敗させます。jmapc が知らないメソッドを扱うときは `jmaptest.WithoutChecks()` で検証を無効にします。
- **失敗。** メソッドレベルのエラーには `srv.Fail`、リクエスト全体を拒否する場合には `srv.FailRequest`、そして 200 を返す失敗には、拒否したものを並べた `/set` のレスポンスを返します。
- **何が要求されたか。** `srv.Call("Email/query")` はそのメソッドへの最後の呼び出し、`srv.Calls()` はその全部、`srv.Requests()` はそれが何回のリクエストで済んだかです。複数の呼び出しが一つのリクエストにまとまったかどうかは、これで確かめます。
- **プッシュ。** `srv.Push` は、watch しているクライアントに状態変化を送ります。watch するリクエストのループが待っているものです。

しないのは、何かを保存することです。
これはクライアントをテストするためのサーバであって、JMAP の実装ではありません。
`/set` が作ったものは、テストがそう指定しない限り、後の `/get` からは返りません。

クライアントを一度に全部移すことは稀で、移行の途中では二つの半分を同じテストで応答させることになります。
生成された側は `srv.Client()` からこのサーバに届きますが、まだ手で書かれている側は自分のパスに投げます。
そのパスを載せる場所が `srv.Mux()` で、手で書かれた側を向ける先が `srv.BaseURL()` です。

```go
srv := jmaptest.New(t)
srv.Mux().HandleFunc("/jmap", myOldAPIHandler)
srv.Mux().HandleFunc("/jmap/session", myOldSessionHandler)

old := myOldClient(srv.BaseURL())
```

まだ移していない側も JMAP を話していて、ただ探す場所が違うだけ（セッションと API を自前のベース URL から導いている）という場合は、jmaptest 自身のハンドラをそこに載せます。
両方の名前で応答するようになります。

```go
srv.Mux().HandleFunc("/jmap/session", srv.ServeSession)
srv.Mux().HandleFunc("/jmap", srv.ServeAPI)
```

ですから jmaptest は、最後のメソッドを移し終えてからではなく、最初の一つを移した時点から使えます。
