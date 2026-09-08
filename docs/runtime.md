<p align="right">English | <a href="runtime.ja.md">日本語</a></p>

# The runtime

## Errors at run time

JMAP fails at two levels, and so does the runtime.

A **request-level** failure, where the server rejected the request as a whole,
is a `*jmapc.RequestError` carrying the problem type from RFC 8620 §3.6.1. The client
catches some of these before sending: a capability the session does not
advertise, or more calls than the server accepts.

A **method-level** failure is a `jmapc.MethodErrors`. JMAP runs the calls it
can, so the response comes back alongside the error, and each error names the
method and call id that failed rather than the bare `"error"` the wire format
carries.

A generated function returns the same thing: the calls the server answered are
decoded, the ones it did not run are left at their zero value, and the result
is returned with the error. A chained request fails this way routinely — the call
another depends on succeeds, and the dependent call cannot resolve its
reference — and the first call's response usually explains why:

```go
res, err := client.DestroyThread(ctx, c, params)
if err != nil {
    if len(res.ThreadGet.NotFound) > 0 {
        return fmt.Errorf("no such thread: %s", res.ThreadGet.NotFound[0])
    }
    return err
}
```

The exception is a request naming one call in `_returns`: that call is the whole
of the answer, so if it is the one that failed, there is nothing to return and
the result is nil.

TypeScript throws rather than returning, so the decoded response is attached to
the error. `MethodErrors` carries the response it came from, and `result` holds as much of
what the request returns as the server answered. Read it as a `Partial`, since a
call the server would not run is not in it at all:

```ts
try {
  await destroyThread(client, params)
} catch (e) {
  if (e instanceof MethodErrors) {
    const partial = e.result as Partial<DestroyThreadResult>
    if (partial.threadGet?.notFound?.length) {
      throw new Error(`no such thread: ${partial.threadGet.notFound[0]}`)
    }
  }
  throw e
}
```

Rust returns an `Err`, and attaches it there: `MethodErrors::result` returns the
result the request would have returned, with a call the server did not run left at
its default rather than missing, since Rust has `Default` for it:

```rust
if let Error::Method(failed) = &err {
    if let Some(out) = failed.result::<DestroyThreadResult>() {
        if !out.thread_get.not_found.is_empty() { /* which thread was missing */ }
    }
}
```

There is a third level, and it is the one most often missed. A `/set` answers
**200 with no error in it** and lists the records it would not act on:

```json
["Email/set", {"notCreated": {"draft": {"type": "invalidProperties",
                                        "properties": ["subject"]}}}, "write"]
```

Read only the transport error and this is a success where nothing happened.
Generated code checks it, so a refused record is a `*jmapc.SetErrors`:

```go
res, err := client.SendEmail(ctx, c, params)
if err != nil {
    var refused *jmapc.SetErrors
    if errors.As(err, &refused) {
        for _, f := range refused.Failures {
            log.Printf("%s: %v", f.Key, f.Err) // draft: invalidProperties [subject]
        }
    }
    return err
}
```

`res` is returned alongside the error, since the part of the request the server
did carry out still happened. Calls the request does not name in `_returns` are
checked too — naming one call should not exempt the others from the check.

In TypeScript the same failure is a thrown `SetErrors`, with the response on
`err.result`. In Rust it is an `Error::Set`, and the response is retrieved with
the type the function would have returned, through `err.result::<T>()`.

### Temporary and permanent failures

A failure arrives as an error, and what to do with it depends on what the server
said. Three functions classify it, so that a caller does not pick apart status
codes and error type strings itself:

```go
res, err := client.SendEmail(ctx, c, params)
if err != nil {
    if d, ok := jmapc.RetryAfter(err); ok {
        return job.again(d) // the server said when to come back
    }
    if jmapc.IsTemporary(err) {
        return job.again(backoff(job.attempts))
    }
    return job.fail(err) // the request itself is wrong, and sending it again
                         // gets the same answer
}
```

`IsTemporary` is false for what the server reported about the request — a 4xx
that is not a 429, and a method or record error such as `invalidArguments` or
`invalidProperties` — and true for what it reported about itself: a 5xx, a 429,
`serverUnavailable`, `serverFail`, `rateLimit`. A failure that cannot be
classified, such as a request that never reached the server, is temporary, since
nothing about it says the next attempt will fail as well. Where a request failed
for several reasons at once, one reason that will not pass with time makes the
whole of it permanent.

It says what to do with a failure, not whether the request is safe to send
again. A request that failed in transit may have been carried out, and a `/set`
sent twice creates twice, which is why `RetryPolicy` retries a 429 and a 503 and
nothing else by default.

`IsRateLimited` picks out the server asking for fewer requests, which arrives in
three shapes: a 429, a `rateLimit` reported for a call or a record, and a 400
refusing the request for exceeding `maxConcurrentRequests` or
`maxConcurrentUpload`.

`RetryAfter` returns the delay the server asked for, and whether it asked. The
client waits out a short one itself where the retry policy allows another
attempt; this is how a long one reaches the caller, since a server asking for an
hour is not waited out.

`HasErrorType` answers a different question: whether a particular error type is
among what failed. JMAP reports one condition at whichever level the server
refused at. A request that is too large is refused as a whole, an `Email/set`
that would take the account over its quota is refused as a call, and a single
message over the quota is refused as a record while the rest of the call goes
through. What the caller does about `overQuota` is usually the same in all
three:

```go
switch {
case jmapc.HasErrorType(err, jmapc.ErrOverQuota):
    return status(http.StatusInsufficientStorage)
case jmapc.HasErrorType(err, jmapc.ErrNotFound):
    return status(http.StatusNotFound)
case jmapc.HasErrorType(err, jmapc.ErrInvalidProperties):
    return status(http.StatusBadRequest)
}
```

The type is one of the constants the package declares — `ErrNotFound` and the
rest for a call and a record, `ErrTypeLimit` and the rest for a request, which
are URIs and so cannot be confused with them — or a string a server defines
itself. A `/set` that refused several records is read in full, so a type
reported for any one of them is found.

Which record was refused, and why, is `SetErrors.Failures`, reached with
`errors.As`. Which HTTP status a JMAP error type becomes, or which of them the
application retries, is the application's own table: jmapc says what the server
reported, not what it means to the program that asked.

### The session object

The session here is the Session object of RFC 8620, Section 2: the document a
server answers with at its session resource, saying where a request is sent,
which capabilities the server has, which accounts the user can reach, and what
the limits are. It is not a login session and it carries no credentials. What
authenticates a request is the Authorization header, and a token that expires is
a separate matter, in Tokens that expire below.

The client fetches the session when something first needs it and holds it from
then on.

A server's session changes: an account is added or removed, a limit is raised,
an endpoint moves, a push key is rotated. Every response carries the server's
`sessionState` for this reason, and the client compares it with the session it
holds. Where the two differ, the next call that needs the session fetches it
again. The comparison costs nothing, and the fetch happens where the session is
needed rather than in the response that reported the change.

Requests that arrive together after a change share one fetch between them. A
fetch that fails leaves the session as it was: what the client holds is out of
date, which is what it was a moment ago, so the request goes on and the next
call tries again. The failure reaches an `Observer` as a request of
`KindSession` that did not succeed.

The number of requests the client keeps in flight follows the session as well.
Where `maxConcurrentRequests` has gone down, the requests already in flight are
not cancelled: each returns its slot as it finishes, and no further slot is
given out until fewer than the new number are held. The client therefore never
has more in flight than the server last stated, which matters because a server
refuses the request that goes over with a 400, and a 400 is not sent again by
any retry policy.

`WithoutSessionRefresh` turns this off, for a client that will not outlive a
change and has no use for the comparison. `RefreshSession` fetches the session
whether or not a response reported a change, for a client that learns of one by
some other means.

### Splitting a large /get

A `/get` naming more ids than the server's `maxObjectsInGet` is refused.
`WithSplitGets` sends it in several requests instead, and joins the answers
into the one response the caller asked for:

```go
c := jmapc.New(url, jmapc.WithBearerToken(token), jmapc.WithSplitGets())
```

It is off by default, for two reasons. One call to `Do` then costs several
round trips. And the records no longer arrive as one snapshot: each request is
answered separately, and the account may change between them. Where the `state`
a `/get` reports differs between requests, the joined response is returned
together with a `*jmapc.StateChanged`, which `errors.As` reaches — the same
shape as a method error, so a caller that needs one snapshot can fetch again
and one that does not can ignore it.

Only the ids written into the request are counted, and two calls are sent as they
are. One whose ids come from a back reference, since how many they resolve to
is known to the server alone. And one that another call refers to, since a
reference resolves within one request, and splitting the call it names would
leave nothing to resolve against.

The ids that did not fit travel in further requests of their own, no more calls
in one request than `maxCallsInRequest` allows. The rest of the request is sent
once, in the first request, so the back references between its other calls
resolve as they did before.

### Tokens that expire

`WithBearerToken` holds one string for the life of the client. An OAuth 2.0
access token does not last that long, and replacing it means building another
client, which discards the cached session and the count of the requests in
flight along with it. `WithTokenSource` takes a function instead:

```go
c := jmapc.New(url, jmapc.WithTokenSource(func(ctx context.Context) (jmapc.Token, error) {
	tok, err := oauthConfig.TokenSource(ctx, refreshToken).Token()
	if err != nil {
		return jmapc.Token{}, err
	}
	return jmapc.Token{Value: tok.AccessToken, Expiry: tok.Expiry}, nil
}))
```

The token is held until it expires. A source that reports an `Expiry` is called
again shortly before it; one that reports none is called again only when a
server answers 401. Requests arriving together share one call, so a source that
exchanges a refresh token is not asked to do so several times at once — some
servers accept a refresh token only once.

A 401 also sends that one request again, once, with a newly fetched token. A
second 401 is reported to the caller, since a source returning a token the
server does not accept is not resolved by sending the request again. This is
separate from `WithRetry`, which retries what a server reported it did not
carry out.

### Retries

`WithRetry` retries when the server answers with HTTP 429 or 503.

```go
c := jmapc.New(url, jmapc.WithBearerToken(token), jmapc.WithRetry(3))
```

The argument is how many attempts to make. The delay is the value of the
server's `Retry-After`, or, where the server sends none, a delay that doubles
from 0.2 seconds to 30 seconds.

### Observability

`WithObserver` makes the client report what it does. The report affects
neither the request sent nor the response received, and a hook left nil is
never called.

```go
c := jmapc.New(url, jmapc.WithBearerToken(token),
	jmapc.WithObserver(jmapc.SlogObserver(slog.Default())))
```

There are three hooks, and they nest. `SlogObserver` writes a debug record for
each. `Request` covers one JMAP request: the calls it carries and its outcome.
`Attempt` covers one HTTP request under it, which includes the session fetch a
first request triggers and every retry. `Wait` covers a delay applied instead
of sending — for one of the slots `maxConcurrentRequests` allows, or before a
retry after a 429 or a 503.

```
Request   Email/query, Email/get
  Attempt GET  /.well-known/jmap  200
  Wait    for a slot, where the server accepts two requests at once
  Attempt POST /jmap/api          429
  Wait    for the two seconds of the server's Retry-After
  Attempt POST /jmap/api          200
```

An HTTP-level instrument already records the round trips, and where that is
enough, `WithHTTPClient` takes a client with an instrumented transport. What it
cannot record is the JMAP: which methods were sent together in one request, how
long the caller waited for a slot, and that a call was refused although the
request returned 200.

There is no dependency on OpenTelemetry, and none is needed. `Request` and
`Attempt` return the context used for the operation they cover, so a span
started in one becomes the parent of the spans started under it:

```go
tracer := otel.Tracer("jmapc")

obs := &jmapc.Observer{
	Request: func(ctx context.Context, info jmapc.RequestInfo) (context.Context, func(jmapc.ResponseInfo)) {
		methods := make([]string, len(info.Calls))
		for i, call := range info.Calls {
			methods[i] = call.Name
		}
		ctx, span := tracer.Start(ctx, "jmap.request",
			trace.WithAttributes(attribute.StringSlice("jmap.methods", methods)))
		return ctx, func(done jmapc.ResponseInfo) {
			span.SetAttributes(attribute.Int("jmap.method_errors", len(done.Errors)))
			if done.Err != nil {
				span.RecordError(done.Err)
			}
			span.End()
		}
	},
	Attempt: func(ctx context.Context, info jmapc.AttemptInfo) (context.Context, func(jmapc.AttemptInfo, jmapc.Answer)) {
		ctx, span := tracer.Start(ctx, "jmap."+string(info.Kind),
			trace.WithAttributes(attribute.Int("http.attempt", info.Attempt)))
		return ctx, func(info jmapc.AttemptInfo, answer jmapc.Answer) {
			span.SetAttributes(attribute.Int("http.status_code", answer.Status))
			span.End()
		}
	},
}
```

`Observer` exists only in the Go client. In Rust and TypeScript, the
equivalent belongs in the transport.


## Blobs

Attachments are not transferred through the API endpoint. They are uploaded and
downloaded over plain HTTP, at the URLs the session advertises, and the runtime
handles both:

```go
info, err := c.Upload(ctx, accountID, "application/pdf", file)
// info.BlobID now goes into an Email/set that attaches it.

blob, err := c.Download(ctx, accountID, part.BlobID, &jmapc.DownloadOptions{
	Name: *part.Name,
	Type: part.Type,
})
defer blob.Close()
```

Both stream. `Upload` reads from an `io.Reader` and `Download` returns an
`io.ReadCloser`, so an attachment larger than memory is copied from a file to
the server, and from the server to a file, without being held in either
direction. An upload larger than the server's `maxSizeUpload` fails before it
is sent.

`From` and `Length` fetch part of a blob, which is how a download interrupted
part way is resumed:

```go
blob, err := c.Download(ctx, accountID, blobID, &jmapc.DownloadOptions{From: written})
...
blob.Range // the part that came back: 4096-8191 of 8192
```

They are sent as an HTTP `Range` header. JMAP defines none for the download
endpoint, so a server is free to ignore it and answer with the whole blob;
where that happens the download fails rather than returning content the caller
would write at the wrong offset.

A server offering `urn:ietf:params:jmap:blob` can also create and read blobs
through the API, which the endpoints cannot: `Blob/upload` puts a blob in the
same request as the call that uses it, so the id never comes back to the client
in between.
