package shared

import (
	"fmt"
	"strings"

	"github.com/linyows/jmapc/internal/request"
	"github.com/linyows/jmapc/internal/spec"
)

// JoinMethods renders a list of method names for prose.
func JoinMethods(names []string) string {
	switch len(names) {
	case 0:
		return "no calls"
	case 1:
		return "one " + names[0] + " call"
	case 2:
		return names[0] + " and " + names[1] + " calls"
	default:
		return strings.Join(names[:len(names)-1], ", ") + ", and " + names[len(names)-1] + " calls"
	}
}

// RoundTripPhrase describes what batching the calls buys.
func RoundTripPhrase(n int) string {
	if n <= 1 {
		return "the server is asked once"
	}
	return fmt.Sprintf("%d dependent calls cost one round trip", n)
}

// HeaderPropertyDoc describes a property naming one header field of a message.
func HeaderPropertyDoc(h *spec.HeaderProperty) string {
	which := "The " + h.Name + " header field"
	if h.All {
		which = "Every " + h.Name + " header field in the message, in the order they appear"
	} else {
		which += ", or the last of them where the message has several"
	}
	switch h.Form {
	case "", "asRaw":
		return which + ", as it appears in the message."
	case "asText":
		return which + ", decoded and unfolded into text."
	case "asAddresses":
		return which + ", parsed as a list of addresses."
	case "asGroupedAddresses":
		return which + ", parsed as a list of addresses, keeping the groups they were written in."
	case "asMessageIds":
		return which + ", parsed as message ids, without their angle brackets."
	case "asDate":
		return which + ", parsed as a date."
	case "asURLs":
		return which + ", parsed as a list of URLs."
	}
	return which + "."
}

// DynamicPropertyDoc describes a property whose meaning comes from the server
// rather than from the data model.
func DynamicPropertyDoc(name string) string {
	switch {
	case strings.HasPrefix(name, "header:"):
		return "The " + name + " header field, in the form the request asked for."
	case strings.HasPrefix(name, "digest:"):
		return "The digest of the blob under the " + strings.TrimPrefix(name, "digest:") +
			" algorithm, as base64."
	}
	return "The " + name + " property, whose meaning the server decides."
}

// PrimaryAccountPhrase describes the account a request is sent to where the
// request does not say. A session has a primary account for each capability
// rather than one for everything, so the capability is named: a request reading
// identities and one creating a mailbox may be talking to two different
// accounts, and the only place that shows is here.
func PrimaryAccountPhrase(capabilities []string) string {
	const cost = ", which costs a session lookup on first use."
	switch len(capabilities) {
	case 0:
		return ""
	case 1:
		return "The request does not say which account to use, so the session's primary account for " +
			capabilities[0] + " is used" + cost
	}
	return "The request does not say which account to use, so the session's primary account is used for each of " +
		joinURIs(capabilities) + cost + " They need not be the same account."
}

// joinURIs renders a list of capability URIs for prose.
func joinURIs(uris []string) string {
	if len(uris) == 2 {
		return uris[0] + " and " + uris[1]
	}
	return strings.Join(uris[:len(uris)-1], ", ") + ", and " + uris[len(uris)-1]
}

// FuncDoc returns the documentation of the function a request is generated as,
// named as the language names it: what the request's author wrote of it, what
// calls it makes, what it returns, and the session lookups and creation ids it
// involves. capabilities are those whose primary account it looks up.
func FuncDoc(q *request.Request, name string, capabilities []string) string {
	doc := strings.TrimSpace(q.Doc)
	if doc == "" {
		doc = fmt.Sprintf("%s sends the JMAP request in %s.", name, q.Path)
	}
	methods := make([]string, len(q.Calls))
	for i, c := range q.Calls {
		methods[i] = c.Method.Name
	}
	doc += fmt.Sprintf("\n\nIt makes %s in a single request, so that %s.",
		JoinMethods(methods), RoundTripPhrase(len(q.Calls)))
	if q.Returns != nil {
		doc += fmt.Sprintf(" It returns the response to the %s call.", q.Returns.Method.Name)
	}
	if len(capabilities) > 0 {
		doc += "\n\n" + PrimaryAccountPhrase(capabilities)
	}
	if q.CreatedIDs {
		doc += "\n\nIt takes the creation ids of an earlier request and reports its own, so that a reference to something created there still resolves here."
	}
	return doc
}
