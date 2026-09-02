package spec

// Standard returns a catalogue holding the core specification, RFC 8620, and
// JMAP for Mail, RFC 8621. Each call builds a fresh catalogue, so a caller that
// extends it with vendor types does not disturb anyone else's.
func Standard() *Spec {
	s := New()
	registerCore(s)
	registerMail(s)
	registerSubmission(s)
	registerVacation(s)
	registerContacts(s)
	registerCalendars(s)
	return s
}
