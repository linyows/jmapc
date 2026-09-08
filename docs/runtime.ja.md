<p align="right"><a href="runtime.md">English</a> | 日本語</p>

# ランタイム

## 実行時のエラー

JMAP は二つのレベルで失敗します。
ランタイムもそれに対応します。

**リクエストレベル**の失敗は、サーバがリクエスト全体を拒否した場合で、`*jmapc.RequestError` になります。
RFC 8620 §3.6.1 の problem type を持ちます。
このうちいくつかは送信前にクライアントが捕まえます。
セッションが広告していないケイパビリティや、サーバが受け付ける数を超える呼び出しなどです。

**メソッドレベル**の失敗は `jmapc.MethodErrors` になります。
JMAP は実行できる呼び出しを実行するので、レスポンスはエラーと一緒に返ります。
各エラーは、ワイヤフォーマット上の `"error"` ではなく、失敗したメソッド名と呼び出し id を報告します。

生成された関数も同じものを返します。
サーバが答えた呼び出しはデコードされ、実行されなかった呼び出しはゼロ値のまま残り、結果がエラーと一緒に返ります。
連鎖したリクエストはこの形で失敗するのが普通です。後続に値を渡す呼び出しが成功し、それを受ける呼び出しが参照を解決できずに失敗します。
そして理由を語っているのは、たいてい先に成功したほうの応答です。

```go
res, err := client.DestroyThread(ctx, c, params)
if err != nil {
    if len(res.ThreadGet.NotFound) > 0 {
        return fmt.Errorf("no such thread: %s", res.ThreadGet.NotFound[0])
    }
    return err
}
```

例外は `_returns` で一つの呼び出しを指定したリクエストです。
その呼び出しが答えのすべてなので、失敗したのがそれであれば返すものがなく、結果は nil になります。

TypeScript は値を返すのではなく throw するので、読み取った内容はエラーに載ります。
`MethodErrors` は元になったレスポンスを持ち、`result` にはリクエストの戻り値のうちサーバが答えた分が入ります。
サーバが実行しなかった呼び出しはそこに存在しないので、`Partial` として読んでください。

```ts
try {
  await destroyThread(client, params)
} catch (e) {
  if (e instanceof MethodErrors) {
    const partial = e.result as Partial<DestroyThreadResult>
    if (partial.threadGet?.notFound?.length) {
      throw new Error(`no such thread: ${partial.threadGet.notFound[0]}`)
    }
  }
  throw e
}
```

Rust も `Err` を返すので、同じくエラーに載ります。
`MethodErrors::result` が、そのリクエストの戻り値をそのまま返します。
サーバが実行しなかった呼び出しは、欠けるのではなく既定値のまま残ります。Rust には置いておける既定値があるからです。

```rust
if let Error::Method(failed) = &err {
    if let Some(out) = failed.result::<DestroyThreadResult>() {
        if !out.thread_get.not_found.is_empty() { /* どのスレッドが無かったか */ }
    }
}
```

第三のレベルがあり、見落とされるのはこれです。
`/set` は**エラーを含まない 200** を返しながら、処理を拒んだレコードを列挙します。

```json
["Email/set", {"notCreated": {"draft": {"type": "invalidProperties",
                                        "properties": ["subject"]}}}, "write"]
```

転送レベルのエラーだけを見ると、何も起きていないのに成功に見えます。
生成コードがこれを検査するので、拒否されたレコードは `*jmapc.SetErrors` になります。

```go
res, err := client.SendEmail(ctx, c, params)
if err != nil {
    var refused *jmapc.SetErrors
    if errors.As(err, &refused) {
        for _, f := range refused.Failures {
            log.Printf("%s: %v", f.Key, f.Err) // draft: invalidProperties [subject]
        }
    }
    return err
}
```

`res` はエラーと一緒に返ります。
サーバが実際に実行した部分は起きているからです。
リクエストが `_returns` で名指ししていない呼び出しも検査されます。
一つの呼び出しを名指ししたことで、他が見られなくなるべきではないからです。

TypeScript では同じ失敗が `SetErrors` の throw になり、レスポンスは `err.result` に載ります。
Rust では `Error::Set` になり、レスポンスは関数が返すはずだった型を指定して `err.result::<T>()` で取り出します。

### 一時的な失敗と恒久的な失敗

失敗はエラーとして届きますが、どう対処すべきかはサーバが何を述べたかで決まります。
呼び出し側がステータスコードやエラー型の文字列を自分で解きほぐさずに済むよう、三つの関数がそれを分類します。

```go
res, err := client.SendEmail(ctx, c, params)
if err != nil {
    if d, ok := jmapc.RetryAfter(err); ok {
        return job.again(d) // サーバがいつ戻ってくればよいか述べた
    }
    if jmapc.IsTemporary(err) {
        return job.again(backoff(job.attempts))
    }
    return job.fail(err) // リクエスト自体が誤っていて、送り直しても答えは同じ
}
```

`IsTemporary` は、サーバがリクエストについて述べたもの（429 以外の 4xx、`invalidArguments` や `invalidProperties` のようなメソッドやレコードのエラー）に対して false を返します。
サーバが自分自身について述べたもの（5xx、429、`serverUnavailable`、`serverFail`、`rateLimit`）には true を返します。
サーバに届かなかったリクエストのように分類できない失敗は一時的として扱います。
次の試みも失敗すると言えるものが何もないからです。
一つのリクエストが複数の理由で失敗した場合、時間で解消しない理由が一つでもあれば全体が恒久的な失敗になります。

これが答えるのは失敗にどう対処するかであって、そのリクエストを送り直して安全かではありません。
転送中に失敗したリクエストは実行済みかもしれませんし、`/set` を二度送れば二つ作られます。
`RetryPolicy` が既定で 429 と 503 だけを送り直すのはそのためです。

`IsRateLimited` は、サーバが「もっと少なく送れ」と述べている場合を取り出します。
これは三つの形で届きます。
429、呼び出しやレコードに対する `rateLimit`、そして `maxConcurrentRequests` や `maxConcurrentUpload` の超過を理由に 400 で拒否される場合です。

`RetryAfter` は、サーバが求めた待ち時間と、そもそも求めたかどうかを返します。
再送方針がもう一度の試行を許す場合、短い待ち時間はクライアントが自分で消化します。
長いものが呼び出し側に届くのはこの関数を通してで、一時間待てというサーバの求めは消化されないからです。

`HasErrorType` が答えるのは別の問いです。
失敗した中に特定のエラー型が含まれているかどうかを返します。
JMAP では、同じ事情がサーバの拒否した階層で報告されます。
大きすぎるリクエストはリクエスト全体が拒否され、アカウントの容量を超える `Email/set` は呼び出しが拒否され、容量を超えるメッセージが 1 通だけなら、そのレコードだけが拒否されて残りは実行されます。
`overQuota` に対して呼び出し側がすることは、三つとも同じであることがほとんどです。

```go
switch {
case jmapc.HasErrorType(err, jmapc.ErrOverQuota):
    return status(http.StatusInsufficientStorage)
case jmapc.HasErrorType(err, jmapc.ErrNotFound):
    return status(http.StatusNotFound)
case jmapc.HasErrorType(err, jmapc.ErrInvalidProperties):
    return status(http.StatusBadRequest)
}
```

型にはパッケージが宣言している定数を渡します。
呼び出しとレコードには `ErrNotFound` などを、リクエストには `ErrTypeLimit` などを使います。
後者は URI なので前者と取り違えることはありません。
サーバが独自に定義した型を文字列で渡すこともできます。
`/set` が複数のレコードを拒否した場合も全件を対象にするので、そのうち 1 件の型でも見つかります。

どのレコードがなぜ拒否されたかは `SetErrors.Failures` にあり、`errors.As` で取り出します。
JMAP のエラー型をどの HTTP ステータスに対応させるか、どれを再送するかは、アプリケーション側の対応表です。
jmapc が示すのはサーバの報告した内容までで、それが呼び出し側にとって何を意味するかは扱いません。

### Session オブジェクト

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

### 大きな /get の分割

サーバの `maxObjectsInGet` を超える数の id を並べた `/get` は拒否されます。
`WithSplitGets` を渡すと、これを複数のリクエストに分けて送り、答えを一つのレスポンスに連結します。

```go
c := jmapc.New(url, jmapc.WithBearerToken(token), jmapc.WithSplitGets())
```

既定では無効です。理由は二つあります。
一度の `Do` が複数の往復になること。
そしてレコードが一つのスナップショットではなくなることです。
各リクエストは別々に処理されるので、その間にアカウントの状態が変わりえます。
`/get` が報告する `state` がリクエスト間で異なった場合は、連結したレスポンスと一緒に `*jmapc.StateChanged` を返します。
`errors.As` で取り出せる、メソッドエラーと同じ扱い方です。
一貫したスナップショットが必要な呼び出し側は取得し直せますし、不要なら無視できます。

数えるのはリクエストに書かれた id だけで、次の二つはそのまま送ります。
id をバックリファレンスから得ている呼び出し。解決後の件数はサーバにしか分からないからです。
そして他の呼び出しから参照されている呼び出し。参照は一つのリクエスト内でしか解決せず、参照先を分割すると解決先がなくなるからです。

入りきらなかった id は、それ専用の後続リクエストで送ります。
一つのリクエストに入れる呼び出しの数は `maxCallsInRequest` を超えません。
リクエストの残りの部分は最初のリクエストで一度だけ送るので、他の呼び出し同士のバックリファレンスはこれまでどおり解決します。

### 期限切れのあるトークン

`WithBearerToken` は、クライアントの生存期間中ずっと一つの文字列を保持します。
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
同時に発生したリクエストは一回の呼び出しを共有します。
リフレッシュトークンを交換するソースを同時に何度も実行しないためで、リフレッシュトークンを一度しか受け付けないサーバがあるからです。

401 を受けたときは、そのリクエストを新しいトークンで一度だけ送り直します。
二度目の 401 は呼び出し側に返します。
サーバが受け付けないトークンをソースが返している状態は、送り直しても解決しないからです。
これは `WithRetry` とは別の仕組みです。
`WithRetry` が送り直すのは、サーバが処理しなかったと報告したリクエストです。

### 再送

サーバがHTTP 429 と 503 を返す場合、`WithRetry` は再送を行います。

```go
c := jmapc.New(url, jmapc.WithBearerToken(token), jmapc.WithRetry(3))
```

引数は試行回数です。
待ち時間は、サーバが `Retry-After` で求めた長さです。
特に何も求められなければ、0.2 秒から 30 秒へ倍々に待ちます。

### 可観測性

`WithObserver` を渡すと、クライアントは送信したリクエストと待機した時間を `Observer` のフックに渡します。
フックに渡す値は送信内容にも受信内容にも影響せず、nil のままにしたフックは呼ばれません。

```go
c := jmapc.New(url, jmapc.WithBearerToken(token),
	jmapc.WithObserver(jmapc.SlogObserver(slog.Default())))
```

フックは三つあり、入れ子の関係にあります。
`SlogObserver` はこの三つそれぞれについて debug レベルのレコードを出力します。
`Request` は JMAP リクエスト一件に対応し、含まれる呼び出しと処理結果を持ちます。
`Attempt` はその内側の HTTP リクエスト一件に対応します。
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
どのメソッドが同じリクエストに含まれていたか、呼び出し側が枠の空きを待った時間、そしてリクエストが 200 で返っていても呼び出しが拒否されていたこと、の三つです。

OpenTelemetry への依存はなく、追加する必要もありません。
処理を囲む二つのフックは、その処理に使う context を返します。
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


## Blob

添付ファイルは API エンドポイントを通りません。
セッションが広告する URL に対して、素の HTTP でアップロードとダウンロードを行います。
ランタイムが両方を扱います。

```go
info, err := c.Upload(ctx, accountID, "application/pdf", file)
// info.BlobID を Email/set に渡して添付する。

blob, err := c.Download(ctx, accountID, part.BlobID, &jmapc.DownloadOptions{
	Name: *part.Name,
	Type: part.Type,
})
defer blob.Close()
```

どちらもストリーミングです。
`Upload` は `io.Reader` から読み、`Download` は `io.ReadCloser` を返します。
メモリより大きい添付ファイルも、ファイルからサーバへ、サーバからファイルへ、どちらの向きも保持せずに転送できます。
サーバが受け付けると表明したサイズを超えるアップロードは、送信前に失敗します。

`From` と `Length` で blob の一部だけを取得できます。
途中で中断したダウンロードを再開する方法です。

```go
blob, err := c.Download(ctx, accountID, blobID, &jmapc.DownloadOptions{From: written})
...
blob.Range // 返ってきた範囲。8192 バイト中の 4096-8191
```

これらは HTTP の `Range` ヘッダとして送ります。
JMAP はダウンロードエンドポイントに Range を定義していないので、サーバがこれを無視して blob 全体を返すことがあります。
その場合はダウンロードを失敗させます。
呼び出し側が誤ったオフセットに書き込む内容を返すよりよいからです。

`urn:ietf:params:jmap:blob` を提供するサーバでは、API 経由で blob を作成し読み取ることもできます。
エンドポイントにはできないことです。
`Blob/upload` は blob を、それを使う呼び出しと同じリクエストに置けるので、id がクライアントに戻ってきません。
