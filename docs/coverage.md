<p align="right">English | <a href="coverage.ja.md">日本語</a></p>

# Coverage

JMAP is a family of specifications: a server advertises capability URIs, and
each brings its own types and methods. These are the ones
[IANA lists](https://www.iana.org/assignments/jmap/jmap.xhtml), and where jmapc
stands on each.

| Capability | Specification | Supported |
|---|---|---|
| `urn:ietf:params:jmap:core` | [RFC 8620](https://www.rfc-editor.org/rfc/rfc8620) | ✅ |
| `urn:ietf:params:jmap:mail` | [RFC 8621](https://www.rfc-editor.org/rfc/rfc8621) | ✅ |
| `urn:ietf:params:jmap:submission` | [RFC 8621](https://www.rfc-editor.org/rfc/rfc8621) | ✅ |
| `urn:ietf:params:jmap:vacationresponse` | [RFC 8621](https://www.rfc-editor.org/rfc/rfc8621) | ✅ |
| `urn:ietf:params:jmap:contacts` | [RFC 9610](https://www.rfc-editor.org/rfc/rfc9610) | ✅ |
| `urn:ietf:params:jmap:calendars` | [draft-ietf-jmap-calendars](https://datatracker.ietf.org/doc/draft-ietf-jmap-calendars/) | ✅ |
| `urn:ietf:params:jmap:principals:availability` | [draft-ietf-jmap-calendars](https://datatracker.ietf.org/doc/draft-ietf-jmap-calendars/) | ✅ |
| `urn:ietf:params:jmap:principals` | [RFC 9670](https://www.rfc-editor.org/rfc/rfc9670) | ✅ |
| `urn:ietf:params:jmap:principals:owner` | [RFC 9670](https://www.rfc-editor.org/rfc/rfc9670) | ✅ |
| `urn:ietf:params:jmap:smimeverify` | [RFC 9219](https://www.rfc-editor.org/rfc/rfc9219) | ✅ |
| `urn:ietf:params:jmap:blob` | [RFC 9404](https://www.rfc-editor.org/rfc/rfc9404) | ✅ |
| `urn:ietf:params:jmap:quota` | [RFC 9425](https://www.rfc-editor.org/rfc/rfc9425) | ✅ |
| `urn:ietf:params:jmap:sieve` | [RFC 9661](https://www.rfc-editor.org/rfc/rfc9661) | ✅ |
| `urn:ietf:params:jmap:mdn` | [RFC 9007](https://www.rfc-editor.org/rfc/rfc9007) | ✅ |
| `urn:ietf:params:jmap:webpush-vapid` | [RFC 9749](https://www.rfc-editor.org/rfc/rfc9749) | ✅ |

Two of these store objects from specifications of their own: a contact card is
a [JSContact](https://www.rfc-editor.org/rfc/rfc9553) Card, and a calendar event
is a [JSCalendar](https://www.rfc-editor.org/rfc/rfc8984) JSEvent. Both name
types that JMAP also names, and each other's too — there are three different
`Link` types between them. So those carry a prefix: `ContactEmailAddress` is an
address on a card, `EmailAddress` is one in a header field, and `EventLink` is a
resource attached to a meeting. Each type's documentation gives the name its
specification uses.

JSCalendar also brings time types JMAP does not have. An event's `start` is a
`LocalDateTime` with no zone, and its `duration` is an ISO 8601 `Duration`,
because "P1D" across a daylight saving change is not always 24 hours. Both are
checked in a query, so a `start` written with a `Z` on the end, or a duration
written as `90m`, fails to build.

Not every capability brings types of its own. S/MIME verification adds four
properties to `Email` and nothing else, so a query needs it without any method
name saying so. jmapc works out which capabilities the properties a query
touches belong to, and declares them: ask for `smimeStatus` and
`urn:ietf:params:jmap:smimeverify` appears in `using` on its own.

Some define neither types nor methods, only a value for the client. VAPID is
one, and that value is a key. Those are read from the session, and
`Session.Capability` reads any of them, including one jmapc does not know.

```go
vapid, err := session.WebPushVAPID()
// vapid.ApplicationServerKey goes to the push service when subscribing there.

var limits struct{ MaxSizeScript int `json:"maxSizeScript"` }
err = session.Accounts[accountID].Capability(jmapc.CapabilitySieve, &limits)
```

A capability that is not built in can still be used: describe its types in a
[schema file](extensions.md) and queries against them are checked like any
other. That is the same mechanism a vendor extension uses, and the work is
declarative — no Go to write.

## Methods

81 methods, all of them checked and generated the same way.

| Type | Methods |
|---|---|
| `Mailbox` | `get` `changes` `set` `query` `queryChanges` |
| `Thread` | `get` `changes` |
| `Email` | `get` `changes` `set` `copy` `query` `queryChanges` `import` `parse` |
| `SearchSnippet` | `get` |
| `Identity` | `get` `changes` `set` |
| `EmailSubmission` | `get` `changes` `set` `query` `queryChanges` |
| `VacationResponse` | `get` `set` |
| `AddressBook` | `get` `changes` `set` |
| `ContactCard` | `get` `changes` `set` `copy` `query` `queryChanges` |
| `Calendar` | `get` `changes` `set` |
| `CalendarEvent` | `get` `changes` `set` `copy` `query` `queryChanges` `parse` |
| `CalendarEventNotification` | `get` `changes` `set` `query` `queryChanges` |
| `ParticipantIdentity` | `get` `changes` `set` |
| `Principal` | `get` `changes` `set` `query` `queryChanges` `getAvailability` |
| `ShareNotification` | `get` `changes` `set` `query` `queryChanges` |
| `Quota` | `get` `changes` `query` `queryChanges` |
| `SieveScript` | `get` `set` `query` `validate` |
| `MDN` | `send` `parse` |
| `Blob` | `copy` `upload` `get` `lookup` |
| `PushSubscription` | `get` `set` |
| `Core` | `echo` |

## What is not checked

One thing, and it is on purpose.

**Open sets are not checked, deliberately.** Where a specification fixes the
values a property takes, jmapc checks them. Where it leaves the set open — a
mailbox `role`, an email keyword, a `Content-Disposition` — it does not, because
rejecting a value the server would have accepted is worse than letting a typo
through.

## Generation

`internal/spec` is a plain Go declaration of the data model, and the runtime
types in `types_gen.go` are generated from the same catalogue the queries are
checked against, so the two cannot drift apart.
