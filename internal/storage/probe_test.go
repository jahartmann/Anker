package storage

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestStatProbeBoundsUnavailableMountAndDoesNotAccumulateBlockedCalls(t *testing.T) {
	p := newStatProbe(10 * time.Millisecond)
	release := make(chan struct{})
	defer close(release)
	var calls atomic.Int32
	stat := func(string) (Stats, error) { calls.Add(1); <-release; return Stats{Total: 1000}, nil }
	for i := 0; i < 3; i++ {
		if _, err := p.read("/unavailable", stat); err == nil {
			t.Fatal("unavailable mount returned success")
		}
	}
	if calls.Load() != 1 {
		t.Fatal("blocked syscalls accumulated", calls.Load())
	}
	got, err := p.read("/healthy", func(string) (Stats, error) { return Stats{Total: 5000}, nil })
	if err != nil || got.Total != 5000 {
		t.Fatal("unavailable mount blocked healthy volume", got, err)
	}
}
