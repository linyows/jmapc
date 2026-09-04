package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/linyows/jmapc/internal/jsonschema"
)

// schemaUsage describes the schema command, which writes rather than reads a
// schema, and so takes different flags from the rest.
const schemaUsage = `jmapc schema writes a JSON Schema describing the query files, for an editor
to check and complete them against.

Usage:
	jmapc schema [flags]

Flags:
	-config string   settings file to read (default ` + ConfigName + ` if present)
	-schema string   schema file describing a vendor extension, to describe too; repeatable
	-out string      file to write to (default: standard output)

Point an editor at what it writes, either from the query file itself:

	{"$schema": "../jmapc.schema.json", "methodCalls": [...]}

or from the editor's own settings, matching every file at once.
`

// writeSchema writes the JSON Schema for the query files, which is what lets an
// editor complete a method name and report a misspelled argument where it was
// written rather than at the next build.
func writeSchema(args []string) error {
	fs := flag.NewFlagSet("jmapc schema", flag.ContinueOnError)
	var (
		configPath = fs.String("config", "", "settings file to read")
		out        = fs.String("out", "", "file to write to")
		schemas    stringList
	)
	fs.Var(&schemas, "schema", "schema file describing a vendor extension; repeatable")
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, schemaUsage) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	if len(schemas) > 0 {
		cfg.Schemas = append(cfg.Schemas, schemas...)
	}
	catalogue, err := loadCatalogue(cfg.Schemas)
	if err != nil {
		return err
	}
	doc, err := (&jsonschema.Generator{Spec: catalogue}).Generate()
	if err != nil {
		return err
	}
	if *out == "" {
		_, err := stdout.Write(doc)
		return err
	}
	if err := os.WriteFile(*out, doc, 0o644); err != nil {
		return err
	}
	fmt.Fprintln(stdout, *out)
	return nil
}
