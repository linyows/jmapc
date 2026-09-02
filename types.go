package jmapc

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

// ID is the JMAP Id data type defined in RFC 8620, Section 1.2. It is a string
// of at least 1 and at most 255 octets drawn from the URL and filename safe
// base64 alphabet, and it must not begin with a "-" or "#".
type ID string

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,255}$`)

// Valid reports whether the id satisfies the syntactic restrictions the
// specification places on the Id type. Servers are free to assign any id that
// matches, so this only rejects values that no conformant server could produce.
func (i ID) Valid() bool {
	if !idPattern.MatchString(string(i)) {
		return false
	}
	return i[0] != '-' && i[0] != '#'
}

func (i ID) String() string { return string(i) }

// Int is the JMAP Int data type: a signed integer in the range that survives a
// round trip through an IEEE 754 double.
type Int int64

// UnsignedInt is the JMAP UnsignedInt data type: an Int that is never negative.
type UnsignedInt uint64

const (
	// MaxInt is the largest value the JMAP Int type may hold, 2^53-1.
	MaxInt Int = 1<<53 - 1
	// MinInt is the smallest value the JMAP Int type may hold, -(2^53-1).
	MinInt Int = -(1<<53 - 1)
)

// UTCDate is the JMAP UTCDate data type: a date-time in UTC, always serialised
// with a "Z" suffix and no fractional seconds.
type UTCDate struct {
	time.Time
}

// NewUTCDate returns t converted to UTC and truncated to the second, which is
// the precision the wire format carries.
func NewUTCDate(t time.Time) UTCDate {
	return UTCDate{Time: t.UTC().Truncate(time.Second)}
}

const utcDateLayout = "2006-01-02T15:04:05Z"

func (d UTCDate) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Time.UTC().Format(utcDateLayout))
}

func (d *UTCDate) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	t, err := time.Parse(utcDateLayout, s)
	if err != nil {
		// Some servers emit fractional seconds or an offset even where the
		// specification calls for UTCDate, so fall back to the wider Date form
		// rather than failing the whole response.
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return fmt.Errorf("jmapc: invalid UTCDate %q: %w", s, err)
		}
	}
	d.Time = t.UTC()
	return nil
}

// Date is the JMAP Date data type: a date-time that carries its own UTC offset.
type Date struct {
	time.Time
}

// NewDate returns t as a JMAP Date, preserving its location.
func NewDate(t time.Time) Date {
	return Date{Time: t.Truncate(time.Second)}
}

func (d Date) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Time.Format(time.RFC3339))
}

func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return fmt.Errorf("jmapc: invalid Date %q: %w", s, err)
	}
	d.Time = t
	return nil
}

// PatchObject is the JMAP PatchObject data type used by the update argument of
// a /set call. Each key is a JSON pointer into the object being patched and
// each value is the replacement, or nil to remove the pointed-at member.
type PatchObject map[string]any

// Set records that the value at the given JSON pointer should be replaced.
func (p PatchObject) Set(pointer string, value any) PatchObject {
	p[pointer] = value
	return p
}

// Remove records that the member at the given JSON pointer should be deleted.
func (p PatchObject) Remove(pointer string) PatchObject {
	p[pointer] = nil
	return p
}
