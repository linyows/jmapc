// Command jmapc generates a typed Go client from the JMAP queries in a
// directory. Write the query you want the server to answer; jmapc checks it
// against the JMAP data model and writes the Go that sends it.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/linyows/jmapc/internal/gen"
	"github.com/linyows/jmapc/internal/gen/ts"
	"github.com/linyows/jmapc/internal/query"
	"github.com/linyows/jmapc/internal/spec"
)

// ConfigName is the file jmapc reads its settings from when one is present.
const ConfigName = "jmapc.json"

// version is stamped into a release build. Built any other way — go install,
// go tool, go run — it stays empty, and the version the module system knows
// about is used instead.
var version string

// Config holds the settings for a run, whether they came from the config file
// or from the command line.
type Config struct {
	// Queries is the directory holding the query files.
	Queries string `json:"queries"`
	// Out is the directory the generated Go is written to.
	Out string `json:"out"`
	// Lang is the language to generate, "go" or "typescript". It defaults to
	// Go.
	Lang string `json:"lang"`
	// Package is the name of the generated package, defaulting to the base
	// name of the output directory. It has no meaning for TypeScript, where a
	// module is a file.
	Package string `json:"package"`
	// Schemas are files describing the types and methods a server offers
	// beyond the specifications jmapc knows, which queries may then use.
	Schemas []string `json:"schemas"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "jmapc: %v\n", err)
		os.Exit(1)
	}
}

// usage describes the commands, and is printed when the arguments make no
// sense.
const usage = `jmapc generates a typed Go client from JMAP queries.

Usage:
	jmapc generate [flags]   check the queries and write the generated client
	jmapc check [flags]      check the queries without writing anything
	jmapc version            print the version

Flags:
	-config string    settings file to read (default ` + ConfigName + ` if present)
	-queries string   directory holding the query files (default "queries")
	-out string       directory to write the generated client to (default "jmapq")
	-lang string      language to generate: go or typescript (default go)
	-package string   name of the generated package, for Go (default: the name of -out)
	-schema string    schema file describing a vendor extension; repeatable

A query file is named after the function to generate, as in
ListInboxEmails` + query.Extension + `, and holds a JMAP request.
`

func run(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return errors.New("no command given")
	}
	command := args[0]
	switch command {
	case "generate", "check":
	case "version", "-version", "--version":
		fmt.Println(versionString())
		return nil
	case "-h", "-help", "--help", "help":
		fmt.Print(usage)
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", command)
	}

	fs := flag.NewFlagSet("jmapc "+command, flag.ContinueOnError)
	var (
		configPath = fs.String("config", "", "settings file to read")
		queries    = fs.String("queries", "", "directory holding the query files")
		out        = fs.String("out", "", "directory to write the generated client to")
		lang       = fs.String("lang", "", "language to generate: go or typescript")
		pkg        = fs.String("package", "", "name of the generated package")
		schemas    stringList
	)
	fs.Var(&schemas, "schema", "schema file describing a vendor extension; repeatable")
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	if *queries != "" {
		cfg.Queries = *queries
	}
	if *out != "" {
		cfg.Out = *out
	}
	if *lang != "" {
		cfg.Lang = *lang
	}
	if *pkg != "" {
		cfg.Package = *pkg
	}
	if len(schemas) > 0 {
		cfg.Schemas = append(cfg.Schemas, schemas...)
	}
	cfg.applyDefaults()
	if err := cfg.check(); err != nil {
		return err
	}

	catalogue, err := loadCatalogue(cfg.Schemas)
	if err != nil {
		return err
	}

	queryFiles, err := findQueries(cfg.Queries)
	if err != nil {
		return err
	}
	if len(queryFiles) == 0 {
		return fmt.Errorf("no %s files under %s", query.Extension, cfg.Queries)
	}

	parser := query.NewParser(catalogue)
	parsed := make([]*query.Query, 0, len(queryFiles))
	var failures int
	for _, path := range queryFiles {
		q, err := parser.ParseFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			failures++
			continue
		}
		parsed = append(parsed, q)
	}
	if failures > 0 {
		return fmt.Errorf("%s", plural(failures, "query", "queries")+" did not check out")
	}

	if command == "check" {
		fmt.Printf("checked %s\n", plural(len(parsed), "query", "queries"))
		return nil
	}
	return write(cfg, catalogue, parsed)
}

// versionString returns the version to report. A release build has it stamped
// in; anything else asks the module system, which knows it for a binary that
// came from a module version and says "(devel)" for one built from a checkout.
func versionString() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

// loadCatalogue returns the JMAP data model, extended with any vendor schemas
// the configuration names.
func loadCatalogue(schemas []string) (*spec.Spec, error) {
	catalogue := spec.Standard()
	for _, path := range schemas {
		sc, err := spec.LoadSchema(path)
		if err != nil {
			return nil, err
		}
		if err := catalogue.Extend(sc); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	return catalogue, nil
}

// stringList collects a flag given more than once.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ", ") }

func (l *stringList) Set(v string) error {
	*l = append(*l, v)
	return nil
}

// write generates the client and puts it on disk.
func write(cfg *Config, catalogue *spec.Spec, queries []*query.Query) error {
	files, err := generate(cfg, catalogue, queries)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.Out, 0o755); err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(cfg.Out, name)
		if err := os.WriteFile(path, files[name], 0o644); err != nil {
			return err
		}
		fmt.Println(path)
	}
	return nil
}

// generate produces the files for the configured language. TypeScript takes
// the runtime with it: there is no package to depend on, so the client and the
// data types are written alongside the queries.
func generate(cfg *Config, catalogue *spec.Spec, queries []*query.Query) (map[string][]byte, error) {
	if cfg.Lang == LangTypeScript {
		files, err := (&ts.QueryGenerator{Spec: catalogue, Queries: queries}).Generate()
		if err != nil {
			return nil, err
		}
		types, err := (&ts.TypeGenerator{
			Spec: catalogue,
			// PatchObject is written by hand in the runtime, being a shape
			// rather than a record.
			Skip: map[string]bool{"PatchObject": true},
		}).Generate()
		if err != nil {
			return nil, err
		}
		client, err := (&ts.ClientGenerator{}).Generate()
		if err != nil {
			return nil, err
		}
		files["types.ts"] = types
		files["client.ts"] = client
		return files, nil
	}
	return (&gen.QueryGenerator{
		Spec:      catalogue,
		Package:   cfg.Package,
		Qualifier: "jmapc.",
		Queries:   queries,
	}).Generate()
}

// loadConfig reads the settings file, treating a missing default file as an
// empty configuration rather than an error.
func loadConfig(path string) (*Config, error) {
	explicit := path != ""
	if !explicit {
		path = ConfigName
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !explicit && os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	var cfg Config
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return &cfg, nil
}

// Languages jmapc can generate.
const (
	LangGo         = "go"
	LangTypeScript = "typescript"
)

// applyDefaults fills in the settings that were not given.
func (c *Config) applyDefaults() {
	if c.Queries == "" {
		c.Queries = "queries"
	}
	if c.Out == "" {
		c.Out = "jmapq"
	}
	if c.Lang == "" {
		c.Lang = LangGo
	}
	if c.Package == "" {
		c.Package = filepath.Base(c.Out)
	}
}

// check reports a setting that cannot be acted on.
func (c *Config) check() error {
	switch c.Lang {
	case LangGo, LangTypeScript:
		return nil
	}
	return fmt.Errorf("cannot generate %q; the languages are %s and %s", c.Lang, LangGo, LangTypeScript)
}

// findQueries returns the query files under dir, in a stable order.
func findQueries(dir string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), query.Extension) {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

// plural renders a count with the right form of its noun.
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
