package example

import (
	"errors"
	"testing"
	"time"

	"github.com/linyows/jmapc"
)

// mustTime parses a UTC timestamp for use in a comparison.
func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parsing %q: %v", s, err)
	}
	return parsed.UTC()
}

// asMethodErrors reports whether err is or wraps a jmapc.MethodErrors.
func asMethodErrors(err error, target *jmapc.MethodErrors) bool {
	return errors.As(err, target)
}

// asRequestError reports whether err is or wraps a *jmapc.RequestError.
func asRequestError(err error, target **jmapc.RequestError) bool {
	return errors.As(err, target)
}
