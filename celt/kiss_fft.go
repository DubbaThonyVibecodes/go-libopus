package celt

// C: celt/kiss_fft.h, celt/kiss_fft.c, celt/_kiss_fft_guts.h
// This code is originally from Mark Borgerding's KISS-FFT but has been
// heavily modified to better suit Opus. Only the static-mode subset is
// ported (no opus_fft_alloc / kf_factor / compute_twiddles).

// kiss_fft_scalar is opus_int32 in the fixed-point build; kiss_twiddle_scalar is opus_int16.
type kiss_fft_cpx struct {
	r int32
	i int32
}

type kiss_twiddle_cpx struct {
	r int16
	i int16
}

const MAXFACTORS = 8

// e.g. an fft of length 128 has 4 factors as far as kissfft is concerned: 4*4*4*2
type kiss_fft_state struct {
	nfft     int
	shift    int
	factors  [2 * MAXFACTORS]int16
	bitrev   []int16
	twiddles []kiss_twiddle_cpx
}

const (
	SAMP_MAX     = 2147483647
	TWID_MAX     = 32767
	TRIG_UPSCALE = 1
	SAMP_MIN     = -SAMP_MAX
)

// C: S_MUL(a,b) MULT16_32_Q15(b, a)
func S_MUL(a int32, b int16) int32 { return MULT16_32_Q15(b, a) }

// C: C_MUL(m,a,b)
func C_MUL(a kiss_fft_cpx, b kiss_twiddle_cpx) (m kiss_fft_cpx) {
	m.r = SUB32(S_MUL(a.r, b.r), S_MUL(a.i, b.i))
	m.i = ADD32(S_MUL(a.r, b.i), S_MUL(a.i, b.r))
	return
}

// C: C_MULC(m,a,b)
func C_MULC(a kiss_fft_cpx, b kiss_twiddle_cpx) (m kiss_fft_cpx) {
	m.r = ADD32(S_MUL(a.r, b.r), S_MUL(a.i, b.i))
	m.i = SUB32(S_MUL(a.i, b.r), S_MUL(a.r, b.i))
	return
}

// C: C_MUL4(m,a,b)
func C_MUL4(a kiss_fft_cpx, b kiss_twiddle_cpx) (m kiss_fft_cpx) {
	m.r = SHR32(SUB32(S_MUL(a.r, b.r), S_MUL(a.i, b.i)), 2)
	m.i = SHR32(ADD32(S_MUL(a.r, b.i), S_MUL(a.i, b.r)), 2)
	return
}

// C: C_MULBYSCALAR( c, s )
func C_MULBYSCALAR(c *kiss_fft_cpx, s int16) {
	c.r = S_MUL(c.r, s)
	c.i = S_MUL(c.i, s)
}

// C: DIVSCALAR(x,k) (x) = S_MUL( x, (TWID_MAX-((k)>>1))/(k)+1 )
func DIVSCALAR(x int32, k int) int32 { return S_MUL(x, int16((TWID_MAX-(k>>1))/k+1)) }

// C: C_FIXDIV(c,div)
func C_FIXDIV(c *kiss_fft_cpx, div int) {
	c.r = DIVSCALAR(c.r, div)
	c.i = DIVSCALAR(c.i, div)
}

// C: C_ADD( res, a,b)
func C_ADD(a, b kiss_fft_cpx) (res kiss_fft_cpx) {
	res.r = ADD32(a.r, b.r)
	res.i = ADD32(a.i, b.i)
	return
}

// C: C_SUB( res, a,b)
func C_SUB(a, b kiss_fft_cpx) (res kiss_fft_cpx) {
	res.r = SUB32(a.r, b.r)
	res.i = SUB32(a.i, b.i)
	return
}

// C: C_ADDTO( res , a)
func C_ADDTO(res *kiss_fft_cpx, a kiss_fft_cpx) {
	res.r = ADD32(res.r, a.r)
	res.i = ADD32(res.i, a.i)
}

// C: HALF_OF(x) ((x)>>1)
func HALF_OF(x int32) int32 { return x >> 1 }

// C: static void kf_bfly2(kiss_fft_cpx * Fout, const size_t fstride, const kiss_fft_state *st, int m, int N, int mm)
func kf_bfly2(Fout_beg []kiss_fft_cpx, fstride int, st *kiss_fft_state, m, N, mm int) {
	var i, j int
	for i = 0; i < N; i++ {
		Fout := Fout_beg[i*mm:]
		Fout2 := Fout[m:]
		tw1 := 0
		for j = 0; j < m; j++ {
			var t kiss_fft_cpx
			Fout[j].r = SHR32(Fout[j].r, 1)
			Fout[j].i = SHR32(Fout[j].i, 1)
			Fout2[j].r = SHR32(Fout2[j].r, 1)
			Fout2[j].i = SHR32(Fout2[j].i, 1)
			t = C_MUL(Fout2[j], st.twiddles[tw1])
			tw1 += fstride
			Fout2[j] = C_SUB(Fout[j], t)
			C_ADDTO(&Fout[j], t)
		}
	}
}

// C: static void ki_bfly2(kiss_fft_cpx * Fout, const size_t fstride, const kiss_fft_state *st, int m, int N, int mm)
func ki_bfly2(Fout_beg []kiss_fft_cpx, fstride int, st *kiss_fft_state, m, N, mm int) {
	var t kiss_fft_cpx
	var i, j int
	for i = 0; i < N; i++ {
		Fout := Fout_beg[i*mm:]
		Fout2 := Fout[m:]
		tw1 := 0
		for j = 0; j < m; j++ {
			t = C_MULC(Fout2[j], st.twiddles[tw1])
			tw1 += fstride
			Fout2[j] = C_SUB(Fout[j], t)
			C_ADDTO(&Fout[j], t)
		}
	}
}

// C: static void kf_bfly4(kiss_fft_cpx * Fout, const size_t fstride, const kiss_fft_state *st, int m, int N, int mm)
func kf_bfly4(Fout_beg []kiss_fft_cpx, fstride int, st *kiss_fft_state, m, N, mm int) {
	var scratch [6]kiss_fft_cpx
	m2 := 2 * m
	m3 := 3 * m
	var i, j int

	for i = 0; i < N; i++ {
		Fout := Fout_beg[i*mm:]
		tw1, tw2, tw3 := 0, 0, 0
		for j = 0; j < m; j++ {
			scratch[0] = C_MUL4(Fout[j+m], st.twiddles[tw1])
			scratch[1] = C_MUL4(Fout[j+m2], st.twiddles[tw2])
			scratch[2] = C_MUL4(Fout[j+m3], st.twiddles[tw3])

			Fout[j].r = PSHR32(Fout[j].r, 2)
			Fout[j].i = PSHR32(Fout[j].i, 2)
			scratch[5] = C_SUB(Fout[j], scratch[1])
			C_ADDTO(&Fout[j], scratch[1])
			scratch[3] = C_ADD(scratch[0], scratch[2])
			scratch[4] = C_SUB(scratch[0], scratch[2])
			Fout[j+m2].r = PSHR32(Fout[j+m2].r, 2)
			Fout[j+m2].i = PSHR32(Fout[j+m2].i, 2)
			Fout[j+m2] = C_SUB(Fout[j], scratch[3])
			tw1 += fstride
			tw2 += fstride * 2
			tw3 += fstride * 3
			C_ADDTO(&Fout[j], scratch[3])

			Fout[j+m].r = scratch[5].r + scratch[4].i
			Fout[j+m].i = scratch[5].i - scratch[4].r
			Fout[j+m3].r = scratch[5].r - scratch[4].i
			Fout[j+m3].i = scratch[5].i + scratch[4].r
		}
	}
}

// C: static void ki_bfly4(kiss_fft_cpx * Fout, const size_t fstride, const kiss_fft_state *st, int m, int N, int mm)
func ki_bfly4(Fout_beg []kiss_fft_cpx, fstride int, st *kiss_fft_state, m, N, mm int) {
	var scratch [6]kiss_fft_cpx
	m2 := 2 * m
	m3 := 3 * m
	var i, j int

	for i = 0; i < N; i++ {
		Fout := Fout_beg[i*mm:]
		tw1, tw2, tw3 := 0, 0, 0
		for j = 0; j < m; j++ {
			scratch[0] = C_MULC(Fout[j+m], st.twiddles[tw1])
			scratch[1] = C_MULC(Fout[j+m2], st.twiddles[tw2])
			scratch[2] = C_MULC(Fout[j+m3], st.twiddles[tw3])

			scratch[5] = C_SUB(Fout[j], scratch[1])
			C_ADDTO(&Fout[j], scratch[1])
			scratch[3] = C_ADD(scratch[0], scratch[2])
			scratch[4] = C_SUB(scratch[0], scratch[2])
			Fout[j+m2] = C_SUB(Fout[j], scratch[3])
			tw1 += fstride
			tw2 += fstride * 2
			tw3 += fstride * 3
			C_ADDTO(&Fout[j], scratch[3])

			Fout[j+m].r = scratch[5].r - scratch[4].i
			Fout[j+m].i = scratch[5].i + scratch[4].r
			Fout[j+m3].r = scratch[5].r + scratch[4].i
			Fout[j+m3].i = scratch[5].i - scratch[4].r
		}
	}
}

// C: static void kf_bfly3(kiss_fft_cpx * Fout, const size_t fstride, const kiss_fft_state *st, int m, int N, int mm)
func kf_bfly3(Fout_beg []kiss_fft_cpx, fstride int, st *kiss_fft_state, m, N, mm int) {
	var i, k int
	m2 := 2 * m
	var scratch [5]kiss_fft_cpx
	var epi3 kiss_twiddle_cpx

	epi3 = st.twiddles[fstride*m]
	for i = 0; i < N; i++ {
		Fout := Fout_beg[i*mm:]
		tw1, tw2 := 0, 0
		k = m
		j := 0
		for {
			C_FIXDIV(&Fout[j], 3)
			C_FIXDIV(&Fout[j+m], 3)
			C_FIXDIV(&Fout[j+m2], 3)

			scratch[1] = C_MUL(Fout[j+m], st.twiddles[tw1])
			scratch[2] = C_MUL(Fout[j+m2], st.twiddles[tw2])

			scratch[3] = C_ADD(scratch[1], scratch[2])
			scratch[0] = C_SUB(scratch[1], scratch[2])
			tw1 += fstride
			tw2 += fstride * 2

			Fout[j+m].r = Fout[j].r - HALF_OF(scratch[3].r)
			Fout[j+m].i = Fout[j].i - HALF_OF(scratch[3].i)

			C_MULBYSCALAR(&scratch[0], epi3.i)

			C_ADDTO(&Fout[j], scratch[3])

			Fout[j+m2].r = Fout[j+m].r + scratch[0].i
			Fout[j+m2].i = Fout[j+m].i - scratch[0].r

			Fout[j+m].r -= scratch[0].i
			Fout[j+m].i += scratch[0].r

			j++
			k--
			if k == 0 {
				break
			}
		}
	}
}

// C: static void ki_bfly3(kiss_fft_cpx * Fout, const size_t fstride, const kiss_fft_state *st, int m, int N, int mm)
func ki_bfly3(Fout_beg []kiss_fft_cpx, fstride int, st *kiss_fft_state, m, N, mm int) {
	var i, k int
	m2 := 2 * m
	var scratch [5]kiss_fft_cpx
	var epi3 kiss_twiddle_cpx

	epi3 = st.twiddles[fstride*m]
	for i = 0; i < N; i++ {
		Fout := Fout_beg[i*mm:]
		tw1, tw2 := 0, 0
		k = m
		j := 0
		for {
			scratch[1] = C_MULC(Fout[j+m], st.twiddles[tw1])
			scratch[2] = C_MULC(Fout[j+m2], st.twiddles[tw2])

			scratch[3] = C_ADD(scratch[1], scratch[2])
			scratch[0] = C_SUB(scratch[1], scratch[2])
			tw1 += fstride
			tw2 += fstride * 2

			Fout[j+m].r = Fout[j].r - HALF_OF(scratch[3].r)
			Fout[j+m].i = Fout[j].i - HALF_OF(scratch[3].i)

			C_MULBYSCALAR(&scratch[0], -epi3.i)

			C_ADDTO(&Fout[j], scratch[3])

			Fout[j+m2].r = Fout[j+m].r + scratch[0].i
			Fout[j+m2].i = Fout[j+m].i - scratch[0].r

			Fout[j+m].r -= scratch[0].i
			Fout[j+m].i += scratch[0].r

			j++
			k--
			if k == 0 {
				break
			}
		}
	}
}

// C: static void kf_bfly5(kiss_fft_cpx * Fout, const size_t fstride, const kiss_fft_state *st, int m, int N, int mm)
func kf_bfly5(Fout_beg []kiss_fft_cpx, fstride int, st *kiss_fft_state, m, N, mm int) {
	var i, u int
	var scratch [13]kiss_fft_cpx
	twiddles := st.twiddles
	tw := st.twiddles
	var ya, yb kiss_twiddle_cpx

	ya = twiddles[fstride*m]
	yb = twiddles[fstride*2*m]

	for i = 0; i < N; i++ {
		Fout := Fout_beg[i*mm:]
		Fout0 := Fout
		Fout1 := Fout[m:]
		Fout2 := Fout[2*m:]
		Fout3 := Fout[3*m:]
		Fout4 := Fout[4*m:]

		for u = 0; u < m; u++ {
			C_FIXDIV(&Fout0[u], 5)
			C_FIXDIV(&Fout1[u], 5)
			C_FIXDIV(&Fout2[u], 5)
			C_FIXDIV(&Fout3[u], 5)
			C_FIXDIV(&Fout4[u], 5)
			scratch[0] = Fout0[u]

			scratch[1] = C_MUL(Fout1[u], tw[u*fstride])
			scratch[2] = C_MUL(Fout2[u], tw[2*u*fstride])
			scratch[3] = C_MUL(Fout3[u], tw[3*u*fstride])
			scratch[4] = C_MUL(Fout4[u], tw[4*u*fstride])

			scratch[7] = C_ADD(scratch[1], scratch[4])
			scratch[10] = C_SUB(scratch[1], scratch[4])
			scratch[8] = C_ADD(scratch[2], scratch[3])
			scratch[9] = C_SUB(scratch[2], scratch[3])

			Fout0[u].r += scratch[7].r + scratch[8].r
			Fout0[u].i += scratch[7].i + scratch[8].i

			scratch[5].r = scratch[0].r + S_MUL(scratch[7].r, ya.r) + S_MUL(scratch[8].r, yb.r)
			scratch[5].i = scratch[0].i + S_MUL(scratch[7].i, ya.r) + S_MUL(scratch[8].i, yb.r)

			scratch[6].r = S_MUL(scratch[10].i, ya.i) + S_MUL(scratch[9].i, yb.i)
			scratch[6].i = -S_MUL(scratch[10].r, ya.i) - S_MUL(scratch[9].r, yb.i)

			Fout1[u] = C_SUB(scratch[5], scratch[6])
			Fout4[u] = C_ADD(scratch[5], scratch[6])

			scratch[11].r = scratch[0].r + S_MUL(scratch[7].r, yb.r) + S_MUL(scratch[8].r, ya.r)
			scratch[11].i = scratch[0].i + S_MUL(scratch[7].i, yb.r) + S_MUL(scratch[8].i, ya.r)
			scratch[12].r = -S_MUL(scratch[10].i, yb.i) + S_MUL(scratch[9].i, ya.i)
			scratch[12].i = S_MUL(scratch[10].r, yb.i) - S_MUL(scratch[9].r, ya.i)

			Fout2[u] = C_ADD(scratch[11], scratch[12])
			Fout3[u] = C_SUB(scratch[11], scratch[12])
		}
	}
}

// C: static void ki_bfly5(kiss_fft_cpx * Fout, const size_t fstride, const kiss_fft_state *st, int m, int N, int mm)
func ki_bfly5(Fout_beg []kiss_fft_cpx, fstride int, st *kiss_fft_state, m, N, mm int) {
	var i, u int
	var scratch [13]kiss_fft_cpx
	twiddles := st.twiddles
	tw := st.twiddles
	var ya, yb kiss_twiddle_cpx

	ya = twiddles[fstride*m]
	yb = twiddles[fstride*2*m]

	for i = 0; i < N; i++ {
		Fout := Fout_beg[i*mm:]
		Fout0 := Fout
		Fout1 := Fout[m:]
		Fout2 := Fout[2*m:]
		Fout3 := Fout[3*m:]
		Fout4 := Fout[4*m:]

		for u = 0; u < m; u++ {
			scratch[0] = Fout0[u]

			scratch[1] = C_MULC(Fout1[u], tw[u*fstride])
			scratch[2] = C_MULC(Fout2[u], tw[2*u*fstride])
			scratch[3] = C_MULC(Fout3[u], tw[3*u*fstride])
			scratch[4] = C_MULC(Fout4[u], tw[4*u*fstride])

			scratch[7] = C_ADD(scratch[1], scratch[4])
			scratch[10] = C_SUB(scratch[1], scratch[4])
			scratch[8] = C_ADD(scratch[2], scratch[3])
			scratch[9] = C_SUB(scratch[2], scratch[3])

			Fout0[u].r += scratch[7].r + scratch[8].r
			Fout0[u].i += scratch[7].i + scratch[8].i

			scratch[5].r = scratch[0].r + S_MUL(scratch[7].r, ya.r) + S_MUL(scratch[8].r, yb.r)
			scratch[5].i = scratch[0].i + S_MUL(scratch[7].i, ya.r) + S_MUL(scratch[8].i, yb.r)

			scratch[6].r = -S_MUL(scratch[10].i, ya.i) - S_MUL(scratch[9].i, yb.i)
			scratch[6].i = S_MUL(scratch[10].r, ya.i) + S_MUL(scratch[9].r, yb.i)

			Fout1[u] = C_SUB(scratch[5], scratch[6])
			Fout4[u] = C_ADD(scratch[5], scratch[6])

			scratch[11].r = scratch[0].r + S_MUL(scratch[7].r, yb.r) + S_MUL(scratch[8].r, ya.r)
			scratch[11].i = scratch[0].i + S_MUL(scratch[7].i, yb.r) + S_MUL(scratch[8].i, ya.r)
			scratch[12].r = S_MUL(scratch[10].i, yb.i) - S_MUL(scratch[9].i, ya.i)
			scratch[12].i = -S_MUL(scratch[10].r, yb.i) + S_MUL(scratch[9].r, ya.i)

			Fout2[u] = C_ADD(scratch[11], scratch[12])
			Fout3[u] = C_SUB(scratch[11], scratch[12])
		}
	}
}

// Fft performs a forward FFT on a complex input buffer (fin must not alias fout).
//
// C: void opus_fft(const kiss_fft_state *st,const kiss_fft_cpx *fin,kiss_fft_cpx *fout)
func Fft(st *kiss_fft_state, fin, fout []kiss_fft_cpx) {
	var m2, m, p, L int
	var fstride [MAXFACTORS]int
	var i, shift int

	// st->shift can be -1
	if st.shift > 0 {
		shift = st.shift
	} else {
		shift = 0
	}

	celt_assert2(&fin[0] != &fout[0], "In-place FFT not supported")
	// Bit-reverse the input
	for i = 0; i < st.nfft; i++ {
		fout[st.bitrev[i]] = fin[i]
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
			kf_bfly2(fout, fstride[i]<<uint(shift), st, m, fstride[i], m2)
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

// Ifft performs an inverse FFT on a complex input buffer (fin must not alias fout).
//
// C: void opus_ifft(const kiss_fft_state *st,const kiss_fft_cpx *fin,kiss_fft_cpx *fout)
func Ifft(st *kiss_fft_state, fin, fout []kiss_fft_cpx) {
	var m2, m, p, L int
	var fstride [MAXFACTORS]int
	var i, shift int

	// st->shift can be -1
	if st.shift > 0 {
		shift = st.shift
	} else {
		shift = 0
	}
	celt_assert2(&fin[0] != &fout[0], "In-place FFT not supported")
	// Bit-reverse the input
	for i = 0; i < st.nfft; i++ {
		fout[st.bitrev[i]] = fin[i]
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
			ki_bfly2(fout, fstride[i]<<uint(shift), st, m, fstride[i], m2)
		case 4:
			ki_bfly4(fout, fstride[i]<<uint(shift), st, m, fstride[i], m2)
		case 3:
			ki_bfly3(fout, fstride[i]<<uint(shift), st, m, fstride[i], m2)
		case 5:
			ki_bfly5(fout, fstride[i]<<uint(shift), st, m, fstride[i], m2)
		}
		m = m2
	}
}
