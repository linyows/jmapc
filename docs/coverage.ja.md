<p align="right"><a href="coverage.md">English</a> | 日本語</p>

# 対応範囲

JMAP は仕様の集まりです。
サーバはケイパビリティ URI を広告し、それぞれが固有の型とメソッドを持ち込みます。
以下は [IANA が登録しているもの](https://www.iana.org/assignments/jmap/jmap.xhtml)と、それぞれに対する jmapc の状況です。

| ケイパビリティ | 仕様 | サポート |
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

このうち二つは、それ自体が別仕様のオブジェクトを格納します。
連絡先カードは [JSContact](https://www.rfc-editor.org/rfc/rfc9553) の Card であり、カレンダーの予定は [JSCalendar](https://www.rfc-editor.org/rfc/rfc8984) の JSEvent です。
どちらも JMAP が使っている型名を使い、しかも互いの型名とも衝突します。
三つの異なる `Link` 型が存在することになります。
そこでこれらには接頭辞を付けています。
`ContactEmailAddress` はカード上のアドレス、`EmailAddress` はヘッダフィールドのアドレス、`EventLink` は会議に添付されたリソースです。
各型のドキュメントには、その仕様が使っている名前を記載しています。

JSCalendar は JMAP にない時刻の型も持ち込みます。
予定の `start` はタイムゾーンを持たない `LocalDateTime` で、`duration` は ISO 8601 の `Duration` です。
`Duration` が独自の型なのは、サマータイムの切り替えを跨ぐ `P1D` が常に 24 時間とは限らないからです。
どちらもクエリで検証されるので、末尾に `Z` の付いた `start` や、`90m` と書いた duration はビルドに失敗します。

ケイパビリティのすべてが固有の型を持ち込むわけではありません。
S/MIME の検証は `Email` に四つのプロパティを足すだけで、型もメソッドも増やしません。
つまりメソッド名からは、そのケイパビリティが必要だと分かりません。
jmapc はクエリが触れたプロパティがどのケイパビリティに属するかを判断し、`using` に加えます。
`smimeStatus` を要求すれば、`urn:ietf:params:jmap:smimeverify` が自動で現れます。

型もメソッドも持たず、クライアントに伝えることだけを持つケイパビリティもあります。
VAPID がそれで、伝えるのは鍵です。
こうしたものはセッションから読みます。
`Session.Capability` は、jmapc が知らないケイパビリティも含めて、どれでも読めます。

```go
vapid, err := session.WebPushVAPID()
// vapid.ApplicationServerKey を push service への購読時に渡す。

var limits struct{ MaxSizeScript int `json:"maxSizeScript"` }
err = session.Accounts[accountID].Capability(jmapc.CapabilitySieve, &limits)
```

サポートしていないケイパビリティも、手が届かないわけではありません。
[スキーマファイル](extensions.ja.md)に型を記述すれば、それに対するクエリも他と同じように検証されます。
ベンダ拡張と同じ仕組みであり、記述するのは宣言だけで、Go を書く必要はありません。

## メソッド

81 のメソッドがあり、すべて同じ方法で検証され生成されます。

| 型 | メソッド |
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

## 検証しないもの

一つだけあり、それは意図的なものです。

**開いた集合は意図的に検証しない**：仕様が値を固定しているプロパティは検証します。
一方、集合が開いているもの、たとえばメールボックスの `role`、メールのキーワード、`Content-Disposition` は検証しません。
サーバが受け付けたはずの値を拒否するほうが、綴り間違いを通すより害が大きいからです。
