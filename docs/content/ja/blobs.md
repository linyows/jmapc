# Blob

添付ファイルはAPIエンドポイントを通りません。
セッションが広告するURLに対して、素のHTTPでアップロードとダウンロードを行います。
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
`Upload`は`io.Reader`から読み、`Download`は`io.ReadCloser`を返します。
メモリより大きい添付ファイルも、ファイルからサーバへ、サーバからファイルへ、どちらの向きも保持せずに転送できます。
サーバが受け付けると表明したサイズを超えるアップロードは、送信前に失敗します。

`From`と`Length`でblobの一部だけを取得できます。
途中で中断したダウンロードを再開する方法です。

```go
blob, err := c.Download(ctx, accountID, blobID, &jmapc.DownloadOptions{From: written})
...
blob.Range // 返ってきた範囲。8192 バイト中の 4096-8191
```

これらはHTTPの`Range`ヘッダとして送ります。
JMAPはダウンロードエンドポイントにRangeを定義していないので、サーバがこれを無視してblob全体を返すことがあります。
その場合はダウンロードを失敗させます。
呼び出し側が誤ったオフセットに書き込む内容を返すよりよいからです。

`urn:ietf:params:jmap:blob`を提供するサーバでは、API経由でblobを作成し読み取ることもできます。
エンドポイントにはできないことです。
`Blob/upload`はblobを、それを使う呼び出しと同じリクエストに置けるので、idがクライアントに戻ってきません。
