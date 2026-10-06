# The jmapc command

`jmapc` has six subcommands.

| Subcommand | |
|---|---|
| `jmapc generate` | Checks the requests and writes the generated client. |
| `jmapc validate` | Checks the requests and writes nothing; with `-session`, against a running server as well. See [Verification](verification.md). |
| `jmapc run <request>` | Sends one request to a server and prints the response. See [Sending a request](run.md). |
| `jmapc schema` | Writes a JSON Schema describing the request files, for an editor. See [Editor support](verification.md#editor-support). |
| `jmapc guide` | Prints how to write requests, for a coding agent. See [Coding agents](#coding-agents). |
| `jmapc version` | Prints the version. |

`jmapc -h` lists the flags `generate` and `validate` take, and `jmapc run -h`,
`jmapc schema -h` and `jmapc guide -h` the flags of their own.

## Coding agents

`jmapc guide` prints a guide to writing requests for a coding agent to read
before it adds or changes one: the format, the members jmapc reads, the
parameters, the steps from writing a request to generating the client, and the
mistakes `validate` refuses, with what to write instead. It is built into the
command, so it describes the version of jmapc that prints it. `jmapc guide
mistakes` prints one section; `jmapc guide -h` lists them.

`jmapc guide -install` writes a skill that has the agent read the guide before
writing a request and run `jmapc validate` after, to
`.claude/skills/jmapc/SKILL.md` for Claude Code and `.agents/skills/jmapc/SKILL.md`
for Codex. `-for claude` or `-for codex` writes one of them. The skill only
points at the guide, so it does not need writing again when jmapc is upgraded,
and `-install` does not replace a `SKILL.md` it did not write.

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

`jmapc generate` puts all three right: it writes the first two again, and
removes the third.

A file that does not start with the banner every generated file carries was
written by hand, so `-check` does not report it and generating again does not
remove it.

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
setting of the same name. The paths a settings file gives, and the defaults it
leaves in place, are relative to the file, so it means the same wherever jmapc
runs; a path given as a flag is relative to where jmapc runs.

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
