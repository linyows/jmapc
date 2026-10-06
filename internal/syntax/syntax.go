// Package syntax holds the forms of the JMAP and JSCalendar types that are
// strings with a shape of their own. The runtime checks values against them,
// the request checker checks what a request file writes, and the editor schema
// states them for an editor, so each is written once and the three agree.
//
// Each pattern is written in the syntax both Go's regexp and a JSON Schema
// validator read.
package syntax

// Duration is the JSCalendar Duration of RFC 8984, Section 1.4.6: weeks, or
// days with an optional time, with no years or months. Each part names at
// least one amount, so "P", "PT" and "P1DT" denote nothing and do not match.
const Duration = `^P(?:\d+W|\d+D(?:` + durationTime + `)?|` + durationTime + `)$`

// SignedDuration is a Duration with at most one sign before it, RFC 8984,
// Section 1.4.7.
const SignedDuration = `^[-+]?` + `P(?:\d+W|\d+D(?:` + durationTime + `)?|` + durationTime + `)$`

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
