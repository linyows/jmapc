package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// emailSets names one shape of Email, and one that adds to it.
const emailSets = `{
  "EmailSummary": {
    "doc": "EmailSummary is what a list of messages shows.",
    "type": "Email",
    "properties": ["id", "threadId", "subject", "receivedAt"]
  },
  "EmailWithFlags": {
    "extends": "EmailSummary",
    "properties": ["keywords"]
  }
}`

// listEmails and readEmails ask for the same set, which is what makes the two
// requests answer with one type.
const listEmails = `{
  "methodCalls": [["Email/get", {"ids": "{{ids}}", "properties": "@EmailSummary"}, "get"]],
  "_returns": "get"
}`

const readEmails = `{
  "methodCalls": [["Email/get", {"ids": "{{ids}}", "properties": "@EmailWithFlags"}, "get"]],
  "_returns": "get"
}`

func TestGenerateNamedPropertySets(t *testing.T) {
	dir := workspace(t, map[string]string{
		"requests/properties.json":         emailSets,
		"requests/ListEmails.jmap.json":    listEmails,
		"requests/ReadEmails.jmap.json":    readEmails,
		"requests/ListMailboxes.jmap.json": listMailboxes,
	})
	out := filepath.Join(dir, "client")

	if err := run([]string{"generate", "-requests", filepath.Join(dir, "requests"), "-out", out}); err != nil {
		t.Fatalf("generate: %v", err)
	}

	sets, err := os.ReadFile(filepath.Join(out, "properties_gen.go"))
	if err != nil {
		t.Fatalf("reading the generated sets: %v", err)
	}
	for _, want := range []string{
		"type EmailSummary struct {",
		"type EmailWithFlags struct {",
		// The set that extends another embeds it, so nothing is copied
		// between the two.
		"\tEmailSummary\n",
	} {
		if !strings.Contains(string(sets), want) {
			t.Errorf("the generated sets do not contain %q:\n%s", want, sets)
		}
	}

	// Both requests answer with the type the set names, and neither declares a
	// record type of its own.
	for name, want := range map[string]string{
		"listemails_gen.go": "List []EmailSummary",
		"reademails_gen.go": "List []EmailWithFlags",
	} {
		src, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		if !strings.Contains(string(src), want) {
			t.Errorf("%s does not contain %q:\n%s", name, want, src)
		}
		if strings.Contains(string(src), "GetEmail struct") {
			t.Errorf("%s declares a record type of its own:\n%s", name, src)
		}
	}

	// A request that asks for no set is generated as it was before.
	if _, err := os.Stat(filepath.Join(out, "listmailboxes_gen.go")); err != nil {
		t.Errorf("the request asking for no set was not generated: %v", err)
	}
}

func TestGenerateNamedPropertySetsInEveryLanguage(t *testing.T) {
	tests := []struct {
		lang string
		file string
		want []string
	}{{
		lang: "typescript",
		file: "properties.ts",
		want: []string{"export interface EmailSummary {", "export interface EmailWithFlags extends EmailSummary {"},
	}, {
		lang: "rust",
		file: "properties.rs",
		want: []string{"pub struct EmailSummary {", "#[serde(flatten)]", "pub email_summary: EmailSummary,"},
	}}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			dir := workspace(t, map[string]string{
				"requests/properties.json":      emailSets,
				"requests/ListEmails.jmap.json": listEmails,
				"requests/ReadEmails.jmap.json": readEmails,
			})
			out := filepath.Join(dir, "client")
			args := []string{"generate", "-requests", filepath.Join(dir, "requests"), "-out", out, "-lang", tt.lang}
			if err := run(args); err != nil {
				t.Fatalf("generate: %v", err)
			}
			src, err := os.ReadFile(filepath.Join(out, tt.file))
			if err != nil {
				t.Fatalf("reading %s: %v", tt.file, err)
			}
			for _, want := range tt.want {
				if !strings.Contains(string(src), want) {
					t.Errorf("%s does not contain %q:\n%s", tt.file, want, src)
				}
			}
		})
	}
}

func TestCheckReportsTheSetsItRead(t *testing.T) {
	dir := workspace(t, map[string]string{
		"requests/properties.json":      emailSets,
		"requests/ListEmails.jmap.json": listEmails,
	})
	out, _, err := capture(t, []string{"check", "-requests", filepath.Join(dir, "requests")})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(out, "checked 1 request and 2 sets of properties") {
		t.Errorf("check said %q, want it to count the sets as well", out)
	}
}

func TestCheckReportsAProblemInTheSets(t *testing.T) {
	dir := workspace(t, map[string]string{
		"requests/properties.json":      `{"EmailSummary": {"type": "Email", "properties": ["id", "subjcet"]}}`,
		"requests/ListEmails.jmap.json": listEmails,
	})
	_, errOut, err := capture(t, []string{"check", "-requests", filepath.Join(dir, "requests")})
	if err == nil {
		t.Fatal("expected the check to fail, it did not")
	}
	if !strings.Contains(errOut, `Email has no property "subjcet"`) {
		t.Errorf("the check said %q, want it to point at the property", errOut)
	}
	if !strings.Contains(errOut, "properties.json") {
		t.Errorf("the check said %q, want it to name the file", errOut)
	}
}

func TestGenerateRefusesASetNamedAfterARequest(t *testing.T) {
	dir := workspace(t, map[string]string{
		"requests/properties.json":        emailSets,
		"requests/EmailSummary.jmap.json": listEmails,
	})
	_, _, err := capture(t, []string{"generate", "-requests", filepath.Join(dir, "requests"), "-out", filepath.Join(dir, "client")})
	if err == nil {
		t.Fatal("expected generating to fail, it did not")
	}
	if !strings.Contains(err.Error(), "EmailSummary") {
		t.Errorf("generating failed with %q, want it to name what clashes", err)
	}
}
