package model

import (
	"testing"
	"time"
)

func TestSessionModel_BasicLifecycle(t *testing.T) {
	m := NewSessionModel()
	base := time.Unix(0, 0)

	// Start at t0 and run for 5s.
	m.OnTick(true, base)
	// Advance 5s.
	m.OnTick(true, base.Add(5*time.Second))
	session, total := m.Values()
	if session < 5*time.Second || total < 5*time.Second {
		t.Fatalf("expected ~5s session & total; got session=%v total=%v", session, total)
	}

	// Stop at 5s.
	m.OnTick(false, base.Add(5*time.Second))
	session, total = m.Values()
	if session < 5*time.Second || total < 5*time.Second {
		t.Fatalf("after stop expected persisted 5s; got session=%v total=%v", session, total)
	}

	// Idle 2s (no change expected).
	m.OnTick(false, base.Add(7*time.Second))
	session2, total2 := m.Values()
	if session2 != session || total2 != total {
		t.Fatalf("idle tick should not change durations: before session=%v total=%v after session=%v total=%v", session, total, session2, total2)
	}

	// Second session at 10s lasting 3s.
	m.OnTick(true, base.Add(10*time.Second))
	m.OnTick(true, base.Add(13*time.Second))
	s3, t3 := m.Values()
	if s3 < 3*time.Second {
		t.Fatalf("second session expected >=3s, got %v", s3)
	}
	if t3 < 8*time.Second { // 5 + 3 ongoing
		t.Fatalf("total should include previous 5s + current >=3s (>=8s); got %v", t3)
	}

	// stop second session finalizing totals (13s)
	m.OnTick(false, base.Add(13*time.Second))
	sFinal, tFinal := m.Values()
	if sFinal < 3*time.Second || tFinal < 8*time.Second {
		t.Fatalf("final expected session >=3s total >=8s got session=%v total=%v", sFinal, tFinal)
	}
}
