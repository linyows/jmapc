<p align="right">English | <a href="languages.ja.md">日本語</a></p>

# Other languages

## Rust

The same requests generate a Rust client:

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

The runtime comes with it — `client.rs`, `types.rs`, and the `mod.rs` that
declares them beside the requests — so `mod jmap_client;` is the whole of what a crate
has to add. The generated code requires **serde and serde_json** and nothing
else. Transmission is a `Transport` you implement over whichever HTTP client
the program already has, so no HTTP stack, no TLS backend and no async runtime
is added with it:

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

That is also where authentication that a bearer token does not cover belongs —
a signature over the request, a token refreshed on expiry — since the transport
is the last stage before a request is sent.

A nullable property is an `Option`, so `subject` is `Option<String>`. A union of
shapes is an enum: a filter is `Option<FilterOperatorOrEmailFilterCondition>`,
untagged, where Go has a struct of the same name with a field per shape. The
primitives that
carry a format rather than a shape are named aliases of `String`, so an `Id` and
a `TimeZoneId` are distinguishable in a signature. And a record derives
`Default`, so a type with fifty optional properties is built by naming the two
that differ from the default and omitting the rest.

The generated code is already formatted the way rustfmt formats it, so
`cargo fmt` over the crate changes nothing.

## TypeScript

The same requests generate a TypeScript client:

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

The runtime comes with it — `client.ts` and `types.ts` are generated alongside
the requests — so the output has **no dependencies**. The only platform
requirement is `fetch`.

TypeScript expresses some things more precisely than Go. A nullable property is
a union rather than a pointer, so `subject` is `string | null`. A union of
shapes is written as one: a filter is `FilterOperator | EmailFilterCondition |
null`, where Go has a struct with a field per shape. And the primitives that carry a format
rather than a shape are named aliases of `string`, so an `Id` and a
`TimeZoneId` cannot be swapped by accident.
