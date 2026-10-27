// Package sim implements the deterministic simulation environment (World A).
//
// All simulated code runs on a single goroutine — no concurrency, no wall clock,
// no real I/O. Determinism is enforced by architecture, not by runtime hacks.
package sim

import (
	"fmt"

	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

// ---------------------------------------------------------------------------
// VirtualClock — the simulation's clock implementation
// ---------------------------------------------------------------------------

// VirtualClock is the simulation's clock. It holds a single Time value that
// the event loop sets before dispatching each event. Because the event loop
// is single-threaded, there is no race: the clock is simply assigned, then
// read by the node handler, then assigned again for the next event.
//
// Zero value is usable (starts at time 0).
type VirtualClock struct {
	now t.Time
}

// Now returns the current virtual time. This is what node code calls.
func (c *VirtualClock) Now() t.Time {
	return c.now
}

// Set advances the clock to the given time. Only the event loop calls this.
// The time must be >= the current time (virtual time never goes backwards).
// Panics on regression to catch event-ordering bugs immediately.
func (c *VirtualClock) Set(at t.Time) {
	if at < c.now {
		panic(fmt.Sprintf("sim: virtual clock regression: current=%s, attempted=%s", c.now, at))
	}
	c.now = at
}

// Compile-time check that VirtualClock implements Clock.
var _ t.Clock = (*VirtualClock)(nil)
