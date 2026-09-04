package request

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/linyows/jmapc"
	"github.com/linyows/jmapc/internal/query"
	"github.com/linyows/jmapc/internal/spec"
)

// ParseValue turns the text a caller wrote for a parameter into the JSON value
// the parameter's type calls for. A parameter standing in for a string is the
// text itself, so nothing has to be quoted on a command line; one standing in
// for anything with a shape is written as JSON.
func ParseValue(p *query.Param, text string) (Value, error) {
	raw, err := parseAs(p.ValueType(), text)
	if err != nil {
		return Value{}, fmt.Errorf("parameter %s: %w", p.Name, err)
	}
	return Value{Text: text, JSON: raw}, nil
}

// parseAs renders text as a JSON value of the given type.
func parseAs(t *spec.Type, text string) (json.RawMessage, error) {
	switch t.Name {
	case spec.String, spec.TimeZoneIDType:
		return quote(text), nil

	case spec.IdType:
		if !jmapc.ID(text).Valid() {
			return nil, fmt.Errorf("%q is not a valid id\n\tan id is 1 to 255 characters from A-Z, a-z, 0-9, _ and -", text)
		}
		return quote(text), nil

	case spec.Boolean:
		b, err := strconv.ParseBool(text)
		if err != nil {
			return nil, fmt.Errorf("%q is not a boolean\n\twrite true or false", text)
		}
		return quoteBool(b), nil

	case spec.Number:
		if _, err := strconv.ParseFloat(text, 64); err != nil {
			return nil, fmt.Errorf("%q is not a number", text)
		}
		return json.RawMessage(text), nil

	case spec.Int, spec.UnsignedInt:
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%q is not a whole number", text)
		}
		if t.Name == spec.UnsignedInt && n < 0 {
			return nil, fmt.Errorf("%s is negative, and an UnsignedInt is not", text)
		}
		if n > int64(jmapc.MaxInt) || n < int64(jmapc.MinInt) {
			return nil, fmt.Errorf("%s is outside the range of %s\n\tJMAP integers are limited to the range a double can hold exactly", text, t)
		}
		return json.RawMessage(strconv.FormatInt(n, 10)), nil

	case spec.UTCDateType:
		if _, err := time.Parse("2006-01-02T15:04:05Z", text); err != nil {
			return nil, fmt.Errorf("%q is not a UTCDate\n\ta UTCDate is written as 2006-01-02T15:04:05Z", text)
		}
		return quote(text), nil

	case spec.DateType:
		if _, err := time.Parse(time.RFC3339, text); err != nil {
			return nil, fmt.Errorf("%q is not a Date\n\ta Date is written as 2006-01-02T15:04:05Z07:00", text)
		}
		return quote(text), nil

	case spec.LocalDateTimeType:
		if !jmapc.LocalDateTime(text).Valid() {
			return nil, fmt.Errorf("%q is not a LocalDateTime\n\ta LocalDateTime is written as 2006-01-02T15:04:05, with no time zone", text)
		}
		return quote(text), nil

	case spec.DurationType:
		if !jmapc.Duration(text).Valid() {
			return nil, fmt.Errorf("%q is not a Duration\n\ta Duration is written as PT1H30M or P1D, with no years or months", text)
		}
		return quote(text), nil

	case spec.SignedDurationType:
		if !jmapc.SignedDuration(text).Valid() {
			return nil, fmt.Errorf("%q is not a SignedDuration\n\ta SignedDuration is a Duration, optionally prefixed with - or +", text)
		}
		return quote(text), nil

	case spec.Any:
		// Any accepts whatever the server accepts, so JSON is taken as JSON
		// and everything else as the string it looks like.
		if raw, err := compact(text); err == nil {
			return raw, nil
		}
		return quote(text), nil
	}

	// Anything else has a shape, and a shape is written as JSON.
	raw, err := compact(text)
	if err != nil {
		return nil, fmt.Errorf("%s is written as JSON, and this is not: %w", t, err)
	}
	return raw, nil
}

// compact returns text as JSON with its whitespace taken out, reporting text
// that is not JSON at all.
func compact(text string) (json.RawMessage, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(text)); err != nil {
		return nil, err
	}
	return json.RawMessage(buf.Bytes()), nil
}

// quote renders a string as the JSON string it becomes.
func quote(s string) json.RawMessage {
	raw, err := json.Marshal(s)
	if err != nil {
		return json.RawMessage(strconv.Quote(s))
	}
	return raw
}

// quoteBool renders a boolean as JSON.
func quoteBool(b bool) json.RawMessage {
	if b {
		return json.RawMessage("true")
	}
	return json.RawMessage("false")
}
