<p align="right"><a href="cli.md">English</a> | 日本語</p>

# jmapc コマンド

## リクエストを送る

リクエストは、それを呼ぶコードができる前に試せるほうがよいので、`jmapc run` が一つ送って、返ってきたものを表示します。

```
jmapc run ListInboxEmails -p mailboxId=mbx1 -p limit=25
```

値は型の言うとおりに書きます。
`String` や `Id` はテキストそのものなので、シェルの先で引用符を付ける必要はなく、形を持つものは JSON で書きます。
型が受け付けない値は、何かが送られる前に拒まれます。

```
jmapc: parameter limit: "soon" is not a whole number
```

サーバは `-session` で指定します。
セッションの URL でも、それが置かれているホスト名でも構いません。
資格情報は `-token` か `-user` です。
いずれも環境変数 `$JMAP_SESSION_URL`、`$JMAP_TOKEN`、`$JMAP_USER` にフォールバックするので、トークンをシェルの履歴に残さずに済みます。
リクエストが省いた account id は、生成された関数がそうするのと同じように、セッションから引かれます。
`-account` を渡せばそちらが使われます。

`-dry-run` は、送る代わりにリクエストを表示します。
生成された関数が組み立てるのと同じリクエストで、サーバが予期しない答えを返したときに見るべきものです。

```
jmapc run MarkEmailRead -dry-run -p emailId=m1
{
  "using": [
    "urn:ietf:params:jmap:core",
    "urn:ietf:params:jmap:mail"
  ],
  "methodCalls": [
    [
      "Email/set",
      {
        "accountId": "ACCOUNT_ID",
        "update": {
          "m1": {
            "keywords/$seen": true
          }
        }
      },
      "mark"
    ]
  ]
}
```

account id だけは、dry run には知りようがありません。
取りにいかないセッションから来る値だからです。
そこで `ACCOUNT_ID` がその場に立ち、そのことを標準エラーに書きます。

実行は、生成されたコードと同じようにレスポンスを読みます。
200 で拒否を返す `/set` はここでもエラーで、それを運んできたレスポンスを表示した後に報告されます。

## 生成物が最新かを確かめる

生成されたクライアントはリポジトリにコミットするものなので、生成元のリクエストから遅れることがあります。
リクエストを直して生成し直さなかった場合や、jmapc を上げて生成し直さなかった場合です。
`generate -check` は、ディスクにあるものと、いま生成したら書かれるものとを比べます。
何も書かず、両者が違えば失敗します。

```
jmapc generate -check
client/listinboxemails_gen.go: out of date
client/oldreport_gen.go: generated from a request that is no longer there
jmapc: 2 files are out of date; run jmapc generate
```

報告するのは三つです。

- リクエストがいま生成するものと違うファイル
- まだ生成されていないファイル
- jmapc が以前書いたが、どのリクエストももう生成しないファイル（リクエストファイルを消したときに残るもの）

生成されたことを示す先頭行を持たないファイルは手で書かれたものなので、報告もしませんし、触りもしません。

検査する生成と同じ引数を渡してください。
リクエストの置かれていたパスは、そこから生成されたファイルに書き込まれるので、`-requests` に別のパスを渡して検査すると、すべてのファイルが古いと報告されます。

ワークフローでは次のように書きます。

```yaml
- run: go tool jmapc generate -check
```

## 設定

フラグで指定するか、モジュールの隣に `jmapc.json` を置きます。

```json
{
  "requests": "requests",
  "out": "internal/client",
  "package": "client",
  "schemas": ["schema/notes.json"]
}
```

`requests` ディレクトリにはリクエストを 1 ファイルに 1 つずつ置きます。
リクエストが求めるプロパティ集合を宣言する `properties.json` を置くこともできます。
[名前付きのプロパティ集合](requests.ja.md#名前付きのプロパティ集合)を参照してください。
