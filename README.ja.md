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

jmapc は JMAP から**型安全なコード**を、Go、TypeScript、Rust で生成します。

1. JMAP でクエリを書きます。
1. jmapc を実行して、そのクエリに型安全なインターフェースを持つコードを生成します。
1. 生成されたコードを呼ぶアプリケーションコードを書きます。

## 動機

JMAP は一つのリクエスト中に複数のメソッド呼び出しを記述でき、後の呼び出しは前の呼び出しの結果を参照できます。
依存関係のある一連の操作が、一往復で済みます。

```json
{
  "using": [
    "urn:ietf:params:jmap:core",
    "urn:ietf:params:jmap:mail"
  ],
  "methodCalls": [
    [
      "Email/query",
      {
        "filter": {
          "inMailbox": "mbx1"
        }
      },
      "search"
    ],
    [
      "Email/get",
      {
        "#ids": {
          "resultOf": "search",
          "name": "Email/query",
          "path": "/ids"
        }
      },
      "fetch"
    ]
  ]
}
```

一つ目のメソッドの戻りで得られるid はクライアントに戻ってきません。
この設計が、JMAP クライアントが REST クライアントのような形、つまりリソースごとの型とパスごとのメソッドという形にならない理由です。

ほとんどの JMAP クライアントはこれをビルダーとして提供します。
この場合、利用者は JMAP に加えてビルダーの使い方も覚えることになります。
しかし利用者の関心があるのはJMAPクエリであって、クライアントの使い方ではありません。
であれば、クエリだけを書いて、クライアントは jmapc に書かせればよいはずです。
このアイデアは、SQL における [sqlc](https://sqlc.dev) から着想を得たものです。

利用者が書くのはクエリだけで、手で書くと面倒で間違えやすい部分は jmapc に任せることができます。

- クエリのリンティング
- レスポンスの型安全
- 網羅的なエラー処理

結果参照は参照先のメソッドに照らして、引数はデータモデルに、プロパティ名は型に照らして検証されます。
プロパティ名の綴り間違いは、ビルドが失敗するのでサーバに届く前に分かります。
クエリが要求したプロパティだけを持つ構造体にデコードされるので、`map[string]any` を辿る必要はなく、レスポンスは型安全です。
JMAP はリクエスト、メソッド、レコードの三つのレベルで失敗します。
レコードレベルは HTTP 200 で返るので見落とされがちですが、生成コードがこれを検査します。

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
TypeScript や Rust のプロジェクトではこちらを使います。

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

`bodyProperties` はメッセージのパートを同じように絞り込み、その絞り込みは入れ子のサブパートにも及びます。
ヘッダフィールドを指すプロパティは、要求した形式によって型が決まります。
`header:List-Id:asText` は `*string`、`header:To:asAddresses` は `[]jmapc.EmailAddress` です。

### 生成される名前

生成される名前はすべてファイル名から決まります。
そのためファイル名は Go の識別子でなければなりません。
英数字とアンダースコアからなり、数字で始まらない名前です。

`ListInboxEmails.jmap.json` からは次の名前が生成されます。

| 生成物 | 名前 |
|---|---|
| 関数 | `ListInboxEmails` |
| パラメータ。クエリが値を開けている場合に生成されます | `ListInboxEmailsParams` |
| プロパティを絞り込んだレコード | `ListInboxEmailsEmail`。ボディパートを絞り込めば `ListInboxEmailsEmailBodyPart` も生成されます |
| そのレコードを返す呼び出しのレスポンス | `ListInboxEmailsEmailGetResponse` |
| 結果。`_returns` が呼び出しを指定しない場合に生成されます | `ListInboxEmailsResult` |
| 変更を追う関数。クエリが `_watches` を持つ場合に生成されます | `SyncEmailsWatch` |
| 答えを最後まで読み通すもの。クエリが `_pages` を持つ場合に生成されます | `SearchEmailsPages` |
| ファイル | `listinboxemails_gen.go` |

絞り込みのない呼び出しは、クエリごとの型ではなく共有の型で返ります。
`SendEmail` の戻り値が `*jmapc.EmailSubmissionSetResponse` なのはそのためです。
同じパッケージに同名のクエリを二つ置くことはできません。
生成される型の名前がすでに使われている場合は、`ListInboxEmailsEmail2` のように連番が付きます。

パラメータを持たないクエリは、Params 引数自体を取りません。
`MailQuota(ctx, c, MailQuotaParams{})` ではなく `MailQuota(ctx, c)` になります。
そのため、すでに使われているクエリに最初の `{{param}}` を追加すると、生成される関数の引数の数が変わり、呼び出し箇所がすべて壊れます。
これは意図的なトレードオフです。パラメータを持たないクエリが、ただの関数呼び出しのように読めることを優先しています。

TypeScript では関数名とファイル名の先頭が小文字になり、`listInboxEmails.ts` の `listInboxEmails` になります。
型名は上の表のままです。

Rust では関数名とモジュール名が snake_case になり、`list_inbox_emails.rs` の `list_inbox_emails` になります。
型名も上の表のままですが、頭字語は一語として綴られます。
これは Rust の綴り方に倣ったもので、`UTCDate` は `UtcDate` になります。
プロパティは snake_case になり、それがワイヤ上の名前と違う場合には serde の rename が付きます。

### 用例

[`example/queries`](example/queries) には、メール、連絡先、カレンダー、共有、フィルタにまたがる 25 個のクエリがあります。
検索、既知の状態からの同期、送信、連絡先カードの作成、繰り返し予定のうち一回だけを他に触れずに動かす操作などです。

## TypeScript

同じクエリから TypeScript 用のクライアントを生成できます。

```
jmapc generate -lang typescript -out src/jmapq
```

```typescript
import { Client } from "./jmapq/client.js"
import { listInboxEmails } from "./jmapq/listInboxEmails.js"

const client = new Client("https://example.com/.well-known/jmap", { auth: token })

const res = await listInboxEmails(client, { mailboxId: inbox, limit: 25 })
for (const email of res.list) {
  console.log(email.receivedAt, email.from?.[0].email, email.subject)
}
```

ランタイムも一緒に生成されます。
`client.ts` と `types.ts` がクエリと並んで出力されるので、生成物には**依存がありません**。
プラットフォームに求めるのは `fetch` だけです。

TypeScript のほうが正確に言えることもあります。
null を取りうるプロパティはポインタではなく union なので、`subject` は `string | null` です。
複数の形を取る値も union のままです。
フィルタは `FilterOperator | EmailFilterCondition | null` であり、Go で `any` に落ちていたものが型として残ります。
形ではなく書式を持つプリミティブは `string` の名前付き別名になるので、`Id` と `TimeZoneId` を取り違えることがありません。

## Rust

同じクエリから Rust 用のクライアントを生成できます。

```
jmapc generate -lang rust -out src/jmapq
```

```rust
use jmapq::list_inbox_emails::{list_inbox_emails, ListInboxEmailsParams};
use jmapq::Client;

let client = Client::with_bearer_token("https://example.com/.well-known/jmap", http, token);

let res = list_inbox_emails(&client, ListInboxEmailsParams {
    mailbox_id: inbox,
    limit: 25,
})
.await?;
for email in &res.list {
    println!("{} {:?}", email.received_at, email.subject);
}
```

ランタイムも一緒に生成されます。
`client.rs`、`types.rs`、そしてそれらをクエリと並べて宣言する `mod.rs` が出力されるので、クレート側が足すのは `mod jmapq;` の一行だけです。
生成されたコードが求めるのは **serde と serde_json** だけです。
バイトをどう運ぶかは `Transport` としてあなたが書きます。
プログラムがすでに持っている HTTP クライアントの上に実装すればよく、HTTP スタックも TLS バックエンドも非同期ランタイムも生成物には付いてきません。

```rust
struct Http(reqwest::Client);

impl Transport for Http {
    async fn send(&self, req: HttpRequest) -> Result<HttpResponse, TransportError> {
        let mut out = self.0.request(req.method.parse()?, &req.url);
        for (name, value) in req.headers {
            out = out.header(name, value);
        }
        if let Some(body) = req.body {
            out = out.body(body);
        }
        let res = out.send().await?;
        Ok(HttpResponse {
            status: res.status().as_u16(),
            content_type: res
                .headers()
                .get("content-type")
                .and_then(|v| v.to_str().ok())
                .unwrap_or("")
                .to_string(),
            body: res.bytes().await?.to_vec(),
        })
    }
}
```

ベアラトークンでは足りない認証も、ここが置き場所です。
リクエストへの署名や、期限切れで更新するトークンがそれにあたります。
トランスポートは、リクエストが出ていく前の最後の通過点だからです。

null を取りうるプロパティは `Option` なので、`subject` は `Option<String>` です。
複数の形を取る値も union のままです。
フィルタは `Option<FilterOperatorOrEmailFilterCondition>` という untagged な enum であり、Go で `any` に落ちていたものが型として残ります。
形ではなく書式を持つプリミティブは `String` の名前付き別名になるので、シグネチャの上で `Id` と `TimeZoneId` が読み分けられます。
そしてレコードは `Default` を導出します。
省略可能なプロパティを五十個持つ型を組み立てられるのはこれのおかげで、必要な二つだけを名指しして残りは任せられます。

生成されるコードは rustfmt が整形した形そのものなので、クレートに `cargo fmt` をかけても何も動きません。

## クエリの書き方

クエリファイルは [RFC 8620](https://www.rfc-editor.org/rfc/rfc8620) が定義する JMAP の Request オブジェクトそのものです。
これに、jmapcが読みjmapサーバが見ることのない四つのメンバが加わります。

アンダースコアで始まるメンバはジェネレータが読むもので、それ以外は RFC 8620 が定義するリクエストそのものです。

| メンバ | |
|---|---|
| `methodCalls` | 呼び出しを `[name, arguments, callId]` の形で並べます。必須です。 |
| `using` | リクエストが宣言するケイパビリティです。省略でき、その場合は呼び出すメソッドから導出されます。 |
| `_doc` | 生成される関数のドキュメントです。省略できます。 |
| `_returns` | どの呼び出しのレスポンスを関数の戻り値にするかを指定します。省略すると全てのレスポンスが返ります。 |
| `_createdIds` | 先行するリクエストの creation id を受け取り、このリクエストのものを返します。省略可。次項を参照してください。 |
| `_watches` | 生成されたクライアントが変更を追う呼び出しを指定します。サーバが「進んだ」と言うたびに追いつきます。省略可。[プッシュ](#プッシュ)を参照してください。 |
| `_pages` | 生成されたループが進める呼び出しを指定します。一度のリクエストが一部だけ返す答えを、最後まで読み通せます。省略可。[一度のリクエストに収まらない答えを読み通す](#一度のリクエストに収まらない答えを読み通す)を参照してください。 |
| `_comment` | その呼び出しが何のためにあるかを書きます。呼び出しの引数の中に置きます。次項を参照してください。 |

クエリファイルは素の JSON です。
`jq` で読めますし、エディタも理解します。
呼び出しの意図を書くには、その引数に `_comment` を置きます。

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

#### 呼び出し側が省略できる引数

呼び出し側が引数ごと省略してよい位置には `{{name?}}` と書きます。

```json
["Email/changes", {"sinceState": "{{sinceState}}", "maxChanges": "{{maxChanges?}}"}, "changes"]
```

値を渡さなければ、その引数はリクエストに入りません。
これは null を送るのとは別のことです。
RFC 8620 でも違いは二度出てきます。
`maxChanges` が無いことは上限なしを意味しますが、`maxChanges: 0` は何も要求しないことになります。
PatchObject では、ポインタに null を入れるとそのプロパティが消え、ポインタ自体が無いときだけ触らずに残ります。
この記法がなければ、ときどきしか送らない引数ごとに別のクエリが必要になり、n 個あれば 2^n 個要ることになります。

省略の伝え方は、その言語がすでに持っている形に従います。
Go はポインタを取り、型自身に nil があるならそのままです。

```go
limit := jmapc.UnsignedInt(25)
jmapq.FindPeople(ctx, c, jmapq.FindPeopleParams{Phrase: "ada", Limit: &limit})
jmapq.FindPeople(ctx, c, jmapq.FindPeopleParams{Phrase: "ada"}) // limit 引数は送られません
```

TypeScript ではメンバー自体が省略可能になり (`limit?: number`)、Rust では `Option` に包まれます。
`jmapc run` では、`-p` で指定しなかった引数がそのまま省略されます。

省略できるのはメソッド呼び出しの引数そのものだけで、しかもそのパラメータが他の場所で使われていない場合に限ります。
こうすることで「省略された」の意味が一つに定まります。つまり、そのメンバーが無い、ということです。
フィルタや配列の中にあるパラメータは、より大きな値の一部です。
そこだけ落とすと、空の `AND` と、フィルタが無いことのどちらなのかという、クエリが答えていない問いが残ります。
形の変わるフィルタは、フィルタ全体を一つのパラメータとして渡してください。

### リクエストを跨ぐ creation id

一つのリクエストの中で `#draft` を参照するのに準備は要りません。
サーバが解決します。
あるリクエストから次のリクエストへ参照を持ち越すには id 自体が移動する必要があり、それを求めるのが `_createdIds` です。

```json
{
  "_createdIds": true,
  "methodCalls": [
    ["Mailbox/set", {"create": {"box": {"name": "{{name}}"}}}, "make"],
    ["Email/set", {"update": {"{{emailId}}": {"mailboxIds/#box": true}}}, "file"]
  ]
}
```

生成される関数は、それを受け取って返します。

```go
res, err := jmapq.FileIntoNewMailbox(ctx, c, params, carried)
// res.CreatedIDs を次のリクエストへ渡す。
```

RFC 8620 がこれを用意しているのはプロキシのためです。
一つのリクエストを複数のサーバに分割しても、参照が解決できるようにするものです。
これを使うクエリは、単一のレスポンスではなく全てのレスポンスを返します。
creation id はリクエスト全体のものであって、その中のどの呼び出しのものでもないからです。

### アカウント id

`accountId` を省略すると、生成された関数がセッションのプライマリアカウントから補います。
セッションの取得は一度だけです。
パラメータにしたい場合は `"{{accountId}}"` と書きます。

## 検証

以下はすべて、サーバへの往復ではなくビルド時の失敗になります。

- メソッドが存在し、仕様どおりに綴られていること
- 引数がそのメソッドのものであり、メソッドが要求する型であること
- 結果参照が**先行する**呼び出しを指し、その呼び出しのメソッド名を正しく名指し、参照先の引数が受け取れる値を選んでいること
- フィルタ条件が、クエリ対象の型に照らして検証されること。`AND`、`OR`、`NOT` の中に入れ子になったものも含みます
- `properties` がその型の持つプロパティを、`bodyProperties` が `EmailBodyPart` の持つプロパティを指していること
- ヘッダフィールドを指すプロパティが、仕様の定めるパース形式を要求していること。`header:List-Id:asText` は文字列に、`header:To:asAddresses` はアドレスのリストになります
- `PatchObject` が、パッチ対象のレコードが実際に持つプロパティを指し、正しい型の値を設定していること。キーの書き方は RFC 8620 に従います。ポインタ先頭の `/` は暗黙なので、キーワードを立てる位置は `/keywords/$seen` ではなく `keywords/$seen` です
- `sort` がその型で実際にソートできるプロパティを指し、`hasKeyword` のような比較子が要求する追加メンバを与えていること
- 仕様が値を固定しているプロパティに、その値のいずれかが与えられていること。文字列の値と、参加者の `roles` のような集合のキーの両方が対象です
- id、日付、整数の形式が正しいこと
- リクエストが宣言するケイパビリティが、呼び出すメソッドを網羅していること
- `_watches` が指す呼び出しが、ある状態からの変更を報告するものであり、そこから進む状態がクエリに書き込まれずループに委ねられていること
- `_pages` が指す呼び出しが、より長い答えの一部を返して残りの在り処を告げるものであり、次のリクエストの開始位置がループに委ねられていること

綴り間違いには候補が提示されます。

```
queries/BadQuery.jmap.json: methodCalls[0].arguments.filter.hasAttachmnt: EmailFilterCondition has no property "hasAttachmnt"
	did you mean "hasAttachment"?
queries/BadQuery.jmap.json: methodCalls[1].arguments.#ids.name: the referenced call is Email/query, but the reference names Email/get
	call "c0" invokes Email/query
```

`jmapc check` は、何も書き出さずに検証だけを実行します。

### サーバだけが知っていること

ここまではすべて、仕様が言っていることです。
仕様がサーバに委ねていること —— どのケイパビリティを持つか、どのアカウントを持つか、一度のリクエストでどれだけ引き受けるか —— は、ビルドには分かりません。
JMAP については正しく、目の前のサーバについては間違っているクエリは、実行時に失敗します。
`-session` はそれをサーバに尋ねます。

```
jmapc check -session jmap.example.com -token $JMAP_TOKEN
checked 25 queries against https://jmap.example.com/api/, as someone@example.com
```

報告するのは次のものです。

- リクエストが宣言していて、サーバが広告していないケイパビリティ
- クエリが名指していてセッションが持たないアカウント、そのケイパビリティの primary account がなくセッションが埋められないアカウント、呼び出しに必要なものをサポートしないアカウント
- `maxCallsInRequest` を超える呼び出し数、`maxObjectsInGet` を超えるレコード数、`maxObjectsInSet` を超える変更数、パラメータを埋める前から `maxSizeRequest` を超えるリクエスト
- サーバが文字列の比較に使えない `collation`

クエリが呼び出し側に委ねているものには触れません。
id のリストを表すパラメータは何個にでもなり得るので、そこを推測すれば、問題のないクエリを問題ありと報告することになります。

セッションの URL だけは環境変数から読みません。
`-token` と `-user` は `$JMAP_TOKEN` と `$JMAP_USER` にフォールバックします。
ネットワークに出る検証は、周囲にたまたま設定されているものによってではなく、コマンドラインでそう言われて出るべきだからです。

## エディタ対応

上の検証は jmapc を走らせたときに実行されます。
その多くは、クエリを書いている最中に走らせることもできます。
ファイルそのものに対する検証だからです。
そしてスキーマを名指しした JSON ファイルなら、エディタは検証も補完もすでに知っています。

```
jmapc schema -out jmapc.schema.json
```

これがカタログの JSON Schema を、ベンダ拡張も含めて書き出します。
クエリファイルからそれを指すか、

```json
{
  "$schema": "../jmapc.schema.json",
  "methodCalls": [["Email/query", {"filter": {"inMailbox": "{{mailboxId}}"}}, "search"]]
}
```

エディタ側からまとめて指します。
VS Code なら次のとおりです。

```json
{
  "json.schemas": [
    {"fileMatch": ["*.jmap.json"], "url": "./jmapc.schema.json"}
  ]
}
```

どちらでも、エディタはメソッド名を補完し、そのメソッドが取る引数とその型が持つプロパティを提示し、綴り間違いをその場に下線で示します。
`AND` の中に入れ子になったフィルタも外側と同じように検証され、比較子はその型が実際にソートできるプロパティを提示し、`{{パラメータ}}` は値が置ける場所ならどこでも受け付けられます。

スキーマに言えないのは、他の呼び出しに依存する部分です。
結果参照が先行する呼び出しを名指し、引数が受け取れる値を選んでいるか、そこは jmapc の仕事のままです。
エディタはビルドの代わりではなく、その手前の一段だと考えてください。

## クエリを送る

クエリは、それを呼ぶコードができる前に試せるほうがよいので、`jmapc run` が一つ送って、返ってきたものを表示します。

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
クエリが省いた account id は、生成された関数がそうするのと同じように、セッションから引かれます。
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
  "queries": "queries",
  "out": "internal/jmapq",
  "package": "jmapq",
  "schemas": ["schema/notes.json"]
}
```

## 実行時のエラー

JMAP は二つのレベルで失敗します。
ランタイムもそれに対応します。

**リクエストレベル**の失敗は、サーバがリクエスト全体を拒否した場合で、`*jmapc.RequestError` になります。
RFC 8620 §3.6.1 の problem type を持ちます。
このうちいくつかは送信前にクライアントが捕まえます。
セッションが広告していないケイパビリティや、サーバが受け付ける数を超える呼び出しなどです。

**メソッドレベル**の失敗は `jmapc.MethodErrors` になります。
JMAP は実行できる呼び出しを実行するので、レスポンスはエラーと一緒に返ります。
各エラーは、ワイヤフォーマットが運ぶ `"error"` ではなく、失敗したメソッド名と呼び出し id を報告します。

第三のレベルがあり、見落とされるのはこれです。
`/set` は**エラーを含まない 200** を返しながら、処理を拒んだレコードを列挙します。

```json
["Email/set", {"notCreated": {"draft": {"type": "invalidProperties",
                                        "properties": ["subject"]}}}, "write"]
```

転送レベルのエラーだけを見ると、何も起きていないのに成功に見えます。
生成コードがこれを検査するので、拒否されたレコードは `*jmapc.SetErrors` になります。

```go
res, err := jmapq.SendEmail(ctx, c, params)
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
クエリが `_returns` で名指ししていない呼び出しも検査されます。
一つの呼び出しを名指ししたことで、他が見られなくなるべきではないからです。

TypeScript では同じ失敗が `SetErrors` の throw になり、レスポンスは `err.result` に載ります。
Rust では `Error::Set` になり、レスポンスは関数が返すはずだった型を指定して `err.result::<T>()` で取り出します。

### 再送

サーバがHTTP 429 と 503 を返す場合、`WithRetry` は再送を行います。

```go
c := jmapc.New(url, jmapc.WithBearerToken(token), jmapc.WithRetry(3))
```

引数は試行回数です。
待ち時間は、サーバが `Retry-After` で求めた長さです。
特に何も求められなければ、0.2 秒から 30 秒へ倍々に待ちます。

## テスト

生成されたクライアントの周りに書いたコードをテストするということは、複数のメソッド呼び出しを載せたリクエストに答えるということです。
しかもその一部は、他の呼び出しの結果を参照しています。
一つのテストのために手で書いたスタブは、そこを無視してサーバらしさを失うか、さもなければこれへと育っていきます。

```go
srv := jmaptest.New(t)
srv.Reply("Email/query", jmapc.EmailQueryResponse{
	AccountID: jmaptest.AccountID,
	IDs:       []jmapc.ID{"m1", "m2"},
})
srv.Handle("Email/get", func(c *jmaptest.Call) (any, error) {
	// id は query の呼び出しが答えたものです。結果参照は、
	// サーバが解決するのと同じように、すでに解決されています。
	return emailsFor(c.IDs()), nil
})

res, err := jmapq.ListInboxEmails(ctx, srv.Client(), params)
```

テストから引き受けるのは次のことです。

- **結果参照。** RFC 8620 の言うとおりに解決します。パスをリストに写像する `*` も含みます。ですから連鎖したクエリは、実際の値が入った状態でハンドラに届きます。
- **検証。** リクエストは、ビルドがクエリを照らすのと同じデータモデルに照らされます。どのメソッドも持たない引数を送る呼び出しは、黙って通るのではなくテストを失敗させます。jmapc が知らないメソッドを扱うときは `jmaptest.WithoutChecks()` が逃げ道です。
- **失敗。** メソッドレベルのエラーには `srv.Fail`、サーバが中身を見もしないリクエストには `srv.FailRequest`、そして 200 を返す失敗には、拒否したものを並べた `/set` のレスポンスを返します。
- **何を尋ねられたか。** `srv.Call("Email/query")` はそのメソッドへの最後の呼び出し、`srv.Calls()` はその全部、`srv.Requests()` はそれが何回のリクエストで済んだかです。呼び出しが一度に運ばれたかどうかは、これで確かめます。
- **プッシュ。** `srv.Push` は、見ているクライアントに状態変化を送ります。watch するクエリのループが待っているものです。

しないのは、何かを保存することです。
これはクライアントをテストするためのサーバであって、JMAP の実装ではありません。
`/set` が作ったものは、テストがそう言わない限り、後の `/get` からは返りません。

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

## 一度のリクエストに収まらない答えを読み通す

JMAP の答えは、しばしば答え全体の一部にすぎません。
`/query` は呼び出し側が求めた件数の窓だけを返し、その窓が結果全体のどこに位置するかを告げます。
`/changes` はサーバの気が向くだけの変更を返し、まだ続きがあるかどうかを告げます。

どちらの場合も、答えの残りを得るには次のリクエストを送り直す必要があります。
次のリクエストの中身は直前の答えから決まり、それは `/query` なら次に求める `position`、`/changes` なら次に渡す `sinceState` です。
この「答えを見て、次のリクエストを組み立てて、また送る」というループは、クエリの `_pages` に呼び出し名を書くだけで生成できます。

`_pages` に指定した呼び出しが、ループがそのたびに送り直す呼び出しです。
次のリクエストがどこから始まるかを言う値（`/query` の `position`、`/changes` の `sinceState`）は、呼び出し側ではなくループが管理するので、クエリの中ではパラメータとして書いておくだけで構いません。

```json
{
  "_pages": "search",

  "methodCalls": [
    ["Email/query", {"filter": {"text": "{{phrase}}"}, "position": "{{position}}",
                     "limit": 50, "calculateTotal": true}, "search"],
    ["Email/get", {"#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"}}, "fetch"]
  ]
}
```

Go にはイテレータが生成されます。
読み進めるたびに、裏で次のリクエストが送られます。

```go
for page, err := range jmapq.SearchEmailsPages(ctx, c, params) {
	if err != nil {
		return err
	}
	for _, email := range page.EmailGet.List {
		fmt.Println(*email.Subject)
	}
}
```

TypeScript には非同期ジェネレータが生成され、同じように読み進めるたびにリクエストを送ります。
失敗は、クエリ自体と同じように throw されます。

```ts
for await (const page of searchEmailsPages(client, params)) {
  for (const email of page.emailGet.list) console.log(email.subject)
}
```

Rust には、どこまで進んだかを覚えている値が生成されます。
ストリームを返すにはそれを定義するクレートが要り、生成されるコードが求めるのは serde だけだからです。

```rust
let mut pages = search_emails_pages(params);
while let Some(page) = pages.next(&client).await? {
    for email in &page.email_get.list {
        println!("{:?}", email.subject);
    }
}
```

読み通しが終わるタイミングは、`/query` と `/changes` とで違います。

`/query` の場合、パラメータが持つ `position` から読み始めるので、前回の続きから読み進められます。
中身のない窓は呼び出し側には渡さず、そこで読み通しを終えます。
ですから呼び出し側が受け取る窓には必ず中身があります。
呼び出しが `calculateTotal` で総数を求めていれば、その総数を超えて窓を求めることもありません。

`/changes` の場合は、何も変わっていないという答えも呼び出し側に渡します。
その答えが、次に進むための `sinceState` を運んでいるからです。
終わるのは、サーバが「もう変更はない」と言ったときです。

watch するクエリ（`_watches` を持つクエリ）は、サーバが「まだ変更がある」と言う間はもともと尋ね直しています。
つまり同じ仕組みをすでに内蔵しているので、`_watches` と `_pages` を同じクエリに書くことはありません。

## プッシュ

イベントが伝えるのは、どのアカウントのどの型が進んだかであって、何が変わったかではありません。
ですから変更が欲しいクライアントはループを書きます。
接続し、手元の状態からの差分を尋ね、それを適用し、次に言われるのを待つ。
このループは毎回同じで、そのどの部分にも間違いが入り込みえます。
そこで、クエリの側から要求できるようにしました。

`_watches` は、ループが状態を読む呼び出しを指定します。
指定できるのは、`Email/changes` のように、ある状態からの変更を報告する呼び出しです。

```json
{
  "_watches": "changes",

  "methodCalls": [
    ["Email/changes", {"sinceState": "{{sinceState}}", "maxChanges": 128}, "changes"],
    ["Email/get", {"#ids": {"resultOf": "changes", "name": "Email/changes", "path": "/created"}}, "created"]
  ]
}
```

`SyncEmails` はこれまでどおり生成され、その隣に `SyncEmailsWatch` が生成されます。

```go
err := jmapq.SyncEmailsWatch(ctx, c, jmapq.SyncEmailsParams{SinceState: state},
	func(ctx context.Context, res *jmapq.SyncEmailsResult) error {
		for _, email := range res.EmailGet.List {
			fmt.Println("new:", *email.Subject)
		}
		state = res.EmailChanges.NewState // 保存して、次はここから始める
		return nil
	})
```

ループはパラメータが持つ状態から始まり、各回の答えが報告する状態へ進みます。
呼び出し側が持たずに済むのは次のことです。

- **ストリームは接続であって購読ではありません。** 切れたら別の接続を開き、最後に届いたイベントから再開します。サーバに届かない間は、1 秒から 30 秒へと倍々に待ちます。
- **接続がない間に変わったものは、誰にもプッシュされていません。** ですから接続のたびに、まず追いつきます。
- **サーバは `/changes` に好きなだけ答えて** `hasMoreChanges` でそう言います。ループは、そう言わなくなるまで尋ね直します。
- **他のアカウント、他の型、あるいは既に到達済みの状態のイベント**にリクエストの価値はありません。最後のものはよく起きます。自分の追いつきが、サーバに「今伝えたばかりのこと」をプッシュさせるからです。

ループはコンテキストが終わるまで走り、そのエラーを返します。
コールバックが返したエラーはループを止め、そのまま返ります。
接続を明確に拒んだサーバのエラーは、待たずに返します。403 は待っても変わらないからです。
`jmapc.WithPing` と `jmapc.WithReconnect` が、調整する価値のある二つです。

その下にあるのが `Client.Watch` で、追いつき方を関数で受け取ります。
追いつきが一つのクエリで済まないときは、これを直接呼びます。

```go
err := c.Watch(ctx, accountID, "Email", state,
	func(ctx context.Context, since string) (newState string, more bool, err error) {
		// since からの /changes を呼び、返った id に応じて取得する
	})
```

`_watches` を追うのは Go のクライアントだけです。
接続を保持するのは生成コードではなくランタイムの仕事で、TypeScript と Rust のランタイムはそれをしません。
それらの言語で watch するクエリを生成すると、ループのないクエリだけが生成され、その旨が表示されます。

`Watch` のさらに下にあるのが `Client.EventSource` で、プッシュエンドポイントに接続してイベントをそのまま返します。

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
		_ = state
	}
}
```

これはイベントソース形式のプッシュで、接続を保持できるクライアントに向いています。
もう一つの形式は、サーバが送る先の URL を登録するもので、スマートフォンのアプリにはこちらが必要です。
[`example/queries`](example/queries) の `RegisterPush` と `ConfirmPush` を参照してください。
購読は作成した時点ではまだ有効ではありません。
サーバが URL にコードを送り、クライアントが `PushSubscription/set` でそれを書き戻すまで、他には何も送られません。
届いたものは `jmapc.PushVerification` でデコードします。

## ベンダ拡張

JMAP は拡張される前提の設計です。
サーバは独自のケイパビリティ URI を広告し、それとともに jmapc の知らない型とメソッドが現れます。
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
go generate ./...    # ランタイムの型と、全言語のサンプルクライアントを再生成する
```

サンプルは言語ごとに三度生成され、`example/jmapq`、`example/ts`、`example/rust/src/jmapq` に出力されます。
残る二つがコンパイルできるかどうかは Go のテストでは分からないので、CI は TypeScript に `tsc --strict` を、Rust に `cargo fmt --check` と `cargo test` を実行します。
どちらにも生成コードと並ぶ手書きの検査があり、スタブを相手にランタイムを動かします。
ヘッダが送られること、認証がそれに優先すること、セッションがキャッシュされること、そして 200 を返しながら拒否を含む `/set` がやはりエラーになることを確かめます。

スキーマも同じやり方で、同じ理由から検証します。
バリデータが example のクエリを受け入れ、スキーマが捕まえると主張する間違いを拒むかどうかは、Go のテストには言えません。
`example/schema/check.mjs` が、その時点のカタログから書き出したスキーマに対してバリデータに尋ねます。

ここでジェネレータをソースから実行しているのは、このリポジトリがジェネレータの居場所だからです。

ランタイムの型とサンプルのクライアントはコミットされていて、それらをカタログが今生成する結果と比較するテストがあります。
データモデルを変えたのに再生成し忘れると、見逃されるのではなくビルドが失敗します。
CI では同じ検証に加えて、gofmt、go vet、govulncheck を実行します。

## 対応範囲

JMAP は仕様の集まりです。
サーバはケイパビリティ URI を広告し、それぞれが固有の型とメソッドを持ち込みます。
以下は [IANA が登録しているもの](https://www.iana.org/assignments/jmap/jmap.xhtml)と、それぞれに対する jmapc の状況です。

| ケイパビリティ | 仕様 | サポート |
|---|---|---|
| `urn:ietf:params:jmap:core` | [RFC 8620](https://www.rfc-editor.org/rfc/rfc8620) | ✅ |
| `urn:ietf:params:jmap:mail` | [RFC 8621](https://www.rfc-editor.org/rfc/rfc8621) | ✅ |
| `urn:ietf:params:jmap:submission` | [RFC 8621](https://www.rfc-editor.org/rfc/rfc8621) | ✅ |
| `urn:ietf:params:jmap:vacationresponse` | [RFC 8621](https://www.rfc-editor.org/rfc/rfc8621) | ✅ |
| `urn:ietf:params:jmap:contacts` | [RFC 9610](https://www.rfc-editor.org/rfc/rfc9610) | ✅ |
| `urn:ietf:params:jmap:calendars` | [draft-ietf-jmap-calendars](https://datatracker.ietf.org/doc/draft-ietf-jmap-calendars/) | ✅ |
| `urn:ietf:params:jmap:principals:availability` | [draft-ietf-jmap-calendars](https://datatracker.ietf.org/doc/draft-ietf-jmap-calendars/) | ✅ |
| `urn:ietf:params:jmap:principals` | [RFC 9670](https://www.rfc-editor.org/rfc/rfc9670) | ✅ |
| `urn:ietf:params:jmap:principals:owner` | [RFC 9670](https://www.rfc-editor.org/rfc/rfc9670) | ✅ |
| `urn:ietf:params:jmap:smimeverify` | [RFC 9219](https://www.rfc-editor.org/rfc/rfc9219) | ✅ |
| `urn:ietf:params:jmap:blob` | [RFC 9404](https://www.rfc-editor.org/rfc/rfc9404) | ✅ |
| `urn:ietf:params:jmap:quota` | [RFC 9425](https://www.rfc-editor.org/rfc/rfc9425) | ✅ |
| `urn:ietf:params:jmap:sieve` | [RFC 9661](https://www.rfc-editor.org/rfc/rfc9661) | ✅ |
| `urn:ietf:params:jmap:mdn` | [RFC 9007](https://www.rfc-editor.org/rfc/rfc9007) | ✅ |
| `urn:ietf:params:jmap:webpush-vapid` | [RFC 9749](https://www.rfc-editor.org/rfc/rfc9749) | ✅ |

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

ケイパビリティのすべてが固有の型を持ち込むわけではありません。
S/MIME の検証は `Email` に四つのプロパティを足すだけで、型もメソッドも増やしません。
つまりメソッド名からは、そのケイパビリティが必要だと分かりません。
jmapc はクエリが触れたプロパティがどのケイパビリティに属するかを判断し、`using` に加えます。
`smimeStatus` を要求すれば、`urn:ietf:params:jmap:smimeverify` が自動で現れます。

型もメソッドも持たず、クライアントに伝えることだけを持つケイパビリティもあります。
VAPID がそれで、伝えるのは鍵です。
こうしたものはセッションから読みます。
`Session.Capability` は、jmapc が知らないケイパビリティも含めて、どれでも読めます。

```go
vapid, err := session.WebPushVAPID()
// vapid.ApplicationServerKey を push service への購読時に渡す。

var limits struct{ MaxSizeScript int `json:"maxSizeScript"` }
err = session.Accounts[accountID].Capability(jmapc.CapabilitySieve, &limits)
```

サポートしていないケイパビリティも、手が届かないわけではありません。
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

### 検証しないもの

一つだけあり、それは意図的なものです。

**開いた集合は意図的に検証しない**：仕様が値を固定しているプロパティは検証します。
一方、集合が開いているもの、たとえばメールボックスの `role`、メールのキーワード、`Content-Disposition` は検証しません。
サーバが受け付けたはずの値を拒否するほうが、綴り間違いを通すより害が大きいからです。
