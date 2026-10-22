// Package siding is the public Go API for embedding siding in other CLIs
// (for example AEOS). It resolves checkout slots, manages machine state.json,
// runs docker compose on the host, and controls the shared loopback proxy.
//
// The human-facing cobra CLI lives in cmd/siding and calls this package.
// Import github.com/TheBlackhowling/siding/siding — not internal/*.
package siding
