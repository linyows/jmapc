# Getting started

This page takes a Go module from nothing to a call that reads a mailbox. A Rust
or TypeScript project follows the same steps with a different `-lang`; see
[Rust and TypeScript](languages.md) for what differs.

## Installing jmapc

The client is generated with `go generate`, so record jmapc as a tool of the
module that uses it:

```
go get -tool github.com/linyows/jmapc/cmd/jmapc
```

That pins a version in `go.mod`, and `go tool jmapc` runs it. Everyone who
builds the project, and CI, then generates with the same version, which matters
for a tool whose output is committed.

To put it on your PATH instead:

```
go install github.com/linyows/jmapc/cmd/jmapc@latest
```

A Rust or TypeScript project, having no Go toolchain to run `go tool` with,
takes a binary from the [releases](https://github.com/linyows/jmapc/releases).

## Writing a request

A request lives in a file of its own under `requests/`, and the file name is
the name of the function to generate.

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
      "_comment": "Fetch the messages from their ids.",
      "#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"},
      "properties": ["id", "subject", "from", "receivedAt"]
    }, "fetch"]
  ],

  "_returns": "fetch"
}
```

(`requests/ListInboxEmails.jmap.json`)

Everything without an underscore is the JMAP request RFC 8620 defines. The
`{{mailboxId}}` and `{{limit}}` are left for the caller to fill in, `_returns`
makes the `Email/get` response the function's result, and `accountId` is left
out, so the generated function takes it from the session.
[Writing a request](requests.md) describes each of these.

## Generating the client

Put the directive beside `go.mod`:

```go
package mail

//go:generate go tool jmapc generate
```

Then generate:

```
go generate ./...
```

jmapc checks the request against the specification first, and a request that
is wrong about JMAP stops here, with a suggestion where a name is misspelled:

```
requests/ListInboxEmails.jmap.json: methodCalls[1].arguments.properties[1]: Email has no property "subjct"
	did you mean "subject"?
```

A request that checks out becomes `client/listinboxemails_gen.go`, in a package
named `client`. The directories are the defaults; a `jmapc.json` changes them,
as [The jmapc command](cli.md#configuration) describes. `jmapc run` sends the
request to a server before any code calls it; see
[Sending a request](run.md).

## Calling it

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

`jmapc.New` takes the session URL, which `WellKnownURL` finds from the host
name, and the options that authenticate and configure the client; see
[Configuring the client](client.md).

`res.List` is a `[]client.ListInboxEmailsFetchEmail`, holding the four
properties the request asked for and nothing else. Add a property to the
request and generate again, and the struct gains a field. The names of the
generated types come from the file name and the call ids, as
[Generated names](generated-code.md) describes.

`err` is not only a failed HTTP request. JMAP also reports a failure in a
response that returns 200, and the generated function returns that as an error
too; see [Errors](errors.md).

## Keeping the client up to date

The generated files are committed, so they can fall behind a request that was
changed without generating again. `generate -check` writes nothing and fails
where the files on disk differ from what the requests generate now, which is
what a CI step runs:

```yaml
- run: go tool jmapc generate -check
```

## Where to go next

The pages under *Requests* describe what a request file may hold and what jmapc
checks. The pages under *Generated code* describe what the generated client does
at run time: errors, push, paging, blobs, the client's options, and testing
your own code against a JMAP server.
[`example/requests`](https://github.com/linyows/jmapc/tree/main/example/requests)
holds twenty-five requests, over mail, contacts, calendars, sharing and
filtering.
