package checkers

import (
	"testing"

	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

func TestCheckOneLeaderPerTerm(tst *testing.T) {
	// Valid: 1 leader in term 2, 2 followers
	valid := []t.NodeView{
		{ID: t.NodeID(1), Role: t.RoleLeader, Term: 2, Alive: true},
		{ID: t.NodeID(2), Role: t.RoleFollower, Term: 2, Alive: true},
		{ID: t.NodeID(3), Role: t.RoleFollower, Term: 2, Alive: true},
	}
	if v := CheckOneLeaderPerTerm(valid); v != nil {
		tst.Fatalf("unexpected violation for valid cluster: %v", v)
	}

	// Invalid: two leaders in term 2
	splitBrain := []t.NodeView{
		{ID: t.NodeID(1), Role: t.RoleLeader, Term: 2, Alive: true},
		{ID: t.NodeID(2), Role: t.RoleLeader, Term: 2, Alive: true},
		{ID: t.NodeID(3), Role: t.RoleFollower, Term: 2, Alive: true},
	}
	if v := CheckOneLeaderPerTerm(splitBrain); v == nil {
		tst.Fatalf("expected violation for split brain, got nil")
	} else if v.Checker != "one_leader_per_term" {
		tst.Fatalf("expected one_leader_per_term checker, got %s", v.Checker)
	}
}

func TestCheckConvergence(tst *testing.T) {
	kv1 := map[string]string{"foo": "bar", "alpha": "beta"}
	kv2 := map[string]string{"foo": "bar", "alpha": "beta"}
	kvDivergent := map[string]string{"foo": "bar", "alpha": "gamma"}

	converged := []t.NodeView{
		{ID: t.NodeID(1), KVState: kv1, Alive: true},
		{ID: t.NodeID(2), KVState: kv2, Alive: true},
	}
	if v := CheckConvergence(converged); v != nil {
		tst.Fatalf("unexpected violation for converged cluster: %v", v)
	}

	diverged := []t.NodeView{
		{ID: t.NodeID(1), KVState: kv1, Alive: true},
		{ID: t.NodeID(2), KVState: kvDivergent, Alive: true},
	}
	if v := CheckConvergence(diverged); v == nil {
		tst.Fatalf("expected convergence violation, got nil")
	} else if v.Checker != "convergence" {
		tst.Fatalf("expected convergence checker, got %s", v.Checker)
	}
}

func TestOracle(tst *testing.T) {
	o := NewOracle()
	o.ApplyPut("k1", "v1")
	o.ApplyPut("k2", "v2")
	o.ApplyDelete("k1")

	matched := []t.NodeView{
		{ID: t.NodeID(1), KVState: map[string]string{"k2": "v2"}, Alive: true},
	}
	if v := o.CheckState(matched); v != nil {
		tst.Fatalf("unexpected oracle violation: %v", v)
	}

	mismatched := []t.NodeView{
		{ID: t.NodeID(1), KVState: map[string]string{"k1": "v1", "k2": "v2"}, Alive: true},
	}
	if v := o.CheckState(mismatched); v == nil {
		tst.Fatalf("expected oracle violation, got nil")
	}
}
