// Package example holds a worked example: a few JMAP queries and the client
// jmapc generates from them.
package example

//go:generate go run ../cmd/jmapc generate -queries queries -out jmapq -package jmapq
//go:generate go run ../cmd/jmapc generate -queries queries -out ts -lang typescript
