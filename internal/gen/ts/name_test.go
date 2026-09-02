package ts

import "testing"

func TestTSMemberName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"MailboxID", "mailboxId"},
		{"AccountID", "accountId"},
		{"EmailIDs", "emailIds"},
		{"ID", "id"},
		{"Limit", "limit"},
		{"HeaderListIDAsText", "headerListIdAsText"},
		{"SMIMEStatus", "smimeStatus"},
		{"EmailQuery", "emailQuery"},
		{"URL", "url"},
		{"PublicKey", "publicKey"},
		{"DeviceClientID", "deviceClientId"},
	}
	for _, tt := range tests {
		if got := tsMemberName(tt.in); got != tt.want {
			t.Errorf("tsMemberName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
