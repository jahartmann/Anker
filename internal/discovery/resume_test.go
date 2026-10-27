package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func finish(t *testing.T, s *Scanner) State {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := s.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	return s.Snapshot()
}

func TestTwoStageEntire16AndCompactCheckpoint(t *testing.T) {
	var fast, slow, https atomic.Int64
	s := &Scanner{Unpaced: true, TCPProbe: func(_ context.Context, _ string, _ int, timeout time.Duration) (bool, bool) {
		if timeout < time.Second {
			fast.Add(1)
			return false, true
		}
		slow.Add(1)
		return false, false
	}, Identify: func(context.Context, string, int, int) (*Result, error) { https.Add(1); return nil, nil }}
	if err := s.Start(Config{Ranges: []string{"10.20.0.0/16"}}); err != nil {
		t.Fatal(err)
	}
	v := finish(t, s)
	if v.Status != "completed" || v.Checked != 65534 || v.TCPChecked != v.Total || fast.Load() != 2*65534 || slow.Load() != 2*65534 || https.Load() != 0 {
		t.Fatalf("%+v fast=%d slow=%d https=%d", v, fast.Load(), slow.Load(), https.Load())
	}
	data, err := json.Marshal(s.Checkpoint())
	if err != nil || len(data) > 300000 {
		t.Fatalf("checkpoint size=%d %v", len(data), err)
	}
}

func TestRestorePendingPBSAndOnlyRetryUnfinishedTCP(t *testing.T) {
	c := Config{Ranges: []string{"10.20.0.1", "10.20.0.2", "10.20.0.3"}, WebPort: 8006, PBSPort: 8007}
	cp := Checkpoint{Version: 1, State: State{Status: "running", Config: c, Results: []Result{{Address: "10.20.0.1", Kind: "pve", WebPort: 8006}}}, Progress: []byte{fastDone | addressDone, fastDone | retryNeeded, 0}, Pending: []byte{0, 2, 0}, Verified: []byte{1, 0, 0}}
	var first, secondFast, secondSlow, thirdFast, tls atomic.Int32
	s := &Scanner{Unpaced: true, TCPProbe: func(_ context.Context, a string, _ int, timeout time.Duration) (bool, bool) {
		switch a {
		case "10.20.0.1":
			first.Add(1)
		case "10.20.0.2":
			if timeout < time.Second {
				secondFast.Add(1)
			} else {
				secondSlow.Add(1)
			}
		case "10.20.0.3":
			thirdFast.Add(1)
		}
		return false, false
	}, Identify: func(_ context.Context, a string, p, ssh int) (*Result, error) {
		tls.Add(1)
		if a != "10.20.0.2" || p != 8007 {
			t.Errorf("wrong endpoint %s:%d", a, p)
		}
		return &Result{Kind: "pbs"}, nil
	}}
	if err := s.Restore(cp); err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Status != "paused" {
		t.Fatal("automatically restarted")
	}
	if err := s.Resume(); err != nil {
		t.Fatal(err)
	}
	v := finish(t, s)
	if v.Checked != 3 || len(v.Results) != 2 || first.Load() != 0 || secondFast.Load() != 0 || secondSlow.Load() != 2 || thirdFast.Load() != 2 || tls.Load() != 1 {
		t.Fatalf("%+v calls %d %d %d %d %d", v, first.Load(), secondFast.Load(), secondSlow.Load(), thirdFast.Load(), tls.Load())
	}
}

func TestHTTPSOnlyForOpenCandidatesAndBothServices(t *testing.T) {
	var https atomic.Int32
	s := &Scanner{Unpaced: true, TCPProbe: func(_ context.Context, a string, p int, _ time.Duration) (bool, bool) {
		return (a == "10.20.0.11" && p == 8006) || (a == "10.20.0.12" && p == 8007), false
	}, Identify: func(_ context.Context, a string, p, ssh int) (*Result, error) {
		https.Add(1)
		if a == "10.20.0.11" && p == 8006 {
			return &Result{Kind: "pve"}, nil
		}
		if a == "10.20.0.12" && p == 8007 {
			return &Result{Kind: "pbs"}, nil
		}
		t.Errorf("HTTPS on closed endpoint %s:%d", a, p)
		return nil, nil
	}}
	if err := s.Start(Config{Ranges: []string{"10.20.0.0/24"}}); err != nil {
		t.Fatal(err)
	}
	v := finish(t, s)
	if https.Load() != 2 || v.Checked != 254 || v.Candidates != 2 || len(v.Results) != 2 || v.Results[0].Kind != "pve" || v.Results[1].Kind != "pbs" {
		t.Fatal(v, https.Load())
	}
	if err := new(Scanner).Restore(s.Checkpoint()); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreRejectsImpossibleProgress(t *testing.T) {
	for _, p := range []byte{fastDone, slowDone, retryNeeded, addressDone, fastDone | slowDone | addressDone} {
		cp := Checkpoint{Version: 1, State: State{Config: Config{Ranges: []string{"10.20.0.1"}}}, Progress: []byte{p}, Pending: []byte{0}, Verified: []byte{0}}
		if err := new(Scanner).Restore(cp); err == nil {
			t.Errorf("accepted flags %d", p)
		}
	}
}

func TestTerminalCheckpointErrorIsReportedToUpdateWaiter(t *testing.T) {
	s := &Scanner{Unpaced: true, TCPProbe: func(context.Context, string, int, time.Duration) (bool, bool) { return false, false }, Save: func(cp Checkpoint) error {
		if cp.State.Checked > 0 {
			return errors.New("disk full")
		}
		return nil
	}}
	if err := s.Start(Config{Ranges: []string{"10.20.0.1"}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Wait(ctx); err == nil {
		t.Fatal("lost checkpoint accepted")
	}
}

func TestTimeoutAndCancellationRemainResumable(t *testing.T) {
	s := &Scanner{Budget: 20 * time.Millisecond, Unpaced: true, TCPProbe: func(ctx context.Context, _ string, _ int, _ time.Duration) (bool, bool) {
		<-ctx.Done()
		return false, true
	}}
	if err := s.Start(Config{Ranges: []string{"10.0.0.0/24"}}); err != nil {
		t.Fatal(err)
	}
	v := finish(t, s)
	if v.Status != "paused" || !v.Resumable || v.Checked != 0 {
		t.Fatal(v)
	}
	cp := s.Checkpoint()
	resumed := &Scanner{Unpaced: true, TCPProbe: func(context.Context, string, int, time.Duration) (bool, bool) { return false, false }}
	if err := resumed.Restore(cp); err != nil {
		t.Fatal(err)
	}
	if err := resumed.Resume(); err != nil {
		t.Fatal(err)
	}
	if v = finish(t, resumed); v.Checked != 254 || v.Resumable || v.Status != "completed" {
		t.Fatal(v)
	}
}

func TestCheckpointWriteFailurePreventsTraffic(t *testing.T) {
	var calls atomic.Int32
	s := &Scanner{Save: func(Checkpoint) error { return errors.New("disk full") }, TCPProbe: func(context.Context, string, int, time.Duration) (bool, bool) { calls.Add(1); return false, false }}
	if err := s.Start(Config{Ranges: []string{"10.0.0.1"}}); err == nil {
		t.Fatal("accepted failed checkpoint")
	}
	if calls.Load() != 0 || s.Snapshot().Status == "running" {
		t.Fatal("network started without checkpoint")
	}
}

func TestCancellationSavesCompletedWorkBeforeRestart(t *testing.T) {
	var calls atomic.Int32
	blocked := make(chan struct{}, 1)
	var saved Checkpoint
	s := &Scanner{Unpaced: true, Save: func(cp Checkpoint) error { saved = cp; return nil }, TCPProbe: func(ctx context.Context, _ string, _ int, _ time.Duration) (bool, bool) {
		if calls.Add(1) <= 32 {
			return false, false
		}
		select {
		case blocked <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return false, true
	}}
	if err := s.Start(Config{Ranges: []string{"10.20.0.0/24"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("probe did not start")
	}
	s.Cancel()
	finish(t, s)
	completed := map[string]bool{}
	_, addresses, _ := Normalize(saved.State.Config)
	for i, p := range saved.Progress {
		if p&addressDone != 0 {
			completed[addresses[i]] = true
		}
	}
	if len(completed) == 0 || len(completed) == 254 {
		t.Fatal("did not save partial progress", len(completed))
	}
	resumed := &Scanner{Unpaced: true, TCPProbe: func(_ context.Context, a string, _ int, _ time.Duration) (bool, bool) {
		if completed[a] {
			t.Errorf("repeated completed address %s", a)
		}
		return false, false
	}}
	if err := resumed.Restore(saved); err != nil {
		t.Fatal(err)
	}
	if err := resumed.Resume(); err != nil {
		t.Fatal(err)
	}
	if v := finish(t, resumed); v.Checked != 254 {
		t.Fatal(v)
	}
}

func TestHintsStayInScopeAndPatternsLearnWithoutSkipping(t *testing.T) {
	_, addresses, err := Normalize(Config{Ranges: []string{"10.20.0.0/24", "10.21.0.0/24"}})
	if err != nil {
		t.Fatal(err)
	}
	q := newQueue(addresses, []Hint{{Address: "10.21.0.77", Source: "cluster"}, {Address: "10.99.0.77", Source: "cluster"}})
	progress := make([]byte, len(addresses))
	seen := map[int]bool{}
	i := q.next(progress, false, nil)
	if addresses[i] != "10.21.0.77" {
		t.Fatal("exact hint not first", addresses[i])
	}
	seen[i] = true
	i = q.next(progress, false, []Result{{Address: "10.21.0.88"}, {Address: "10.20.0.88"}})
	if addresses[i] != "10.20.0.88" {
		t.Fatal("learned pattern not prioritised", addresses[i])
	}
	seen[i] = true
	for {
		i = q.next(progress, false, nil)
		if i < 0 {
			break
		}
		if seen[i] {
			t.Fatal("duplicate", i)
		}
		seen[i] = true
	}
	if len(seen) != 508 {
		t.Fatal("skipped standalone addresses", len(seen))
	}
	s := &Scanner{Unpaced: true, Seeds: func(context.Context, Config) ([]Hint, []string) {
		return []Hint{{Address: "10.99.0.77", Source: "cluster"}, {Address: "10.20.0.1", Source: "known"}}, nil
	}, TCPProbe: func(_ context.Context, a string, _ int, _ time.Duration) (bool, bool) {
		if a != "10.20.0.1" {
			t.Errorf("scope escaped: %s", a)
		}
		return false, false
	}}
	if err := s.Start(Config{Ranges: []string{"10.20.0.1"}}); err != nil {
		t.Fatal(err)
	}
	finish(t, s)
	if len(s.Checkpoint().Hints) != 1 {
		t.Fatal("out of scope hint persisted")
	}
}
