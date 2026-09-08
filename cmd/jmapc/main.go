// Command jmapc generates a typed client from the JMAP requests in a directory,
// in Go, Rust or TypeScript. Write the request you want the server to answer;
// jmapc checks it against the JMAP data model and writes the code that sends
// it.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/linyows/jmapc/internal/gen"
	"github.com/linyows/jmapc/internal/gen/rust"
	"github.com/linyows/jmapc/internal/gen/shared"
	"github.com/linyows/jmapc/internal/gen/ts"
	"github.com/linyows/jmapc/internal/request"
	"github.com/linyows/jmapc/internal/spec"
)

// ConfigName is the file jmapc reads its settings from when one is present.
const ConfigName = "jmapc.json"

// stdout and stderr are where the commands write, named here so that a test
// can read what one wrote.
var (
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
)

// version is stamped into a release build. Built any other way — go install,
// go tool, go run — it stays empty, and the version the module system knows
// about is used instead.
var version string

// Config holds the settings for a run, whether they came from the config file
// or from the command line.
type Config struct {
	// Requests is the directory holding the request files.
	Requests string `json:"requests"`
	// Out is the directory the generated code is written to.
	Out string `json:"out"`
	// Lang is the language to generate: "go", "typescript" or "rust". It
	// defaults to Go.
	Lang string `json:"lang"`
	// Package is the name of the generated package, defaulting to the base
	// name of the output directory. It has no meaning for Rust or TypeScript,
	// where a module is a file.
	Package string `json:"package"`
	// Schemas are files describing the types and methods a server offers
	// beyond the specifications jmapc knows, which requests may then use.
	Schemas []string `json:"schemas"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "jmapc: %v\n", err)
		os.Exit(1)
	}
}

// usage describes the commands, and is printed under the banner whenever the
// arguments make no sense or help is asked for.
const usage = `jmapc generates a typed client from JMAP requests, in Go, Rust or TypeScript.

Usage:
	jmapc generate [flags]   check the requests and write the generated client
	jmapc check [flags]      check the requests without writing anything
	jmapc run <request>      send one request to a server and print the response
	jmapc schema [flags]     write a JSON Schema describing the request files
	jmapc version            print the version

Flags:
	-config string    settings file to read (default ` + ConfigName + ` if present)
	-requests string  directory holding the request files (default "requests")
	-out string       directory to write the generated client to (default "client")
	-lang string      language to generate: go, rust or typescript (default go)
	-package string   name of the generated package, for Go (default: the name of -out)
	-schema string    schema file describing a vendor extension; repeatable

The generate command also takes -check, which writes nothing and reports the
generated files that differ from what generating them now would produce. A
build step runs it to stop where a request was changed and the client was not
generated again.

The check command also takes -session, to check the requests against a server
rather than against the specifications alone, with -token or -user to
authenticate and -timeout to bound the wait.

The run and schema commands take flags of their own, which "jmapc run -h"
and "jmapc schema -h" describe.

A request file is named after the function to generate, as in
ListInboxEmails` + request.Extension + `, and holds the JMAP request that
function sends.
`

func run(args []string) error {
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("no command given")
	}
	command := args[0]
	switch command {
	case "generate", "check":
	case "run":
		return runRequest(args[1:])
	case "schema":
		return writeSchema(args[1:])
	case "version", "-version", "--version":
		fmt.Println(versionString())
		return nil
	case "-h", "-help", "--help", "help":
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", command)
	}

	fs := flag.NewFlagSet("jmapc "+command, flag.ContinueOnError)
	var (
		configPath = fs.String("config", "", "settings file to read")
		requests   = fs.String("requests", "", "directory holding the request files")
		out        = fs.String("out", "", "directory to write the generated client to")
		lang       = fs.String("lang", "", "language to generate: go, rust or typescript")
		pkg        = fs.String("package", "", "name of the generated package")
		schemas    stringList
	)
	fs.Var(&schemas, "schema", "schema file describing a vendor extension; repeatable")
	// Comparing what is on disk with what would be written is generation
	// without the writing, and it is offered where the writing is.
	var compare *bool
	if command == "generate" {
		compare = fs.Bool("check", false, "write nothing, and report the generated files that are out of date")
	}
	// Checking against a live server is the one thing generation has no use
	// for, and the flags for it are offered only where they mean something.
	var session, token, user *string
	var timeout *time.Duration
	if command == "check" {
		// The session URL is not read from the environment, unlike the
		// credentials: a check that reaches the network should say so on the
		// command line rather than because of what is set around it.
		session = fs.String("session", "", "session URL to check the requests against, or the host to find it under")
		token = fs.String("token", os.Getenv("JMAP_TOKEN"), "bearer token to authenticate with")
		user = fs.String("user", os.Getenv("JMAP_USER"), "user:password to authenticate with instead")
		timeout = fs.Duration("timeout", 30*time.Second, "how long to wait for the server")
	}
	fs.Usage = func() { printUsage(stderr) }
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	if *requests != "" {
		cfg.Requests = *requests
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

	queryFiles, err := findRequests(cfg.Requests)
	if err != nil {
		return err
	}
	if len(queryFiles) == 0 {
		return fmt.Errorf("no %s files under %s", request.Extension, cfg.Requests)
	}

	props, err := loadProperties(cfg.Requests, catalogue)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return errors.New("the sets of properties did not check out")
	}

	parser := request.NewParser(catalogue)
	parser.Properties = props
	parsed := make([]*request.Request, 0, len(queryFiles))
	var failures int
	for _, path := range queryFiles {
		q, err := parser.ParseFile(path)
		if err != nil {
			fmt.Fprintln(stderr, err)
			failures++
			continue
		}
		parsed = append(parsed, q)
	}
	if failures > 0 {
		return fmt.Errorf("%s", plural(failures, "request", "requests")+" did not check out")
	}
	noteSameRequests(parsed)

	if command == "check" {
		if *session != "" {
			return checkAgainstServer(catalogue, parsed, *session, *token, *user, *timeout)
		}
		fmt.Fprintf(stdout, "checked %s\n", checked(parsed, props))
		return nil
	}
	if *compare {
		return verify(cfg, catalogue, parsed, props)
	}
	return write(cfg, catalogue, parsed, props)
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

// loadProperties reads the sets of properties the requests may ask for by
// name. They sit beside the requests, in a file of their own: they are not a
// JMAP request, and a request asking for one is asking for something the
// project declared rather than something the specification did.
func loadProperties(dir string, catalogue *spec.Spec) (*request.PropertySets, error) {
	return request.LoadPropertySets(filepath.Join(dir, request.PropertiesName), catalogue)
}

// checked reports what a check looked at, counting the sets of properties
// alongside the requests where the project names any.
func checked(requests []*request.Request, props *request.PropertySets) string {
	out := plural(len(requests), "request", "requests")
	if n := len(props.Names()); n > 0 {
		out += " and " + plural(n, "set of properties", "sets of properties")
	}
	return out
}

// stringList collects a flag given more than once.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ", ") }

func (l *stringList) Set(v string) error {
	*l = append(*l, v)
	return nil
}

// write generates the client and puts it on disk.
func write(cfg *Config, catalogue *spec.Spec, requests []*request.Request, props *request.PropertySets) error {
	noteUnwatched(cfg, requests)
	files, err := generate(cfg, catalogue, requests, props)
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
		fmt.Fprintln(stdout, path)
	}
	return nil
}

// verify reports whether the client in the output directory is what generating
// it now would produce, and writes nothing. A build step runs it so that a
// request changed without the client being generated again stops the build,
// rather than reaching the repository as a client that sends the request as it
// used to be.
//
// It compares the whole of each file, so a client generated by another version
// of jmapc is out of date as much as one generated from another request.
func verify(cfg *Config, catalogue *spec.Spec, requests []*request.Request, props *request.PropertySets) error {
	noteUnwatched(cfg, requests)
	files, err := generate(cfg, catalogue, requests, props)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	var differences int
	for _, name := range names {
		path := filepath.Join(cfg.Out, name)
		have, err := os.ReadFile(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			fmt.Fprintf(stderr, "%s: not generated yet\n", path)
			differences++
		case err != nil:
			return err
		case !bytes.Equal(have, files[name]):
			fmt.Fprintf(stderr, "%s: out of date\n", path)
			differences++
		}
	}
	left, err := leftBehind(cfg.Out, files)
	if err != nil {
		return err
	}
	for _, name := range left {
		fmt.Fprintf(stderr, "%s: generated from a request that is no longer there\n", filepath.Join(cfg.Out, name))
		differences++
	}
	if differences > 0 {
		return fmt.Errorf("%s out of date; run jmapc generate", plural(differences, "file is", "files are"))
	}
	fmt.Fprintf(stdout, "%s up to date\n", plural(len(files), "file is", "files are"))
	return nil
}

// leftBehind returns the files in dir that jmapc wrote and would not write now,
// which is what deleting a request file leaves behind. A file counts as jmapc's
// where it starts with the banner every generated file carries; anything else
// in the directory was put there by hand and is not jmapc's to report on.
func leftBehind(dir string, files map[string][]byte) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var left []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if _, generated := files[entry.Name()]; generated {
			continue
		}
		written, err := writtenByJmapc(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if written {
			left = append(left, entry.Name())
		}
	}
	sort.Strings(left)
	return left, nil
}

// writtenByJmapc reports whether a file starts with the banner jmapc puts at
// the head of everything it generates. It reads that much of the file and no
// more.
func writtenByJmapc(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	head := make([]byte, len(shared.GeneratedBanner))
	if _, err := io.ReadFull(f, head); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			// A file shorter than the banner cannot carry it.
			return false, nil
		}
		return false, err
	}
	return string(head) == shared.GeneratedBanner, nil
}

// noteSameRequests says so where two request files hold one request. A request that
// differs from another only in what it calls its parameters and its calls
// makes the same request, so one of them would do for both, and the second
// brings a second set of generated types along with it.
//
// It is a note rather than an error: a project may want two names for one
// request, and jmapc is not the one to say it may not.
func noteSameRequests(requests []*request.Request) {
	byShape := map[string][]string{}
	var order []string
	for _, q := range requests {
		if q.Shape == "" {
			continue
		}
		if _, seen := byShape[q.Shape]; !seen {
			order = append(order, q.Shape)
		}
		byShape[q.Shape] = append(byShape[q.Shape], q.Name)
	}
	for _, shape := range order {
		names := byShape[shape]
		if len(names) < 2 {
			continue
		}
		sort.Strings(names)
		fmt.Fprintf(stderr, "jmapc: %s are the same request under different names; one of them would do for all of them\n",
			strings.Join(names, ", "))
	}
}

// noteUnwatched says so where a request asks to be watched in a language that
// cannot follow it. Following a type's changes means holding a connection to
// the server's push endpoint open, which only the Go runtime does; the request
// itself is generated all the same, so the note is a note rather than an
// error.
func noteUnwatched(cfg *Config, requests []*request.Request) {
	if cfg.Lang == LangGo {
		return
	}
	for _, q := range requests {
		if q.Watches == nil {
			continue
		}
		fmt.Fprintf(stderr, "jmapc: %s asks to be watched, which only the Go client does; the %s client has the request without the loop\n",
			q.Name, cfg.Lang)
	}
}

// generate produces the files for the configured language. TypeScript and Rust
// take the runtime with them: there is no package to depend on, so the client
// and the data types are written alongside the requests.
func generate(cfg *Config, catalogue *spec.Spec, requests []*request.Request, props *request.PropertySets) (map[string][]byte, error) {
	switch cfg.Lang {
	case LangRust:
		return generateRust(catalogue, requests, props)
	case LangTypeScript:
		files, err := (&ts.RequestGenerator{Spec: catalogue, Requests: requests, Properties: props}).Generate()
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
	return (&gen.RequestGenerator{
		Spec:       catalogue,
		Package:    cfg.Package,
		Qualifier:  "jmapc.",
		Requests:   requests,
		Properties: props,
	}).Generate()
}

// generateRust produces the Rust module directory: one file per request, the data
// model, the runtime, and the mod.rs that declares them all.
func generateRust(catalogue *spec.Spec, requests []*request.Request, props *request.PropertySets) (map[string][]byte, error) {
	files, err := (&rust.RequestGenerator{Spec: catalogue, Requests: requests, Properties: props}).Generate()
	if err != nil {
		return nil, err
	}
	types, err := (&rust.TypeGenerator{
		Spec: catalogue,
		// PatchObject is written by hand in the runtime, being a shape rather
		// than a record.
		Skip: map[string]bool{"PatchObject": true},
	}).Generate()
	if err != nil {
		return nil, err
	}
	client, err := (&rust.ClientGenerator{}).Generate()
	if err != nil {
		return nil, err
	}
	files["types.rs"] = types
	files["client.rs"] = client
	return files, nil
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
	LangRust       = "rust"
)

// applyDefaults fills in the settings that were not given.
func (c *Config) applyDefaults() {
	if c.Requests == "" {
		c.Requests = "requests"
	}
	if c.Out == "" {
		c.Out = "client"
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
	case LangGo, LangTypeScript, LangRust:
		return nil
	}
	return fmt.Errorf("cannot generate %q; the languages are %s, %s and %s",
		c.Lang, LangGo, LangTypeScript, LangRust)
}

// findRequests returns the request files under dir, in a stable order.
func findRequests(dir string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), request.Extension) {
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
