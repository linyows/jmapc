# Changelog

What changed in each release, and what it means for the code that uses it. The release on GitHub carries the same text.

This starts at v0.12.0. What went into the releases before it is in the commit history.

## v0.21.0 (2026-10-07)

### Breaking changes

- **A call left to its default properties answers with what the specification has it return.** An Email/get that names no properties is generated as narrowed to the list RFC 8621 gives, without `headers` or `bodyStructure`, and a CalendarEvent/get as every property but `utcStart` and `utcEnd`, which it returns only where they are named; a back reference reading one of those from such a call is refused, as it would read nothing at the server. A call that fetches body parts and leaves `bodyProperties` out holds them without `headers` or `subParts`. Each now answers with a response type generated for it, where it held the runtime's `Email`, `CalendarEvent` or `EmailBodyPart`; regenerate, and read the records through the new types. ([#183](https://github.com/linyows/jmapc/pull/183), [#184](https://github.com/linyows/jmapc/pull/184))
- **The records of a call given its properties by a parameter may lack any of them.** Where such a call also narrowed `bodyProperties`, its record type had every member required, so Rust failed to decode any property the caller left out with ``missing field``, and an Email/parse failed on its `id`, which comes back as null, whatever was asked for. Every member but the `id` a /get always returns is now optional: `Option` in Rust and `?` in TypeScript. Go is unchanged but for the properties a parse returns as null, which the type no longer holds. ([#182](https://github.com/linyows/jmapc/pull/182))
- **A parse that does not name its properties answers with a type of its own.** An Email/parse given its properties by a parameter or a back reference, and a CalendarEvent/parse naming none, held the runtime's `Email` or `CalendarEvent`, with an `id` the parse returns as null and, for an email, a `threadId` that could not be null. They now hold a type generated for the call, without the properties the parse returns as null and with every member optional, and a parsed email's `threadId` may be null. CalendarEvent/parse returns `id`, `baseEventId`, `calendarIds`, `isDraft` and `isOrigin` as null, so a call asking for one or a back reference reading one is now refused. ([#182](https://github.com/linyows/jmapc/pull/182), [#183](https://github.com/linyows/jmapc/pull/183))
- **A back reference that may select null is refused where the argument does not take null.** `/list/*/parentId` of a Mailbox/get holds a null for each mailbox at the top, and the `threadId` of an Email/parse may be null, yet both passed as `ids` or an `Id`, which the server refuses with `invalidArguments`. The error says when null is the only difference. ([#181](https://github.com/linyows/jmapc/pull/181))

## v0.20.0 (2026-10-07)

### Breaking changes

- **The Rust and TypeScript `total` and `limit` of a /query response are optional.** RFC 8620 has the server leave `total` out unless `calculateTotal` was set and `limit` out unless it lowered the limit, and the generated Rust failed to decode an ordinary Email/query answer with ``missing field `limit` ``. They are now `Option<u64>` in Rust and `total?: number` in TypeScript, and so is the `total` of /queryChanges; code reading them needs to handle their absence. Go is unchanged. ([#131](https://github.com/linyows/jmapc/pull/131))
- **A parse's records are the narrowed type, and a blob's data is two members.** The response of an Email/parse or CalendarEvent/parse that names its properties now holds the record type generated for them under `parsed`, as a /get's `list` does, where it held the full `Email` or `CalendarEvent`. An Email/parse that names none is generated as narrowed to the defaults RFC 8621 gives, so it has no `id`, and a parsed email's `threadId` may be null: `*jmapc.ID`, `Option<Id>`, `Id | null`. A Blob/get asking for `data`, or naming no properties, holds `data:asText` and `data:asBase64`, optional in Rust and TypeScript, where it held a member named `data` that never came back. Regenerate, and read the records through the new types. ([#176](https://github.com/linyows/jmapc/pull/176), [#177](https://github.com/linyows/jmapc/pull/177), [#178](https://github.com/linyows/jmapc/pull/178))
- **Requests the server would refuse are refused before they are sent.** A patch reaching inside a list (`replyTo/0/email`), a patch setting a key a set's specification does not allow, a header field asked of a type that has none or in a form RFC 8621 does not allow for that field (`header:Subject:asDate`), a blob's `data` asked of anything but a blob, and an Email/parse asking for or referring to `id`, `mailboxIds`, `keywords` or `receivedAt`, which it returns as null, all passed and are now refused with what to write instead. ([#153](https://github.com/linyows/jmapc/pull/153), [#154](https://github.com/linyows/jmapc/pull/154), [#157](https://github.com/linyows/jmapc/pull/157), [#175](https://github.com/linyows/jmapc/pull/175), [#178](https://github.com/linyows/jmapc/pull/178))
- **A vendor schema that cannot be used is refused when it is read.** A type named as a primitive or with characters a generator cannot write, a property or argument given twice, an argument a standard method already has, a `patchTarget` or `sortTarget` naming no type, a method selecting `properties` without a `resultProperty` or holding its records in anything but a list or a map of them, and a method whose `dataType`, `properties` or `resultProperty` names nothing are each refused with what is wrong and where. ([#142](https://github.com/linyows/jmapc/pull/142), [#174](https://github.com/linyows/jmapc/pull/174), [#177](https://github.com/linyows/jmapc/pull/177))
- **The paths in a settings file are relative to the file.** `requests`, `out` and `schemas` in `jmapc.json`, and the defaults it leaves in place, were taken relative to where jmapc ran, so `-config project/jmapc.json` from the parent read the wrong directories. A path given as a flag is still relative to where jmapc runs, and a `jmapc.json` in that directory reads as before. ([#147](https://github.com/linyows/jmapc/pull/147))
- **`-token` and `-user` given together are refused**, rather than one silently ignored, in `jmapc run` and `jmapc validate -session`; a flag given on the command line now wins over the other's environment variable. ([#132](https://github.com/linyows/jmapc/pull/132))
- **jmaptest behaves as a server does.** Its push endpoint sends a client only the types it subscribed to, honours `closeafter` and `ping`, and numbers each event; it holds requests to the `maxCallsInRequest`, `maxObjectsInGet` and `maxObjectsInSet` its session states; and `RequestCheck` reads a sent request as sent, so `"{{x}}"` is a string rather than a parameter, and a property from a capability the request does not declare is refused. A test that passed by relying on the looser stub may now fail. ([#139](https://github.com/linyows/jmapc/pull/139), [#149](https://github.com/linyows/jmapc/pull/149), [#155](https://github.com/linyows/jmapc/pull/155))

### Security

- **A `-user` without a colon is no longer printed in an error.** Such a value may be nothing but the password, and `JMAP_USER=supersecret` printed it. ([#132](https://github.com/linyows/jmapc/pull/132))

### Added

- **A vendor schema's method can say what it answers with.** `returnsId` says it returns the id of every record whatever it is asked for, as a /get does; `defaultProperties` the properties it returns where a call names none; `nullProperties` those it returns as null; and `nullableProperties` those it may return as null though their type does not say so. ([#174](https://github.com/linyows/jmapc/pull/174), [#178](https://github.com/linyows/jmapc/pull/178))
- **A header field can be named in a back reference's path, a patch and a created email**, as RFC 8621 lets one be, where only `properties` could name one; a body part's `bodyProperties` takes header fields too. ([#157](https://github.com/linyows/jmapc/pull/157), [#158](https://github.com/linyows/jmapc/pull/158))
- **The Mailbox/changes response carries `updatedProperties`**, which RFC 8621 gives it, so a request can fetch only the counts that changed. ([#151](https://github.com/linyows/jmapc/pull/151))
- **The emails an Email/import creates are named**, as a /set's creation ids are, so the response is read by a constant rather than by spelling the id again. ([#152](https://github.com/linyows/jmapc/pull/152))

### Fixed

- **A watch against a server without push returns at once with the reason**, rather than reconnecting until its context ended. ([#133](https://github.com/linyows/jmapc/pull/133))
- **An event stream resumes from the last event read to its end.** An id was taken as soon as it was read, so a stream dropped in the middle of an event resumed after it, and the event was lost. ([#134](https://github.com/linyows/jmapc/pull/134))
- **A download answered with a part that starts or ends where it was not asked for is refused**, as a whole blob answering a range is, and `IsRangeIgnored` reports it. ([#135](https://github.com/linyows/jmapc/pull/135))
- **A caller waiting on a shared fetch of the session or the token outlives the caller that started it.** It was handed `context canceled` when the first caller gave up, though its own context had not ended. ([#136](https://github.com/linyows/jmapc/pull/136))
- **Requests refused the same token together replace it once**, rather than each calling the token source, which a server accepting a refresh token only once refuses. ([#145](https://github.com/linyows/jmapc/pull/145))
- **A push subscription the server gives an expiry to later is extended for as long as it granted**, where each extension asked for an expiry of now. ([#141](https://github.com/linyows/jmapc/pull/141))
- **The observer is told the error `Do` returns** where the parts of a split /get could not be joined; it was told nil. ([#146](https://github.com/linyows/jmapc/pull/146))
- **A back reference to a call asking for `properties: []` is held to the id alone**, where it was taken to fetch everything. ([#137](https://github.com/linyows/jmapc/pull/137))
- **Rust and TypeScript compile for a request named after a keyword or stating a string with escapes.** Rust writes such a name as a raw identifier and strings with its own escapes; TypeScript appends an underscore to a reserved word. ([#143](https://github.com/linyows/jmapc/pull/143))
- **TypeScript imports a nullable alias**, so a `Date|null` member is jmapc's `Date` string rather than the global JavaScript `Date`. ([#177](https://github.com/linyows/jmapc/pull/177))
- **The generated Go imports what it uses**, not a package a `_doc` happens to name. ([#156](https://github.com/linyows/jmapc/pull/156))
- **Renaming a request only in case keeps its generated file** on a file system that ignores case, where `generate` removed it. ([#138](https://github.com/linyows/jmapc/pull/138))
- **A Go package name Go cannot take is refused before generating**, with a hint to set `-package`, rather than failing in formatting. ([#148](https://github.com/linyows/jmapc/pull/148))
- **The editor schema agrees with the checks on dates and durations**: a duration in weeks and a date with a fraction of a second are accepted, and `P`, `PT`, `P1DT` and a duration with two signs refused, by both. ([#140](https://github.com/linyows/jmapc/pull/140))
- **`jmapc generate -h` and `jmapc validate -h` exit 0**, as the other commands do. ([#144](https://github.com/linyows/jmapc/pull/144))

### Documentation

- **A chapter on how jmapc works** follows a request from checking to generated code, with diagrams of the stages and of the checks a back reference goes through. ([#130](https://github.com/linyows/jmapc/pull/130), [#179](https://github.com/linyows/jmapc/pull/179))

## v0.19.0 (2026-10-05)

### Added

- **A call that asks what an earlier call in its request asked is noted.** Where a call has the same method and arguments as an earlier one in the same request, with only reads between them, the server gives the same answer twice, and `validate`, `generate` and `generate -check` now say which call repeats which and to refer to the first instead. It is a note, so nothing fails. Arguments are compared as checked — the order of members and `_comment` do not count, parameters are compared by name, and a value stated outright is compared as written — and a back reference to a repeated call counts as one to the call it repeats, so a query and its get written twice are both noted. RFC 8620 lets data change between calls, so a write between the two keeps them apart, and so does any method a vendor schema defines, whatever its name. ([#127](https://github.com/linyows/jmapc/pull/127))

## v0.18.0 (2026-10-05)

### Breaking changes

- **`PushReceiver` makes its subscription with keys, and accepts only pushes encrypted for them.** A server that does not encrypt pushes cannot deliver to such a subscription, and every post it makes is refused with a 400. Set `PushReceiverOptions.PlainText` for such a server, which makes the subscription without keys and takes posts as they come, as before. ([#122](https://github.com/linyows/jmapc/pull/122))

### Security

- **A push to a `PushReceiver` can no longer be forged by someone who learns its URL.** JMAP gives a push no signature, so anyone who knew the URL could post a state change or a verification code to it. `Run` now makes each subscription with a P-256 key pair and an auth secret of its own, the server encrypts every push for them as RFC 8291 describes, and `ServeHTTP` refuses a post that is not encrypted or does not decrypt with them. A subscription made again after the server lost one gets new keys. Stalwart encrypts pushes for a subscription with keys, and the end-to-end run against it passes encrypted. ([#122](https://github.com/linyows/jmapc/pull/122))

### Added

- **`jmapc guide` prints how to write requests, for a coding agent.** It covers the format, the members jmapc reads, parameters, the steps from a request to a generated client, the mistakes `validate` refuses with what to write instead, and complete examples; `jmapc guide <topic>` prints one section. It is built into the command, so it describes the version that prints it. `jmapc guide -install` writes a short skill to `.claude/skills/jmapc/SKILL.md` for Claude Code and `.agents/skills/jmapc/SKILL.md` for Codex that has the agent read the guide before writing a request and run `jmapc validate` after, and never replaces a `SKILL.md` it did not write. ([#123](https://github.com/linyows/jmapc/pull/123))

### Fixed

- **`jmapc validate` refuses the names `jmapc generate` refuses.** A request named after a file jmapc generates for itself — `Verify`, `Properties` where sets of properties are declared, `Client` or `Types` in TypeScript or Rust, `Mod` in Rust — passed `validate` and was refused only by `generate`. `validate` now generates in memory for the language it is given and keeps nothing, so it refuses whatever generating would. ([#124](https://github.com/linyows/jmapc/pull/124))

## v0.17.1 (2026-10-05)

### Fixed

- **A watch notices a connection that went quiet, and connects again.** A watch asks the server to ping every 30 seconds, and RFC 8620 has the server send a ping whenever that long passes without another event, but nothing read the pings: a connection dropped somewhere between client and server with neither end told, as by a NAT or a proxy that forgot it, left `Next` waiting on a read that would never return and `Watch` waiting with it, for as long as TCP took to give up. A stream asked for pings now fails `Next` once it has carried nothing for twice the interval while `Next` waited, keeping to the interval each ping reports, and `Watch` connects again from the last event id. Time spent between calls does not count, and a stream asked for no pings waits as before. A `Ping` below a second asked for no pings at all, the interval being rounded down to whole seconds; it is now rounded up. ([#118](https://github.com/linyows/jmapc/pull/118))
- **A parameter used as a map key is documented as a key of the map it is in.** The generated documentation led with "The id of the record this entry applies to." and went on with the documentation of the property holding the map, which read as though it described the key, and said nothing of which map it was a key of. It now names the property — "An id that is a key of mailboxIds: the mailboxes to file the imported email in." — in all three languages, including a parameter in a patch path such as `mailboxIds/{{id}}`. Regenerating changes these comments and nothing else. ([#117](https://github.com/linyows/jmapc/pull/117))

## v0.17.0 (2026-10-04)

### Breaking changes

- **A request is refused where its file would be one another file is generated into.** A request's file is named after the request, so a request named `verify` was written to `verify_gen.go` and then replaced by the new `Verify` function, one named `Client` or `Types` was replaced by the TypeScript or Rust runtime, a Rust one named `Mod` by `mod.rs`, one named `properties` by the sets of properties, and two requests differing only in case went to one file — each without an error, and with a request missing from the client. Generation now fails and names both, and a request or a set of properties named `Verify` is refused. Rename the request where this happens; it was not being generated. ([#109](https://github.com/linyows/jmapc/pull/109), [#110](https://github.com/linyows/jmapc/pull/110))
- **The Rust runtime's `Error` has a `Verify` variant**, which `verify` returns. A `match` on `Error` without a wildcard arm needs one for it. ([#110](https://github.com/linyows/jmapc/pull/110))

### Added

- **`Verify` checks every request against the session as the program starts.** The checks that refuse a request before it is sent run as each request is sent, so a request on a path a program rarely takes was checked only when that path was taken. The generated client now has a `Verify(ctx, c)` in Go, and `verify(client)` in TypeScript and Rust, which checks every request in the package at once: the capabilities it declares, the primary account it leaves the account to and whether that account supports the capability, and its number of calls against `maxCallsInRequest`. Every problem is reported, each as a `VerifyError` naming its request and holding the error sending it would have failed with. ([#109](https://github.com/linyows/jmapc/pull/109), [#110](https://github.com/linyows/jmapc/pull/110))
- **`PushReceiver` keeps a push subscription to a URL.** It is the `http.Handler` the URL reaches, and its `Run` creates the subscription, waits for the server to post the verification code and sends it back, extends the subscription before it expires within what the server grants, makes it again where the server no longer has it, and removes it when the context ends. Each state change the server posts goes to `OnStateChange`, and each step to `OnEvent`. Pushes are not encrypted: the subscription is made without keys. Go only. ([#113](https://github.com/linyows/jmapc/pull/113))
- **An `Observer` can be given the bodies of requests and responses.** With `Observer.Redact` set, `RequestInfo.Body` and `ResponseInfo.Body` hold the request as the caller made it and the response as the caller received it, after passing through `Redact`, and `SlogObserver` writes them. A body carries the subjects, addresses and text of the user's mail, so nothing is reported without `Redact`. `RedactContent` keeps method names, call ids, ids, states, property names, numbers and every key, and replaces every other string with `[redacted]`; `KeepBodies` keeps everything, for development. ([#111](https://github.com/linyows/jmapc/pull/111))
- **`IsRangeIgnored` reports a download whose range the server ignored.** JMAP defines no range on the download endpoint, and `Download` refuses a whole blob answered for a range; a caller resuming a download can now tell that from any other failure and download the whole blob instead. ([#108](https://github.com/linyows/jmapc/pull/108))

### Fixed

- **`IsTemporary` is false for a range the server ignored**, which it counted as temporary, so a retry loop asked the same server for the same range again. ([#108](https://github.com/linyows/jmapc/pull/108))
- **`HasErrorType` and `IsTemporary` read every request error in a joined error.** They read only the first `errors.As` found, so in an error joined from several, such as `Verify` returns, the rest were missed. Each node is matched as `errors.As` matches it, including through an `As` method of its own. ([#109](https://github.com/linyows/jmapc/pull/109))

### Documentation

- **The end-to-end tests are one probe workflow, and cover push.** `probe e2e/workflow.yml` starts Stalwart, sets it up, runs the scenarios and removes the container however the run ends, and CI runs it through probe-action. The scenarios now also follow `/changes` a few at a time, follow a push through `Watch`, run `Verify` against the session, and receive a push Stalwart posts to a URL through `PushReceiver`. They need probe 1.14.0 or later. ([#107](https://github.com/linyows/jmapc/pull/107), [#109](https://github.com/linyows/jmapc/pull/109), [#112](https://github.com/linyows/jmapc/pull/112), [#114](https://github.com/linyows/jmapc/pull/114))

## v0.16.1 (2026-10-03)

### Fixed

- **An event stream stays open past the `Timeout` of the `http.Client` it was opened through.** An `http.Client` counts reading the body against its `Timeout`, and the body of an event stream does not end, so a client given a timeout through `WithHTTPClient` had the stream from `EventSource` cut each time the timeout ran out. `Watch` reconnected after each cut and ran a catch-up, which hid it: a watch on a client with a 10-second timeout opened a new connection every 10 seconds. The `Timeout` now bounds the wait for the response instead, so a server that never answers is still given up on, with an error `IsTemporary` reports as temporary, and a stream that has started stays open until the server closes it, `Close` is called or the context ends. Every other request keeps the `Timeout` as before, and the `http.Client` passed in is not modified. ([#103](https://github.com/linyows/jmapc/pull/103))

### Documentation

- **jmapc is now tested against a real JMAP server.** `e2e/run.sh` runs the generated client against Stalwart in a container, and CI runs it on every change: the session, mail delivered over SMTP, mail imported through jmapc and checked over IMAP, and blobs checked against a direct download. Each scenario checks what jmapc did over a path that does not go through jmapc. The contributing page describes how to run them. ([#104](https://github.com/linyows/jmapc/pull/104), [#105](https://github.com/linyows/jmapc/pull/105))

## v0.16.0 (2026-10-03)

### Breaking changes

- **`jmapc check` is now `jmapc validate`.** The command validates the requests, and `jmapc generate -check` compares the generated client with what is on disk; with both called "check", which one was meant depended on where the word stood. The command takes the name of what it does, and its output reads `validated 25 requests`. `jmapc check` now fails with `unknown command "check"`, and no alias is kept. `generate -check` is unchanged, so a workflow that runs it needs no edit. ([#101](https://github.com/linyows/jmapc/pull/101))

### Fixed

- **`jmapc generate` removes the files of deleted requests.** `generate -check` reports a file a deleted request left behind and says to run `jmapc generate`, but generating only wrote files, so the file stayed and the check went on failing until it was deleted by hand. Generating now removes each file in the output directory that carries the generated banner and that no request generates any more, and names it on stderr. The removal comes after the new files are written, so a generation that fails part of the way through removes nothing, and a file without the banner is left alone. ([#100](https://github.com/linyows/jmapc/pull/100))

### Documentation

- **The documentation is published as a site at https://jmapc.linyo.ws**, in English and Japanese, built from the same files in `docs/` that read on GitHub. ([#88](https://github.com/linyows/jmapc/pull/88))
- **The documentation is grouped under Overview, Requests, Generated code and Reference, with the pages a new reader needs first.** Introduction describes JMAP itself, Why jmapc carries the motivation and the comparison with JMAP client libraries, and Getting started goes from an empty module to a working call and `generate -check` in CI. The runtime page is split into Configuring the client, Errors and Blobs, and the command page becomes a reference with a table of every setting. Pages such as `/runtime` went away without redirects. ([#90](https://github.com/linyows/jmapc/pull/90))

## v0.15.0 (2026-09-08)

### Added

- **A set of properties can be named once and asked for by name.** `properties.json`, beside the requests, gives a name to a shape — `{"EmailSummary": {"type": "Email", "properties": [...]}}` — and a request asks for it with `"properties": "@EmailSummary"` in place of the list. The generated record type takes the set's name, so every request asking for the set answers with one type rather than one type per call, and a function written for that type takes what any of them returned. The property list is written once as well, so adding a property is one edit rather than one edit per request. A set may `extend` another, which the generated type embeds in Go, flattens with serde in Rust and extends as an interface in TypeScript, so a function written for the base takes a record of the derived set without a conversion. `jmapc check` reads the file and reports a misspelled property where it was declared. ([#83](https://github.com/linyows/jmapc/pull/83))
- **`HasErrorType` checks for a JMAP error type at every level a server reports one.** JMAP reports one condition at whichever level the server refused at: a request that is too large is refused as a whole, an `Email/set` over the account's quota is refused as a call, and a single message over the quota is refused as a record while the rest of the call goes through. Telling them apart meant an `errors.As` per level and a field comparison per failure, and a `/set` that refused several records exposed only the first of them through `errors.As`. `jmapc.HasErrorType(err, jmapc.ErrOverQuota)` reads all three levels and every failure in the response. Request types are URIs — `jmapc.ErrTypeLimit` — so they cannot be confused with the rest, and a type a server defines itself is compared as it was given. `ErrAlreadyExists` joins the constants, which `SetError.ExistingID` referred to without one. ([#85](https://github.com/linyows/jmapc/pull/85))

### Changed

- **A back reference is checked against what the call it reads from fetches.** `{"resultOf": "matched", "name": "Email/get", "path": "/list/*/threadId"}` resolved against the data model alone, which has a `threadId` on an `Email` whether or not the call asked for one, so a reference into a call that narrowed its `properties` checked out and then selected nothing at the server. The path is now held to the `properties` and `bodyProperties` of the referenced call, and the diagnostic says where the property is added — the call, or the named set it asks for. The `id` a `/get` answers with whether or not it was asked for is still readable, and a call whose properties the caller supplies says nothing about what comes back, so it is left alone. A request that read a property it did not fetch used to check out and fail at the server; it is now reported where it was written. ([#86](https://github.com/linyows/jmapc/pull/86))

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
