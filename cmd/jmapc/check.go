package main

import (
	"context"
	"fmt"
	"time"

	"github.com/linyows/jmapc/internal/limits"
	"github.com/linyows/jmapc/internal/query"
	"github.com/linyows/jmapc/internal/spec"
)

// checkAgainstServer checks the queries against a running server as well as
// against the specifications.
//
// The two say different things. A specification says what JMAP is; a session
// says what this server does, how much of it it will do at once, and which
// accounts it holds. A query can be right about the first and wrong about the
// second, and that is a failure at run time unless something asks.
func checkAgainstServer(catalogue *spec.Spec, queries []*query.Query, session, token, user string, timeout time.Duration) error {
	client, err := newClient(session, token, user)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	s, err := client.Session(ctx)
	if err != nil {
		return err
	}

	var failures int
	for _, q := range queries {
		if err := limits.Check(catalogue, s, q); err != nil {
			fmt.Fprintln(stderr, err)
			failures++
		}
	}
	if failures > 0 {
		return fmt.Errorf("%s the server would not accept", plural(failures, "query", "queries"))
	}
	fmt.Fprintf(stdout, "checked %s against %s, as %s\n",
		plural(len(queries), "query", "queries"), s.APIURL, s.Username)
	return nil
}
