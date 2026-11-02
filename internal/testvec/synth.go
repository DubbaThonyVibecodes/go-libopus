package testvec

import (
	"fmt"
	"strconv"
	"strings"
)

// Synth renders a deterministic synthetic test signal using integer
// arithmetic only, so the corpus generator and the tests (possibly on a
// different architecture) produce bit-identical input.
//
// spec is "kind" or "kind:param".  Output is interleaved int16 with ch
// channels, n = rate*secs frames.  Channels beyond the first are derived
// from the base signal with channel-dependent delays and gains so that
// stereo and multistream paths see genuinely different channels.
func Synth(spec string, rate, ch int, secs float64) ([]int16, error) {
	kind, param, _ := strings.Cut(spec, ":")
	p := 0
	if param != "" {
		v, err := strconv.Atoi(param)
		if err != nil {
			return nil, fmt.Errorf("synth %q: bad param: %w", spec, err)
		}
		p = v
	}
	n := int(float64(rate)*secs + 0.5)
	g := &synthGen{rate: rate, n: n, p: p}
	if !g.known(kind) {
		return nil, fmt.Errorf("synth: unknown kind %q", kind)
	}
	base := make([][]int32, ch)
	for c := 0; c < ch; c++ {
		base[c] = g.render(kind, c)
	}
	out := make([]int16, n*ch)
	noise := LCG{State: 777}
	// Tiny independent noise on derived channels (±4 LSB) keeps them from
	// being exact scaled copies; silence stays exactly silent.
	dither := func() int32 {
		if kind == "silence" {
			return 0
		}
		return int32(noise.Next()&0x7) - 4
	}
	for i := 0; i < n; i++ {
		for c := 0; c < ch; c++ {
			var v int32
			switch {
			case c == 0:
				v = base[0][i]
			case ch == 2:
				// Right: same signal delayed 7 samples at 0.7 gain, plus a
				// touch of independent noise.
				j := i - 7
				if j < 0 {
					j = 0
				}
				v = base[0][j]*7/10 + dither()
			default:
				j := i - 11*c
				if j < 0 {
					j = 0
				}
				v = base[c][j]*int32(10-c)/10 + dither()
			}
			out[i*ch+c] = sat16(v)
		}
	}
	return out, nil
}

func sat16(v int32) int16 {
	if v > 32767 {
		return 32767
	}
	if v < -32768 {
		return -32768
	}
	return int16(v)
}

type synthGen struct {
	rate, n, p int
}

func (g *synthGen) known(kind string) bool {
	switch kind {
	case "silence", "dc", "sine", "multitone", "sweep", "noise", "pink", "impulses", "square", "clip", "bursts", "lowlevel", "fade":
		return true
	}
	return false
}

// sinQ15 returns sin(phase) in Q15 for a 32-bit phase (full turn = 2^32).
func sinQ15(phase uint32) int32 {
	idx := phase >> 20 // 12 bits: 4096 steps per turn
	quad := idx >> 10
	i := idx & 1023
	switch quad {
	case 0:
		return qsin[i]
	case 1:
		return qsin[1024-i]
	case 2:
		return -qsin[i]
	default:
		return -qsin[1024-i]
	}
}

// phaseIncMilli returns the per-sample phase increment for a frequency given in milli-Hz.
func (g *synthGen) phaseIncMilli(fMilli int64) uint32 {
	return uint32((fMilli << 32) / (int64(g.rate) * 1000))
}

func (g *synthGen) render(kind string, c int) []int32 {
	out := make([]int32, g.n)
	rng := LCG{State: uint32(12345 + 17*c)}
	noise := func() int32 { return int32(rng.Next()&0xFFFF) - 32768 }
	var ph0, ph1, ph2, ph3 uint32
	var b0, b1, b2 int64
	switch kind {
	case "silence":
	case "dc":
		for i := range out {
			out[i] = 8192
		}
	case "sine":
		f := g.p
		if f == 0 {
			f = 440
		}
		inc := g.phaseIncMilli(int64(f) * (1000 + 10*int64(c)))
		for i := range out {
			out[i] = sinQ15(ph0) / 2
			ph0 += inc
		}
	case "multitone":
		i0, i1, i2, i3 := g.phaseIncMilli(220000), g.phaseIncMilli(1375000), g.phaseIncMilli(6100000), g.phaseIncMilli(15000000)
		ph1 = uint32(c) * 0x20000000
		for i := range out {
			out[i] = sinQ15(ph0)/5 + sinQ15(ph1)*3/20 + sinQ15(ph2)/10 + sinQ15(ph3)/20
			ph0 += i0
			ph1 += i1
			ph2 += i2
			ph3 += i3
		}
	case "sweep":
		// Geometric sweep 20 Hz .. 0.95*Nyquist: the frequency is multiplied
		// by a constant Q24 factor every 256 samples.  The factor is found by
		// integer bisection so that the last step lands near the top.
		fLo := int64(20000)
		fHi := int64(g.rate) * 950 / 2
		steps := int64(g.n/256 + 1)
		lo, hi := int64(1<<24), int64(2<<24)
		for it := 0; it < 40; it++ {
			mid := (lo + hi) / 2
			f := fLo
			for s := int64(0); s < steps; s++ {
				f = f * mid >> 24
				if f > fHi*2 {
					break
				}
			}
			if f < fHi {
				lo = mid
			} else {
				hi = mid
			}
		}
		r := lo
		f := fLo
		ph0 = uint32(c) * 0x10000000
		for i := range out {
			if i > 0 && i%256 == 0 {
				f = f * r >> 24
				if f > fHi {
					f = fHi
				}
			}
			out[i] = sinQ15(ph0) / 2
			ph0 += g.phaseIncMilli(f)
		}
	case "noise":
		for i := range out {
			out[i] = noise() / 2
		}
	case "pink":
		// Paul Kellet's economy pink filter with Q16 coefficients.
		for i := range out {
			w := int64(noise())
			b0 = (b0*65382 + w*6491) >> 16
			b1 = (b1*63111 + w*19433) >> 16
			b2 = (b2*37355 + w*68989) >> 16
			out[i] = int32((b0 + b1 + b2 + (w*12113)>>16) / 7)
		}
	case "impulses":
		per := g.p
		if per == 0 {
			per = 4
		}
		period := g.rate / per
		for i := range out {
			if i%period < 3 {
				out[i] = 29490
			} else {
				out[i] = noise() / 50
			}
		}
	case "square":
		f := g.p
		if f == 0 {
			f = 200
		}
		inc := g.phaseIncMilli(int64(f) * 1000)
		for i := range out {
			if ph0 < 1<<31 {
				out[i] = 13107
			} else {
				out[i] = -13107
			}
			ph0 += inc
		}
	case "clip":
		inc := g.phaseIncMilli(330000)
		for i := range out {
			v := sinQ15(ph0) * 3
			if v > 32767 {
				v = 32767
			} else if v < -32767 {
				v = -32767
			}
			out[i] = v
			ph0 += inc
		}
	case "bursts":
		// Speech-like: harmonic buzz with a slowly wobbling pitch, amplitude
		// modulated at ~4 Hz, plus a little noise.
		envInc := g.phaseIncMilli(4000)
		wobInc := g.phaseIncMilli(500)
		var hph [12]uint32
		for i := range out {
			env := int64(sinQ15(ph0) + 32768)
			f0 := int64(120000) + int64(sinQ15(ph1))*30000/32768
			var buzz int64
			for h := 0; h < 12; h++ {
				buzz += int64(sinQ15(hph[h])) / int64(h+1)
				hph[h] += g.phaseIncMilli(f0 * int64(h+1))
			}
			v := (buzz/4 + int64(noise())/10) * env >> 16
			out[i] = int32(v)
			ph0 += envInc
			ph1 += wobInc
		}
	case "lowlevel":
		inc := g.phaseIncMilli(1000000)
		for i := range out {
			out[i] = sinQ15(ph0) / 1000
			ph0 += inc
		}
	case "fade":
		inc := g.phaseIncMilli(880000)
		envInc := uint32((uint64(1) << 31) / uint64(g.n))
		for i := range out {
			e := int64(sinQ15(ph1))
			v := int64(sinQ15(ph0)) * e * e >> 30
			out[i] = int32(v * 8 / 10)
			ph0 += inc
			ph1 += envInc
		}
	}
	return out
}
