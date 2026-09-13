// This module exists only to keep the docs site out of the root module's
// package tree. Without it, `go list ./...` walks into
// pages/node_modules/flatted/golang -- a third-party Go package that npm
// installs as a transitive dependency of the docs site -- and every tool that
// expands ./... picks it up: go test, go vet, go build, govulncheck, and the
// coverage gate, whose total it drags below the threshold with its 0%.
//
// A module boundary fixes this once for all of them, which per-command
// `grep -v /node_modules/` filters cannot: those have to be repeated at every
// call site and silently stop covering the ones added later.
//
// There is no Go code of ours under pages/. If that ever changes, this file
// becomes a real module and should be treated as one.
module github.com/alibaba/open-code-review/pages

go 1.25.5
