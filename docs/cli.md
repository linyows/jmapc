<p align="right">English | <a href="cli.ja.md">日本語</a></p>

# The jmapc command

## Sending a query

A query is worth trying before there is any code that calls it, so `jmapc run`
sends one and prints what came back.

```
jmapc run ListInboxEmails -p mailboxId=mbx1 -p limit=25
```

A value is written the way its type requires. A `String` or an `Id` is the text
itself, so nothing has to be quoted past the shell, and anything with a shape is
JSON. A value the type does not accept is refused before anything is sent:

```
jmapc: parameter limit: "soon" is not a whole number
```

The server comes from `-session`, which takes the session URL or the host to
find it under, and the credentials from `-token` or `-user`. Each falls back to an
environment variable — `$JMAP_SESSION_URL`, `$JMAP_TOKEN`, `$JMAP_USER` — which
is what keeps a token out of shell history. The account id a query leaves out is looked up in the session, exactly
as the generated function looks it up, and `-account` overrides it.

`-dry-run` prints the request rather than sending it — the same request the
generated function builds, which is the thing to look at when a server answers
something unexpected:

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

The account id is the one value a dry run cannot know, since it comes from a
session it never fetches, so `ACCOUNT_ID` stands in for it and the run says so
on standard error.

A run reads the response the way generated code does: a `/set` that answers 200
with a refusal in it is an error here too, printed after the response that
carries it.

## Configuration

Flags, or a `jmapc.json` beside your module:

```json
{
  "queries": "queries",
  "out": "internal/jmapq",
  "package": "jmapq",
  "schemas": ["schema/notes.json"]
}
```
