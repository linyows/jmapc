<p align="right">English | <a href="testing.ja.md">日本語</a></p>

# Testing

Testing the code you write around a generated client means answering a request
that carries several method calls, some of which refer to the results of the
others. A stub written by hand for one test either ignores that, and no longer
resembles a server, or grows into this:

```go
srv := jmaptest.New(t)
srv.Reply("Email/query", jmapc.EmailQueryResponse{
	AccountID: jmaptest.AccountID,
	IDs:       []jmapc.ID{"m1", "m2"},
})
srv.Handle("Email/get", func(c *jmaptest.Call) (any, error) {
	// The ids are the ones the request call answered with: the back reference
	// has already been resolved, the way a server resolves it.
	return emailsFor(c.IDs()), nil
})

res, err := client.ListInboxEmails(ctx, srv.Client(), params)
```

What it removes from the test:

- **The back references.** They are resolved as RFC 8620 defines, including the
  `*` that maps a path over a list, so a chained request reaches the handlers
  with resolved values.
- **The checking.** The request is checked against the data model the same way
  the build checks a request, so a call with an argument no method has fails the
  test rather than passing quietly. `jmaptest.WithoutChecks()` disables it, for
  a method jmapc does not know.
- **The failures.** `srv.Fail` for a method-level error, `srv.FailRequest` for a
  request rejected as a whole, and a `/set` response listing what it refused for
  the failure that answers 200.
- **What was requested.** `srv.Call("Email/query")` is the last call to a method,
  `srv.Calls()` all of them, and `srv.Requests()` how many requests they took —
  which is how to check that calls were sent in one request rather than one at
  a time.
- **The push.** `srv.Push` sends a state change to a watching client, which is
  what a watching request's loop waits for.

What it does not do is store anything. It is a server to test a client against
rather than an implementation of JMAP: nothing a `/set` creates comes back from
a later `/get` unless the test says it does.

A client is rarely converted all at once, and a half-converted one has two
halves to answer in the same test: the generated half, which reaches the
server through `srv.Client()`, and the half still written by hand, which posts
to paths of its own. `srv.Mux()` is where those paths go, and `srv.BaseURL()`
is what the other half is pointed at:

```go
srv := jmaptest.New(t)
srv.Mux().HandleFunc("/jmap", myOldAPIHandler)
srv.Mux().HandleFunc("/jmap/session", myOldSessionHandler)

old := myOldClient(srv.BaseURL())
```

Where the other half already uses JMAP and only expects it at different paths —
deriving the session and the API from a base URL of its own — mount jmaptest's
own handlers there instead, and it answers under both paths:

```go
srv.Mux().HandleFunc("/jmap/session", srv.ServeSession)
srv.Mux().HandleFunc("/jmap", srv.ServeAPI)
```

So jmaptest is worth adopting on the first method converted rather than the
last.
