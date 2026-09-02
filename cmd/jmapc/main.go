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
	"sort"
	"strings"

	"github.com/linyows/jmapc/internal/gen"
	"github.com/linyows/jmapc/internal/query"
	"github.com/linyows/jmapc/internal/spec"
)

// ConfigName is the file jmapc reads its settings from when one is present.
const ConfigName = "jmapc.json"

// Config holds the settings for a run, whether they came from the config file
// or from the command line.
type Config struct {
	// Queries is the directory holding the query files.
	Queries string `json:"queries"`
	// Out is the directory the generated Go is written to.
	Out string `json:"out"`
	// Package is the name of the generated package, defaulting to the base
	// name of the output directory.
	Package string `json:"package"`
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

Flags:
	-config string    settings file to read (default ` + ConfigName + ` if present)
	-queries string   directory holding the query files (default "queries")
	-out string       directory to write the generated client to (default "jmapq")
	-package string   name of the generated package (default: the name of -out)

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
		pkg        = fs.String("package", "", "name of the generated package")
	)
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
	if *pkg != "" {
		cfg.Package = *pkg
	}
	cfg.applyDefaults()

	queryFiles, err := findQueries(cfg.Queries)
	if err != nil {
		return err
	}
	if len(queryFiles) == 0 {
		return fmt.Errorf("no %s files under %s", query.Extension, cfg.Queries)
	}

	parser := query.NewParser(spec.Standard())
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
	return write(cfg, parsed)
}

// write generates the client and puts it on disk.
func write(cfg *Config, queries []*query.Query) error {
	g := &gen.QueryGenerator{
		Spec:      spec.Standard(),
		Package:   cfg.Package,
		Qualifier: "jmapc.",
		Queries:   queries,
	}
	files, err := g.Generate()
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

// applyDefaults fills in the settings that were not given.
func (c *Config) applyDefaults() {
	if c.Queries == "" {
		c.Queries = "queries"
	}
	if c.Out == "" {
		c.Out = "jmapq"
	}
	if c.Package == "" {
		c.Package = filepath.Base(c.Out)
	}
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
