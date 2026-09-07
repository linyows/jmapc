<p align="right"><a href="push.md">English</a> | 日本語</p>

# プッシュ

イベントが伝えるのは、どのアカウントのどの型が変わったかであって、何が変わったかではありません。
ですから変更を取得するクライアントはループを書きます。
接続し、手元の状態からの差分を要求し、それを適用し、次のイベントを待つ。
このループは毎回同じで、そのどの部分にも間違いが入り込みえます。
そこで、リクエストの側から要求できるようにしました。

`_watches` は、ループが状態を読む呼び出しを指定します。
指定できるのは、`Email/changes` のように、ある状態からの変更を報告する呼び出しです。

```json
{
  "_watches": "changes",

  "methodCalls": [
    ["Email/changes", {"sinceState": "{{sinceState}}", "maxChanges": 128}, "changes"],
    ["Email/get", {"#ids": {"resultOf": "changes", "name": "Email/changes", "path": "/created"}}, "created"]
  ]
}
```

`SyncEmails` はこれまでどおり生成され、その隣に `SyncEmailsWatch` が生成されます。

```go
err := client.SyncEmailsWatch(ctx, c, client.SyncEmailsParams{SinceState: state},
	func(ctx context.Context, res *client.SyncEmailsResult) error {
		for _, email := range res.EmailGet.List {
			fmt.Println("new:", *email.Subject)
		}
		state = res.EmailChanges.NewState // 保存して、次はここから始める
		return nil
	})
```

ループはパラメータが持つ状態から始まり、各回の答えが報告する状態へ進みます。
呼び出し側が持たずに済むのは次のことです。

- **ストリームは接続であって購読ではありません。** 切れたら別の接続を開き、最後に届いたイベントから再開します。サーバに届かない間は、1 秒から 30 秒へと倍々に待ちます。
- **接続がない間の変更はプッシュされません。** ですから接続のたびに、まず追いつきます。
- **サーバは `/changes` に返すと決めた数だけ答え**、`hasMoreChanges` にその旨を設定します。ループは、これが false になるまでリクエストを繰り返します。
- **他のアカウント、他の型、あるいは既に到達済みの状態のイベント**にはリクエストが不要です。最後のものはよく起きます。自分の追いつきによって、サーバが受け取ったばかりの状態をプッシュするからです。

ループはコンテキストが終わるまで走り、そのエラーを返します。
コールバックが返したエラーはループを止め、そのまま返ります。
接続を明確に拒んだサーバのエラーは、待たずに返します。403 は待っても変わらないからです。
`jmapc.WithPing` と `jmapc.WithReconnect` が、調整する価値のある二つです。

十分に長く止まっていた後で再開したウォッチは、そのままでは先へ進めないエラーに出会います。
`/changes` は、渡された state がサーバの保持する範囲より古いときに `cannotCalculateChanges` を返します。
同じ state で問い直しても答えは変わりません。
RFC 8620 はこの場合にレコードを取り直すことを求めていて、`jmapc.WithResync` がその置き場所です。

```go
err := client.SyncEmailsWatch(ctx, c, client.SyncEmailsParams{SinceState: state},
	func(ctx context.Context, res *client.SyncEmailsResult) error {
		state = res.Changes.NewState
		return nil
	},
	jmapc.WithResync(func(ctx context.Context) (string, error) {
		res, err := client.ListInboxEmails(ctx, c, client.ListInboxEmailsParams{
			MailboxID: inbox,
			Limit:     500,
		})
		if err != nil {
			return "", err
		}
		cache.replace(res.List)
		return res.State, nil
	}))
```

ウォッチは、この関数が報告した state から続きます。
渡さなかった場合、ウォッチはこのエラーを返して止まります。
変更を追っていたプログラムは追わなくなり、それを伝えるものはそのエラーだけです。
再同期が報告したばかりの state からもサーバが変更を計算できない場合は、ウォッチは止まります。
もう一度取り直しても同じ state を報告し、同じ答えに出会うだけだからです。

その下にあるのが `Client.Watch` で、追いつき方を関数で受け取ります。
追いつきが一つのリクエストで済まないときは、これを直接呼びます。

```go
err := c.Watch(ctx, accountID, "Email", state,
	func(ctx context.Context, since string) (newState string, more bool, err error) {
		// since からの /changes を呼び、返った id に応じて取得する
	})
```

`_watches` を追うのは Go のクライアントだけです。
接続を保持するのは生成コードではなくランタイムの仕事で、Rust と TypeScript のランタイムはそれをしません。
それらの言語で watch するリクエストを生成すると、ループのないコードだけが生成され、その旨が表示されます。

`Watch` のさらに下にあるのが `Client.EventSource` で、プッシュエンドポイントに接続してイベントをそのまま返します。

```go
stream, err := c.EventSource(ctx, &jmapc.EventSourceOptions{
	Types: []string{"Email"},
	Ping:  30 * time.Second,
})
defer stream.Close()

for {
	change, err := stream.Next()
	if err != nil {
		break // stream.LastEventID() を渡して再接続する
	}
	if state, ok := change.StateOf(accountID, "Email"); ok {
		_ = state
	}
}
```

これはイベントソース形式のプッシュで、接続を保持できるクライアントに向いています。
もう一つの形式は、サーバが送る先の URL を登録するもので、スマートフォンのアプリにはこちらが必要です。
[`example/requests`](../example/requests) の `RegisterPush` と `ConfirmPush` を参照してください。
購読は作成した時点ではまだ有効ではありません。
サーバが URL にコードを送り、クライアントが `PushSubscription/set` でそれを書き戻すまで、他には何も送られません。
届いたものは `jmapc.PushVerification` でデコードします。
