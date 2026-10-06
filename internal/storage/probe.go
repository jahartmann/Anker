package storage

import (
	"errors"
	"sync"
	"time"
)

// A disconnected network mount can leave statfs in an uninterruptible syscall.
// Bound both the response time and the number of outstanding kernel calls.
type statResult struct {
	value Stats
	err   error
}
type statCall struct {
	done   chan struct{}
	result statResult
}
type statProbe struct {
	mu      sync.Mutex
	calls   map[string]*statCall
	timeout time.Duration
}

func newStatProbe(timeout time.Duration) *statProbe {
	return &statProbe{calls: map[string]*statCall{}, timeout: timeout}
}

var filesystemProbe = newStatProbe(3 * time.Second)

func (p *statProbe) read(path string, stat func(string) (Stats, error)) (Stats, error) {
	p.mu.Lock()
	call := p.calls[path]
	if call == nil {
		if len(p.calls) >= 32 {
			p.mu.Unlock()
			return Stats{}, errors.New("Zu viele nicht erreichbare Dateisysteme; Mounts prüfen")
		}
		call = &statCall{done: make(chan struct{})}
		p.calls[path] = call
		go func() {
			value, err := stat(path)
			p.mu.Lock()
			call.result = statResult{value, err}
			delete(p.calls, path)
			close(call.done)
			p.mu.Unlock()
		}()
	}
	p.mu.Unlock()
	timer := time.NewTimer(p.timeout)
	defer timer.Stop()
	select {
	case <-call.done:
		return call.result.value, call.result.err
	case <-timer.C:
		return Stats{}, errors.New("Dateisystem antwortet nicht; Mount und Verbindung prüfen")
	}
}
