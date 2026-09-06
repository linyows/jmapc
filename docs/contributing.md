<p align="right">English | <a href="contributing.ja.md">日本語</a></p>

# Working on jmapc

```
go test ./...        # everything, including the end-to-end tests
go generate ./...    # regenerate the runtime types and every example client
```

The example is generated three times, once per language, into `example/jmapq`,
`example/rust/src/jmapq` and `example/ts`. Go's tests cannot say whether the
other two compile, so CI runs `cargo fmt --check` and `cargo test` over the
Rust and `tsc --strict` over the TypeScript. Each of the two has a
hand-written check beside the generated code, exercising the runtime against a
stub: that the headers are sent, that authentication overrides them, that the
session is cached, and that a `/set` answering 200 with a refusal in it is
still an error.

The schema is checked the same way, and for the same reason: whether a
validator accepts the example queries and refuses the mistakes the schema
claims to catch is not something Go's tests can say. `example/schema/check.mjs`
runs one, over a schema written from the catalogue as it stands.

The generator is run from source here, not through `go tool`, because this is
the repository that defines it.

The runtime types and the example client are committed, and a test compares
them against what the catalogue produces now, so a change to the data model
that was not regenerated fails the build rather than going unnoticed. CI runs
the same checks, plus gofmt, go vet, and govulncheck.

A release is a tag pushed once the work is on main, and its notes are the
section of [CHANGELOG.md](../CHANGELOG.md) for that tag: what changed, grouped by
what it means for the code that uses this, with the breaking changes first.
Write that section before the tag. A tag with no section fails the release
rather than publishing an empty one.
