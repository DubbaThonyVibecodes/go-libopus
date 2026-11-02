package testvec

// LCG is the deterministic generator used by gen.c for every "random"
// decision (packet loss, random frame sizes, random FEC toggling).  The Go
// oracle twin must consume it in exactly the same order as the C driver.
type LCG struct{ State uint32 }

// Next mirrors lcg() in gen.c.
func (l *LCG) Next() uint32 {
	l.State = l.State*1664525 + 1013904223
	return l.State >> 8
}
