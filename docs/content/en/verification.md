# Verification

Everything below is a compile-time failure rather than a server round trip:

- the method exists, and is spelled the way the specification spells it
- every argument belongs to the method, with the type the method requires
- a back reference points at an *earlier* call, names that call's method
  correctly, and selects a value the target argument can accept, null
  included: a value that may be null fills only an argument that accepts null
- a back reference reading a property of the records reads one that call
  fetches: `/list/*/threadId` against a call that narrowed its `properties` to
  the subject would resolve to nothing at the server; a call that leaves its
  `properties` out fetches the method's defaults, which for `Email/get` and
  `Email/parse` are the lists RFC 8621 gives, as are the body parts of a call
  that leaves `bodyProperties` out; a property the method returns as null, as
  the id of a parsed email, is refused in `properties` and in a back reference
  alike
- filter conditions are checked against the type being queried, including the
  ones nested inside `AND`, `OR`, and `NOT` operators
- `properties` names properties the type has, and `bodyProperties` names
  properties an `EmailBodyPart` has
- a property naming a header field is asked of an `Email` or an
  `EmailBodyPart`, the types that have header fields, and asks for a parsed
  form the specification defines, so `header:List-Id:asText` is a string and
  `header:To:asAddresses` a list of addresses, and one it allows for that
  field, so `header:Subject:asDate` is refused; an email created with one is
  given no `Content-*` field, which belongs to a body part, and no header field
  twice, in two forms or as the property that stands for it, such as `subject`
- a `PatchObject` points at properties the record being patched actually has,
  and sets them to values of the right type, its keys written the way RFC 8620
  writes them: the leading `/` of the pointer is implicit, so a keyword is set
  at `keywords/$seen` rather than at `/keywords/$seen`, and the pointer stops at
  a list, which is replaced as a whole rather than patched in place
- `sort` names properties the type can actually be sorted by, and supplies the
  extra member a comparator like `hasKeyword` needs
- a property whose specification fixes the values it may take is given one of
  them, whether it is a string or the keys of a set like a participant's `roles`
- ids, dates, and integers are well formed
- the capabilities the request declares cover the methods it calls
- a watched call is one that reports what changed since a state, and the state
  it continues from is supplied by the loop rather than written into the request
- a paged call is one that returns part of a longer result and reports where the
  rest is, and where the next request starts is supplied by the walk

A misspelling produces a suggestion:

```
requests/BadQuery.jmap.json: methodCalls[0].arguments.filter.hasAttachmnt: EmailFilterCondition has no property "hasAttachmnt"
	did you mean "hasAttachment"?
requests/BadQuery.jmap.json: methodCalls[1].arguments.#ids.name: the referenced call is Email/query, but the reference names Email/get
	call "c0" invokes Email/query
```

Two requests that differ only in what they call their parameters and their
calls are one request written twice, and jmapc says so rather than failing:

```
jmapc: ListArchiveEmails, ListInboxEmails are the same request under different names; one of them would do for all of them
```

Both are generated all the same, since a project may want two names for one
request. It is worth knowing about because each name produces a set of
generated types of its own.

Within one request, a call that asks what an earlier call asked is noted the
same way: the same method with the same arguments, whatever order they are
written in and whatever their `_comment` says, and the same parameters by name.
A value the request states outright is compared as it was written.

```
jmapc: ListInboxEmails: call "again" asks Email/get what call "fetch" already asked, with nothing between them that changes data; refer to "fetch" instead
```

RFC 8620 lets data change between the calls of one request, so reading, writing
and reading again is a request that means something, and a call is noted only
where every call between the two only reads: a `/get`, `/query`, `/changes`,
`/queryChanges`, `/parse` or `/validate` of the specifications jmapc knows,
`Core/echo`, `Blob/lookup` or `Principal/getAvailability`. Any other method is
taken as one that may change data, and so is every method a vendor schema
defines, whatever its name: nothing says a vendor's `Note/get` only reads.

`jmapc validate` runs the checks without writing anything.

## What only the server can report

Everything above is what the specifications say. What they leave to the server —
which capabilities it has, which accounts it holds, how much it accepts in one
request — a build cannot know, and a request that is right about JMAP and wrong
about the server it runs against fails at run time. `-session` checks against a
running server:

```
jmapc validate -session jmap.example.com -token $JMAP_TOKEN
validated 25 requests against https://jmap.example.com/api/, as someone@example.com
```

What it reports:

- a capability the request declares and the server does not advertise
- an account the request names that the session does not hold, an account the
  session cannot fill in because it has no primary account for the capability,
  and an account that does not support what the call needs
- more calls than `maxCallsInRequest`, more records than `maxObjectsInGet`, more
  changes than `maxObjectsInSet`, a request already larger than `maxSizeRequest`
  before its parameters are filled in
- a `collation` the server does not compare strings with

What the request leaves to its caller is not checked: a parameter standing for a
list of ids may be any length, and an assumption about it would report a
problem in a request that is correct.

What those parameters turn out to be is checked at run time, where the request
has been encoded and its size is known. A request larger than `maxSizeRequest`
is refused before it is sent, as is one declaring a capability the session does
not advertise or holding more calls than `maxCallsInRequest`. The server answers
each of those with a 400 that no retry policy sends again, so the round trip
buys nothing. `jmapc.WithoutPreflightChecks` turns them off for a client whose
server under-reports what it takes.

The session URL is the one value not read from the environment — `-token` and
`-user` fall back to `$JMAP_TOKEN` and `$JMAP_USER` — because a check that
reaches the network should be requested on the command line rather than
triggered by whatever the environment happens to hold.

## At startup

`-session` checks against whichever server it is pointed at when it runs. The
server a program meets in production may not be that one, and a request on a
path the program rarely takes is checked only when the path is taken. The
client is generated with a function that checks every request in its package
against the session the client holds, in each of the three languages:

```go
if err := client.Verify(ctx, c); err != nil {
	log.Fatal(err)
}
```

```ts
import { verify } from "./client/verify.js"

await verify(client) // throws VerifyErrors, with a VerifyError per problem in errors
```

```rust
jmap_client::verify(&client).await?; // Error::Verify(Vec<VerifyError>)
```

It reports a capability a request declares and the server does not advertise,
a primary account the session does not name for a request that leaves the
account to it, a primary account that does not support the capability it is
used for, and more calls than `maxCallsInRequest`: what a request is refused
for before it is sent, for every request at once. Every problem is reported,
each as a `VerifyError` naming its request and holding the error sending the
request would have failed with. Go returns them joined, each a
`*jmapc.VerifyError`; TypeScript throws a `VerifyErrors` whose `errors` hold
them; Rust returns `Error::Verify` holding them:

```
jmapc: SendReadReceipt would be refused: server does not support urn:ietf:params:jmap:mdn
jmapc: VerifiedSignatures would be refused: server does not support urn:ietf:params:jmap:smimeverify
```

It checks what a request file says, not what a caller passes for its
parameters, so it reports less than `-session` does: the number of records a
call names and the size of a request are known only once it is sent. A request
or a set of properties cannot be named `Verify`, since the function is
generated under that name, and a request is refused where its file would be
one another file is generated into: `verify_gen.go` for a request named
`verify`, `client.ts` for one named `Client`, `mod.rs` for one named `Mod`.
`jmapc validate` refuses these as `generate` does, for the language it is given.

## Editor support

The checks above run when jmapc does. Most of them can run while the request is
being typed instead, because they are checks on the file itself, and a JSON file
that names a schema is one an editor can already check and complete.

```
jmapc schema -out jmapc.schema.json
```

That writes a JSON Schema for the catalogue, vendor extensions and all. Point a
request file at it:

```json
{
  "$schema": "../jmapc.schema.json",
  "methodCalls": [["Email/query", {"filter": {"inMailbox": "{{mailboxId}}"}}, "search"]]
}
```

or point the editor at every request at once, which in VS Code is:

```json
{
  "json.schemas": [
    {"fileMatch": ["*.jmap.json"], "url": "./jmapc.schema.json"}
  ]
}
```

Either way the editor completes a method name, offers the arguments that method
takes and the properties the type has, and underlines a misspelling where it was
written. A filter nested inside an `AND` is checked like one outside it, a
comparator offers the properties the type can actually be sorted by, and a
`{{parameter}}` is accepted anywhere a value goes.

What a schema cannot say is the part that depends on another call: that a back
reference names an earlier call and selects a value the argument accepts. That
stays jmapc's to check, which is why the editor is a first pass rather than a
replacement for the build.
