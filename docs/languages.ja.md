<p align="right"><a href="languages.md">English</a> | 日本語</p>

# 他の言語

## Rust

同じリクエストから Rust 用のクライアントを生成できます。

```
jmapc generate -lang rust -out src/jmap_client
```

```rust
use jmap_client::list_inbox_emails::{list_inbox_emails, ListInboxEmailsParams};
use jmap_client::Client;

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
`client.rs`、`types.rs`、そしてそれらをリクエストと並べて宣言する `mod.rs` が出力されるので、クレート側が足すのは `mod jmap_client;` の一行だけです。
生成されたコードが求めるのは **serde と serde_json** だけです。
送受信は `Transport` として利用者が実装します。
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
複数の形を取る値は enum になります。
フィルタは `Option<FilterOperatorOrEmailFilterCondition>` という untagged な enum で、Go では同じ名前の、形ごとにフィールドを持つ構造体になります。
形ではなく書式を持つプリミティブは `String` の名前付き別名になるので、シグネチャの上で `Id` と `TimeZoneId` が読み分けられます。
そしてレコードは `Default` を導出します。
省略可能なプロパティを五十個持つ型を組み立てられるのはこれのおかげで、必要な二つだけを名指しして残りは任せられます。

生成されるコードは rustfmt が整形した形そのものなので、クレートに `cargo fmt` をかけても何も動きません。

## TypeScript

同じリクエストから TypeScript 用のクライアントを生成できます。

```
jmapc generate -lang typescript -out src/jmapClient
```

```typescript
import { Client } from "./jmapClient/client.js"
import { listInboxEmails } from "./jmapClient/listInboxEmails.js"

const client = new Client("https://example.com/.well-known/jmap", { auth: token })

const res = await listInboxEmails(client, { mailboxId: inbox, limit: 25 })
for (const email of res.list) {
  console.log(email.receivedAt, email.from?.[0].email, email.subject)
}
```

ランタイムも一緒に生成されます。
`client.ts` と `types.ts` がリクエストと並んで出力されるので、生成物には**依存がありません**。
プラットフォームに求めるのは `fetch` だけです。

TypeScript のほうが正確に言えることもあります。
null を取りうるプロパティはポインタではなく union なので、`subject` は `string | null` です。
複数の形を取る値も union で書けます。
フィルタは `FilterOperator | EmailFilterCondition | null` で、Go では形ごとにフィールドを持つ構造体になるところです。
形ではなく書式を持つプリミティブは `string` の名前付き別名になるので、`Id` と `TimeZoneId` を取り違えることがありません。
