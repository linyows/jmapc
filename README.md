<p align="right">English | <a href="https://github.com/linyows/jmapc/blob/main/README.ja.md">日本語</a></p>

<p align="center">
  <br><br><br>
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://github.com/linyows/jmapc/blob/main/misc/jmapc-dark-bg.svg?raw=true">
    <img alt="jmapc" src="https://github.com/linyows/jmapc/blob/main/misc/jmapc.svg?raw=true" width="280">
  </picture>
  <br><br><br>
</p>

<p align="center">
  <strong>jmapc</strong> is a JMAP compiler: you write the request, it writes the client.
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

jmapc is a compiler for JMAP. You write the request you want the server to
answer — the JSON the specification already defines — and jmapc generates a
**type-safe client** for it, in Go, Rust or TypeScript, having checked the
request against the specification first.

1. You write requests in JMAP.
1. You run jmapc to generate code with type-safe interfaces to those requests.
1. You write application code that calls the generated code.

A request, `requests/ListInboxEmails.jmap.json`:

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

and what `jmapc generate` makes of it:

```go
res, err := client.ListInboxEmails(ctx, c, client.ListInboxEmailsParams{
	MailboxID: inbox,
	Limit:     25,
})
```

`res.List` holds exactly the four properties the request asked for.

## Features

The features that distinguish jmapc come down to these five.

- Requests are written in JMAP itself, so there is no API of jmapc's own to learn
- Every request is checked before a line of code is generated, in the editor as
  it is typed, and against a running server
- The generated code decodes a type-safe response, handles the errors JMAP
  reports at every level, and carries the loops for push and for paging
- One set of requests generates a Go, a Rust and a TypeScript client, with
  external dependencies kept to a minimum
- It comes with a command that sends a request to a real server, a JMAP server
  to test your own code against, and a schema file that adds a capability jmapc
  does not know

## Motivation

JMAP is built around one idea. A request carries several method calls, and a
call may refer to the result of an earlier one, so a chain of dependent
operations costs a single round trip:

```json
{
  "using": ["urn:ietf:params:jmap:core", "urn:ietf:params:jmap:mail"],
  "methodCalls": [
    ["Email/query", {"filter": {"inMailbox": "mbx1"}}, "search"],
    ["Email/get", {"#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"}}, "fetch"]
  ]
}
```

The ids never come back to the client, which is why a JMAP client does not look
like a REST client, with a type per resource and a method per path.

Most clients expose this through a builder, which means learning JMAP *and*
learning the builder. But the request is the part you care about; the client is
not. So write the request, and let jmapc write the client — an approach it
takes from [sqlc](https://sqlc.dev).

## Compared with other JMAP clients

The JMAP clients in use today are libraries — [go-jmap](https://github.com/rockorager/go-jmap)
for Go, [jmap-client](https://github.com/stalwartlabs/jmap-client) for Rust,
[Jam](https://github.com/htunnicliff/jmap-jam) for TypeScript, and the others
[jmap.io lists](https://jmap.io/software/) — and they hand you the protocol as
an API to call at run time. jmapc runs before that, which shows in three
places.

**The request is written in JMAP, not in an API of the library's own.** A
builder spells the request its own way: `req.Invoke(&email.Get{...})` with a
`jmap.ResultReference{ResultOf, Name, Path}` in Go, `client.build()` and
`.updated_reference()` in Rust. You learn JMAP, and then you learn how that
library says it. A jmapc request file is the request object RFC 8620 defines and
nothing besides, so it can be lifted out of the specification, sent as it
stands with `jmapc run`, read by `jq`, and completed in an editor.

**The mistakes are found before the program runs.** A library checks what its
types describe — that a field exists, that its type fits — but what makes a
JMAP request correct is mostly values: `"name": "Email/query"` naming the call
a reference points at, `"path": "/ids"` selecting from it, the names in
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

What you give up is that a request is fixed when jmapc runs. A request whose
shape is decided at run time — a filter assembled from what a user typed — is
handed over as one parameter rather than built call by call, and a program that
assembles arbitrary requests is what a builder is for. Generated code is also
code: a step in the build, and files committed to the repository.

## Install

You generate the client with `go generate`, so record it in the module that
uses it:

```
go get -tool github.com/linyows/jmapc/cmd/jmapc
```

That pins a version in `go.mod`, and `go tool jmapc` runs it. Everyone who
builds the project — and CI — then generates with the same version, which
matters for a tool whose output is committed.

```go
//go:generate go tool jmapc generate
```

To put it on your PATH instead:

```
go install github.com/linyows/jmapc/cmd/jmapc@latest
```

Or take a binary from the [releases](https://github.com/linyows/jmapc/releases),
which is how a Rust or TypeScript project installs it, having no Go toolchain
to run `go tool` with.

## Use

The file name is the name of the function to generate.

```json
{
  "_doc": "ListInboxEmails returns the newest emails in one mailbox.",

  "methodCalls": [
    ["Email/query", {
      "_comment": "Find the ids of the matching emails.",
      "filter": {"inMailbox": "{{mailboxId}}"},
      "sort": [{"property": "receivedAt", "isAscending": false}],
      "limit": "{{limit}}"
    }, "search"],

    ["Email/get", {
      "_comment": "Fetch the message from its id.",
      "#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"},
      "properties": ["id", "subject", "from", "receivedAt"]
    }, "fetch"]
  ],

  "_returns": "fetch"
}
```

(`requests/ListInboxEmails.jmap.json`)

Generate:

```
jmapc generate                 # or: go generate ./...
```

Use it:

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

`res.List` is `[]ListInboxEmailsFetchEmail`, holding the four properties the request
asked for and nothing else. Ask for another property and the struct grows; ask
for one that does not exist and the build fails, with a suggestion.

`bodyProperties` narrows the parts of a message the same way, reaching through
their sub-parts, and a property naming a header field is typed by the form it
names: `header:List-Id:asText` is a `*string`, `header:To:asAddresses` a
`[]jmapc.EmailAddress`.

The file name determines every name in the generated code, and the call ids
determine the names within it: see
[Writing a request](docs/requests.md#generated-names).
[`example/requests`](example/requests) holds twenty-five requests, over mail,
contacts, calendars, sharing and filtering.

## Other languages

The same requests generate a Rust or a TypeScript client:

```
jmapc generate -lang rust -out src/jmap_client
jmapc generate -lang typescript -out src/jmapClient
```

Each runtime is generated alongside the requests, so the Rust output requires
serde and nothing else, and the TypeScript output has no dependencies at all,
its one platform requirement being `fetch`. Each spells the generated names the
way its own language spells them, and both express a nullable property and a
union of shapes more precisely than Go does:
[Other languages](docs/languages.md).

## Verification

Requests are checked when jmapc runs, so one that is wrong about JMAP is a
build failure rather than a server round trip: that the method exists, that
every argument belongs to it, that a back reference points at an earlier call
and selects a value the target argument accepts, that filters, `properties`,
`sort` orders and patch pointers name what the type actually has. A misspelling
produces a suggestion:

```
requests/BadQuery.jmap.json: methodCalls[0].arguments.filter.hasAttachmnt: EmailFilterCondition has no property "hasAttachmnt"
	did you mean "hasAttachment"?
```

`jmapc check` runs the checks without writing anything, and `-session` adds what
only a running server can answer — the capabilities it advertises, the accounts
it holds, how much it accepts in one request. Most of the checks run in the
editor too, from a JSON Schema jmapc writes. The whole list is in
[Verification](docs/verification.md).

## Documentation

Everything above is the whole of jmapc in outline. The rest is under `docs/`,
one file to a subject, in the order they are worth reading in.

The first three are what you have in front of you while writing a request: what a
request file may hold, what jmapc checks before it generates anything, and how to
send a request and look at the answer before there is any code that calls it. The
next four describe what the generated code calls into once it runs — errors,
blobs, changes the server pushes, an answer that arrives one part at a time, and
a server to test your own code against. The last four are reference: the Rust
and TypeScript clients, capabilities jmapc does not know, the ones it does, and
jmapc itself.

| | |
|---|---|
| [Writing a request](docs/requests.md) | The request file, its parameters, and the names generated from it |
| [Verification](docs/verification.md) | What is checked at build time, against a server, and in the editor |
| [The jmapc command](docs/cli.md) | Sending a request with `jmapc run`, and configuration |
| [The runtime](docs/runtime.md) | Errors, large `/get`s, tokens, retries, observability, and blobs |
| [Push](docs/push.md) | Following changes as the server reports them |
| [Walking an answer that does not fit in one request](docs/paging.md) | Reading a result the server returns one part at a time |
| [Testing](docs/testing.md) | jmaptest, a JMAP server to test your code against |
| [Other languages](docs/languages.md) | The Rust and the TypeScript client |
| [Vendor extensions](docs/extensions.md) | Types and methods jmapc does not know, described in a schema file |
| [Coverage](docs/coverage.md) | The capabilities and the methods supported |
| [Working on jmapc](docs/contributing.md) | Building, testing and releasing jmapc itself |

## Coverage

jmapc supports every JMAP capability [IANA
lists](https://www.iana.org/assignments/jmap/jmap.xhtml) — core, mail,
submission, contacts, calendars, principals, sieve, quota, blob and the rest —
and the 81 methods they bring, all checked and generated the same way. A
capability that is not among them is described in a
[schema file](docs/extensions.md) and checked like any other. What each
capability brings, and the one thing jmapc deliberately does not check, is in
[Coverage](docs/coverage.md).

## Author

[linyows](https://github.com/linyows)
