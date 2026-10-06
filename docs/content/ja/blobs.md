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
HTTPは、一部を返すサーバが要求に合わない部分を返すことを認めていませんが、Rangeの扱いを誤ったサーバは返すことがあります。
要求に合わない部分とは、別の位置から始まるもの、要求より後ろまで続くもの、どこに当たるかを示さないものです。
どちらの場合もダウンロードを失敗させます。
呼び出し側が誤ったオフセットに書き込む内容を返すよりよいからです。
要求した位置から始まり、要求より手前で終わる部分は、正しい位置に書き込めるので返します。
どこまで返ってきたかは`blob.Range`でわかります。
この失敗は`IsRangeIgnored`で判別できます。
何度要求してもサーバの答えは変わらないので、再開できないダウンロードは最初からやり直します。

```go
blob, err := c.Download(ctx, accountID, blobID, &jmapc.DownloadOptions{From: written})
if jmapc.IsRangeIgnored(err) {
	// サーバがRangeに対応していない。blob全体をダウンロードし直す。
	written = 0
	blob, err = c.Download(ctx, accountID, blobID, nil)
}
```

`urn:ietf:params:jmap:blob`を提供するサーバでは、API経由でblobを作成し読み取ることもできます。
エンドポイントにはできないことです。
`Blob/upload`はblobを、それを使う呼び出しと同じリクエストに置けるので、idがクライアントに戻ってきません。
