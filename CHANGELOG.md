# Changelog

What changed in each release, and what it means for the code that uses it. The release on GitHub carries the same text.

This starts at v0.12.0. What went into the releases before it is in the commit history.

## Unreleased

### Added

- **A set of properties can be named once and asked for by name.** `properties.json`, beside the requests, gives a name to a shape — `{"EmailSummary": {"type": "Email", "properties": [...]}}` — and a request asks for it with `"properties": "@EmailSummary"` in place of the list. The generated record type takes the set's name, so every request asking for the set answers with one type rather than one type per call, and a function written for that type takes what any of them returned. The property list is written once as well, so adding a property is one edit rather than one edit per request. A set may `extend` another, which the generated type embeds in Go, flattens with serde in Rust and extends as an interface in TypeScript, so a function written for the base takes a record of the derived set without a conversion. `jmapc check` reads the file and reports a misspelled property where it was declared. ([#83](https://github.com/linyows/jmapc/pull/83))
- **`HasErrorType` says whether a failure carries a particular JMAP error type.** JMAP reports one condition at whichever level the server refused at: a request that is too large is refused as a whole, an `Email/set` over the account's quota is refused as a call, and a single message over the quota is refused as a record while the rest of the call goes through. Telling them apart meant an `errors.As` per level and a field comparison per failure, and a `/set` that refused several records exposed only the first of them through `errors.As`. `jmapc.HasErrorType(err, jmapc.ErrOverQuota)` reads all three levels and every failure in the response. Request types are URIs — `jmapc.ErrTypeLimit` — so they cannot be confused with the rest, and a type a server defines itself is compared as it was given. `ErrAlreadyExists` joins the constants, which `SetError.ExistingID` referred to without one. ([#85](https://github.com/linyows/jmapc/pull/85))

## v0.14.0 (2026-09-07)

### Added

- **`jmapc generate -check` reports a generated client that is out of date.** It generates into memory, compares the result with what is on disk, writes nothing, and exits non-zero where the two differ. Each file is named with the reason: it is not what its request generates now, it was never generated, or it was left behind by a request that has since been deleted. A file jmapc did not write is neither reported nor touched. A build step or a workflow runs it so that a request changed without the client being generated again stops the build. ([#74](https://github.com/linyows/jmapc/pull/74))
- **`WithResync` gives a watch a way back from `cannotCalculateChanges`.** A `/changes` call answers that where the state it was given is older than the server keeps, which is what a server answers when a watch is resumed after a long enough pause. `Watch` used to stop and return the error, and a program that followed changes stopped following them. The option takes a function that reads the records again and reports the state they were read at, and the watch continues from there. It is called for that error alone, and not a second time for a state a resync has just reported. ([#77](https://github.com/linyows/jmapc/pull/77))
- **`IsTemporary`, `IsRateLimited` and `RetryAfter` say what a failure is.** `IsTemporary` is false for what the server reported about the request — a 4xx that is not a 429, and an error type such as `invalidArguments` — and true for what it reported about itself. `IsRateLimited` picks out the server asking for fewer requests, including the 400 that refuses a request for exceeding `maxConcurrentRequests`, which does not look like rate limiting from the status alone. `RequestError` carries a `RetryAfter` field, read from the header when the error is built: a server asking for longer than the client waits out used to fail the request with that number dropped. ([#78](https://github.com/linyows/jmapc/pull/78))

### Changed

- **The client fetches the session again when a response reports it changed.** Every response carries the server's `sessionState`, which the client now compares with the session it holds; where the two differ, the next call that needs the session fetches it. An account added or moved, a capability the server has since gained, an endpoint that moved and a limit that changed therefore reach a client that outlives them, where before the session was fetched once and held for the life of the client. Requests arriving together share one fetch, and a fetch that fails leaves the session as it was rather than failing the request. `WithoutSessionRefresh` turns it off. ([#75](https://github.com/linyows/jmapc/pull/75))
- **The number of requests in flight follows `maxConcurrentRequests`.** It was read once, at the first request, and kept from then on. Where the number goes down, the requests already in flight are not cancelled: each returns its slot as it finishes, and no further slot is given out until fewer than the new number are held, so the count never rises above what the server last stated. Slots are handed out in the order they were asked for, which the previous implementation did not guarantee. ([#76](https://github.com/linyows/jmapc/pull/76))
- **A request larger than `maxSizeRequest` is refused before it is sent**, as a `*RequestError` naming that limit, in the shape `Upload` already returns for `maxSizeUpload`. How large a request is becomes known only once it is encoded, which is why this is the one preflight check made at that point rather than before. `WithoutPreflightChecks` turns it off with the rest. ([#79](https://github.com/linyows/jmapc/pull/79))

## v0.13.0 (2026-09-06)

### Breaking changes

- **What you write is called a request, not a query.** In JMAP, `Foo/query` is the method that searches, and a file holding `methodCalls` is a Request object as RFC 8620 defines it, so calling that file a query collided with the specification's own word. `-queries` is now `-requests`, the `jmapc.json` key `"queries"` is `"requests"`, and the defaults are `requests` for the input and `client` for the output, in place of `queries` and `jmapq`. A project keeping its old layout passes the paths it already uses; one on the defaults renames the two directories and regenerates. The generated code itself is unchanged apart from its package name. ([#70](https://github.com/linyows/jmapc/pull/70))

### Documentation

- **The README says what jmapc does before why it exists**, opening with a request and the call generated from it, then a list of five features. The reference moved to `docs/`, a file to a subject: writing a request, verification, the command, the runtime, push, paging, testing, the Rust and TypeScript clients, vendor extensions, coverage, and working on jmapc. A new section says how jmapc differs from the JMAP client libraries — the request written in JMAP rather than in an API of the library's own, the checking done before the program runs, and the response typed to what the request asked for. Both languages are split the same way. ([#68](https://github.com/linyows/jmapc/pull/68))

## v0.12.0 (2026-09-06)

### Breaking changes

- **A record type is named after the call that read it.** `ListInboxEmailsEmail` is `ListInboxEmailsFetchEmail`, after the call id `fetch`. The types were numbered by the position of the call, so inserting a call moved a name onto a different shape, which the build reported only where the two shapes differed enough to stop compiling. Every generated record type is renamed by this, in Go, Rust and TypeScript. ([#64](https://github.com/linyows/jmapc/pull/64))
- **The filter of a `/query` is a typed union rather than `any`.** It is a `FilterOperatorOrEmailFilterCondition`, a struct with one field per shape, of which exactly one is set. Code that built a `/query` or `/queryChanges` arguments struct by hand has to name the shape it is passing. ([#58](https://github.com/linyows/jmapc/pull/58))

### Added

- **`WithObserver` reports what the client does**, for logging, metrics and tracing. Three hooks that nest: one JMAP request, each HTTP request under it, and each delay for a concurrency slot or before a retry. `SlogObserver` writes them to a `slog.Logger`, and the hooks return the context their work runs under, so a tracer can nest its spans the same way. Nothing depends on OpenTelemetry. ([#59](https://github.com/linyows/jmapc/pull/59))
- **`WithTokenSource` authenticates with a token that expires.** The client calls the source when it has no token, shortly before the one it holds expires, and when a server answers 401; requests arriving together share one call, since some servers accept a refresh token only once. A 401 sends that one request again, once, with a newly fetched token. ([#61](https://github.com/linyows/jmapc/pull/61))
- **`WithSplitGets` sends a `/get` holding more ids than `maxObjectsInGet` in several requests** and joins the answers into one response. It is off by default: the round trips multiply, and the records are no longer one snapshot, so a `state` that differs between requests is reported as a `*StateChanged` alongside the joined response. ([#62](https://github.com/linyows/jmapc/pull/62))
- **A blob download can ask for part of a blob.** `DownloadOptions.From` and `Length` are sent as an HTTP `Range`, which is how a download interrupted part way is resumed, and `Blob.Range` reports what came back. A server that ignores the range fails the download rather than returning the whole blob to a caller writing at an offset. ([#63](https://github.com/linyows/jmapc/pull/63))
- **The CLI help opens with a banner**: the name in ASCII, in green, and under it one line saying what jmapc is and where it lives. Neither is coloured where the output is not a terminal or `NO_COLOR` is set. ([#65](https://github.com/linyows/jmapc/pull/65))

### Documentation

- **The prose was rewritten in plain English.** It attributed intent and speech to programs — a client "asking to be refused", a server "telling the caller to come back later" — which is not how technical documentation reads. The README, the package documentation, the comments, and the documentation the generator writes into every generated client all state what happens instead. ([#60](https://github.com/linyows/jmapc/pull/60))
- **The three languages are listed in one order everywhere**, Go, Rust, TypeScript, including the order the README's sections come in. ([#66](https://github.com/linyows/jmapc/pull/66))
