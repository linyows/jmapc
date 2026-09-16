# クライアントの設定

生成された関数は、`*jmapc.Client` を通してリクエストを送ります。
クライアントは、`jmapc.New` にセッションの URL とオプションを渡して作ります。

```go
c := jmapc.New(jmapc.WellKnownURL("example.com"), jmapc.WithBearerToken(token))
```

リクエストの認証は `WithBearerToken` か `WithBasicAuth` で指定します。
クライアントはそのほかに、セッションを保持してサーバが変更を報告すれば取得し直し、一度に送るには大きすぎる `/get` を分割し、期限の切れるトークンを取得し直し、失敗したリクエストを送り直し、送信したリクエストを `Observer` に渡します。
以下の各節で、それぞれの挙動と、それを変えるオプションを説明します。

## Session オブジェクト

ここでいうセッションは、RFC 8620 の Section 2 が Session オブジェクトと呼ぶものです。
サーバがセッションリソースで返す文書で、リクエストの送り先、サーバが持つ capability、ユーザが到達できるアカウント、そして各種の上限を伝えます。
ログインのセッションではなく、資格情報も持ちません。
リクエストを認証するのは Authorization ヘッダで、期限切れのあるトークンは別の話です。
これは後の「期限切れのあるトークン」で扱います。

クライアントは、セッションが最初に必要になったときに取得し、以後は保持します。

サーバのセッションは変わります。
アカウントが増えたり減ったり、上限が引き上げられたり、エンドポイントが移ったり、push の鍵が更新されたりします。
すべてのレスポンスがサーバの `sessionState` を運んでくるのはこのためで、クライアントはそれを保持しているセッションと比べます。
両者が違えば、次にセッションを必要とする呼び出しが取得し直します。
比較そのものには費用がかかりませんし、取得は、変更を報告してきたレスポンスの中ではなく、セッションが必要になった場所で起きます。

変更のあとに同時に届いたリクエストは、一度の取得を共有します。
取得に失敗した場合、セッションはそのまま残ります。
保持しているものは古いままですが、それは少し前まで古いと分かっていなかっただけで、リクエストはそのまま進み、次の呼び出しがもう一度試みます。
失敗は `KindSession` のリクエストとして `Observer` に届きます。

クライアントが同時に飛ばすリクエストの数も、セッションに追随します。
`maxConcurrentRequests` が下がった場合、すでに飛んでいるリクエストは取り消しません。
それぞれが終わった順に枠を返し、保持している数が新しい上限を下回るまで、次の枠は渡されません。
そのため、サーバが最後に述べた数を超えて飛ぶことはありません。
これが重要なのは、上限を超えたリクエストをサーバが 400 で拒否し、400 はどの再送方針でも送り直されないからです。

`WithoutSessionRefresh` はこの動作を止めます。
変更をまたぐほど長く生きないクライアントには、比較の意味がないからです。
`RefreshSession` は、レスポンスが変更を報告したかどうかに関わらずセッションを取得します。
別の手段で変更を知ったクライアントのためのものです。

## 大きな /get の分割

サーバの `maxObjectsInGet` を超える数の id を並べた `/get` は拒否されます。
`WithSplitGets` を渡すと、これを複数のリクエストに分けて送り、答えを1つのレスポンスに連結します。

```go
c := jmapc.New(url, jmapc.WithBearerToken(token), jmapc.WithSplitGets())
```

既定では無効です。理由は2つあります。
一度の `Do` が複数の往復になること。
そしてレコードが1つのスナップショットではなくなることです。
各リクエストは別々に処理されるので、その間にアカウントの状態が変わりえます。
`/get` が報告する `state` がリクエスト間で異なった場合は、連結したレスポンスと一緒に `*jmapc.StateChanged` を返します。
`errors.As` で取り出せる、メソッドエラーと同じ扱い方です。
一貫したスナップショットが必要な呼び出し側は取得し直せますし、不要なら無視できます。

数えるのはリクエストに書かれた id だけで、次の2つはそのまま送ります。
id をバックリファレンスから得ている呼び出し。解決後の件数はサーバにしか分からないからです。
そして他の呼び出しから参照されている呼び出し。参照は1つのリクエスト内でしか解決せず、参照先を分割すると解決先がなくなるからです。

入りきらなかった id は、それ専用の後続リクエストで送ります。
1つのリクエストに入れる呼び出しの数は `maxCallsInRequest` を超えません。
リクエストの残りの部分は最初のリクエストで一度だけ送るので、他の呼び出し同士のバックリファレンスはこれまでどおり解決します。

## 期限切れのあるトークン

`WithBearerToken` は、クライアントの生存期間中ずっと1つの文字列を保持します。
OAuth 2.0 のアクセストークンはそこまで長く有効ではなく、差し替えるにはクライアントを作り直すことになります。
作り直すと、キャッシュしたセッションと、実行中のリクエスト数の管理も一緒に失われます。
`WithTokenSource` は、文字列の代わりに関数を受け取ります。

```go
c := jmapc.New(url, jmapc.WithTokenSource(func(ctx context.Context) (jmapc.Token, error) {
	tok, err := oauthConfig.TokenSource(ctx, refreshToken).Token()
	if err != nil {
		return jmapc.Token{}, err
	}
	return jmapc.Token{Value: tok.AccessToken, Expiry: tok.Expiry}, nil
}))
```

取得したトークンは、期限が切れるまで保持されます。
`Expiry` を返すソースは期限の少し前に再度呼ばれ、`Expiry` を返さないソースはサーバが 401 を返したときにだけ再度呼ばれます。
同時に発生したリクエストは1回の呼び出しを共有します。
リフレッシュトークンを交換するソースを同時に何度も実行しないためで、リフレッシュトークンを一度しか受け付けないサーバがあるからです。

401 を受けたときは、そのリクエストを新しいトークンで一度だけ送り直します。
2度目の 401 は呼び出し側に返します。
サーバが受け付けないトークンをソースが返している状態は、送り直しても解決しないからです。
これは `WithRetry` とは別の仕組みです。
`WithRetry` が送り直すのは、サーバが処理しなかったと報告したリクエストです。

## 再送

サーバがHTTP 429 と 503 を返す場合、`WithRetry` は再送を行います。

```go
c := jmapc.New(url, jmapc.WithBearerToken(token), jmapc.WithRetry(3))
```

引数は試行回数です。
待ち時間は、サーバが `Retry-After` で求めた長さです。
特に何も求められなければ、0.2 秒から 30 秒へ倍々に待ちます。

## 可観測性

`WithObserver` を渡すと、クライアントは送信したリクエストと待機した時間を `Observer` のフックに渡します。
フックに渡す値は送信内容にも受信内容にも影響せず、nil のままにしたフックは呼ばれません。

```go
c := jmapc.New(url, jmapc.WithBearerToken(token),
	jmapc.WithObserver(jmapc.SlogObserver(slog.Default())))
```

フックは3つあり、入れ子の関係にあります。
`SlogObserver` はこの3つそれぞれについて debug レベルのレコードを出力します。
`Request` は JMAP リクエスト1件に対応し、含まれる呼び出しと処理結果を持ちます。
`Attempt` はその内側の HTTP リクエスト1件に対応します。
最初のリクエストの前に行われるセッション取得と、再送の各回もここに含まれます。
`Wait` はリクエストを送信せずに待機した箇所です。
`maxConcurrentRequests` が許す枠の空きを待つ場合と、拒否されて指定された時間だけ待つ場合があります。

```
Request   Email/query, Email/get
  Attempt GET  /.well-known/jmap  200
  Wait    枠の空き待ち（サーバが同時に受け付けるのは 2 件）
  Attempt POST /jmap/api          429
  Wait    サーバの Retry-After が指定した 2 秒
  Attempt POST /jmap/api          200
```

このうち HTTP の往復は HTTP 層の計測でも取得できるので、それで足りるなら、トランスポートを差し替えた `http.Client` を `WithHTTPClient` に渡してください。
HTTP 層の計測で取得できないのは JMAP 側の情報です。
どのメソッドが同じリクエストに含まれていたか、呼び出し側が枠の空きを待った時間、そしてリクエストが 200 で返っていても呼び出しが拒否されていたこと、の3つです。

OpenTelemetry への依存はなく、追加する必要もありません。
処理を囲む2つのフックは、その処理に使う context を返します。
外側のフックで開始したスパンが、内側のフックで開始したスパンの親になります。

```go
tracer := otel.Tracer("jmapc")

obs := &jmapc.Observer{
	Request: func(ctx context.Context, info jmapc.RequestInfo) (context.Context, func(jmapc.ResponseInfo)) {
		methods := make([]string, len(info.Calls))
		for i, call := range info.Calls {
			methods[i] = call.Name
		}
		ctx, span := tracer.Start(ctx, "jmap.request",
			trace.WithAttributes(attribute.StringSlice("jmap.methods", methods)))
		return ctx, func(done jmapc.ResponseInfo) {
			span.SetAttributes(attribute.Int("jmap.method_errors", len(done.Errors)))
			if done.Err != nil {
				span.RecordError(done.Err)
			}
			span.End()
		}
	},
	Attempt: func(ctx context.Context, info jmapc.AttemptInfo) (context.Context, func(jmapc.AttemptInfo, jmapc.Answer)) {
		ctx, span := tracer.Start(ctx, "jmap."+string(info.Kind),
			trace.WithAttributes(attribute.Int("http.attempt", info.Attempt)))
		return ctx, func(info jmapc.AttemptInfo, answer jmapc.Answer) {
			span.SetAttributes(attribute.Int("http.status_code", answer.Status))
			span.End()
		}
	},
}
```

`Observer` は Go のクライアントにのみあります。
Rust と TypeScript では、同じ処理をトランスポートの実装に書きます。
