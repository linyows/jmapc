# The jmapc command

`jmapc` has five subcommands.

| Subcommand | |
|---|---|
| `jmapc generate` | Checks the requests and writes the generated client. |
| `jmapc check` | Checks the requests and writes nothing; with `-session`, against a running server as well. See [Verification](verification.md). |
| `jmapc run <request>` | Sends one request to a server and prints the response. See [Sending a request](run.md). |
| `jmapc schema` | Writes a JSON Schema describing the request files, for an editor. See [Editor support](verification.md#editor-support). |
| `jmapc version` | Prints the version. |

`jmapc -h` lists the flags `generate` and `check` take, and `jmapc run -h` and
`jmapc schema -h` the flags of their own.

## Checking that the generated client is up to date

The generated client is committed to the repository, so it can fall behind the
requests it was generated from: a request is changed and the client is not
generated again, or jmapc is upgraded and nothing is regenerated. `generate
-check` compares what is on disk with what generating now would produce. It
writes nothing, and it fails where the two differ.

```
jmapc generate -check
client/listinboxemails_gen.go: out of date
client/oldreport_gen.go: generated from a request that is no longer there
jmapc: 2 files are out of date; run jmapc generate
```

It reports three things:

- a file that is not what its request generates now,
- a file that has not been generated at all,
- a file jmapc wrote earlier that no request generates any more, which is what
  deleting a request file leaves behind.

A file that does not start with the banner every generated file carries was
written by hand, so it is neither reported nor touched.

Give it the arguments the generation it checks was given. The path a request
came from is written into the file generated from it, so a check run with a
different `-requests` path reports every file as out of date.

In a workflow:

```yaml
- run: go tool jmapc generate -check
```

## Configuration

Every subcommand reads its settings from `jmapc.json` in the directory it runs
in, where there is one, or from the file `-config` names. A flag overrides the
setting of the same name.

```json
{
  "requests": "requests",
  "out": "internal/client",
  "package": "client",
  "schemas": ["schema/notes.json"]
}
```

| Setting | Flag | Default | |
|---|---|---|---|
| `requests` | `-requests` | `requests` | The directory holding the request files |
| `out` | `-out` | `client` | The directory the generated client is written to |
| `lang` | `-lang` | `go` | The language to generate: `go`, `rust` or `typescript` |
| `package` | `-package` | the last element of `out` | The name of the generated Go package |
| `schemas` | `-schema` | | Schema files describing a vendor extension; see [Vendor extensions](extensions.md). The flag may be repeated, and adds to the files the setting lists |

The `requests` directory holds one file per request, and may hold a
`properties.json` naming the sets of properties they ask for; see
[Property sets](properties.md).
