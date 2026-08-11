package ranking

import "testing"

func TestVersionGateRejectsCommitAfterMutation(t *testing.T) {
	gate := &VersionGate{}
	stale := gate.current()
	done := gate.BeginMutation()
	done()
	called := false
	committed, err := gate.commit(stale, func() error { called = true; return nil })
	if err != nil || committed || called {
		t.Fatalf("stale commit executed: committed=%v called=%v error=%v", committed, called, err)
	}
	current := gate.current()
	committed, err = gate.commit(current, func() error { called = true; return nil })
	if err != nil || !committed || !called {
		t.Fatalf("current commit rejected: committed=%v called=%v error=%v", committed, called, err)
	}
}

func TestVersionGateRejectsCommitWhileMutationIsActive(t *testing.T) {
	gate := &VersionGate{}
	done := gate.BeginMutation()
	version := gate.current()
	called := false
	committed, err := gate.commit(version, func() error { called = true; return nil })
	done()
	if err != nil || committed || called {
		t.Fatalf("committed=%v called=%v error=%v", committed, called, err)
	}
}
