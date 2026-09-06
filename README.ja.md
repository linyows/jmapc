<p align="right"><a href="https://github.com/linyows/jmapc/blob/main/README.md">English</a> | 日本語</p>

<p align="center">
  <br><br><br>
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://github.com/linyows/jmapc/blob/main/misc/jmapc-dark-bg.svg?raw=true">
    <img alt="jmapc" src="https://github.com/linyows/jmapc/blob/main/misc/jmapc.svg?raw=true" width="280">
  </picture>
  <br><br><br>
</p>

<p align="center">
  <strong>jmapc</strong> は JMAP のコンパイラです。リクエストを書けば、クライアントが生成されます。
</p>

<p align="center">
  <a href="https://github.com/linyows/jmapc/actions/workflows/test.yml">
    <img alt="GitHub Workflow Status" src="https://img.shields.io/github/actions/workflow/status/linyows/jmapc/test.yml?branch=main&style=for-the-badge&labelColor=666666">
  </a>
  <a href="https://github.com/linyows/jmapc/releases">
    <img alt="GitHub Release" src="http://img.shields.io/github/release/linyows/jmapc.svg?style=for-the-badge&labelColor=666666&color=DDDDDD">
  </a>
  <a href="https://pkg.go.dev/github.com/linyows/jmapc">
    <img alt="Go Documentation" src="http://img.shields.io/badge/go-docs-blue.svg?style=for-the-badge&labelColor=666666&color=DDDDDD">
  </a>
  <a href="https://deepwiki.com/linyows/jmapc">
    <img alt="Deepwiki Documentation" src="http://img.shields.io/badge/deepwiki-docs-purple.svg?style=for-the-badge&labelColor=666666&color=DDDDDD">
  </a>
</p>

jmapc は JMAP のコンパイラです。
サーバに答えてほしいリクエストを、仕様がすでに定めている JSON で書くと、jmapc はそれを仕様に照らして検証したうえで、Go、Rust、TypeScript の**型安全なクライアント**を生成します。

1. JMAP でリクエストを書きます。
1. jmapc を実行して、そのリクエストに型安全なインターフェースを持つコードを生成します。
1. 生成されたコードを呼ぶアプリケーションコードを書きます。

リクエスト `requests/ListInboxEmails.jmap.json` は次のように書きます。

```json
{
  "methodCalls": [
    ["Email/query", {"filter": {"inMailbox": "{{mailboxId}}"}, "limit": "{{limit}}"}, "search"],
    ["Email/get", {"#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"},
                   "properties": ["id", "subject", "from", "receivedAt"]}, "fetch"]
  ],
  "_returns": "fetch"
}
```

`jmapc generate` はこれを次のように呼べるコードにします。

```go
res, err := client.ListInboxEmails(ctx, c, client.ListInboxEmailsParams{
	MailboxID: inbox,
	Limit:     25,
})
```

`res.List` は、リクエストが要求した四つのプロパティだけを持ちます。

## 機能

jmapc を特徴づける機能は、次の五つにまとめられます。

- リクエストは jmapc 独自の API ではなく、JMAP そのもので書きます
- リクエストの検証は、コードを生成する前、エディタで書いている最中、そして稼働中のサーバに対しても行えます
- 生成されるコードは、型安全なレスポンス、網羅的なエラー処理、プッシュとページングのループを備えます
- 一つのリクエストから Go、Rust、TypeScript のクライアントを生成でき、外部への依存は最低限です
- 実際にリクエストを送信するコマンドやテスト用の JMAP サーバ、そして jmapc が知らないケイパビリティを足すスキーマファイルが付属します

## 動機

JMAP は一つのリクエスト中に複数のメソッド呼び出しを記述でき、後の呼び出しは前の呼び出しの結果を参照できます。
依存関係のある一連の操作が、一往復で済みます。

```json
{
  "using": ["urn:ietf:params:jmap:core", "urn:ietf:params:jmap:mail"],
  "methodCalls": [
    ["Email/query", {"filter": {"inMailbox": "mbx1"}}, "search"],
    ["Email/get", {"#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"}}, "fetch"]
  ]
}
```

一つ目のメソッドの戻りで得られるid はクライアントに戻ってきません。
この設計が、JMAP クライアントが REST クライアントのような形、つまりリソースごとの型とパスごとのメソッドという形にならない理由です。

ほとんどの JMAP クライアントはこれをビルダーとして提供します。
この場合、利用者は JMAP に加えてビルダーの使い方も覚えることになります。
しかし利用者の関心があるのはJMAPリクエストであって、クライアントの使い方ではありません。
であれば、リクエストだけを書いて、クライアントは jmapc に書かせればよいはずです。
このアイデアは、SQL における [sqlc](https://sqlc.dev) から着想を得たものです。

## 既存の JMAP クライアントとの違い

いま使われている JMAP クライアントはライブラリです。
Go の [go-jmap](https://github.com/rockorager/go-jmap)、Rust の [jmap-client](https://github.com/stalwartlabs/jmap-client)、TypeScript の [Jam](https://github.com/htunnicliff/jmap-jam)、そして [jmap.io が挙げている](https://jmap.io/software/)ものがあります。
いずれもプロトコルを、実行時に呼ぶ API として提供します。
jmapc が動くのは、それより前、コードを生成する時点です。
違いは次の三つです。

**リクエストを、ライブラリ独自の API ではなく JMAP で書きます。**
ビルダーは、リクエストを独自の書き方で表現します。
Go なら `jmap.ResultReference{ResultOf, Name, Path}` を持たせた `req.Invoke(&email.Get{...})`、Rust なら `client.build()` と `.updated_reference()` です。
利用者は JMAP を覚え、そのうえでそのライブラリでの書き方を覚えることになります。
jmapc のリクエストファイルは RFC 8620 が定めるリクエストオブジェクトそのもので、それ以上のものはありません。
仕様からそのまま持ってこられますし、そのまま `jmapc run` で送れて、`jq` で読めて、エディタで補完できます。

**間違いはプログラムを動かす前に見つかります。**
ライブラリが検査できるのは、その型が表しているもの、つまりフィールドが存在すること、型が合っていることです。
しかし JMAP のリクエストが正しいかどうかは、その多くが値によって決まります。
参照先の呼び出しを名指す `"name": "Email/query"`、そこから値を選ぶ `"path": "/ids"`、`properties` に並ぶ名前、フィルタの条件、パッチのポインタです。
ライブラリにとってこれらは文字列でしかないので、間違いはサーバからのエラーとして返ってきます。
jmapc は生成時にこれらをデータモデルに照らして検証するので、参照先のメソッド名を間違えた結果参照はビルドを失敗させます。

**レスポンスはリクエストが要求したものを持ちます。**
go-jmap は `Invocation.Args` を `any` で返すので、型スイッチで取り出します。
jmap-client は `unwrap_method_responses()` から `unwrap_get_mailbox()` へと unwrap を連ねます。
生成された関数は、リクエストが並べたプロパティだけを持つ名前の付いた型を返します。
TypeScript の Jam は同じことを実現しています。リテラル型が `properties` の指定でレスポンスの型を絞れるからです。
jmapc は、型システムだけではそれができない Go と Rust でも同じ絞り込みを得ます。

引き換えになるのは、リクエストが jmapc の実行時に固定されることです。
実行時に形が決まるリクエスト、たとえば利用者の入力から組み立てるフィルタは、呼び出しを一つずつ組み立てるのではなく、一つのパラメータとして渡すことになります。
任意のリクエストを組み立てるプログラムは、ビルダーの領分です。
生成されたコードも、リポジトリで管理するコードです。ビルドに一手間が加わり、生成物をコミットすることになります。

## インストール

go generateでクライアントを生成するため、モジュールに追加しておきます。

```
go get -tool github.com/linyows/jmapc/cmd/jmapc
```

これでバージョンが `go.mod` に固定され、`go tool jmapc` で実行できます。
そのプロジェクトをビルドする全員と CI が同じバージョンで生成することになります。

```go
//go:generate go tool jmapc generate
```

PATH に置きたい場合は次のようにします。

```
go install github.com/linyows/jmapc/cmd/jmapc@latest
```

Go のツールチェインがない環境では、[リリース](https://github.com/linyows/jmapc/releases)からバイナリを取得してください。
Rust や TypeScript のプロジェクトではこちらを使います。

## 使い方

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
      "_comment": "idからメッセージを取得する。",
      "#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"},
      "properties": ["id", "subject", "from", "receivedAt"]
    }, "fetch"]
  ],

  "_returns": "fetch"
}
```

(`requests/ListInboxEmails.jmap.json`)

生成します。

```
jmapc generate                 # または go generate ./...
```

呼び出します。

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

`res.List` の型は `[]ListInboxEmailsFetchEmail` で、リクエストが要求した四つのプロパティだけを持ちます。
別のプロパティを要求すれば構造体が増え、存在しないプロパティを要求すればビルドが失敗し、候補が提示されます。

`bodyProperties` はメッセージのパートを同じように絞り込み、その絞り込みは入れ子のサブパートにも及びます。
ヘッダフィールドを指すプロパティは、要求した形式によって型が決まります。
`header:List-Id:asText` は `*string`、`header:To:asAddresses` は `[]jmapc.EmailAddress` です。

生成されるコードの名前はファイル名から、その中の名前は呼び出し id から決まります。
[リクエストの書き方](docs/requests.ja.md#生成される名前)を参照してください。
[`example/requests`](example/requests) には、メール、連絡先、カレンダー、共有、フィルタにまたがる 25 個のリクエストがあります。

## 他の言語

同じリクエストから Rust や TypeScript のクライアントも生成できます。

```
jmapc generate -lang rust -out src/jmap_client
jmapc generate -lang typescript -out src/jmapClient
```

ランタイムはリクエストと一緒に生成されます。
Rust の出力が要求するのは serde だけで、TypeScript の出力に依存はなく、必要なのは `fetch` だけです。
生成される名前はそれぞれの言語の綴り方に従い、null を取りうるプロパティと複数の形を取る値は、どちらも Go より正確に表現されます。
詳しくは[他の言語](docs/languages.ja.md)を参照してください。

## 検証

リクエストは jmapc の実行時に検証されるので、JMAP として誤っているリクエストは、サーバへの往復ではなくビルドの失敗になります。
メソッドが存在すること、引数がそのメソッドのものであること、結果参照が先行する呼び出しを指し、参照先の引数が受け取れる値を選んでいること、フィルタ、`properties`、`sort`、パッチのポインタがその型の持つものを指していること、といった検証です。
綴り間違いには候補が示されます。

```
requests/BadQuery.jmap.json: methodCalls[0].arguments.filter.hasAttachmnt: EmailFilterCondition has no property "hasAttachmnt"
	did you mean "hasAttachment"?
```

`jmapc check` は何も書き出さずに検証だけを行います。
`-session` を付けると、稼働中のサーバにしか分からないこと、つまりサーバが広告するケイパビリティ、保持するアカウント、一度のリクエストに受け付ける量も検証されます。
検証の多くは、jmapc が書き出す JSON Schema を使ってエディタでも動きます。
検証の一覧は[検証](docs/verification.ja.md)にあります。

## ドキュメント

ここまでが jmapc の全体像です。
残りは `docs/` の下に、主題ごとに一つのファイルとして置いてあります。
並びは読む順序でもあります。

はじめの三つは、リクエストを書いているあいだ手元に置くものです。
リクエストファイルに何を書けるか、生成の前に jmapc が何を検証するか、そして呼び出すコードを書く前にリクエストを送って答えを見る方法です。
続く四つは、生成されたコードが実行時に何を呼ぶかを説明します。
エラー、blob、サーバが送ってくる変更、一部ずつ返ってくる答え、そして自分のコードをテストするためのサーバです。
最後の四つはリファレンスです。
Rust と TypeScript のクライアント、jmapc が知らないケイパビリティ、知っているケイパビリティ、そして jmapc 自体です。

| | |
|---|---|
| [リクエストの書き方](docs/requests.ja.md) | リクエストファイル、パラメータ、そこから生成される名前 |
| [検証](docs/verification.ja.md) | ビルド時、サーバに対して、エディタで、それぞれ何が検証されるか |
| [jmapc コマンド](docs/cli.ja.md) | `jmapc run` でリクエストを送る方法と設定 |
| [ランタイム](docs/runtime.ja.md) | エラー、大きな `/get`、トークン、再送、可観測性、blob |
| [プッシュ](docs/push.ja.md) | サーバが報告する変更を追い続ける |
| [一度のリクエストに収まらない答えを読み通す](docs/paging.ja.md) | 一部ずつ返ってくる結果を最後まで読む |
| [テスト](docs/testing.ja.md) | 自分のコードをテストするための JMAP サーバ、jmaptest |
| [他の言語](docs/languages.ja.md) | Rust と TypeScript のクライアント |
| [ベンダ拡張](docs/extensions.ja.md) | jmapc が知らない型とメソッドをスキーマファイルに記述する |
| [対応範囲](docs/coverage.ja.md) | 対応しているケイパビリティとメソッド |
| [jmapc の開発](docs/contributing.ja.md) | jmapc 自体のビルド、テスト、リリース |

## 対応範囲

jmapc は [IANA が挙げている](https://www.iana.org/assignments/jmap/jmap.xhtml) JMAP のケイパビリティをすべてサポートします。
core、mail、submission、contacts、calendars、principals、sieve、quota、blob などで、そこに含まれる 81 個のメソッドはすべて同じように検証され、生成されます。
そこに含まれないケイパビリティも、[スキーマファイル](docs/extensions.ja.md)に記述すれば他と同じように検証されます。
ケイパビリティごとの内容と、jmapc があえて検証しない一つのことは[対応範囲](docs/coverage.ja.md)にあります。

## Author

[linyows](https://github.com/linyows)
