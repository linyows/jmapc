<p align="right">English | <a href="paging.ja.md">日本語</a></p>

# Walking an answer that does not fit in one request

A JMAP answer is often only part of an answer. A `/query` returns just the
window of results the caller asked for, and says where that window sits in the
full result. A `/changes` returns only as many changes as the server chooses to,
and reports whether there are more.

Either way, getting the rest means sending another request, and what goes in it
comes from the last answer: `position` for a `/query`, `sinceState` for a
`/changes`. Naming a call in `_pages` generates that loop, called a walk, so it
does not have to be written by hand.

The call named in `_pages` is the one the walk resends on each step. Where the
next request should start — `position` for a `/query`, `sinceState` for a
`/changes` — is managed by the walk rather than by the caller, so it is written
as a parameter in the request:

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

Go gets an iterator. Advancing it sends the next request:

```go
for page, err := range client.SearchEmailsPages(ctx, c, params) {
	if err != nil {
		return err
	}
	for _, email := range page.EmailGet.List {
		fmt.Println(*email.Subject)
	}
}
```

TypeScript gets an async generator that does the same, one step at a time. A
failure throws as it does from the generated function itself:

```ts
for await (const page of searchEmailsPages(client, params)) {
  for (const email of page.emailGet.list) console.log(email.subject)
}
```

Rust gets a value that holds the current position, and advances the same way. A
stream would require a crate to define one, and the generated code requires
serde and nothing else:

```rust
let mut pages = search_emails_pages(params);
while let Some(page) = pages.next(&client).await? {
    for email in &page.email_get.list {
        println!("{:?}", email.subject);
    }
}
```

When a walk stops depends on what it is walking.

A `/query` walk starts from the `position` the parameters carry, so it can
resume where a previous walk stopped. An empty window ends the walk instead of
being yielded, so every page the walk yields holds at least one record. Where
the call asked for the total, the walk also stops once the next request would
be past that total.

A `/changes` walk yields even an answer reporting no changes, because that
answer still carries the `sinceState` to continue from. It ends only when the
server reports no further changes.

A watched request is resent already while the server reports more changes, so
`_watches` and `_pages` are never written on the same request.
