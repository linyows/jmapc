# Why jmapc

jmapc is a compiler for JMAP. You write the request you want the server to
answer, in the JSON the specification already defines, and jmapc checks it
against the specification and generates a type-safe client for it, in Go, Rust
or TypeScript.

## Motivation

JMAP is built around one idea. A request carries several method calls, and a
call may refer to the result of an earlier one, so a chain of dependent
operations costs a single round trip, as the example in
[Introduction](introduction.md) shows. The ids never come back to the client,
which is why a JMAP client does not look like a REST client, with a type per
resource and a method per path.

Most clients expose this through a builder, which means learning JMAP *and*
learning the builder. But the request is the part you care about; the client is
not. So write the request, and let jmapc write the client, an approach it takes
from [sqlc](https://sqlc.dev).

## Compared with other JMAP clients

The JMAP clients in use today are libraries: [go-jmap](https://github.com/rockorager/go-jmap)
for Go, [jmap-client](https://github.com/stalwartlabs/jmap-client) for Rust,
[Jam](https://github.com/htunnicliff/jmap-jam) for TypeScript, and the others
[jmap.io lists](https://jmap.io/software/). They hand you the protocol as an API
to call at run time. jmapc runs before that, which shows in three places.

**The request is written in JMAP, not in an API of the library's own.** A
builder spells the request its own way: `req.Invoke(&email.Get{...})` with a
`jmap.ResultReference{ResultOf, Name, Path}` in Go, `client.build()` and
`.updated_reference()` in Rust. You learn JMAP, and then you learn how that
library says it. A jmapc request file is the request object RFC 8620 defines and
nothing besides, so it can be lifted out of the specification, sent as it
stands with `jmapc run`, read by `jq`, and completed in an editor.

**The mistakes are found before the program runs.** A library checks what its
types describe (that a field exists, that its type fits), but what makes a JMAP
request correct is mostly values: `"name": "Email/query"` naming the call a
reference points at, `"path": "/ids"` selecting from it, the names in
`properties`, the conditions in a filter, the pointers in a patch. To a library
those are strings, and a wrong one comes back from the server. jmapc checks
them against the data model as it generates, so a back reference naming the
wrong method fails the build.

**The response holds what the request asked for.** go-jmap returns
`Invocation.Args` as an `any` to type-switch on; jmap-client unwraps a chain of
`unwrap_method_responses()` and `unwrap_get_mailbox()`. A generated function
returns a named type carrying exactly the properties the request listed. Jam
reaches this in TypeScript, whose literal types can narrow a response by the
`properties` given; jmapc gets the same narrowing in Go and Rust, where the
type system cannot do it on its own.

## What you give up

A request is fixed when jmapc runs. A request whose shape is decided at run
time, such as a filter assembled from what a user typed, is handed over as one
parameter rather than built call by call, and a program that assembles
arbitrary requests is what a builder is for.

Generated code is also code: a step in the build, and files committed to the
repository. [The jmapc command](cli.md#checking-that-the-generated-client-is-up-to-date)
describes how a build stops where the committed files have fallen behind the
requests.
