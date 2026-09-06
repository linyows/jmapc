# Changelog

What changed in each release, and what it means for the code that uses it.
The release on GitHub carries the same text.

This starts at v0.12.0. What went into the releases before it is in the commit
history.

## v0.12.0 (2026-09-06)

### Breaking changes

- **A record type is named after the call that read it.** `ListInboxEmailsEmail`
  is `ListInboxEmailsFetchEmail`, after the call id `fetch`. The types were
  numbered by the position of the call, so inserting a call moved a name onto a
  different shape, which the build reported only where the two shapes differed
  enough to stop compiling. Every generated record type is renamed by this, in
  Go, Rust and TypeScript. ([#64](https://github.com/linyows/jmapc/pull/64))
- **The filter of a `/query` is a typed union rather than `any`.** It is a
  `FilterOperatorOrEmailFilterCondition`, a struct with one field per shape, of
  which exactly one is set. Code that built a `/query` or `/queryChanges`
  arguments struct by hand has to name the shape it is passing.
  ([#58](https://github.com/linyows/jmapc/pull/58))

### Added

- **`WithObserver` reports what the client does**, for logging, metrics and
  tracing. Three hooks that nest: one JMAP request, each HTTP request under it,
  and each delay for a concurrency slot or before a retry. `SlogObserver`
  writes them to a `slog.Logger`, and the hooks return the context their work
  runs under, so a tracer can nest its spans the same way. Nothing depends on
  OpenTelemetry. ([#59](https://github.com/linyows/jmapc/pull/59))
- **`WithTokenSource` authenticates with a token that expires.** The client
  calls the source when it has no token, shortly before the one it holds
  expires, and when a server answers 401; requests arriving together share one
  call, since some servers accept a refresh token only once. A 401 sends that
  one request again, once, with a newly fetched token.
  ([#61](https://github.com/linyows/jmapc/pull/61))
- **`WithSplitGets` sends a `/get` holding more ids than `maxObjectsInGet` in
  several requests** and joins the answers into one response. It is off by
  default: the round trips multiply, and the records are no longer one
  snapshot, so a `state` that differs between requests is reported as a
  `*StateChanged` alongside the joined response.
  ([#62](https://github.com/linyows/jmapc/pull/62))
- **A blob download can ask for part of a blob.** `DownloadOptions.From` and
  `Length` are sent as an HTTP `Range`, which is how a download interrupted
  part way is resumed, and `Blob.Range` reports what came back. A server that
  ignores the range fails the download rather than returning the whole blob to
  a caller writing at an offset.
  ([#63](https://github.com/linyows/jmapc/pull/63))
- **The CLI help opens with a banner**: the name in ASCII, in green, and under
  it one line saying what jmapc is and where it lives. Neither is coloured
  where the output is not a terminal or `NO_COLOR` is set.
  ([#65](https://github.com/linyows/jmapc/pull/65))

### Documentation

- **The prose was rewritten in plain English.** It attributed intent and speech
  to programs — a client "asking to be refused", a server "telling the caller
  to come back later" — which is not how technical documentation reads. The
  README, the package documentation, the comments, and the documentation the
  generator writes into every generated client all state what happens instead.
  ([#60](https://github.com/linyows/jmapc/pull/60))
- **The three languages are listed in one order everywhere**, Go, Rust,
  TypeScript, including the order the README's sections come in.
  ([#66](https://github.com/linyows/jmapc/pull/66))
- **The README says what jmapc does before why it exists**, opening with a
  query and the call generated from it, then a list of what jmapc does, and the
  reference sections moved to
  `docs/`: writing a query, verification, the command, the runtime, push,
  paging, testing, the Rust and TypeScript clients, vendor extensions,
  coverage, and working on jmapc. It also says how jmapc differs from the JMAP
  client libraries — the request written in JMAP rather than in an API of the
  library's own, the checking done before the program runs, and the response
  typed to what the query asked for. Both languages are split the same way.
