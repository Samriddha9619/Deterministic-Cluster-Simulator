package sim

import (
	"encoding/json"
	"fmt"
	"sort"

	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

// FaultKind defines the type of fault injected into the simulation.
type FaultKind int

const (
	FaultCrashNode FaultKind = iota
	FaultRestartNode
	FaultPartition
	FaultHeal
	FaultLinkCut
	FaultLinkRestore
)

func (k FaultKind) String() string {
	switch k {
	case FaultCrashNode:
		return "crash"
	case FaultRestartNode:
		return "restart"
	case FaultPartition:
		return "partition"
	case FaultHeal:
		return "heal"
	case FaultLinkCut:
		return "link_cut"
	case FaultLinkRestore:
		return "link_restore"
	default:
		return fmt.Sprintf("fault(%d)", int(k))
	}
}

// FaultAction represents an explicit fault scheduled at a specific virtual time.
type FaultAction struct {
	At        t.Time       `json:"at_ns"`
	AtStr     string       `json:"at"`
	Kind      FaultKind    `json:"kind"`
	NodeID    t.NodeID     `json:"node_id,omitempty"`
	FromNode  t.NodeID     `json:"from_node,omitempty"`
	ToNode    t.NodeID     `json:"to_node,omitempty"`
	Partition [][]t.NodeID `json:"partition,omitempty"`
}

func (f FaultAction) String() string {
	switch f.Kind {
	case FaultCrashNode:
		return fmt.Sprintf("[%s] crash %s", f.At, f.NodeID)
	case FaultRestartNode:
		return fmt.Sprintf("[%s] restart %s", f.At, f.NodeID)
	case FaultPartition:
		return fmt.Sprintf("[%s] partition %v", f.At, f.Partition)
	case FaultHeal:
		return fmt.Sprintf("[%s] heal all partitions", f.At)
	case FaultLinkCut:
		return fmt.Sprintf("[%s] cut link %s -> %s", f.At, f.FromNode, f.ToNode)
	case FaultLinkRestore:
		return fmt.Sprintf("[%s] restore link %s -> %s", f.At, f.FromNode, f.ToNode)
	default:
		return fmt.Sprintf("[%s] fault %s", f.At, f.Kind)
	}
}

// FaultPlan is an ordered schedule of fault actions executed during a run.
// Materializing faults into an explicit plan makes delta-debugging shrinking possible.
type FaultPlan struct {
	Actions []FaultAction `json:"actions"`
}

// NewFaultPlan creates an empty fault plan.
func NewFaultPlan() *FaultPlan {
	return &FaultPlan{
		Actions: make([]FaultAction, 0),
	}
}

// Add appends an action to the plan and sorts by virtual time.
func (fp *FaultPlan) Add(action FaultAction) {
	action.AtStr = action.At.String()
	fp.Actions = append(fp.Actions, action)
	sort.SliceStable(fp.Actions, func(i, j int) bool {
		return fp.Actions[i].At < fp.Actions[j].At
	})
}

// ScheduleOn enqueues all actions in the fault plan into the simulator's event queue.
func (fp *FaultPlan) ScheduleOn(s *Simulator) {
	for _, a := range fp.Actions {
		switch a.Kind {
		case FaultCrashNode:
			s.EnqueueCrash(a.At, a.NodeID)
		case FaultRestartNode:
			s.EnqueueRestart(a.At, a.NodeID)
		case FaultPartition:
			s.EnqueuePartition(a.At, a.Partition)
		case FaultHeal:
			s.EnqueueHeal(a.At)
		case FaultLinkCut:
			s.queue.Enqueue(&Event{
				At:     a.At,
				Kind:   EventLinkCut,
				Target: a.ToNode,
				LinkFrom: a.FromNode,
				LinkTo:   a.ToNode,
			})
		case FaultLinkRestore:
			s.queue.Enqueue(&Event{
				At:     a.At,
				Kind:   EventLinkRestore,
				Target: a.ToNode,
				LinkFrom: a.FromNode,
				LinkTo:   a.ToNode,
			})
		}
	}
}

// Clone creates a deep copy of the fault plan (essential for delta-debugging candidates).
func (fp *FaultPlan) Clone() *FaultPlan {
	clone := &FaultPlan{
		Actions: make([]FaultAction, len(fp.Actions)),
	}
	for i, a := range fp.Actions {
		clone.Actions[i] = a
		if len(a.Partition) > 0 {
			partCopy := make([][]t.NodeID, len(a.Partition))
			for j, group := range a.Partition {
				partCopy[j] = make([]t.NodeID, len(group))
				copy(partCopy[j], group)
			}
			clone.Actions[i].Partition = partCopy
		}
	}
	return clone
}

// WithoutAction returns a clone with the action at index removed.
func (fp *FaultPlan) WithoutAction(idx int) *FaultPlan {
	if idx < 0 || idx >= len(fp.Actions) {
		return fp.Clone()
	}
	clone := fp.Clone()
	clone.Actions = append(clone.Actions[:idx], clone.Actions[idx+1:]...)
	return clone
}

// WithoutRange returns a clone with the actions in [from, to) removed.
func (fp *FaultPlan) WithoutRange(from, to int) *FaultPlan {
	if from < 0 {
		from = 0
	}
	if to > len(fp.Actions) {
		to = len(fp.Actions)
	}
	if from >= to {
		return fp.Clone()
	}
	clone := fp.Clone()
	clone.Actions = append(clone.Actions[:from], clone.Actions[to:]...)
	return clone
}

// SubRange returns a clone containing only actions in [from, to).
func (fp *FaultPlan) SubRange(from, to int) *FaultPlan {
	if from < 0 {
		from = 0
	}
	if to > len(fp.Actions) {
		to = len(fp.Actions)
	}
	if from >= to {
		return NewFaultPlan()
	}
	clone := fp.Clone()
	clone.Actions = clone.Actions[from:to]
	return clone
}

// HumanSummary returns a concise description of actions in the plan.
func (fp *FaultPlan) HumanSummary() string {
	if len(fp.Actions) == 0 {
		return "no faults"
	}
	counts := make(map[FaultKind]int)
	for _, a := range fp.Actions {
		counts[a.Kind]++
	}

	kinds := []FaultKind{FaultCrashNode, FaultRestartNode, FaultPartition, FaultHeal, FaultLinkCut, FaultLinkRestore}
	var parts []string
	for _, k := range kinds {
		c := counts[k]
		if c <= 0 {
			continue
		}
		var name string
		switch k {
		case FaultCrashNode:
			if c == 1 {
				name = "1 crash"
			} else {
				name = fmt.Sprintf("%d crashes", c)
			}
		case FaultRestartNode:
			if c == 1 {
				name = "1 restart"
			} else {
				name = fmt.Sprintf("%d restarts", c)
			}
		case FaultPartition:
			if c == 1 {
				name = "1 partition"
			} else {
				name = fmt.Sprintf("%d partitions", c)
			}
		case FaultHeal:
			if c == 1 {
				name = "1 heal"
			} else {
				name = fmt.Sprintf("%d heals", c)
			}
		case FaultLinkCut:
			if c == 1 {
				name = "1 link cut"
			} else {
				name = fmt.Sprintf("%d link cuts", c)
			}
		case FaultLinkRestore:
			if c == 1 {
				name = "1 link restore"
			} else {
				name = fmt.Sprintf("%d link restores", c)
			}
		default:
			name = fmt.Sprintf("%d %s", c, k)
		}
		parts = append(parts, name)
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%d actions", len(fp.Actions))
	}
	res := parts[0]
	for i := 1; i < len(parts); i++ {
		res += ", " + parts[i]
	}
	return res
}

// ToJSON formats the fault plan as JSON.
func (fp *FaultPlan) ToJSON() ([]byte, error) {
	return json.MarshalIndent(fp, "", "  ")
}

// GenerateRandomFaultPlan builds a random fault plan up front using a dedicated RNG stream.
// It reserves the final quiesce window (last 25% of virtual time) where all nodes are
// restarted and network is healed.
func GenerateRandomFaultPlan(rng t.Rand, nodeIDs []t.NodeID, maxTime t.Time, numFaults int) *FaultPlan {
	fp := NewFaultPlan()
	if len(nodeIDs) == 0 || numFaults <= 0 {
		return fp
	}

	// Reserve 25% of time at the end for quiescence.
	activeDuration := t.Time(float64(maxTime) * 0.75)
	if activeDuration <= 2*t.Second {
		activeDuration = maxTime
	}

	crashedNodes := make(map[t.NodeID]bool)
	isPartitioned := false

	// Generate fault events between 1s and activeDuration.
	step := (activeDuration - 1*t.Second) / t.Time(numFaults+1)
	if step < 500*t.Millisecond {
		step = 500 * t.Millisecond
	}

	for i := 0; i < numFaults; i++ {
		faultAt := 1*t.Second + t.Time(i)*step + DurationBetween(rng, 0, step/2)
		roll := rng.Intn(4)

		switch roll {
		case 0: // Crash an alive node
			var aliveCandidates []t.NodeID
			for _, id := range nodeIDs {
				if !crashedNodes[id] {
					aliveCandidates = append(aliveCandidates, id)
				}
			}
			// Keep at least a quorum alive
			if len(aliveCandidates) > (len(nodeIDs)/2 + 1) {
				idx := rng.Intn(len(aliveCandidates))
				victim := aliveCandidates[idx]
				crashedNodes[victim] = true
				fp.Add(FaultAction{
					At:     faultAt,
					Kind:   FaultCrashNode,
					NodeID: victim,
				})

				// Schedule restart 1.5s to 3s later
				restartTime := faultAt + DurationBetween(rng, 1500*t.Millisecond, 3000*t.Millisecond)
				if restartTime < activeDuration {
					crashedNodes[victim] = false
					fp.Add(FaultAction{
						At:     restartTime,
						Kind:   FaultRestartNode,
						NodeID: victim,
					})
				}
			}

		case 1: // Restart a crashed node if any
			var deadList []t.NodeID
			for _, id := range nodeIDs {
				if crashedNodes[id] {
					deadList = append(deadList, id)
				}
			}
			if len(deadList) > 0 {
				node := deadList[rng.Intn(len(deadList))]
				crashedNodes[node] = false
				fp.Add(FaultAction{
					At:     faultAt,
					Kind:   FaultRestartNode,
					NodeID: node,
				})
			}

		case 2: // Network partition
			if !isPartitioned && len(nodeIDs) >= 3 {
				// Partition 1 isolated node from the rest
				isolatedIdx := rng.Intn(len(nodeIDs))
				isolatedNode := nodeIDs[isolatedIdx]
				var rest []t.NodeID
				for _, id := range nodeIDs {
					if id != isolatedNode {
						rest = append(rest, id)
					}
				}
				isPartitioned = true
				fp.Add(FaultAction{
					At:   faultAt,
					Kind: FaultPartition,
					Partition: [][]t.NodeID{
						rest,
						{isolatedNode},
					},
				})

				// Schedule heal
				healTime := faultAt + DurationBetween(rng, 2000*t.Millisecond, 4000*t.Millisecond)
				if healTime < activeDuration {
					isPartitioned = false
					fp.Add(FaultAction{
						At:   healTime,
						Kind: FaultHeal,
					})
				}
			}

		case 3: // Heal
			if isPartitioned {
				isPartitioned = false
				fp.Add(FaultAction{
					At:   faultAt,
					Kind: FaultHeal,
				})
			}
		}
	}

	// Mandatory quiescence reconciliation at activeDuration:
	// 1. Restart all crashed nodes
	for id, dead := range crashedNodes {
		if dead {
			fp.Add(FaultAction{
				At:     activeDuration + 100*t.Millisecond,
				Kind:   FaultRestartNode,
				NodeID: id,
			})
		}
	}
	// 2. Heal all network partitions
	fp.Add(FaultAction{
		At:   activeDuration + 200*t.Millisecond,
		Kind: FaultHeal,
	})

	return fp
}
