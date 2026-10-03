# Working on jmapc

Two commands cover most of the work on jmapc itself. The first runs the tests,
the second regenerates everything the catalogue produces, and both are what CI
runs first.

```
go test ./...        # everything that runs without a server
go generate ./...    # regenerate the runtime types and every example client
```

The example is generated three times, once per language, into `example/client`,
`example/rust/src/jmap_client` and `example/ts`. Go's tests cannot say whether the
other two compile, so CI runs `cargo fmt --check` and `cargo test` over the
Rust and `tsc --strict` over the TypeScript. Each of the two has a
hand-written check beside the generated code, exercising the runtime against a
stub: that the headers are sent, that authentication overrides them, that the
session is cached, and that a `/set` answering 200 with a refusal in it is
still an error.

The schema is checked the same way, and for the same reason: whether a
validator accepts the example requests and refuses the mistakes the schema
claims to catch is not something Go's tests can say. `example/schema/check.mjs`
runs one, over a schema written from the catalogue as it stands.

The generator is run from source here, not through `go tool`, because this is
the repository that defines it.

The tests above run the generated code against stubs, which answer the way
the tests expect a server to. `e2e/` runs it against a real one, Stalwart, in a
container:

```
e2e/run.sh
```

It needs docker, Go and [probe](https://github.com/linyows/probe) on the PATH.
It starts the container, takes Stalwart out of bootstrap mode, creates two
accounts, and runs the scenarios in `e2e/workflow.yml`. Each scenario does
something through jmapc and checks it over a path that does not go through
jmapc: the session jmapc reads against the one fetched directly, a message
delivered over SMTP against what the generated client finds, a message imported
through jmapc against its flags over IMAP and a flag set over IMAP against what
jmapc reads, a blob uploaded through jmapc against the same blob downloaded
directly, changes followed through jmapc a few at a time against the server's
own paging of them, and a message delivered over SMTP against what reaches
jmapc's `Watch` by push, in an account where nothing else changes. The jmapc side is
`e2e/driver`, which calls the client generated from `e2e/requests` and prints
what came back as JSON.

Setup provisions the server and builds the driver at the same time:

```mermaid
flowchart LR
    subgraph provision["Provision the server"]
        provision_step0["Wait for bootstrap mode"]
        provision_step1["Complete bootstrap"]
        provision_step2["Restart out of bootstrap mode"]
        provision_step3["Wait for the server"]
        provision_step4["Find the domain bootstrap created"]
        provision_step5["Create alice and bob"]
    end
    subgraph job_1["Build the driver"]
        job_1_step0["go build"]
    end
```

The scenarios are independent of one another and run in parallel:

```mermaid
flowchart LR
    subgraph job_0["The session as jmapc reads it"]
        job_0_step0["Fetch the session directly"]
        job_0_step1["Fetch it through jmapc"]
    end
    subgraph job_1["Mail delivered over SMTP, found through jmapc"]
        job_1_step0["Deliver to alice on port 25"]
        job_1_step1["Find it through the generated client"]
        job_1_step2["Read alice's mail account"]
        job_1_step3["Find the same email directly"]
    end
    subgraph job_2["Mail imported through jmapc, read over IMAP"]
        job_2_step0["Import a flagged message through the generated client"]
        job_2_step1["Find it in the inbox over IMAP, flagged and unread"]
        job_2_step2["Mark it read over IMAP"]
        job_2_step3["See it read through the generated client"]
    end
    subgraph job_3["A blob uploaded through jmapc, downloaded directly"]
        job_3_step0["Upload and download it through jmapc"]
        job_3_step1["Download the whole of it directly"]
        job_3_step2["Ask for the same range directly"]
    end
    subgraph job_4["Changes followed through jmapc, a few at a time"]
        job_4_step0["Read the state through jmapc"]
        job_4_step1["Deliver three messages to alice"]
        job_4_step2["Follow the changes through the generated client"]
        job_4_step3["Read alice's mail account for the direct request"]
        job_4_step4["See the server page the same changes directly"]
    end
    subgraph job_5["A push followed through jmapc's Watch"]
        job_5_step0["Read the state through jmapc"]
        job_5_step1["Start watching through the generated client"]
        job_5_step2["Wait until the watch follows pushes"]
        job_5_step3["Deliver to bob on port 25"]
        job_5_step4["See the watch report it"]
    end
```

`probe dag --mermaid e2e/vars.yml,e2e/workflow.yml` prints the second of these,
and the same for `e2e/setup.yml` the first; print them again after adding a job
or a step. The container is removed afterwards unless
`E2E_KEEP=1` is set. The Stalwart release is pinned in `e2e/run.sh`; a new
minor version is a change made there on purpose, since Stalwart moves its
settings between them.

The runtime types and the example client are committed, and a test compares
them against what the catalogue produces now, so a change to the data model
that was not regenerated fails the build rather than going unnoticed. CI runs
the same checks, plus gofmt, go vet, and govulncheck.

A release is a tag pushed once the work is on main, and its notes are the
section of [CHANGELOG.md](https://github.com/linyows/jmapc/blob/main/CHANGELOG.md) for that tag: what changed, grouped by
what it means for the code that uses this, with the breaking changes first.
Write that section before the tag. A tag with no section fails the release
rather than publishing an empty one.

Write each entry on one line, however long it is. GitHub renders a release body
with the line breaks it was given, so a paragraph wrapped at 80 columns is a
paragraph broken at 80 columns on the release page.
