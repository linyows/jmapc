# Errors

JMAP fails at two levels, and the errors a generated function returns follow
them.

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

## Temporary and permanent failures

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
