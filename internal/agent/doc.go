// Package agent defines the boundary between Kolchak's experiment engine and
// agent-specific transports.
//
// Adapters translate Request and Result values to and from their transport.
// Transport details belong in the concrete adapter and must not appear in this
// package's shared types. Context cancellation is part of the contract: an
// adapter stops its in-flight work and returns an error matching ctx.Err().
package agent
