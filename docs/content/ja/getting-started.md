# はじめてのクライアント

このページでは、空のGoモジュールから、メールボックスを読む呼び出しを書くところまでを順に説明します。
RustとTypeScriptのプロジェクトも、`-lang`を変えるだけで手順は同じです。
違う点は[RustとTypeScript](languages.md)にあります。

## jmapcのインストール

クライアントはgo generateで生成するので、それを使うモジュールにツールとして記録します。

```
go get -tool github.com/linyows/jmapc/cmd/jmapc
```

これでバージョンが`go.mod`に固定され、`go tool jmapc`で実行できます。
そのプロジェクトをビルドする全員とCIが同じバージョンで生成することになります。
生成物をコミットするツールでは、これが重要です。

PATHに置きたい場合は次のようにします。

```
go install github.com/linyows/jmapc/cmd/jmapc@latest
```

RustやTypeScriptのプロジェクトには`go tool`を動かすGoツールチェインがないので、[リリース](https://github.com/linyows/jmapc/releases)からバイナリを取得してください。

## リクエストを書く

リクエストは`requests/`の下に1ファイルに1つずつ置きます。
ファイル名が、生成される関数の名前になります。

```json
{
  "_doc": "ListInboxEmails returns the newest emails in one mailbox.",

  "methodCalls": [
    ["Email/query", {
      "_comment": "該当するメールの id を探す。",
      "filter": {"inMailbox": "{{mailboxId}}"},
      "sort": [{"property": "receivedAt", "isAscending": false}],
      "limit": "{{limit}}"
    }, "search"],

    ["Email/get", {
      "_comment": "id からメッセージを取得する。",
      "#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"},
      "properties": ["id", "subject", "from", "receivedAt"]
    }, "fetch"]
  ],

  "_returns": "fetch"
}
```

(`requests/ListInboxEmails.jmap.json`)

アンダースコアで始まらない部分は、RFC 8620が定めるJMAPのリクエストそのものです。
`{{mailboxId}}`と`{{limit}}`は呼び出し側が値を埋めるパラメータです。
`_returns`によって`Email/get`のレスポンスが関数の戻り値になります。
`accountId`は省略しているので、生成された関数がセッションから補います。
それぞれの詳細は[リクエストの書き方](requests.md)にあります。

## クライアントの生成

`go.mod`と同じディレクトリに、次のディレクティブを置きます。

```go
package mail

//go:generate go tool jmapc generate
```

生成します。

```
go generate ./...
```

jmapcはまずリクエストを仕様に照らして検証します。
JMAPとして誤っているリクエストはここで止まり、綴り間違いには候補が示されます。

```
requests/ListInboxEmails.jmap.json: methodCalls[1].arguments.properties[1]: Email has no property "subjct"
	did you mean "subject"?
```

検証を通ったリクエストからは、`client`パッケージの`client/listinboxemails_gen.go`が生成されます。
このディレクトリ構成は既定値で、`jmapc.json`で変えられます。
[jmapcコマンド](cli.md#設定)を参照してください。
呼び出すコードを書く前にリクエストをサーバに送って答えを確かめるには、`jmapc run`を使います。
[リクエストを送る](run.md)を参照してください。

## 呼び出し

```go
c := jmapc.New(jmapc.WellKnownURL("example.com"), jmapc.WithBearerToken(token))

res, err := client.ListInboxEmails(ctx, c, client.ListInboxEmailsParams{
	MailboxID: inbox,
	Limit:     25,
})
if err != nil {
	return err
}
for _, email := range res.List {
	fmt.Println(email.ReceivedAt, email.From[0].Email, *email.Subject)
}
```

`jmapc.New`には、セッションのURLと、認証などのオプションを渡します。
`WellKnownURL`は、ホスト名からセッションのURLを作ります。
オプションは[クライアントの設定](client.md)にあります。

`res.List`の型は`[]client.ListInboxEmailsFetchEmail`で、リクエストが要求した4つのプロパティだけを持ちます。
リクエストにプロパティを足して生成し直せば、構造体にフィールドが増えます。
生成される型の名前はファイル名とcall idから決まります。
[生成される名前](generated-code.md)を参照してください。

`err`はHTTPリクエストの失敗だけではありません。
JMAPは200を返すレスポンスの中でも失敗を報告し、生成された関数はそれもエラーとして返します。
[エラー](errors.md)を参照してください。

## 生成物を最新に保つ

生成されたファイルはコミットするので、リクエストを変えたあと生成し直さなければ、リクエストより古いままになります。
`generate -check`は何も書き出さず、ディスク上のファイルがいまのリクエストから生成されるものと違えば失敗します。
CIでは次のように実行します。

```yaml
- run: go tool jmapc generate -check
```

## 次に読むページ

「リクエスト」の各ページは、リクエストファイルに書けることと、jmapcが何を検証するかを説明します。
「生成されたコード」の各ページは、生成されたクライアントが実行時に何をするかを説明します。
エラー、プッシュ、ページング、Blob、クライアントのオプション、そして自分のコードをJMAPサーバに対してテストする方法です。
[`example/requests`](https://github.com/linyows/jmapc/tree/main/example/requests)には、メール、連絡先、カレンダー、共有、フィルタにまたがる25個のリクエストがあります。
