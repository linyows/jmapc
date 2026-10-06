// Package syntax holds the forms of the JMAP and JSCalendar types that are
// strings with a shape of their own. The runtime checks values against them,
// the request checker checks what a request file writes, and the editor schema
// states them for an editor, so each is written once and the three agree.
//
// Each pattern is written in the syntax both Go's regexp and a JSON Schema
// validator read.
package syntax

import (
	"regexp"
	"time"
)

// Duration is the JSCalendar Duration of RFC 8984, Section 1.4.6: weeks, or
// days with an optional time, with no years or months. Each part names at
// least one amount, so "P", "PT" and "P1DT" denote nothing and do not match.
const Duration = `^` + duration + `$`

// SignedDuration is a Duration with at most one sign before it, RFC 8984,
// Section 1.4.7.
const SignedDuration = `^[-+]?` + duration + `$`

// duration is a Duration without the anchors, which the two patterns share.
const duration = `P(?:\d+W|\d+D(?:` + durationTime + `)?|` + durationTime + `)`

// durationTime is the time part of a duration: a T and at least one of hours,
// minutes and seconds, in that order.
const durationTime = `T(?:\d+H(?:\d+M)?(?:\d+(?:\.\d+)?S)?|\d+M(?:\d+(?:\.\d+)?S)?|\d+(?:\.\d+)?S)`

// LocalDateTime is the JSCalendar LocalDateTime of RFC 8984, Section 1.4.4: a
// date and a time with no time zone, the seconds optionally with a fraction.
const LocalDateTime = `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?$`

// UTCDate is the UTCDate of RFC 8620, Section 1.4: a Date whose offset is Z.
// The seconds may carry a fraction, which the specification has a server
// leave out where it is zero.
const UTCDate = `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z$`

// Date is the Date of RFC 8620, Section 1.4: an RFC 3339 date-time.
const Date = `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[-+]\d{2}:\d{2})$`

var (
	utcDate = regexp.MustCompile(UTCDate)
	date    = regexp.MustCompile(Date)
)

// ValidUTCDate reports whether s is a UTCDate: one of the form UTCDate states,
// and a time that exists. The form is checked as well as the time, since Go's
// parser takes a comma before the fraction of a second, which the form, and so
// the editor schema, does not.
func ValidUTCDate(s string) bool {
	if !utcDate.MatchString(s) {
		return false
	}
	_, err := time.Parse(time.RFC3339, s)
	return err == nil
}

// ValidDate reports whether s is a Date, as ValidUTCDate does for a UTCDate.
func ValidDate(s string) bool {
	if !date.MatchString(s) {
		return false
	}
	_, err := time.Parse(time.RFC3339, s)
	return err == nil
}
