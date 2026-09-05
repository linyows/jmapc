package jmapc

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
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

// String returns the date as the wire carries it. A client keeping a JMAP date
// as the text it arrived as — for a response of its own, or a column — would
// otherwise have to know the layout, and a copy of it is a copy that can drift
// from this one without saying so. It also settles what fmt prints, which the
// embedded time.Time would otherwise answer for in a form no JMAP server
// wrote.
func (d UTCDate) String() string {
	return d.Time.UTC().Format(utcDateLayout)
}

func (d UTCDate) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
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

// String returns the date as the wire carries it, offset and all, for the same
// reason UTCDate has one.
func (d Date) String() string {
	return d.Time.Format(time.RFC3339)
}

func (d Date) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
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
//
// The leading "/" of the pointer is implicit, as RFC 8620, Section 5.3 has it:
// a keyword is set at "keywords/$seen" rather than at "/keywords/$seen", and
// writing the slash asks for a property with no name.
type PatchObject map[string]any

// Set records that the value at the given JSON pointer should be replaced. The
// pointer is written without its leading "/", as "keywords/$seen".
func (p PatchObject) Set(pointer string, value any) PatchObject {
	p[pointer] = value
	return p
}

// Remove records that the member at the given JSON pointer should be deleted.
// The pointer is written without its leading "/", as "keywords/$seen".
func (p PatchObject) Remove(pointer string) PatchObject {
	p[pointer] = nil
	return p
}

// LocalDateTime is the JSCalendar LocalDateTime of RFC 8984, Section 1.4.4: a
// date and time with no time zone and no offset, such as "2024-05-01T09:00:00".
// What it means depends on the time zone the enclosing object gives, and for a
// recurring event that time zone is the point: an alarm set for nine in the
// morning stays at nine when the clocks change.
//
// It is a string rather than a time.Time because it is used as a map key, in
// the recurrenceOverrides of an event, and because a time.Time can carry a
// location that this type has no way to mean.
type LocalDateTime string

var localDateTimePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?$`)

// Valid reports whether the value has the form the specification requires.
func (d LocalDateTime) Valid() bool { return localDateTimePattern.MatchString(string(d)) }

func (d LocalDateTime) String() string { return string(d) }

// NewLocalDateTime returns t as a local date-time, discarding its location.
func NewLocalDateTime(t time.Time) LocalDateTime {
	return LocalDateTime(t.Format("2006-01-02T15:04:05"))
}

// In returns the point in time this date-time denotes in the given location.
// The location comes from elsewhere in the event, which is why it cannot be
// resolved here.
func (d LocalDateTime) In(loc *time.Location) (time.Time, error) {
	t, err := time.ParseInLocation("2006-01-02T15:04:05", strings.SplitN(string(d), ".", 2)[0], loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("jmapc: invalid LocalDateTime %q: %w", d, err)
	}
	return t, nil
}

// Duration is the JSCalendar Duration of RFC 8984, Section 1.4.6: a length of
// time in the ISO 8601 form, such as "PT1H30M" for an hour and a half, or "P1D"
// for a day.
//
// A day is not always 24 hours, so this is not a time.Duration: "P1D" across a
// daylight saving change is 23 or 25 hours. ToTimeDuration converts the part
// that can be converted.
type Duration string

// durationPattern matches the subset of ISO 8601 durations that JSCalendar
// allows: weeks, or days with an optional time, with no years or months.
var durationPattern = regexp.MustCompile(`^P(?:\d+W|(?:\d+D)?(?:T(?:\d+H)?(?:\d+M)?(?:\d+(?:\.\d+)?S)?)?)$`)

// Valid reports whether the value has the form the specification requires.
func (d Duration) Valid() bool {
	s := string(d)
	// "P" and "PT" match the pattern but denote nothing.
	if s == "P" || s == "PT" {
		return false
	}
	return durationPattern.MatchString(s)
}

func (d Duration) String() string { return string(d) }

// ToTimeDuration converts the duration to a time.Duration, counting a week as
// seven days and a day as 24 hours. That is exact for a duration expressed in
// hours or less, and an approximation for one in days or weeks, which is why
// the calendar itself works in the original units.
func (d Duration) ToTimeDuration() (time.Duration, error) {
	if !d.Valid() {
		return 0, fmt.Errorf("jmapc: invalid Duration %q", d)
	}
	// Rewrite into the form time.ParseDuration understands, which has no day
	// or week and needs a unit on every number.
	s := strings.TrimPrefix(string(d), "P")
	var total time.Duration
	datePart, timePart, hasTime := strings.Cut(s, "T")
	if weeks, ok := strings.CutSuffix(datePart, "W"); ok {
		n, err := strconv.ParseFloat(weeks, 64)
		if err != nil {
			return 0, fmt.Errorf("jmapc: invalid Duration %q: %w", d, err)
		}
		return time.Duration(n * float64(7*24*time.Hour)), nil
	}
	if days, ok := strings.CutSuffix(datePart, "D"); ok && days != "" {
		n, err := strconv.ParseFloat(days, 64)
		if err != nil {
			return 0, fmt.Errorf("jmapc: invalid Duration %q: %w", d, err)
		}
		total += time.Duration(n * float64(24*time.Hour))
	}
	if hasTime && timePart != "" {
		clock, err := time.ParseDuration(strings.ToLower(timePart))
		if err != nil {
			return 0, fmt.Errorf("jmapc: invalid Duration %q: %w", d, err)
		}
		total += clock
	}
	return total, nil
}

// SignedDuration is the JSCalendar SignedDuration of RFC 8984, Section 1.4.7: a
// Duration that may be negative, which is how an alert says it fires before the
// event it belongs to.
type SignedDuration string

// Valid reports whether the value has the form the specification requires.
func (d SignedDuration) Valid() bool {
	s := string(d)
	negative := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	_ = negative
	return Duration(s).Valid()
}

func (d SignedDuration) String() string { return string(d) }

// ToTimeDuration converts the duration as Duration.ToTimeDuration does,
// carrying the sign.
func (d SignedDuration) ToTimeDuration() (time.Duration, error) {
	s := string(d)
	sign := time.Duration(1)
	if rest, ok := strings.CutPrefix(s, "-"); ok {
		sign, s = -1, rest
	} else if rest, ok := strings.CutPrefix(s, "+"); ok {
		s = rest
	}
	base, err := Duration(s).ToTimeDuration()
	if err != nil {
		return 0, fmt.Errorf("jmapc: invalid SignedDuration %q: %w", d, err)
	}
	return sign * base, nil
}

// TimeZoneID is the JSCalendar TimeZoneId of RFC 8984, Section 1.4.9: the name
// of a time zone in the IANA database, such as "Europe/London", or a name
// beginning with "/" that refers to a custom zone the event itself defines.
type TimeZoneID string

// IsCustom reports whether the id refers to a zone defined in the event's own
// timeZones property rather than to one in the IANA database.
func (z TimeZoneID) IsCustom() bool { return strings.HasPrefix(string(z), "/") }

func (z TimeZoneID) String() string { return string(z) }

// Location returns the time zone the id names, looking it up in the IANA
// database. A custom zone is not there, so it fails; resolve those from the
// event's timeZones instead.
func (z TimeZoneID) Location() (*time.Location, error) {
	if z.IsCustom() {
		return nil, fmt.Errorf("jmapc: %q is a custom time zone, defined by the event itself", z)
	}
	return time.LoadLocation(string(z))
}
