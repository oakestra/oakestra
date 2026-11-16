package events

import (
	"testing"
	"time"
)

func TestActivityNeverTouchedIsIdleForever(t *testing.T) {
	a := &Activity{}
	if a.IdleFor() < 24*time.Hour {
		t.Error("an Activity that was never touched should report as idle for a very long time")
	}
}

func TestActivityTouchResetsIdle(t *testing.T) {
	a := &Activity{}
	a.Touch()
	if a.IdleFor() > time.Second {
		t.Error("Activity should be freshly idle right after Touch")
	}
}

func TestGetOrCreateReturnsSameInstance(t *testing.T) {
	a1 := GetOrCreate("job0")
	a2 := GetOrCreate("job0")
	if a1 != a2 {
		t.Error("GetOrCreate should return the same *Activity for the same target")
	}

	a1.Touch()
	if a2.IdleFor() > time.Second {
		t.Error("touching the target via one handle should be visible via any other handle to the same target")
	}
}

func TestGetOrCreateIsolatesTargets(t *testing.T) {
	a1 := GetOrCreate("job1")
	a2 := GetOrCreate("job2")
	if a1 == a2 {
		t.Error("different targets should get different Activity instances")
	}
}

func TestDeleteThenGetOrCreateReturnsFreshInstance(t *testing.T) {
	a1 := GetOrCreate("job3")
	a1.Touch()
	Delete("job3")

	a2 := GetOrCreate("job3")
	if a1 == a2 {
		t.Error("GetOrCreate after Delete should hand out a fresh Activity")
	}
	if a2.IdleFor() < 24*time.Hour {
		t.Error("the fresh Activity should not carry over the deleted one's timestamp")
	}
}

func TestActivityConcurrentTouch(t *testing.T) {
	a := &Activity{}
	done := make(chan struct{})
	for range 8 {
		go func() {
			for range 1000 {
				a.Touch()
			}
			done <- struct{}{}
		}()
	}
	for range 8 {
		<-done
	}
}
