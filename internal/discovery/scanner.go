package discovery

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"sync"
	"time"
)

// Hints are suggestions only. Only literal IPs inside the operator's selected
// networks can become scan targets; configuration hints never grant SSH trust.
type Hint struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
	Source  string `json:"source,omitempty"`
}
type Result struct {
	Address      string `json:"address"`
	Name         string `json:"name"`
	Version      string `json:"version"`
	Kind         string `json:"kind,omitempty"`
	WebPort      int    `json:"web_port,omitempty"`
	SSHReachable bool   `json:"ssh_reachable"`
	Source       string `json:"source,omitempty"`
}
type State struct {
	Status     string   `json:"status"`
	Phase      string   `json:"phase,omitempty"`
	Total      int      `json:"total"`
	Checked    int      `json:"checked"`
	TCPChecked int      `json:"tcp_checked"`
	Candidates int      `json:"candidates"`
	Resumable  bool     `json:"resumable"`
	Results    []Result `json:"results"`
	Warnings   []string `json:"warnings,omitempty"`
	Error      string   `json:"error,omitempty"`
	Config     Config   `json:"config"`
}

// Parallel arrays keep a /16 checkpoint small. Indices always refer to Normalize's
// stable address list, never to the dynamically prioritised scheduling order.
type Checkpoint struct {
	Version  int    `json:"version"`
	State    State  `json:"state"`
	Progress []byte `json:"progress"`
	Pending  []byte `json:"pending"`
	Verified []byte `json:"verified"`
	Hints    []Hint `json:"hints,omitempty"`
}

const (
	fastDone    byte = 1
	retryNeeded byte = 2
	slowDone    byte = 4
	addressDone byte = 8
)

type Scanner struct {
	mu                          sync.Mutex
	persistMu                   sync.Mutex
	state                       State
	progress, pending, verified []byte
	hints                       []Hint
	addresses                   []string
	cancel                      context.CancelFunc
	done                        chan struct{}
	saveError                   bool
	revision, savedRevision     uint64
	// Callbacks are configured before use. Probe retains the existing test seam.
	Save     func(Checkpoint) error
	Seeds    func(context.Context, Config) ([]Hint, []string)
	Probe    func(context.Context, string, Config) *Result
	TCPProbe func(context.Context, string, int, time.Duration) (bool, bool)
	Identify func(context.Context, string, int, int) (*Result, error)
	Budget   time.Duration
	Unpaced  bool // tests only; production is paced at 256 addresses/second.
}

func cloneState(v State) State {
	v.Config = cloneConfig(v.Config)
	v.Results = append([]Result{}, v.Results...)
	v.Warnings = append([]string{}, v.Warnings...)
	return v
}
func (s *Scanner) Snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := cloneState(s.state)
	if s.cancel != nil {
		v.Status = "running"
	}
	if v.Status == "" {
		v.Status = "idle"
	}
	sort.Slice(v.Results, func(i, j int) bool {
		a, _ := netip.ParseAddr(v.Results[i].Address)
		b, _ := netip.ParseAddr(v.Results[j].Address)
		if a == b {
			return v.Results[i].WebPort < v.Results[j].WebPort
		}
		return a.Less(b)
	})
	return v
}
func (s *Scanner) checkpointLocked() Checkpoint {
	return Checkpoint{Version: 1, State: cloneState(s.state), Progress: append([]byte{}, s.progress...), Pending: append([]byte{}, s.pending...), Verified: append([]byte{}, s.verified...), Hints: append([]Hint{}, s.hints...)}
}
func (s *Scanner) Checkpoint() Checkpoint {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.checkpointLocked()
}
func (s *Scanner) persist() error {
	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	s.mu.Lock()
	if s.revision == s.savedRevision {
		s.mu.Unlock()
		return nil
	}
	revision := s.revision
	cp := s.checkpointLocked()
	save := s.Save
	s.mu.Unlock()
	if save == nil {
		return nil
	}
	if err := save(cp); err != nil {
		s.mu.Lock()
		s.saveError = true
		s.state.Error = "Suchfortschritt konnte nicht gespeichert werden; letzten gespeicherten Stand nach Neustart fortsetzen."
		if s.cancel != nil {
			s.cancel()
		}
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	s.savedRevision = revision
	s.mu.Unlock()
	return nil
}
func (s *Scanner) Restore(cp Checkpoint) error {
	c, addresses, err := Normalize(cp.State.Config)
	if err != nil {
		return err
	}
	n := len(addresses)
	if cp.Version != 1 || len(cp.Progress) != n || len(cp.Pending) != n || len(cp.Verified) != n {
		return errors.New("ungültiger Suchfortschritt")
	}
	if len(cp.Hints) > MaxAddresses || len(cp.State.Results) > 2*n {
		return errors.New("Suchfortschritt überschreitet Grenzen")
	}
	allowed := map[string]bool{}
	indices := map[string]int{}
	for i, address := range addresses {
		allowed[address] = true
		indices[address] = i
	}
	checked, tcp, candidates := 0, 0, 0
	for i, p := range cp.Progress {
		complete := p&fastDone != 0 && cp.Pending[i] == 0 && (p&retryNeeded == 0 || p&slowDone != 0)
		if p&^byte(15) != 0 || cp.Pending[i]&^byte(3) != 0 || cp.Verified[i]&^byte(3) != 0 || cp.Pending[i]&cp.Verified[i] != 0 || (p&addressDone != 0) != complete || (p&fastDone == 0 && (p != 0 || cp.Pending[i] != 0 || cp.Verified[i] != 0)) || (p&slowDone != 0 && p&retryNeeded == 0) {
			return errors.New("inkonsistenter Suchfortschritt")
		}
		if p&addressDone != 0 {
			checked++
		}
		if p&fastDone != 0 {
			tcp++
		}
		candidates += bitsCount(cp.Pending[i] | cp.Verified[i])
	}
	seen := map[string]bool{}
	for _, r := range cp.State.Results {
		key := fmt.Sprintf("%s:%d", r.Address, r.WebPort)
		if !allowed[r.Address] || seen[key] || (r.Kind != "pve" && r.Kind != "pbs") || (r.WebPort != c.WebPort && r.WebPort != c.PBSPort) || len(r.Name) > 256 || len(r.Version) > 100 {
			return errors.New("ungültiger gespeicherter Treffer")
		}
		mask := byte(1)
		if r.WebPort != c.WebPort {
			mask = 2
		}
		if cp.Verified[indices[r.Address]]&mask == 0 {
			return errors.New("Treffer ohne abgeschlossene Erkennung")
		}
		seen[key] = true
	}
	for _, hint := range cp.Hints {
		if !allowed[hint.Address] || len(hint.Name) > 256 || (hint.Source != "known" && hint.Source != "cluster" && hint.Source != "pbs-config") {
			return errors.New("ungültiger gespeicherter Konfigurationshinweis")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return errors.New("Serversuche läuft bereits")
	}
	s.state = cloneState(cp.State)
	s.state.Config = c
	s.state.Total = n
	s.state.Checked = checked
	s.state.TCPChecked = tcp
	s.state.Candidates = candidates
	s.state.Resumable = checked < n
	if s.state.Resumable {
		s.state.Status = "paused"
		s.state.Phase = "paused"
		s.state.Error = "Unterbrochene Suche kann mit dem gespeicherten Fortschritt fortgesetzt werden."
	} else {
		s.state.Status = "completed"
		s.state.Phase = "completed"
	}
	s.addresses = addresses
	s.progress = append([]byte{}, cp.Progress...)
	s.pending = append([]byte{}, cp.Pending...)
	s.verified = append([]byte{}, cp.Verified...)
	s.hints = append([]Hint{}, cp.Hints...)
	s.revision++
	return nil
}
func (s *Scanner) Start(c Config) error {
	c, addresses, err := Normalize(c)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return errors.New("Serversuche läuft bereits")
	}
	old := s.checkpointLocked()
	s.state = State{Status: "running", Phase: "hints", Total: len(addresses), Config: cloneConfig(c), Results: []Result{}, Resumable: true}
	s.addresses = addresses
	s.progress = make([]byte, len(addresses))
	s.pending = make([]byte, len(addresses))
	s.verified = make([]byte, len(addresses))
	s.hints = nil
	s.revision++
	err = s.launchLocked(false)
	if err != nil && len(old.Progress) > 0 {
		_ = s.Restore(old)
	}
	return err
}
func (s *Scanner) Resume() error {
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return errors.New("Serversuche läuft bereits")
	}
	if len(s.progress) == 0 || s.state.Checked == s.state.Total {
		s.mu.Unlock()
		return errors.New("Keine unvollständige Suche vorhanden")
	}
	s.state.Status = "running"
	s.state.Error = ""
	s.revision++
	return s.launchLocked(true)
}
func (s *Scanner) launchLocked(resume bool) error {
	budget := s.Budget
	if budget <= 0 {
		budget = 20 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	s.cancel = cancel
	s.done = make(chan struct{})
	s.saveError = false
	s.mu.Unlock()
	if err := s.persist(); err != nil {
		cancel()
		s.mu.Lock()
		s.cancel = nil
		s.state.Status = "paused"
		s.state.Phase = "paused"
		close(s.done)
		s.mu.Unlock()
		return errors.New("Suchfortschritt konnte nicht gespeichert werden; keine Suche gestartet")
	}
	go s.run(ctx, cancel, resume)
	return nil
}
func (s *Scanner) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
}
func (s *Scanner) Wait(ctx context.Context) error {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		s.mu.Lock()
		failed := s.saveError
		s.mu.Unlock()
		if failed {
			return errors.New("Suchfortschritt konnte nicht dauerhaft gespeichert werden")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Scanner) phase(value string) {
	s.mu.Lock()
	s.state.Phase = value
	s.revision++
	s.mu.Unlock()
}
func (s *Scanner) completeLocked(i int) {
	s.revision++
	p := s.progress[i]
	if p&addressDone == 0 && p&fastDone != 0 && (p&retryNeeded == 0 || p&slowDone != 0) && s.pending[i] == 0 {
		s.progress[i] |= addressDone
		s.state.Checked++
		s.state.Resumable = s.state.Checked < s.state.Total
	}
}
func (s *Scanner) run(ctx context.Context, cancel context.CancelFunc, resume bool) {
	saverDone := make(chan struct{})
	var saver sync.WaitGroup
	saver.Add(1)
	go func() {
		defer saver.Done()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if s.persist() != nil {
					return
				}
			case <-saverDone:
				return
			}
		}
	}()
	if !resume && s.Seeds != nil {
		hints, warnings := s.Seeds(ctx, s.state.Config)
		s.mu.Lock()
		allowed := map[string]bool{}
		for _, a := range s.addresses {
			allowed[a] = true
		}
		seen := map[string]bool{}
		for _, h := range hints {
			if !allowed[h.Address] || seen[h.Address] {
				continue
			}
			if h.Source != "known" && h.Source != "cluster" && h.Source != "pbs-config" {
				continue
			}
			h.Name = bounded(h.Name, 256)
			s.hints = append(s.hints, h)
			seen[h.Address] = true
		}
		if len(warnings) > 10 {
			warnings = warnings[:10]
		}
		s.state.Warnings = append([]string{}, warnings...)
		s.revision++
		s.mu.Unlock()
	}
	s.scan(ctx)
	close(saverDone)
	saver.Wait()
	s.mu.Lock()
	if s.state.Checked == s.state.Total {
		s.state.Status = "completed"
		s.state.Phase = "completed"
		s.state.Resumable = false
	} else {
		s.state.Status = "cancelled"
		s.state.Phase = "paused"
		s.state.Resumable = true
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			s.state.Status = "paused"
			s.state.Error = "Zeitfenster erreicht; Fortschritt ist gespeichert. Mit Fortsetzen nur offene Adressen prüfen."
		}
	}
	s.revision++
	s.mu.Unlock()
	_ = s.persist()
	cancel()
	s.mu.Lock()
	if s.saveError {
		s.state.Status = "paused"
		s.state.Resumable = s.state.Checked < s.state.Total
	}
	s.cancel = nil
	close(s.done)
	s.mu.Unlock()
}
func bounded(v string, n int) string {
	if len(v) > n {
		return v[:n]
	}
	return v
}

// An array, rather than an IP cursor, preserves out-of-order completed work.
func (s *Scanner) scan(ctx context.Context) {
	c := s.state.Config
	if s.Probe != nil {
		s.legacyScan(ctx, c)
		return
	}
	tcp := s.TCPProbe
	if tcp == nil {
		tcp = probeTCP
	}
	identify := s.Identify
	transport := probeTransport()
	defer transport.CloseIdleConnections()
	if identify == nil {
		identify = func(ctx context.Context, a string, p, ssh int) (*Result, error) {
			return identifyHost(ctx, a, p, ssh, transport), nil
		}
	}
	type endpoint struct{ i, port, bit int }
	endpoints := make(chan endpoint, 32)
	var identifiers sync.WaitGroup
	for n := 0; n < 16; n++ {
		identifiers.Add(1)
		go func() {
			defer identifiers.Done()
			for e := range endpoints {
				if ctx.Err() != nil {
					return
				}
				result, _ := identify(ctx, s.addresses[e.i], e.port, c.SSHPort)
				if ctx.Err() != nil {
					continue
				}
				s.mu.Lock()
				s.revision++
				s.pending[e.i] &^= byte(e.bit)
				s.verified[e.i] |= byte(e.bit)
				if result != nil {
					s.addResultLocked(e.i, e.port, *result)
				}
				s.completeLocked(e.i)
				s.mu.Unlock()
			}
		}()
	}
	enqueue := func(i int, bits byte) bool {
		for bit, port := range []int{c.WebPort, c.PBSPort} {
			mask := 1 << bit
			if bits&byte(mask) == 0 {
				continue
			}
			select {
			case endpoints <- endpoint{i, port, mask}:
			case <-ctx.Done():
				return false
			}
		}
		return true
	}
	s.mu.Lock()
	pending := append([]byte{}, s.pending...)
	s.mu.Unlock()
	for i, bits := range pending {
		if bits != 0 {
			if !enqueue(i, bits) {
				break
			}
		}
	}
	runTCP := func(slow bool) {
		if slow {
			s.phase("slow")
		} else {
			s.phase("fast")
		}
		jobs := make(chan int)
		var workers sync.WaitGroup
		for n := 0; n < c.Parallel; n++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for i := range jobs {
					if ctx.Err() != nil {
						return
					}
					timeout := 350 * time.Millisecond
					if slow {
						timeout = 2 * time.Second
					}
					// Exactly two bounded port attempts per address; no TLS work in this pool.
					type verdict struct {
						bit         int
						open, retry bool
					}
					answers := make(chan verdict, 2)
					for bit, port := range []int{c.WebPort, c.PBSPort} {
						go func(bit, port int) {
							open, retry := tcp(ctx, s.addresses[i], port, timeout)
							answers <- verdict{1 << bit, open, retry}
						}(bit, port)
					}
					open, retry := byte(0), false
					for n := 0; n < 2; n++ {
						v := <-answers
						if v.open {
							open |= byte(v.bit)
						}
						retry = retry || v.retry
					}
					if ctx.Err() != nil {
						continue
					}
					s.mu.Lock()
					if slow {
						s.progress[i] |= slowDone
					} else {
						s.progress[i] |= fastDone
						s.state.TCPChecked++
						if retry {
							s.progress[i] |= retryNeeded
						}
					}
					open &^= s.verified[i] | s.pending[i]
					s.pending[i] |= open
					s.state.Candidates += bitsCount(open)
					s.completeLocked(i)
					s.mu.Unlock()
					if !enqueue(i, open) {
						return
					}
				}
			}()
		}
		var tick <-chan time.Time
		var ticker *time.Ticker
		if !s.Unpaced {
			ticker = time.NewTicker(time.Second / 256)
			tick = ticker.C
			defer ticker.Stop()
		}
		queue := newQueue(s.addresses, s.hints)
		for {
			s.mu.Lock()
			i := queue.next(s.progress, slow, s.state.Results)
			s.mu.Unlock()
			if i < 0 {
				break
			}
			if tick != nil {
				select {
				case <-tick:
				case <-ctx.Done():
					break
				}
			}
			if ctx.Err() != nil {
				break
			}
			select {
			case jobs <- i:
			case <-ctx.Done():
			}
			if ctx.Err() != nil {
				break
			}
		}
		close(jobs)
		workers.Wait()
	}
	runTCP(false)
	if ctx.Err() == nil {
		runTCP(true)
	}
	s.phase("confirm")
	close(endpoints)
	identifiers.Wait()
}
func bitsCount(v byte) int {
	n := 0
	for ; v > 0; v >>= 1 {
		n += int(v & 1)
	}
	return n
}
func (s *Scanner) addResultLocked(i, port int, r Result) {
	r.Address = s.addresses[i]
	r.WebPort = port
	if r.Kind == "" {
		r.Kind = "pve"
	}
	if r.Kind != "pve" && r.Kind != "pbs" {
		return
	}
	r.Name = bounded(r.Name, 256)
	r.Version = bounded(r.Version, 100)
	for _, old := range s.state.Results {
		if old.Address == r.Address && old.WebPort == port {
			return
		}
	}
	r.Source = "network"
	for _, h := range s.hints {
		if h.Address == r.Address {
			r.Source = h.Source
			if r.Name == "" {
				r.Name = h.Name
			}
			break
		}
	}
	s.state.Results = append(s.state.Results, r)
}
func (s *Scanner) legacyScan(ctx context.Context, c Config) {
	s.phase("fast")
	jobs := make(chan int)
	var workers sync.WaitGroup
	for n := 0; n < 64; n++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				r := s.Probe(ctx, s.addresses[i], c)
				if ctx.Err() != nil {
					continue
				}
				s.mu.Lock()
				s.revision++
				s.progress[i] = fastDone
				s.state.TCPChecked++
				s.verified[i] |= 1
				if r != nil {
					s.addResultLocked(i, c.WebPort, *r)
				}
				s.completeLocked(i)
				s.mu.Unlock()
			}
		}()
	}
	for i := range s.addresses {
		s.mu.Lock()
		done := s.progress[i]&addressDone != 0
		s.mu.Unlock()
		if done {
			continue
		}
		select {
		case jobs <- i:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(jobs)
	workers.Wait()
}
