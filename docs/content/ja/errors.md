# エラー

JMAP は2つのレベルで失敗します。
生成された関数が返すエラーも、この2つに対応します。

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

例外は `_returns` で1つの呼び出しを指定したリクエストです。
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

第3のレベルがあり、見落とされるのはこれです。
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
1つの呼び出しを名指ししたことで、他が見られなくなるべきではないからです。

TypeScript では同じ失敗が `SetErrors` の throw になり、レスポンスは `err.result` に載ります。
Rust では `Error::Set` になり、レスポンスは関数が返すはずだった型を指定して `err.result::<T>()` で取り出します。

## 一時的な失敗と恒久的な失敗

失敗はエラーとして届きますが、どう対処すべきかはサーバが何を述べたかで決まります。
呼び出し側がステータスコードやエラー型の文字列を自分で解きほぐさずに済むよう、3つの関数がそれを分類します。

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
1つのリクエストが複数の理由で失敗した場合、時間で解消しない理由が1つでもあれば全体が恒久的な失敗になります。

これが答えるのは失敗にどう対処するかであって、そのリクエストを送り直して安全かではありません。
転送中に失敗したリクエストは実行済みかもしれませんし、`/set` を2度送れば2つ作られます。
`RetryPolicy` が既定で 429 と 503 だけを送り直すのはそのためです。

`IsRateLimited` は、サーバが「もっと少なく送れ」と述べている場合を取り出します。
これは3つの形で届きます。
429、呼び出しやレコードに対する `rateLimit`、そして `maxConcurrentRequests` や `maxConcurrentUpload` の超過を理由に 400 で拒否される場合です。

`RetryAfter` は、サーバが求めた待ち時間と、そもそも求めたかどうかを返します。
再送方針がもう一度の試行を許す場合、短い待ち時間はクライアントが自分で消化します。
長いものが呼び出し側に届くのはこの関数を通してで、1時間待てというサーバの求めは消化されないからです。

`HasErrorType` が答えるのは別の問いです。
失敗した中に特定のエラー型が含まれているかどうかを返します。
JMAP では、同じ事情がサーバの拒否した階層で報告されます。
大きすぎるリクエストはリクエスト全体が拒否され、アカウントの容量を超える `Email/set` は呼び出しが拒否され、容量を超えるメッセージが 1 通だけなら、そのレコードだけが拒否されて残りは実行されます。
`overQuota` に対して呼び出し側がすることは、3つとも同じであることがほとんどです。

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
