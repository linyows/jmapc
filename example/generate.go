// Package example holds a worked example: a few JMAP requests and the client
// jmapc generates from them.
package example

//go:generate go run ../cmd/jmapc generate -requests requests -out client
//go:generate go run ../cmd/jmapc generate -requests requests -out ts -lang typescript
//go:generate go run ../cmd/jmapc generate -requests requests -out rust/src/jmap_client -lang rust
