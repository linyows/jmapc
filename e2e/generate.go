// Package e2e holds the tests that run jmapc against a real JMAP server,
// Stalwart, in a container. The requests they send are request files like any
// other, and the client in client/ is generated from them, so the tests
// exercise what jmapc generates as well as the runtime it generates against.
//
// The scenarios are one probe workflow, e2e/workflow.yml, which starts the
// server, sets it up and runs them. See docs/content/en/contributing.md.
package e2e

//go:generate go run ../cmd/jmapc generate -requests requests -out client
