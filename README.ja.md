<p align="right"><a href="https://github.com/linyows/jmapc/blob/main/README.md">English</a> | 日本語</p>

<h1 align="center">jmapc</h1>

<p align="center">
  <strong>jmapc</strong> は JMAP のコンパイラです。クエリを書けば、クライアントが生成されます。
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

jmapc は JMAP から**型安全な Go のコード**を生成します。

1. JMAP でクエリを書きます。
1. jmapc を実行して、そのクエリに型安全なインターフェースを持つコードを生成します。
1. 生成されたコードを呼ぶアプリケーションコードを書きます。

実際に動くものは[サンプル](example/)に、作った動機は[なぜ作ったか](#なぜ作ったか)にあります。

## なぜ作ったか

`jmapc` が JMAP に対して果たす役割は、[sqlc](https://sqlc.dev) が SQL に対して果たす役割と同じです。
理由もほとんど同じです。

JMAP は一つの考え方の上に組み立てられています。
一つのリクエストが複数のメソッド呼び出しを運び、後の呼び出しは前の呼び出しの結果を参照できます。
依存関係のある一連の操作が、一往復で済みます。

```json
["Email/query", {"filter": {"inMailbox": "mbx1"}}, "search"],
["Email/get",   {"#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"}}, "fetch"]
```

id はクライアントに戻ってきません。
これがプロトコルの眼目であり、JMAP クライアントが REST クライアントのような形、つまりリソースごとの型とパスごとのメソッドという形にならない理由でもあります。

多くのクライアントはこれをビルダーとして提供します。
すると利用者は JMAP に加えてビルダーの使い方も覚えることになります。
しかし関心があるのはクエリであって、クライアントではありません。
であれば、クエリだけを書いて、クライアントはツールに書かせればよいはずです。

見返りに得られるのは、手で書くと面倒で間違えやすい部分です。
結果参照は参照先のメソッドに照らして検証され、引数はデータモデルに、プロパティ名は型に照らして検証されます。
レスポンスは、クエリが要求したプロパティだけを持つ構造体にデコードされます。

## インストール

ジェネレータは Go のツールで、生成するものも Go です。
使う側のモジュールに記録しておくのが基本です。

```
go get -tool github.com/linyows/jmapc/cmd/jmapc
```

これでバージョンが `go.mod` に固定され、`go tool jmapc` で実行できます。
そのプロジェクトをビルドする全員と CI が同じバージョンで生成することになります。
生成物をコミットするツールでは、これが効いてきます。

```go
//go:generate go tool jmapc generate
```

PATH に置きたい場合は次のようにします。

```
go install github.com/linyows/jmapc/cmd/jmapc@latest
```

Go のツールチェインがない環境では、[リリース](https://github.com/linyows/jmapc/releases)からバイナリを取得してください。

## 使い方

クエリを書きます。
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
      "_comment": "同じリクエストで取得する。id が往復することはない。",
      "#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"},
      "properties": ["id", "subject", "from", "receivedAt"]
    }, "fetch"]
  ],

  "_returns": "fetch"
}
```

(`queries/ListInboxEmails.jmap.json`)

生成します。

```
jmapc generate                 # または go generate ./...
```

呼び出します。

```go
c := jmapc.New(jmapc.WellKnownURL("example.com"), jmapc.WithBearerToken(token))

res, err := jmapq.ListInboxEmails(ctx, c, jmapq.ListInboxEmailsParams{
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

`res.List` の型は `[]ListInboxEmailsEmail` で、クエリが要求した四つのプロパティだけを持ちます。
別のプロパティを要求すれば構造体が増え、存在しないプロパティを要求すればビルドが失敗し、候補が提示されます。

### 一つのリクエストで複数の手順を踏む

次の例が JMAP の本領です。
メッセージを作り、送信し、下書きから移す。
この三つは途中で分断されてはならない操作であり、ここでは一つのリクエストになっています。

```json
{
  "methodCalls": [
    ["Email/set", {
      "create": {"draft": { /* ... */ }}
    }, "write"],

    ["EmailSubmission/set", {
      "create": {"send": {"emailId": "#draft", "identityId": "{{identityId}}"}},
      "onSuccessUpdateEmail": {
        "#send": {
          "mailboxIds/{{draftsMailboxId}}": null,
          "mailboxIds/{{sentMailboxId}}": true,
          "keywords/$draft": null
        }
      }
    }, "send"]
  ],
  "_returns": "send"
}
```

`#draft` は、最初の呼び出しが作るメッセージを指します。
サーバがまだ id を付けていない段階での参照です。
パッチの中のポインタは `Email` に照らして検証されるので、`mailboxIds` を綴り間違えればビルドが失敗します。
二つのメールボックスのパラメータが `jmapc.ID` になるのは、ポインタがその型で要素を選ぶからです。

[`example/queries`](example/queries) には、メール、連絡先、カレンダー、共有、フィルタにまたがる 23 個のクエリがあります。
検索、既知の状態からの同期、送信、連絡先カードの作成、繰り返し予定のうち一回だけを他に触れずに動かす操作などです。

## クエリの書き方

クエリファイルは [RFC 8620](https://www.rfc-editor.org/rfc/rfc8620) が定義する JMAP の Request オブジェクトそのものです。
これに、ジェネレータが読みサーバが見ることのない三つのメンバが加わります。

アンダースコアで始まるメンバはジェネレータが読むもので、それ以外は RFC 8620 が定義するリクエストそのものです。

| メンバ | |
|---|---|
| `methodCalls` | 呼び出しを `[name, arguments, callId]` の形で並べます。必須です。 |
| `using` | リクエストが宣言する capability です。省略でき、その場合は呼び出すメソッドから導出されます。 |
| `_doc` | 生成される関数のドキュメントです。省略できます。 |
| `_returns` | どの呼び出しのレスポンスを関数の戻り値にするかを指定します。省略すると全てのレスポンスが返ります。 |
| `_comment` | その呼び出しが何のためにあるかを書きます。呼び出しの引数の中に置きます。次項を参照してください。 |

クエリファイルは素の JSON です。
`jq` で読めますし、エディタも理解します。
呼び出しの意図を書くには、その引数に `_comment` を置きます。

```json
["Email/get", {
  "_comment": "同じリクエストで取得する。id が往復することはない。",
  "#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"}
}, "fetch"]
```

ジェネレータはこれを生成コードのコメントに移し、リクエストからは除きます。
除かなければなりません。
RFC 8620 は、知らない引数をサーバが拒否することを求めているからです。

ドットではなくアンダースコアなのは、jmapc が問題の場所をドットで綴るからです。
`methodCalls[0].arguments.filter` のような表記の中では、`.comment` というメンバがその一部に見えてしまいます。

```go
// 同じリクエストで取得する。id が往復することはない。
{Name: "Email/get", CallID: "fetch", Args: map[string]any{
```

### パラメータ

呼び出し側に委ねる値の位置に `{{name}}` と書きます。
Go の型は、その値が埋まる引数から決まります。
`limit` に置いた `{{limit}}` は `jmapc.UnsignedInt` になり、`inMailbox` に置いた `{{mailboxId}}` は `jmapc.ID` になります。
同じ名前を二箇所で使えば一つのフィールドにまとまり、型が一致しているかが検証されます。

マップのキーもパラメータにできます。
`/set` が変更対象のレコードを指定する方法がこれです。

```json
["Email/set", {"update": {"{{emailId}}": {"keywords/$seen": true}}}, "mark"]
```

`$` ではなく波括弧を使うのは、JMAP のキーワード自体が `$seen` のように `$` で始まるからです。

### アカウント id

`accountId` を省略すると、生成された関数がセッションのプライマリアカウントから補います。
セッションの取得は一度だけです。
パラメータにしたい場合は `"{{accountId}}"` と書きます。

## 何が検証されるか

以下はすべて、サーバへの往復ではなくビルド時の失敗になります。

- メソッドが存在し、仕様どおりに綴られていること
- 引数がそのメソッドのものであり、メソッドが要求する型であること
- 結果参照が**先行する**呼び出しを指し、その呼び出しのメソッド名を正しく名指し、参照先の引数が受け取れる値を選んでいること
- フィルタ条件が、クエリ対象の型に照らして検証されること。`AND`、`OR`、`NOT` の中に入れ子になったものも含みます
- `properties` がその型の持つプロパティを指していること
- `PatchObject` が、パッチ対象のレコードが実際に持つプロパティを指し、正しい型の値を設定していること
- `sort` がその型で実際にソートできるプロパティを指し、`hasKeyword` のような比較子が要求する追加メンバを与えていること
- 仕様が値を固定しているプロパティに、その値のいずれかが与えられていること。文字列の値と、参加者の `roles` のような集合のキーの両方が対象です
- id、日付、整数の形式が正しいこと
- リクエストが宣言する capability が、呼び出すメソッドを網羅していること

綴り間違いには候補が提示されます。

```
queries/BadQuery.jmap.json: methodCalls[0].arguments.filter.hasAttachmnt: EmailFilterCondition has no property "hasAttachmnt"
	did you mean "hasAttachment"?
queries/BadQuery.jmap.json: methodCalls[1].arguments.#ids.name: the referenced call is Email/query, but the reference names Email/get
	call "c0" invokes Email/query
```

`jmapc check` は、何も書き出さずに検証だけを実行します。

## 設定

フラグで指定するか、モジュールの隣に `jmapc.json` を置きます。

```json
{
  "queries": "queries",
  "out": "internal/jmapq",
  "package": "jmapq",
  "schemas": ["schema/notes.json"]
}
```

## 実行時のエラー

JMAP は二つの水準で失敗します。
ランタイムもそれに対応します。

**リクエスト水準**の失敗は、サーバがリクエスト全体を拒否した場合で、`*jmapc.RequestError` になります。
RFC 8620 §3.6.1 の problem type を持ちます。
このうちいくつかは送信前にクライアントが捕まえます。
セッションが広告していない capability や、サーバが受け付ける数を超える呼び出しなどです。

**メソッド水準**の失敗は `jmapc.MethodErrors` になります。
JMAP は実行できる呼び出しを実行するので、レスポンスはエラーと一緒に返ります。
各エラーは、ワイヤフォーマットが運ぶ `"error"` ではなく、失敗したメソッド名と呼び出し id を報告します。

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

サーバが受け付けると表明したサイズを超えるアップロードは、送信前に失敗します。

`urn:ietf:params:jmap:blob` を提供するサーバでは、API 経由で blob を作成し読み取ることもできます。
エンドポイントにはできないことです。
`Blob/upload` は blob を、それを使う呼び出しと同じリクエストに置けるので、id がクライアントに戻ってきません。

## プッシュ

`Client.EventSource` はサーバのプッシュエンドポイントに接続します。
イベントが伝えるのは、どのアカウントのどの型が進んだかであって、何が変わったかではありません。
クライアントは続けて `/changes` を呼びます。

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
		// 手元の状態からの Email/changes を呼ぶ
		_ = state
	}
}
```

ストリームは接続であって、ネットワークより長生きする購読ではありません。
`Next` が返すエラーは再接続の合図で、`LastEventID` が再開点です。
そこから再開すれば、間のイベントを取りこぼしません。

これはイベントソース形式のプッシュで、接続を保持できるクライアントに向いています。
もう一つの形式は、サーバが送る先の URL を登録するもので、スマートフォンのアプリにはこちらが必要です。
[`example/queries`](example/queries) の `RegisterPush` と `ConfirmPush` を参照してください。
購読は作成した時点ではまだ有効ではありません。
サーバが URL にコードを送り、クライアントが `PushSubscription/set` でそれを書き戻すまで、他には何も送られません。
届いたものは `jmapc.PushVerification` でデコードします。

## ベンダ拡張

JMAP は拡張される前提の設計です。
サーバは独自の capability URI を広告し、それとともに jmapc の知らない型とメソッドが現れます。
スキーマファイルに記述すれば、それに対するクエリも `Email` に対するものとまったく同じように検証されます。
結果参照、プロパティ名、ソート順、すべてが対象です。

```json
{
  "capability": "urn:example:params:jmap:notes",
  "types": [
    {
      "name": "Note",
      "doc": "Note is a scrap of text the user keeps.",
      "properties": [
        {"name": "id", "type": "Id", "serverSet": true, "immutable": true, "doc": "The id of the note."},
        {"name": "title", "type": "String", "doc": "The note's title."}
      ],
      "methods": ["get", "changes", "set", "query"],
      "sort": [{"name": "createdAt", "doc": "Sorts by when the note was created."}]
    },
    {
      "name": "NoteFilterCondition",
      "doc": "NoteFilterCondition is a condition a note must satisfy to match a Note/query.",
      "properties": [{"name": "text", "type": "String", "doc": "Matches notes containing this text."}]
    }
  ]
}
```

標準の六つのメソッドは、名前を挙げるだけで手に入ります。
引数とレスポンスの形は RFC 8620 が固定しているからです。
その形に従わないメソッドは、引数とレスポンスを書き下して宣言します。

```
jmapc generate -schema schema/notes.json
```

`jmapc.json` の `"schemas"` に列挙することもできます。

## jmapc の開発

```
go test ./...        # エンドツーエンドのテストを含むすべて
go generate ./...    # ランタイムの型とサンプルのクライアントを再生成する
```

ここでジェネレータをソースから実行しているのは、このリポジトリがジェネレータの居場所だからです。

ランタイムの型とサンプルのクライアントはコミットされていて、それらをカタログが今生成する結果と比較するテストがあります。
データモデルを変えたのに再生成し忘れると、見逃されるのではなくビルドが失敗します。
CI では同じ検証に加えて、gofmt、go vet、govulncheck を実行します。

## 対応範囲

### capability

JMAP は仕様の集まりです。
サーバは capability URI を広告し、それぞれが固有の型とメソッドを持ち込みます。
以下は [IANA が登録しているもの](https://www.iana.org/assignments/jmap/jmap.xhtml)と、それぞれに対する jmapc の状況です。

| capability | | 内蔵 |
|---|---|---|
| `urn:ietf:params:jmap:core` | [RFC 8620](https://www.rfc-editor.org/rfc/rfc8620) | あり |
| `urn:ietf:params:jmap:mail` | [RFC 8621](https://www.rfc-editor.org/rfc/rfc8621) | あり |
| `urn:ietf:params:jmap:submission` | [RFC 8621](https://www.rfc-editor.org/rfc/rfc8621) | あり |
| `urn:ietf:params:jmap:vacationresponse` | [RFC 8621](https://www.rfc-editor.org/rfc/rfc8621) | あり |
| `urn:ietf:params:jmap:contacts` | [RFC 9610](https://www.rfc-editor.org/rfc/rfc9610) | あり |
| `urn:ietf:params:jmap:calendars` | [draft-ietf-jmap-calendars](https://datatracker.ietf.org/doc/draft-ietf-jmap-calendars/) | あり |
| `urn:ietf:params:jmap:principals:availability` | [draft-ietf-jmap-calendars](https://datatracker.ietf.org/doc/draft-ietf-jmap-calendars/) | あり |
| `urn:ietf:params:jmap:principals` | [RFC 9670](https://www.rfc-editor.org/rfc/rfc9670) | あり |
| `urn:ietf:params:jmap:principals:owner` | [RFC 9670](https://www.rfc-editor.org/rfc/rfc9670) | あり |
| `urn:ietf:params:jmap:smimeverify` | [RFC 9219](https://www.rfc-editor.org/rfc/rfc9219) | あり |
| `urn:ietf:params:jmap:blob` | [RFC 9404](https://www.rfc-editor.org/rfc/rfc9404) | あり |
| `urn:ietf:params:jmap:quota` | [RFC 9425](https://www.rfc-editor.org/rfc/rfc9425) | あり |
| `urn:ietf:params:jmap:sieve` | [RFC 9661](https://www.rfc-editor.org/rfc/rfc9661) | あり |
| `urn:ietf:params:jmap:mdn` | [RFC 9007](https://www.rfc-editor.org/rfc/rfc9007) | あり |
| `urn:ietf:params:jmap:webpush-vapid` | [RFC 9749](https://www.rfc-editor.org/rfc/rfc9749) | なし |

このうち二つは、それ自体が別仕様のオブジェクトを格納します。
連絡先カードは [JSContact](https://www.rfc-editor.org/rfc/rfc9553) の Card であり、カレンダーの予定は [JSCalendar](https://www.rfc-editor.org/rfc/rfc8984) の JSEvent です。
どちらも JMAP が使っている型名を使い、しかも互いの型名とも衝突します。
三つの異なる `Link` 型が存在することになります。
そこでこれらには接頭辞を付けています。
`ContactEmailAddress` はカード上のアドレス、`EmailAddress` はヘッダフィールドのアドレス、`EventLink` は会議に添付されたリソースです。
各型のドキュメントには、その仕様が使っている名前を記載しています。

JSCalendar は JMAP にない時刻の型も持ち込みます。
予定の `start` はタイムゾーンを持たない `LocalDateTime` で、`duration` は ISO 8601 の `Duration` です。
`Duration` が独自の型なのは、サマータイムの切り替えを跨ぐ `P1D` が常に 24 時間とは限らないからです。
どちらもクエリで検証されるので、末尾に `Z` の付いた `start` や、`90m` と書いた duration はビルドに失敗します。

capability のすべてが固有の型を持ち込むわけではありません。
S/MIME の検証は `Email` に四つのプロパティを足すだけで、型もメソッドも増やしません。
つまりメソッド名からは、その capability が必要だと分かりません。
jmapc はクエリが触れたプロパティがどの capability に属するかを判断し、`using` に加えます。
`smimeStatus` を要求すれば、`urn:ietf:params:jmap:smimeverify` が自動で現れます。

内蔵していない capability も、手が届かないわけではありません。
[スキーマファイル](#ベンダ拡張)に型を記述すれば、それに対するクエリも他と同じように検証されます。
ベンダ拡張と同じ仕組みであり、記述するのは宣言だけで、Go を書く必要はありません。

### メソッド

81 のメソッドがあり、すべて同じ方法で検証され生成されます。

| 型 | メソッド |
|---|---|
| `Mailbox` | `get` `changes` `set` `query` `queryChanges` |
| `Thread` | `get` `changes` |
| `Email` | `get` `changes` `set` `copy` `query` `queryChanges` `import` `parse` |
| `SearchSnippet` | `get` |
| `Identity` | `get` `changes` `set` |
| `EmailSubmission` | `get` `changes` `set` `query` `queryChanges` |
| `VacationResponse` | `get` `set` |
| `AddressBook` | `get` `changes` `set` |
| `ContactCard` | `get` `changes` `set` `copy` `query` `queryChanges` |
| `Calendar` | `get` `changes` `set` |
| `CalendarEvent` | `get` `changes` `set` `copy` `query` `queryChanges` `parse` |
| `CalendarEventNotification` | `get` `changes` `set` `query` `queryChanges` |
| `ParticipantIdentity` | `get` `changes` `set` |
| `Principal` | `get` `changes` `set` `query` `queryChanges` `getAvailability` |
| `ShareNotification` | `get` `changes` `set` `query` `queryChanges` |
| `Quota` | `get` `changes` `query` `queryChanges` |
| `SieveScript` | `get` `set` `query` `validate` |
| `MDN` | `send` `parse` |
| `Blob` | `copy` `upload` `get` `lookup` |
| `PushSubscription` | `get` `set` |
| `Core` | `echo` |

### 未対応の部分

jmapc がまだ行わないことです。
使い始めてから気付くことのないよう、明示しておきます。

**`bodyProperties` による絞り込みが効かない**：`Email/get` は引数を受け取ってそのまま渡しますが、生成されるボディパートは要求された部分集合ではなく `EmailBodyPart` の全プロパティを持ちます。
レコード自体に対する `properties` は、生成される型を絞り込みます。

**ヘッダフィールドのプロパティが無型**：`header:List-Id:asText` のようなプロパティは受け付けます。
その意味はサーバが決めるからです。
ただし生成される構造体には `json.RawMessage` として現れ、デコードは呼び出し側に委ねられます。

**creation id がリクエストを跨がない**：一つのリクエストの中で `#draft` を参照することはでき、それが[メッセージの送信](#一つのリクエストで複数の手順を踏む)を可能にしています。
RFC 8620 が認めている、`createdIds` を次のリクエストへ持ち越す形は、クエリでは表現できません。

**開いた集合は意図的に検証しない**：仕様が値を固定しているプロパティは検証します。
一方、集合が開いているもの、たとえばメールボックスの `role`、メールのキーワード、`Content-Disposition` は検証しません。
サーバが受け付けたはずの値を拒否するほうが、綴り間違いを通すより害が大きいからです。
