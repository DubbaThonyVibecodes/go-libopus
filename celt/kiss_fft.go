package celt

// C: celt/kiss_fft.h, celt/kiss_fft.c, celt/_kiss_fft_guts.h
// This code is originally from Mark Borgerding's KISS-FFT but has been
// heavily modified to better suit Opus. Only the static-mode subset is
// ported (no opus_fft_alloc / kf_factor / compute_twiddles, which are
// CUSTOM_MODES only).

// KissFftCpx is the C kiss_fft_cpx; kiss_fft_scalar is opus_int32 in the
// fixed-point build. The fields are exported so that the opus package's
// analysis code can fill FFT buffers.
type KissFftCpx struct {
	R int32
	I int32
}

// kiss_twiddle_scalar is opus_int16.
type kiss_twiddle_cpx struct {
	r int16
	i int16
}

const MAXFACTORS = 8

// C: typedef struct arch_fft_state { int is_supported; void *priv; }
type arch_fft_state struct {
	is_supported int
	priv         any
}

// e.g. an fft of length 128 has 4 factors as far as kissfft is concerned: 4*4*4*2
type kiss_fft_state struct {
	nfft        int
	scale       int16
	scale_shift int
	shift       int
	factors     [2 * MAXFACTORS]int16
	bitrev      []int16
	twiddles    []kiss_twiddle_cpx
	arch_fft    *arch_fft_state
}

// Nfft returns the FFT size (C: st->nfft).
func (st *kiss_fft_state) Nfft() int { return st.nfft }

const (
	SAMP_MAX     = 2147483647
	TWID_MAX     = 32767
	TRIG_UPSCALE = 1
	SAMP_MIN     = -SAMP_MAX
)

// C: S_MUL(a,b) MULT16_32_Q15(b, a)
func S_MUL(a int32, b int16) int32 { return MULT16_32_Q15(b, a) }

// C: C_MUL(m,a,b)
func C_MUL(a KissFftCpx, b kiss_twiddle_cpx) (m KissFftCpx) {
	m.R = SUB32(S_MUL(a.R, b.r), S_MUL(a.I, b.i))
	m.I = ADD32(S_MUL(a.R, b.i), S_MUL(a.I, b.r))
	return
}

// C: C_MULC(m,a,b)
func C_MULC(a KissFftCpx, b kiss_twiddle_cpx) (m KissFftCpx) {
	m.R = ADD32(S_MUL(a.R, b.r), S_MUL(a.I, b.i))
	m.I = SUB32(S_MUL(a.I, b.r), S_MUL(a.R, b.i))
	return
}

// C: C_MULBYSCALAR( c, s )
func C_MULBYSCALAR(c *KissFftCpx, s int16) {
	c.R = S_MUL(c.R, s)
	c.I = S_MUL(c.I, s)
}

// C: DIVSCALAR(x,k) (x) = S_MUL( x, (TWID_MAX-((k)>>1))/(k)+1 )
func DIVSCALAR(x int32, k int) int32 { return S_MUL(x, int16((TWID_MAX-(k>>1))/k+1)) }

// C: C_FIXDIV(c,div)
func C_FIXDIV(c *KissFftCpx, div int) {
	c.R = DIVSCALAR(c.R, div)
	c.I = DIVSCALAR(c.I, div)
}

// C: C_ADD( res, a,b)
func C_ADD(a, b KissFftCpx) (res KissFftCpx) {
	res.R = ADD32(a.R, b.R)
	res.I = ADD32(a.I, b.I)
	return
}

// C: C_SUB( res, a,b)
func C_SUB(a, b KissFftCpx) (res KissFftCpx) {
	res.R = SUB32(a.R, b.R)
	res.I = SUB32(a.I, b.I)
	return
}

// C: C_ADDTO( res , a)
func C_ADDTO(res *KissFftCpx, a KissFftCpx) {
	res.R = ADD32(res.R, a.R)
	res.I = ADD32(res.I, a.I)
}

// C: C_SUBFROM( res , a)
func C_SUBFROM(res *KissFftCpx, a KissFftCpx) {
	res.R = ADD32(res.R, a.R)
	res.I = SUB32(res.I, a.I)
}

// C: HALF_OF(x) ((x)>>1)
func HALF_OF(x int32) int32 { return x >> 1 }

// C: static void kf_bfly2(kiss_fft_cpx * Fout, int m, int N)
func kf_bfly2(Fout []KissFftCpx, m int, N int) {
	var Fout2 []KissFftCpx
	var i int
	_ = m
	{
		var tw int16
		tw = QCONST16(0.7071067812, 15)
		// We know that m==4 here because the radix-2 is just after a radix-4
		celt_assert(m == 4)
		fo := 0
		for i = 0; i < N; i++ {
			var t KissFftCpx
			Fout2 = Fout[fo+4:]
			t = Fout2[0]
			Fout2[0] = C_SUB(Fout[fo+0], t)
			C_ADDTO(&Fout[fo+0], t)

			t.R = S_MUL(Fout2[1].R+Fout2[1].I, tw)
			t.I = S_MUL(Fout2[1].I-Fout2[1].R, tw)
			Fout2[1] = C_SUB(Fout[fo+1], t)
			C_ADDTO(&Fout[fo+1], t)

			t.R = Fout2[2].I
			t.I = -Fout2[2].R
			Fout2[2] = C_SUB(Fout[fo+2], t)
			C_ADDTO(&Fout[fo+2], t)

			t.R = S_MUL(Fout2[3].I-Fout2[3].R, tw)
			t.I = S_MUL(-Fout2[3].I-Fout2[3].R, tw)
			Fout2[3] = C_SUB(Fout[fo+3], t)
			C_ADDTO(&Fout[fo+3], t)
			fo += 8
		}
	}
}

// C: static void kf_bfly4(kiss_fft_cpx * Fout, const size_t fstride, const kiss_fft_state *st, int m, int N, int mm)
func kf_bfly4(Fout_beg []KissFftCpx, fstride int, st *kiss_fft_state, m, N, mm int) {
	var i int

	if m == 1 {
		// Degenerate case where all the twiddles are 1.
		fo := 0
		for i = 0; i < N; i++ {
			var scratch0, scratch1 KissFftCpx
			Fout := Fout_beg[fo:]

			scratch0 = C_SUB(Fout[0], Fout[2])
			C_ADDTO(&Fout[0], Fout[2])
			scratch1 = C_ADD(Fout[1], Fout[3])
			Fout[2] = C_SUB(Fout[0], scratch1)
			C_ADDTO(&Fout[0], scratch1)
			scratch1 = C_SUB(Fout[1], Fout[3])

			Fout[1].R = scratch0.R + scratch1.I
			Fout[1].I = scratch0.I - scratch1.R
			Fout[3].R = scratch0.R - scratch1.I
			Fout[3].I = scratch0.I + scratch1.R
			fo += 4
		}
	} else {
		var j int
		var scratch [6]KissFftCpx
		m2 := 2 * m
		m3 := 3 * m
		for i = 0; i < N; i++ {
			Fout := Fout_beg[i*mm:]
			tw1, tw2, tw3 := 0, 0, 0
			// m is guaranteed to be a multiple of 4.
			for j = 0; j < m; j++ {
				scratch[0] = C_MUL(Fout[j+m], st.twiddles[tw1])
				scratch[1] = C_MUL(Fout[j+m2], st.twiddles[tw2])
				scratch[2] = C_MUL(Fout[j+m3], st.twiddles[tw3])

				scratch[5] = C_SUB(Fout[j], scratch[1])
				C_ADDTO(&Fout[j], scratch[1])
				scratch[3] = C_ADD(scratch[0], scratch[2])
				scratch[4] = C_SUB(scratch[0], scratch[2])
				Fout[j+m2] = C_SUB(Fout[j], scratch[3])
				tw1 += fstride
				tw2 += fstride * 2
				tw3 += fstride * 3
				C_ADDTO(&Fout[j], scratch[3])

				Fout[j+m].R = scratch[5].R + scratch[4].I
				Fout[j+m].I = scratch[5].I - scratch[4].R
				Fout[j+m3].R = scratch[5].R - scratch[4].I
				Fout[j+m3].I = scratch[5].I + scratch[4].R
			}
		}
	}
}

// C: static void kf_bfly3(kiss_fft_cpx * Fout, const size_t fstride, const kiss_fft_state *st, int m, int N, int mm)
func kf_bfly3(Fout_beg []KissFftCpx, fstride int, st *kiss_fft_state, m, N, mm int) {
	var i int
	var k int
	m2 := 2 * m
	var scratch [5]KissFftCpx
	var epi3 kiss_twiddle_cpx

	// epi3.r = -16384; // Unused
	epi3.i = -28378
	for i = 0; i < N; i++ {
		Fout := Fout_beg[i*mm:]
		tw1, tw2 := 0, 0
		// For non-custom modes, m is guaranteed to be a multiple of 4.
		k = m
		j := 0
		for {
			scratch[1] = C_MUL(Fout[j+m], st.twiddles[tw1])
			scratch[2] = C_MUL(Fout[j+m2], st.twiddles[tw2])

			scratch[3] = C_ADD(scratch[1], scratch[2])
			scratch[0] = C_SUB(scratch[1], scratch[2])
			tw1 += fstride
			tw2 += fstride * 2

			Fout[j+m].R = Fout[j].R - HALF_OF(scratch[3].R)
			Fout[j+m].I = Fout[j].I - HALF_OF(scratch[3].I)

			C_MULBYSCALAR(&scratch[0], epi3.i)

			C_ADDTO(&Fout[j], scratch[3])

			Fout[j+m2].R = Fout[j+m].R + scratch[0].I
			Fout[j+m2].I = Fout[j+m].I - scratch[0].R

			Fout[j+m].R -= scratch[0].I
			Fout[j+m].I += scratch[0].R

			j++
			k--
			if k == 0 {
				break
			}
		}
	}
}

// C: static void kf_bfly5(kiss_fft_cpx * Fout, const size_t fstride, const kiss_fft_state *st, int m, int N, int mm)
func kf_bfly5(Fout_beg []KissFftCpx, fstride int, st *kiss_fft_state, m, N, mm int) {
	var i, u int
	var scratch [13]KissFftCpx
	var tw []kiss_twiddle_cpx
	var ya, yb kiss_twiddle_cpx

	ya.r = 10126
	ya.i = -31164
	yb.r = -26510
	yb.i = -19261
	tw = st.twiddles

	for i = 0; i < N; i++ {
		Fout := Fout_beg[i*mm:]
		Fout0 := Fout
		Fout1 := Fout[m:]
		Fout2 := Fout[2*m:]
		Fout3 := Fout[3*m:]
		Fout4 := Fout[4*m:]

		// For non-custom modes, m is guaranteed to be a multiple of 4.
		for u = 0; u < m; u++ {
			scratch[0] = Fout0[u]

			scratch[1] = C_MUL(Fout1[u], tw[u*fstride])
			scratch[2] = C_MUL(Fout2[u], tw[2*u*fstride])
			scratch[3] = C_MUL(Fout3[u], tw[3*u*fstride])
			scratch[4] = C_MUL(Fout4[u], tw[4*u*fstride])

			scratch[7] = C_ADD(scratch[1], scratch[4])
			scratch[10] = C_SUB(scratch[1], scratch[4])
			scratch[8] = C_ADD(scratch[2], scratch[3])
			scratch[9] = C_SUB(scratch[2], scratch[3])

			Fout0[u].R += scratch[7].R + scratch[8].R
			Fout0[u].I += scratch[7].I + scratch[8].I

			scratch[5].R = scratch[0].R + S_MUL(scratch[7].R, ya.r) + S_MUL(scratch[8].R, yb.r)
			scratch[5].I = scratch[0].I + S_MUL(scratch[7].I, ya.r) + S_MUL(scratch[8].I, yb.r)

			scratch[6].R = S_MUL(scratch[10].I, ya.i) + S_MUL(scratch[9].I, yb.i)
			scratch[6].I = -S_MUL(scratch[10].R, ya.i) - S_MUL(scratch[9].R, yb.i)

			Fout1[u] = C_SUB(scratch[5], scratch[6])
			Fout4[u] = C_ADD(scratch[5], scratch[6])

			scratch[11].R = scratch[0].R + S_MUL(scratch[7].R, yb.r) + S_MUL(scratch[8].R, ya.r)
			scratch[11].I = scratch[0].I + S_MUL(scratch[7].I, yb.r) + S_MUL(scratch[8].I, ya.r)
			scratch[12].R = -S_MUL(scratch[10].I, yb.i) + S_MUL(scratch[9].I, ya.i)
			scratch[12].I = S_MUL(scratch[10].R, yb.i) - S_MUL(scratch[9].R, ya.i)

			Fout2[u] = C_ADD(scratch[11], scratch[12])
			Fout3[u] = C_SUB(scratch[11], scratch[12])
		}
	}
}

// C: int opus_fft_alloc_arch_c(kiss_fft_state *st)
func opus_fft_alloc_arch_c(st *kiss_fft_state) int {
	_ = st
	return 0
}

// C: void opus_fft_free_arch_c(kiss_fft_state *st)
func opus_fft_free_arch_c(st *kiss_fft_state) {
	_ = st
}

// FftImpl runs the butterflies in place on an already bit-reversed buffer.
//
// C: void opus_fft_impl(const kiss_fft_state *st,kiss_fft_cpx *fout)
func FftImpl(st *kiss_fft_state, fout []KissFftCpx) {
	var m2, m int
	var p int
	var L int
	var fstride [MAXFACTORS]int
	var i int
	var shift int

	// st->shift can be -1
	if st.shift > 0 {
		shift = st.shift
	} else {
		shift = 0
	}

	fstride[0] = 1
	L = 0
	for {
		p = int(st.factors[2*L])
		m = int(st.factors[2*L+1])
		fstride[L+1] = fstride[L] * p
		L++
		if m == 1 {
			break
		}
	}
	m = int(st.factors[2*L-1])
	for i = L - 1; i >= 0; i-- {
		if i != 0 {
			m2 = int(st.factors[2*i-1])
		} else {
			m2 = 1
		}
		switch st.factors[2*i] {
		case 2:
			kf_bfly2(fout, m, fstride[i])
		case 4:
			kf_bfly4(fout, fstride[i]<<uint(shift), st, m, fstride[i], m2)
		case 3:
			kf_bfly3(fout, fstride[i]<<uint(shift), st, m, fstride[i], m2)
		case 5:
			kf_bfly5(fout, fstride[i]<<uint(shift), st, m, fstride[i], m2)
		}
		m = m2
	}
}

// Fft performs a forward FFT on a complex input buffer (fin must not alias fout).
//
// C: void opus_fft_c(const kiss_fft_state *st,const kiss_fft_cpx *fin,kiss_fft_cpx *fout)
// C: #define opus_fft(_cfg, _fin, _fout, arch) ((void)(arch), opus_fft_c(_cfg, _fin, _fout))
func Fft(st *kiss_fft_state, fin, fout []KissFftCpx, arch int) {
	_ = arch
	var i int
	var scale int16
	// Allows us to scale with MULT16_32_Q16(), which is faster than
	// MULT16_32_Q15() on ARM.
	var scale_shift int = st.scale_shift - 1
	scale = st.scale

	celt_assert2(&fin[0] != &fout[0], "In-place FFT not supported")
	// Bit-reverse the input
	for i = 0; i < st.nfft; i++ {
		var x KissFftCpx = fin[i]
		fout[st.bitrev[i]].R = SHR32(MULT16_32_Q16(scale, x.R), scale_shift)
		fout[st.bitrev[i]].I = SHR32(MULT16_32_Q16(scale, x.I), scale_shift)
	}
	FftImpl(st, fout)
}

// IfftImpl is the inverse counterpart of FftImpl.
//
// C: void opus_ifft_impl(const kiss_fft_state *st,kiss_fft_cpx *fout)
//
// Upstream 1.1.5 declares opus_ifft_impl in kiss_fft.h but does not define
// it (opus_ifft_c conjugates around opus_fft_impl instead); this does the
// same so that the declared symbol exists.
func IfftImpl(st *kiss_fft_state, fout []KissFftCpx) {
	var i int
	for i = 0; i < st.nfft; i++ {
		fout[i].I = -fout[i].I
	}
	FftImpl(st, fout)
	for i = 0; i < st.nfft; i++ {
		fout[i].I = -fout[i].I
	}
}

// Ifft performs an inverse FFT on a complex input buffer (fin must not alias fout).
//
// C: void opus_ifft_c(const kiss_fft_state *st,const kiss_fft_cpx *fin,kiss_fft_cpx *fout)
// C: #define opus_ifft(_cfg, _fin, _fout, arch) ((void)(arch), opus_ifft_c(_cfg, _fin, _fout))
func Ifft(st *kiss_fft_state, fin, fout []KissFftCpx, arch int) {
	_ = arch
	var i int
	celt_assert2(&fin[0] != &fout[0], "In-place FFT not supported")
	// Bit-reverse the input
	for i = 0; i < st.nfft; i++ {
		fout[st.bitrev[i]] = fin[i]
	}
	for i = 0; i < st.nfft; i++ {
		fout[i].I = -fout[i].I
	}
	FftImpl(st, fout)
	for i = 0; i < st.nfft; i++ {
		fout[i].I = -fout[i].I
	}
}
