# Writing jmapc requests

This guide is for a coding agent adding or changing the requests of a project
that uses jmapc. It matches the jmapc that printed it. Where it and something
remembered from elsewhere disagree, follow this guide.

Run `jmapc guide <topic>` for one section: files, workflow, syntax, mistakes,
examples or reference.

## Files

jmapc reads a directory of request files and generates a typed client from them,
in Go, Rust or TypeScript. You write the requests; you do not write the client.

- One file is one JMAP request, and its name is the name of the function
  generated for it: `ListInboxEmails.jmap.json` becomes `ListInboxEmails`. Name
  it in UpperCamelCase after what it does.
- A file is a JMAP Request object as RFC 8620 defines it, in plain JSON:
  `methodCalls`, and optionally `using`.
- A member beginning with an underscore is read by jmapc and never sent to the
  server. Everything else is JMAP.
- `properties.json`, beside the request files, names sets of properties that
  several requests ask for. It is optional.
- `jmapc.json`, at the project root, says where the requests are (`requests`,
  default `requests`), where the client goes (`out`, default `client`), which
  language (`lang`: go, rust or typescript) and which vendor schemas to load
  (`schemas`).
- Every generated file starts with a banner saying it is generated. Never edit
  one: change the request and generate again.

## Workflow

1. Read `jmapc.json` to find the requests directory and the language.
2. Read the existing requests and `properties.json`. Reuse a property set where
   one has the shape you need, and follow the naming the project already uses.
3. Write or change the request file.
4. Run `jmapc validate`. It checks every request against the JMAP data model:
   method names, argument names and types, result references, properties,
   capabilities, and names that collide with what jmapc generates for the
   language `jmapc.json` sets. Each error names the place in the file and says
   how to fix it. Fix what it reports and run it again until it passes.
5. Run `jmapc generate`.
6. Build and test the project as usual, then run `jmapc generate -check`, which
   fails where the generated client is out of date.

In a Go module that lists jmapc as a tool, run it as `go tool jmapc`.

To see the request a file sends, without a server:

    jmapc run ListInboxEmails -p mailboxId=inbox -p limit=10 -dry-run

With a server, drop `-dry-run` and pass `-session` (or set `JMAP_SESSION_URL`)
and `-token` (or `JMAP_TOKEN`). `jmapc validate -session <url>` checks the
requests against what that server supports.

## Syntax

### Members jmapc reads

| Member | Meaning |
|---|---|
| `_doc` | The generated function's documentation. Start it with the request's name. |
| `_returns` | The call id whose response the function returns. Without it, every response is returned. |
| `_watches` | The call id of a `/changes` call. For Go, generates a function that follows pushes and runs the request whenever there is something to catch up on. TypeScript and Rust get the request without that function. |
| `_pages` | The call id of a `/query` (with `{{position}}`) or `/changes` (with `{{sinceState}}`) call. Generates a function that sends the request again until the whole result is read. |
| `_createdIds` | `true` to take creation ids from an earlier request and report this one's. |
| `_comment` | Inside a call's arguments: why the call is there. Stripped before sending. JSON has no comments; use this. |
| `$schema` | The JSON Schema an editor checks the file against, written by `jmapc schema`. |

### Parameters

- `{{name}}` stands for a value the caller supplies. It must be the whole JSON
  value, never part of a string. Its type comes from the argument it is in, so
  `{{limit}}` in `limit` is an UnsignedInt and `{{mailboxId}}` in `inMailbox`
  is an Id. The same name used twice is one parameter.
- `{{name?}}` is a parameter the caller may leave out, and then the argument is
  not sent at all, which is not the same as sending null. Only a whole argument
  of a method call may be optional, not a value inside a filter or an array.
- A parameter may be a map key, which is how a `/set` names the record to
  change: `"update": {"{{emailId}}": {"keywords/$seen": true}}`.
- To let the caller build a filter of any shape, make the whole filter one
  parameter: `"filter": "{{filter}}"`.

### Other conventions

- Leave `accountId` out. The generated function fills in the primary account.
  Write `"accountId": "{{accountId}}"` only where the caller must choose.
- Leave `using` out. jmapc derives it from the methods called.
- Refer to an earlier call's result with `#` and a ResultReference, as RFC 8620
  defines: `"#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"}`.
  `resultOf` is that call's call id, and `name` its method name.
- A call is `[method, arguments, callId]`. Give each call a short, meaningful
  call id: the generated result fields are named after it.
- `"properties": "@EmailSummary"` asks for a set from `properties.json` instead
  of a list. A set has `type` and `properties`, or `extends` another set and
  adds `properties`, and may have a `doc`.

## Mistakes

Each example in this section is wrong, and `jmapc validate` refuses it. The
text above each says what to write instead.

A parameter written with a dollar sign. JMAP uses `$` in keywords such as
`$seen`, so jmapc uses braces: `"inMailbox": "{{mailboxId}}"`.

```json wrong
{"methodCalls": [["Email/query", {"filter": {"inMailbox": "$mailboxId"}}, "search"]]}
```

A parameter inside a larger string. A parameter is a whole value.

```json wrong
{"methodCalls": [["Email/query", {"filter": {"subject": "Re: {{subject}}"}}, "search"]]}
```

A comment in the JSON. Put it in `_comment` instead.

```json wrong
{"methodCalls": [["Email/query", {
  // newest first
  "sort": [{"property": "receivedAt", "isAscending": false}]
}, "search"]]}
```

An optional parameter inside a filter. Make it required, or make the whole
filter the parameter.

```json wrong
{"methodCalls": [["Email/query", {"filter": {"inMailbox": "{{mailboxId?}}"}}, "search"]]}
```

A result reference to a property the earlier `/get` did not fetch. Add it to
that call's `properties`.

```json wrong
{"methodCalls": [
  ["Email/get", {"ids": "{{ids}}", "properties": ["id", "subject"]}, "emails"],
  ["Thread/get", {"#ids": {"resultOf": "emails", "name": "Email/get", "path": "/list/*/threadId"}}, "threads"]
]}
```

A result reference to a call id that is not there. `resultOf` is a call id, not
a method name.

```json wrong
{"methodCalls": [
  ["Email/query", {}, "search"],
  ["Email/get", {"#ids": {"resultOf": "Email/query", "name": "Email/query", "path": "/ids"}}, "fetch"]
]}
```

A property set named without `@`.

```json wrong
{"methodCalls": [["Email/get", {"ids": "{{ids}}", "properties": "EmailSummary"}, "fetch"]]}
```

A `using` that lacks a capability. Leave `using` out instead.

```json wrong
{"using": ["urn:ietf:params:jmap:core"], "methodCalls": [["Email/query", {}, "search"]]}
```

A request named after something jmapc generates: `Verify`, and for TypeScript
or Rust also `Client` and `Types`, for Rust `Mod`, or two requests whose names
differ only in case. `validate` refuses these for the language `jmapc.json`
sets. Rename the request.

Not refused by `validate`, but wrong all the same:

- Editing a generated file. The next `jmapc generate` replaces it.
- Sending `null` where the argument should be absent. Use `{{name?}}`.
- Writing two requests that differ only in a value. Make the value a parameter.

## Examples

These pass `jmapc validate` as they are.

A property set, in `properties.json`:

```json file=properties.json
{
  "EmailSummary": {
    "doc": "EmailSummary is what a list of messages shows.",
    "type": "Email",
    "properties": ["id", "threadId", "subject", "from", "receivedAt", "preview"]
  },
  "EmailWithFlags": {
    "extends": "EmailSummary",
    "properties": ["mailboxIds", "keywords"]
  }
}
```

A query and a get of what it found, in one request:

```json file=ListInboxEmails.jmap.json
{
  "_doc": "ListInboxEmails returns the most recent emails in one mailbox, newest first.",
  "methodCalls": [
    ["Email/query", {
      "filter": {"inMailbox": "{{mailboxId}}"},
      "sort": [{"property": "receivedAt", "isAscending": false}],
      "limit": "{{limit?}}"
    }, "search"],
    ["Email/get", {
      "_comment": "Fetch them in the same request, so the ids never make a round trip.",
      "#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"},
      "properties": "@EmailSummary"
    }, "fetch"]
  ],
  "_returns": "fetch"
}
```

A change to one record, named by a parameter used as a key:

```json file=MarkEmailRead.jmap.json
{
  "_doc": "MarkEmailRead sets the $seen keyword on one email.",
  "methodCalls": [
    ["Email/set", {"update": {"{{emailId}}": {"keywords/$seen": true}}}, "mark"]
  ],
  "_returns": "mark"
}
```

Catching up from a known state, which the Go client follows whenever the server
pushes a change:

```json file=SyncEmails.jmap.json
{
  "_doc": "SyncEmails fetches the emails that have changed since a known state.",
  "_watches": "changes",
  "methodCalls": [
    ["Email/changes", {"sinceState": "{{sinceState}}", "maxChanges": 128}, "changes"],
    ["Email/get", {
      "#ids": {"resultOf": "changes", "name": "Email/changes", "path": "/created"},
      "properties": "@EmailWithFlags"
    }, "created"],
    ["Email/get", {
      "#ids": {"resultOf": "changes", "name": "Email/changes", "path": "/updated"},
      "properties": ["id", "mailboxIds", "keywords"]
    }, "updated"]
  ]
}
```

A search read in full, one window at a time, with the filter left to the caller:

```json file=SearchEmails.jmap.json
{
  "_doc": "SearchEmails finds the emails matching a filter, newest first.",
  "_pages": "search",
  "methodCalls": [
    ["Email/query", {
      "filter": "{{filter}}",
      "sort": [{"property": "receivedAt", "isAscending": false}],
      "position": "{{position}}",
      "limit": 50
    }, "search"],
    ["Email/get", {
      "#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"},
      "properties": "@EmailSummary"
    }, "fetch"]
  ]
}
```

## Reference

- Method names, arguments and properties: `jmapc schema` writes a JSON Schema of
  everything jmapc knows, with each one's documentation, including the vendor
  extensions `jmapc.json` names. Search it rather than guessing. Pointing
  `$schema` in a request at that file lets an editor check it as it is written.
- Names are as the specifications write them: RFC 8620 (core) and RFC 8621
  (mail), and for the rest the capability's own specification, such as RFC 9610
  for contacts or RFC 9661 for sieve.
- `jmapc -h`, `jmapc run -h` and `jmapc schema -h` list the flags.
