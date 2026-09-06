package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/linyows/jmapc"
	"github.com/linyows/jmapc/internal/request"
	"github.com/linyows/jmapc/internal/spec"
	"github.com/linyows/jmapc/internal/wire"
)

// runUsage describes the run command on its own, since it takes flags no other
// command does.
const runUsage = `jmapc run sends one request to a server and prints what comes back.

Usage:
	jmapc run <request> [flags]

Flags:
	-config string     settings file to read (default ` + ConfigName + ` if present)
	-requests string    directory holding the request files (default "requests")
	-schema string     schema file describing a vendor extension; repeatable
	-p name=value      value for a parameter the request leaves open; repeatable
	-session string    session URL, or the host to find it under (default $JMAP_SESSION_URL)
	-token string      bearer token to authenticate with (default $JMAP_TOKEN)
	-user user:pass    credentials to authenticate with instead (default $JMAP_USER)
	-account string    account id to use where the request leaves accountId out
	-created-id id=id  creation id carried in from an earlier request; repeatable
	-timeout duration  how long to wait for the server (default 30s)
	-dry-run           print the request and send nothing

A value is written as the type says: a String or an Id is the text itself, so
nothing has to be quoted, and anything with a shape is written as JSON.
`

// accountPlaceholder stands in for the account id a dry run has no session to
// look up. It is a valid id, so the request it appears in is one a server would
// accept, with that one value replaced.
const accountPlaceholder = "ACCOUNT_ID"

// runRequest sends one request to a server, or prints the request it would send.
// It is the shortest path from writing a request to seeing it answered: no code
// is generated, and nothing is written to disk.
func runRequest(args []string) error {
	fs := flag.NewFlagSet("jmapc run", flag.ContinueOnError)
	var (
		configPath = fs.String("config", "", "settings file to read")
		requests   = fs.String("requests", "", "directory holding the request files")
		session    = fs.String("session", os.Getenv("JMAP_SESSION_URL"), "session URL, or the host to find it under")
		token      = fs.String("token", os.Getenv("JMAP_TOKEN"), "bearer token to authenticate with")
		user       = fs.String("user", os.Getenv("JMAP_USER"), "user:password to authenticate with instead")
		account    = fs.String("account", "", "account id to use where the request leaves accountId out")
		timeout    = fs.Duration("timeout", 30*time.Second, "how long to wait for the server")
		dryRun     = fs.Bool("dry-run", false, "print the request and send nothing")
		schemas    stringList
		params     stringList
		createdIDs stringList
	)
	fs.Var(&schemas, "schema", "schema file describing a vendor extension; repeatable")
	fs.Var(&params, "p", "value for a parameter the request leaves open; repeatable")
	fs.Var(&createdIDs, "created-id", "creation id carried in from an earlier request; repeatable")
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, runUsage) }

	name, rest := queryName(args)
	if err := fs.Parse(rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if name == "" {
		name = fs.Arg(0)
	}
	if name == "" {
		fmt.Fprint(stderr, runUsage)
		return errors.New("no request named")
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	if *requests != "" {
		cfg.Requests = *requests
	}
	if len(schemas) > 0 {
		cfg.Schemas = append(cfg.Schemas, schemas...)
	}
	cfg.applyDefaults()

	catalogue, err := loadCatalogue(cfg.Schemas)
	if err != nil {
		return err
	}
	q, err := findRequest(catalogue, cfg.Requests, name)
	if err != nil {
		return err
	}

	values, err := paramValues(q, params)
	if err != nil {
		return err
	}
	if err := wire.CheckValues(q, values); err != nil {
		return err
	}
	carried, err := carriedIDs(createdIDs)
	if err != nil {
		return err
	}

	if *dryRun {
		return printRequest(catalogue, q, values, *account, carried)
	}

	client, err := newClient(*session, *token, *user)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	req, err := wire.Build(catalogue, q, values, accountsFrom(ctx, client, *account), carried)
	if err != nil {
		return err
	}
	resp, sendErr := client.Do(ctx, req)
	if resp != nil {
		if err := printJSON(resp); err != nil {
			return err
		}
	}
	if sendErr != nil {
		return sendErr
	}
	return refusals(catalogue, q, resp)
}

// queryName peels the request name off the front of the arguments, where it
// usually is. A name written after the flags is left to the flag package.
func queryName(args []string) (string, []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}

// findRequest parses the one request the run is about, rather than every request in
// the directory: a mistake in a request nobody asked for should not stop this one
// from being sent.
func findRequest(catalogue *spec.Spec, dir, name string) (*request.Request, error) {
	paths, err := findRequests(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, path := range paths {
		if request.RequestName(path) == name {
			return request.NewParser(catalogue).ParseFile(path)
		}
		names = append(names, request.RequestName(path))
	}
	return nil, fmt.Errorf("no request named %s under %s%s", name, dir, didYouMean(name, names))
}

// didYouMean suggests the request the caller probably meant, going by the case
// they wrote it in, which is the mistake a shell makes easy.
func didYouMean(name string, names []string) string {
	for _, candidate := range names {
		if strings.EqualFold(candidate, name) {
			return fmt.Sprintf("\n\tdid you mean %s?", candidate)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return "\n\tthe requests there are " + strings.Join(names, ", ")
}

// paramValues reads the -p flags, checking each value against the type of the
// parameter it stands in for.
func paramValues(q *request.Request, params []string) (map[string]wire.Value, error) {
	byName := make(map[string]*request.Param, len(q.Params))
	for _, p := range q.Params {
		byName[p.Name] = p
	}
	values := make(map[string]wire.Value, len(params))
	for _, pair := range params {
		name, text, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("-p %s: a parameter is given as name=value", pair)
		}
		p, ok := byName[name]
		if !ok {
			// Build reports this, along with everything else the caller got
			// wrong, once every flag has been read.
			values[name] = wire.Value{Text: text, JSON: json.RawMessage(`null`)}
			continue
		}
		value, err := wire.ParseValue(p, text)
		if err != nil {
			return nil, err
		}
		values[name] = value
	}
	return values, nil
}

// carriedIDs reads the -created-id flags, which carry the creation ids of an
// earlier request into this one.
func carriedIDs(pairs []string) (map[jmapc.ID]jmapc.ID, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	ids := make(map[jmapc.ID]jmapc.ID, len(pairs))
	for _, pair := range pairs {
		creation, id, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("-created-id %s: a creation id is given as name=id", pair)
		}
		ids[jmapc.ID(creation)] = jmapc.ID(id)
	}
	return ids, nil
}

// printRequest writes the request the request stands for and sends nothing. The
// account id a request leaves out is looked up in the session at run time, and a
// dry run has no session, so it reports that rather than omitting the id
// silently.
func printRequest(catalogue *spec.Spec, q *request.Request, values map[string]wire.Value, account string, carried map[jmapc.ID]jmapc.ID) error {
	var stood []string
	accounts := func(capability string) (jmapc.ID, error) {
		if account != "" {
			return jmapc.ID(account), nil
		}
		stood = append(stood, capability)
		return accountPlaceholder, nil
	}
	req, err := wire.Build(catalogue, q, values, accounts, carried)
	if err != nil {
		return err
	}
	if err := printJSON(req); err != nil {
		return err
	}
	if len(stood) > 0 {
		sort.Strings(stood)
		fmt.Fprintf(stderr, "note: %s stands in for the primary account of the session, for %s\n",
			accountPlaceholder, strings.Join(stood, " and "))
	}
	return nil
}

// newClient builds the client the run sends through.
func newClient(session, token, user string) (*jmapc.Client, error) {
	if session == "" {
		return nil, errors.New("no server to send to\n\tpass -session, or set JMAP_SESSION_URL")
	}
	if !strings.Contains(session, "://") {
		session = jmapc.WellKnownURL(session)
	}
	var opts []jmapc.Option
	switch {
	case token != "":
		opts = append(opts, jmapc.WithBearerToken(token))
	case user != "":
		name, password, ok := strings.Cut(user, ":")
		if !ok {
			return nil, fmt.Errorf("-user %s: credentials are given as user:password", user)
		}
		opts = append(opts, jmapc.WithBasicAuth(name, password))
	}
	return jmapc.New(session, opts...), nil
}

// accountsFrom resolves the account ids a request leaves out, from the account
// the caller named or from the session's primary account.
func accountsFrom(ctx context.Context, client *jmapc.Client, account string) wire.Accounts {
	return func(capability string) (jmapc.ID, error) {
		if account != "" {
			return jmapc.ID(account), nil
		}
		return client.PrimaryAccountID(ctx, capability)
	}
}

// refusals reports the records the server would not act on. A /set answers 200
// and lists them, so a run that read only the transport error would report
// success where nothing happened.
func refusals(catalogue *spec.Spec, q *request.Request, resp *jmapc.Response) error {
	var failures jmapc.SetErrors
	for _, c := range q.Calls {
		fields := catalogue.SetErrorFields(c.Method.Name)
		if len(fields) == 0 {
			continue
		}
		var args map[string]json.RawMessage
		if err := resp.Decode(c.ID, &args); err != nil {
			return err
		}
		groups := make(map[string]map[jmapc.ID]jmapc.SetError, len(fields))
		for _, field := range fields {
			raw, ok := args[field]
			if !ok {
				continue
			}
			var group map[jmapc.ID]jmapc.SetError
			if err := json.Unmarshal(raw, &group); err != nil {
				return fmt.Errorf("jmapc: decoding %s of call %q: %w", field, c.ID, err)
			}
			groups[field] = group
		}
		failures.Collect(c.Method.Name, c.ID, groups)
	}
	return failures.Err()
}

// printJSON writes a value as indented JSON, which is what a person reading a
// response at a terminal wants.
func printJSON(v any) error {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%s\n", out)
	return err
}
