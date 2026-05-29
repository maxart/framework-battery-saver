package power

// ring is a fixed-size circular buffer of float64 samples used to back the
// sparkline histories. values() returns them in chronological (oldest-first)
// order.
type ring struct {
	buf   []float64
	head  int
	count int
}

func newRing(size int) *ring {
	if size < 1 {
		size = 1
	}
	return &ring{buf: make([]float64, size)}
}

func (r *ring) push(v float64) {
	r.buf[r.head] = v
	r.head = (r.head + 1) % len(r.buf)
	if r.count < len(r.buf) {
		r.count++
	}
}

func (r *ring) values() []float64 {
	out := make([]float64, r.count)
	start := (r.head - r.count + len(r.buf)) % len(r.buf)
	for i := range r.count {
		out[i] = r.buf[(start+i)%len(r.buf)]
	}
	return out
}
