package sim

import (
	"container/heap"
	"fmt"

	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

// ---------------------------------------------------------------------------
// EventKind — what type of event this is
// ---------------------------------------------------------------------------

// EventKind enumerates all event types that the event loop can process.
type EventKind int

const (
	EventTick      EventKind = iota // → Node.OnTick
	EventDeliver                    // → Node.OnMessage
	EventClientReq                  // → Node.OnClient
	EventDiskDone                   // → Node.OnDiskDone
	EventRestart                    // → Node.OnRestart
	EventCrash                      // kill a node
	EventPartition                  // install a network partition
	EventHeal                       // remove a network partition
	EventLinkCut                    // isolate single directed link
	EventLinkRestore                // restore single directed link
	EventEnd                        // end of simulation
)

// String returns the event kind name for logging and traces.
func (k EventKind) String() string {
	names := [...]string{
		"tick", "deliver", "client-req", "disk-done", "restart",
		"crash", "partition", "heal", "link-cut", "link-restore", "end",
	}
	if int(k) < len(names) {
		return names[k]
	}
	return fmt.Sprintf("event(%d)", int(k))
}

// ---------------------------------------------------------------------------
// Event — a single thing that happens at a point in virtual time
// ---------------------------------------------------------------------------

// Event is the fundamental unit of the simulation. Every action is
// represented as an Event in the priority queue, ordered by (At, Seq).
type Event struct {
	At     t.Time
	Seq    uint64
	Kind   EventKind
	Target t.NodeID

	// Payloads (only the relevant field is set, based on Kind).
	Message    *t.Envelope
	ClientReq  *t.ClientReq
	DiskResult *t.DiskOpResult
	DiskImage  *t.DiskImage
	Partition  [][]t.NodeID
	LinkFrom   t.NodeID
	LinkTo     t.NodeID

	// index is the heap position, managed by container/heap.
	index int
}

// String returns a compact representation for traces.
func (e *Event) String() string {
	switch e.Kind {
	case EventDeliver:
		if e.Message != nil {
			return fmt.Sprintf("[%s] %s deliver %s→%s", e.At, e.Kind, e.Message.From, e.Target)
		}
	case EventClientReq:
		if e.ClientReq != nil {
			return fmt.Sprintf("[%s] %s %s→%s", e.At, e.Kind, e.ClientReq, e.Target)
		}
	case EventCrash, EventRestart, EventTick:
		return fmt.Sprintf("[%s] %s %s", e.At, e.Kind, e.Target)
	case EventPartition:
		return fmt.Sprintf("[%s] partition %v", e.At, e.Partition)
	case EventHeal:
		return fmt.Sprintf("[%s] heal", e.At)
	}
	return fmt.Sprintf("[%s] %s target=%s", e.At, e.Kind, e.Target)
}

// ---------------------------------------------------------------------------
// EventQueue — a min-heap ordered by (At, Seq)
// ---------------------------------------------------------------------------

// EventQueue is a min-heap priority queue of events. Events are ordered by
// (At, Seq) where Seq breaks ties deterministically.
//
// Single-threaded — NOT thread-safe (intentional).
type EventQueue struct {
	events  []*Event
	nextSeq uint64
}

// NewEventQueue creates an empty event queue.
func NewEventQueue() *EventQueue {
	return &EventQueue{
		events: make([]*Event, 0, 256),
	}
}

// Enqueue adds an event, assigning it the next sequence number.
func (q *EventQueue) Enqueue(e *Event) {
	e.Seq = q.nextSeq
	q.nextSeq++
	heap.Push(q, e)
}

// Dequeue removes and returns the earliest event. Returns nil if empty.
func (q *EventQueue) Dequeue() *Event {
	if len(q.events) == 0 {
		return nil
	}
	return heap.Pop(q).(*Event)
}

// Peek returns the earliest event without removing it. Returns nil if empty.
func (q *EventQueue) Peek() *Event {
	if len(q.events) == 0 {
		return nil
	}
	return q.events[0]
}

// Len returns the number of events in the queue.
func (q *EventQueue) Len() int {
	return len(q.events)
}

// IsEmpty returns true if the queue has no events.
func (q *EventQueue) IsEmpty() bool {
	return len(q.events) == 0
}

// --- heap.Interface implementation ---

func (q *EventQueue) Less(i, j int) bool {
	ei, ej := q.events[i], q.events[j]
	if ei.At != ej.At {
		return ei.At < ej.At
	}
	return ei.Seq < ej.Seq
}

func (q *EventQueue) Swap(i, j int) {
	q.events[i], q.events[j] = q.events[j], q.events[i]
	q.events[i].index = i
	q.events[j].index = j
}

func (q *EventQueue) Push(x any) {
	e := x.(*Event)
	e.index = len(q.events)
	q.events = append(q.events, e)
}

func (q *EventQueue) Pop() any {
	old := q.events
	n := len(old)
	e := old[n-1]
	old[n-1] = nil
	e.index = -1
	q.events = old[:n-1]
	return e
}
