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
