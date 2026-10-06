package syntax

import "testing"

// TestValidUTCDate checks the dates a UTCDate is and is not. Go's parser takes
// a comma before the fraction of a second, which the form does not, so a date
// written that way is not one, as the editor schema says.
func TestValidUTCDate(t *testing.T) {
	for _, s := range []string{"2026-09-04T09:00:00Z", "2026-09-04T09:00:00.5Z"} {
		if !ValidUTCDate(s) {
			t.Errorf("ValidUTCDate(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"2026-09-04T09:00:00,5Z", "2026-09-04", "2026-09-04T09:00:00+09:00", "2026-02-30T09:00:00Z"} {
		if ValidUTCDate(s) {
			t.Errorf("ValidUTCDate(%q) = true, want false", s)
		}
	}
}

// TestValidDate checks the same of a Date, which carries an offset.
func TestValidDate(t *testing.T) {
	for _, s := range []string{"2026-09-04T09:00:00Z", "2026-09-04T09:00:00.5+09:00"} {
		if !ValidDate(s) {
			t.Errorf("ValidDate(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"2026-09-04T09:00:00,5+09:00", "2026-09-04T09:00:00"} {
		if ValidDate(s) {
			t.Errorf("ValidDate(%q) = true, want false", s)
		}
	}
}
