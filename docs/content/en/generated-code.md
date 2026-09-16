# Generated names

The file name determines every name in the generated file, so it has to be a Go
identifier: letters, digits and underscores, not starting with a digit.
`ListInboxEmails.jmap.json` gives:

| Generated | Name |
| --- | --- |
| The function | `ListInboxEmails` |
| Its parameters, where the request leaves any open | `ListInboxEmailsParams` |
| A record whose properties the request narrows | `ListInboxEmailsFetchEmail`, after the call id `fetch`, and `ListInboxEmailsFetchEmailBodyPart` for a narrowed body part. A call asking for a named set answers with the set's own name instead; see [Property sets](properties.md) |
| The response to a call returning that record | `ListInboxEmailsFetchResponse`, after the call id `fetch` |
| The result, where `_returns` names no call | `ListInboxEmailsResult` |
| The function that follows changes, where the request is watched | `SyncEmailsWatch` |
| The walk over the parts of an answer, where the request is paged | `SearchEmailsPages` |
| The file | `listinboxemails_gen.go` |

The call ids are the names in the generated code: a result holds one field per
call, named after the id the request gave it, and the record and response types
of a call that narrows are named the same way. Nothing is numbered by position,
since a call id is unique within a request already and inserting a call ahead
of another would otherwise move a name onto a different shape:

```json
["Email/query", {...}, "search"],
["Email/get",   {...}, "fetch"]
```

```go
res.Search.IDs      // the Email/query response
res.Fetch.List      // the Email/get response
```

A call id that is not an identifier — RFC 8620 allows any string — falls back
to the method it invokes.

A call the request does not narrow answers with the shared type instead, so
`SendEmail` returns `*jmapc.EmailSubmissionSetResponse`. Two requests in one
package cannot take the same name, and a generated type whose name is already
taken gains a number: `ListInboxEmailsFetchEmail2`.

Two calls of one request that read the same type through the same method and ask
for the same properties describe one record, so they share one type, named
after the first of them. Two calls asking for the same named set share the type
that set names, wherever in the package they are; a call spelling the same
properties out is not one of them, since the name a set stands for is the one
the author chose. Two names for one shape would make a caller convert
between them to hand a record from one call to a function written for the
other. Inserting a call that reads that same shape ahead of both moves the name
to the new call, which the build reports; the shape a name stands for does not
change.

A `/set` that creates gets a constant for each name it gives a record, since
the response reports the record back under that name:

```json
["Mailbox/set", {"create": {"newMailbox": {"name": "{{name}}"}}}, "make"]
```

```go
res, err := client.CreateMailbox(ctx, c, client.CreateMailboxParams{Name: name})
...
created := res.Created[client.CreateMailboxNewMailbox]
```

Without it the name appears in two files with no link between them, and
renaming it in the request still builds: the lookup misses at run time instead.
Rust writes `CREATE_MAILBOX_NEW_MAILBOX` and TypeScript
`createMailboxNewMailbox`. A creation id the request leaves to the caller —
`{"{{creationId}}": ...}` — has no constant, since the caller already has the
name.

A request with no open parameters takes no `Params` argument at all —
`MailQuota(ctx, c)`, not `MailQuota(ctx, c, MailQuotaParams{})` — so adding
the first `{{param}}` to a request already in use changes the generated
function's arity and breaks every call site. That is a deliberate trade for
the common case of a request with no parameters reading like a plain function
call, not an oversight.

Rust writes the function and its module in snake_case — `list_inbox_emails` in
`list_inbox_emails.rs` — and keeps the type names above, except that an
initialism becomes a word, since that is how Rust spells one: a `UTCDate` is a
`UtcDate`. Properties are snake_case, with a serde rename wherever that is not
the name on the wire.

TypeScript lowercases the first letter of the function and of the file —
`listInboxEmails` in `listInboxEmails.ts` — and keeps the type names too.
