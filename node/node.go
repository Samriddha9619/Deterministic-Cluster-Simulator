// Package node provides the Node interface and related types for building
// deterministic replicated nodes.
//
// All concrete types (NodeID, Envelope, Output, etc.) live in the /types
// package to avoid import cycles. This package re-exports them for
// convenience and contains node-specific helpers.
//
// This package must contain zero I/O, zero goroutines, and zero wall-clock
// references. It is imported by both /sim and /runtime/real.
package node
