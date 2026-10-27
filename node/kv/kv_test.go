package kv

import (
	"reflect"
	"testing"
)

func TestStoreOperations(t *testing.T) {
	s := NewStore()

	// Put
	res := s.Apply(Command{
		Type:      CmdPut,
		Key:       "a",
		Value:     "100",
		ClientID:  1,
		RequestID: 1,
	})
	if !res.OK {
		t.Fatalf("expected Put to succeed, got %v", res)
	}

	val, ok := s.Get("a")
	if !ok || val != "100" {
		t.Fatalf("expected a=100, got val=%s ok=%v", val, ok)
	}

	// CAS success
	res = s.Apply(Command{
		Type:      CmdCAS,
		Key:       "a",
		OldValue:  "100",
		Value:     "200",
		ClientID:  1,
		RequestID: 2,
	})
	if !res.OK {
		t.Fatalf("expected CAS to succeed, got %v", res)
	}

	// CAS failure (old value mismatch)
	res = s.Apply(Command{
		Type:      CmdCAS,
		Key:       "a",
		OldValue:  "100",
		Value:     "300",
		ClientID:  1,
		RequestID: 3,
	})
	if res.OK {
		t.Fatalf("expected CAS to fail due to mismatch")
	}

	// Delete
	res = s.Apply(Command{
		Type:      CmdDelete,
		Key:       "a",
		ClientID:  1,
		RequestID: 4,
	})
	if !res.OK {
		t.Fatalf("expected Delete to succeed")
	}

	if _, ok := s.Get("a"); ok {
		t.Fatalf("expected key a to be deleted")
	}
}

func TestStoreClientSessionDedup(t *testing.T) {
	s := NewStore()

	// First Put
	res1 := s.Apply(Command{
		Type:      CmdPut,
		Key:       "counter",
		Value:     "1",
		ClientID:  10,
		RequestID: 1,
	})
	if !res1.OK {
		t.Fatalf("first put failed")
	}

	// Put with new key
	s.Apply(Command{
		Type:      CmdPut,
		Key:       "counter",
		Value:     "2",
		ClientID:  10,
		RequestID: 2,
	})

	// Replay request ID 2 with different value (e.g. malicious or buggy duplicate retry)
	resDup := s.Apply(Command{
		Type:      CmdPut,
		Key:       "counter",
		Value:     "999",
		ClientID:  10,
		RequestID: 2,
	})
	if !resDup.OK {
		t.Fatalf("duplicate should return cached OK")
	}

	// Value should still be 2, NOT 999
	val, _ := s.Get("counter")
	if val != "2" {
		t.Fatalf("expected counter to remain 2, got %s", val)
	}
}

func TestStoreSnapshotSerializeRestore(t *testing.T) {
	s1 := NewStore()
	s1.Apply(Command{Type: CmdPut, Key: "k1", Value: "v1", ClientID: 1, RequestID: 1})
	s1.Apply(Command{Type: CmdPut, Key: "k2", Value: "v2", ClientID: 2, RequestID: 1})

	bytes, err := s1.Serialize()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	s2 := NewStore()
	if err := s2.Restore(bytes); err != nil {
		t.Fatalf("failed to restore: %v", err)
	}

	if !reflect.DeepEqual(s1.Clone(), s2.Clone()) {
		t.Fatalf("restored data mismatch: %v vs %v", s1.Clone(), s2.Clone())
	}

	// Deduplication must also be preserved across snapshot
	resDup := s2.Apply(Command{Type: CmdPut, Key: "k1", Value: "mutated", ClientID: 1, RequestID: 1})
	if !resDup.OK {
		t.Fatalf("expected cached OK from restored session")
	}
	val, _ := s2.Get("k1")
	if val != "v1" {
		t.Fatalf("expected k1 to stay v1, got %s", val)
	}
}
