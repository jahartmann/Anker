package discovery

import "net/netip"

// Spread preferred host addresses across all /24-sized portions first. A hit
// increases its address pattern's priority, but never removes other addresses.
type addressQueue struct {
	buckets  [256][]int
	cursor   [256]int
	scores   [256]int
	used     []bool
	seeds    []int
	observed int
}

func addressPattern(address string) int {
	ip, err := netip.ParseAddr(address)
	if err != nil {
		return 0
	}
	raw := ip.As16()
	return int(raw[15])
}
func newQueue(addresses []string, hints []Hint) *addressQueue {
	q := &addressQueue{used: make([]bool, len(addresses))}
	q.scores[1] = 5
	q.scores[2] = 4
	q.scores[10] = 3
	q.scores[100] = 2
	q.scores[254] = 1
	indices := map[string]int{}
	for i, a := range addresses {
		indices[a] = i
		p := addressPattern(a)
		q.buckets[p] = append(q.buckets[p], i)
	}
	for _, hint := range hints {
		if i, ok := indices[hint.Address]; ok {
			q.seeds = append(q.seeds, i)
			q.scores[addressPattern(hint.Address)] += 20
		}
	}
	return q
}
func (q *addressQueue) next(progress []byte, slow bool, results []Result) int {
	for q.observed < len(results) {
		q.scores[addressPattern(results[q.observed].Address)] += 20
		q.observed++
	}
	eligible := func(i int) bool {
		if q.used[i] || progress[i]&addressDone != 0 {
			return false
		}
		if slow {
			return progress[i]&retryNeeded != 0 && progress[i]&slowDone == 0
		}
		return progress[i]&fastDone == 0
	}
	for len(q.seeds) > 0 {
		i := q.seeds[0]
		q.seeds = q.seeds[1:]
		if eligible(i) {
			q.used[i] = true
			return i
		}
	}
	best := -1
	for p := 0; p < 256; p++ {
		for q.cursor[p] < len(q.buckets[p]) && !eligible(q.buckets[p][q.cursor[p]]) {
			q.cursor[p]++
		}
		if q.cursor[p] < len(q.buckets[p]) && (best < 0 || q.scores[p] > q.scores[best]) {
			best = p
		}
	}
	if best < 0 {
		return -1
	}
	i := q.buckets[best][q.cursor[best]]
	q.cursor[best]++
	q.used[i] = true
	return i
}
