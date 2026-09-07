<p align="right">English | <a href="push.ja.md">日本語</a></p>

# Push

An event reports which types in which accounts have changed, not what changed.
A client that needs the changes therefore writes a loop: connect, request the
changes since the state it holds, apply them, wait for the next event. That
loop is the same every time and every part of it is a place for a mistake, so a
request file can ask for it.

`_watches` names the call the loop reads the state from, which has to be one
that reports what changed since a state, as `Email/changes` does:

```json
{
  "_watches": "changes",

  "methodCalls": [
    ["Email/changes", {"sinceState": "{{sinceState}}", "maxChanges": 128}, "changes"],
    ["Email/get", {"#ids": {"resultOf": "changes", "name": "Email/changes", "path": "/created"}}, "created"]
  ]
}
```

`SyncEmails` is generated as it would be anyway, and `SyncEmailsWatch` alongside
it:

```go
err := client.SyncEmailsWatch(ctx, c, client.SyncEmailsParams{SinceState: state},
	func(ctx context.Context, res *client.SyncEmailsResult) error {
		for _, email := range res.EmailGet.List {
			fmt.Println("new:", *email.Subject)
		}
		state = res.EmailChanges.NewState // keep it, and start there next time
		return nil
	})
```

It starts from the state the parameters carry, and continues from the state
each answer reports. What it removes from the caller:

- **A stream is a connection, not a subscription.** When it drops, another is
  opened, resuming from the last event delivered, with a delay that doubles
  from a second to 30 seconds while the server is unreachable.
- **Changes made while no connection was open are not pushed**, so every
  connection is followed by a catch-up.
- **A server returns as many changes as it chooses** and sets
  `hasMoreChanges`, so the loop repeats the request until that is false.
- **An event about another account, another type, or a state the loop has
  already reached** does not need a request. The last of those is the common
  case: a catch-up causes the server to push the state it has just received.

The loop runs until the context ends, which is the error it returns. An error
from the callback stops the loop and is returned unchanged. A server that
refuses the connection outright returns that error immediately rather than
retrying, because retrying will not change a 403. `jmapc.WithPing` and
`jmapc.WithReconnect` configure the two values worth tuning.

A watch resumed after a long enough pause meets one error it cannot go on from.
A `/changes` call answers `cannotCalculateChanges` where the state it is given
is older than the server keeps, and asking again with the same state gets the
same answer. RFC 8620 has the client read the records again instead, and
`jmapc.WithResync` is where that is done:

```go
err := client.SyncEmailsWatch(ctx, c, client.SyncEmailsParams{SinceState: state},
	func(ctx context.Context, res *client.SyncEmailsResult) error {
		state = res.Changes.NewState
		return nil
	},
	jmapc.WithResync(func(ctx context.Context) (string, error) {
		res, err := client.ListInboxEmails(ctx, c, client.ListInboxEmailsParams{
			MailboxID: inbox,
			Limit:     500,
		})
		if err != nil {
			return "", err
		}
		cache.replace(res.List)
		return res.State, nil
	}))
```

The watch continues from the state that function reports. Without it the watch
stops and returns the error, and a program that followed changes stops following
them, with nothing but that error to say so. Where the server will not calculate
changes from the state a resync has just reported either, the watch stops:
reading the records again would report that state again and meet the same
answer.

Underneath is `Client.Watch`, which takes the catch-up as a function and is what
to call where the catching up is not one request:

```go
err := c.Watch(ctx, accountID, "Email", state,
	func(ctx context.Context, since string) (newState string, more bool, err error) {
		// ... /changes from since, then whatever the ids call for
	})
```

Only the Go client follows a watch. Holding a connection open is the runtime's
responsibility rather than the generated code's, and the Rust and TypeScript
runtimes do not implement it; generating either from a watching request writes
the request without the loop and reports that.

Below `Watch` is `Client.EventSource`, which opens the push endpoint and returns
the events:

```go
stream, err := c.EventSource(ctx, &jmapc.EventSourceOptions{
	Types: []string{"Email"},
	Ping:  30 * time.Second,
})
defer stream.Close()

for {
	change, err := stream.Next()
	if err != nil {
		break // reconnect, passing stream.LastEventID()
	}
	if state, ok := change.StateOf(accountID, "Email"); ok {
		_ = state
	}
}
```

This is the event source form of push, which suits a client that can hold a
connection open. The other form registers a URL for the server to post to, which
is what an app on a phone needs: see `RegisterPush` and `ConfirmPush` in
[`example/requests`](../example/requests). A subscription is not active when it is
created — the server pushes a code to the URL, and the client sends it back with
a `PushSubscription/set` before anything else is sent. `jmapc.PushVerification`
decodes what arrives.
