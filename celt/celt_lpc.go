package celt

// C: celt/celt_lpc.h, celt/celt_lpc.c

const LPC_ORDER = 24

// Lpc computes LPC coefficients from autocorrelation values.
//
// C: void _celt_lpc(opus_val16 *_lpc, const opus_val32 *ac, int p)
func Lpc(_lpc []int16, ac []int32, p int) {
	var i, j int
	var r int32
	var error int32 = ac[0]
	var lpc [LPC_ORDER]int32

	for i = 0; i < p; i++ {
		lpc[i] = 0
	}
	if ac[0] != 0 {
		for i = 0; i < p; i++ {
			// Sum up this iteration's reflection coefficient
			var rr int32 = 0
			for j = 0; j < i; j++ {
				rr += MULT32_32_Q31(lpc[j], ac[i-j])
			}
			rr += SHR32(ac[i+1], 3)
			r = -FracDiv32(SHL32(rr, 3), error)
			// Update LPC coefficients and total error
			lpc[i] = SHR32(r, 3)
			for j = 0; j < (i+1)>>1; j++ {
				var tmp1, tmp2 int32
				tmp1 = lpc[j]
				tmp2 = lpc[i-1-j]
				lpc[j] = tmp1 + MULT32_32_Q31(r, tmp2)
				lpc[i-1-j] = tmp2 + MULT32_32_Q31(r, tmp1)
			}

			error = error - MULT32_32_Q31(MULT32_32_Q31(r, r), error)
			// Bail out once we get 30 dB gain
			if error < SHR32(ac[0], 10) {
				break
			}
		}
	}
	for i = 0; i < p; i++ {
		_lpc[i] = ROUND16(lpc[i], 16)
	}
}

// C: void celt_fir(const opus_val16 *x, const opus_val16 *num, opus_val16 *y, int N, int ord, opus_val16 *mem)
func Fir(x, num, y []int16, N, ord int, mem []int16) {
	var i, j int
	for i = 0; i < N; i++ {
		var sum int32 = SHL32(EXTEND32(x[i]), SIG_SHIFT)
		for j = 0; j < ord; j++ {
			sum += MULT16_16(num[j], mem[j])
		}
		for j = ord - 1; j >= 1; j-- {
			mem[j] = mem[j-1]
		}
		mem[0] = x[i]
		y[i] = ROUND16(sum, SIG_SHIFT)
	}
}

// C: void celt_iir(const opus_val32 *x, const opus_val16 *den, opus_val32 *y, int N, int ord, opus_val16 *mem)
func Iir(x []int32, den []int16, y []int32, N, ord int, mem []int16) {
	var i, j int
	for i = 0; i < N; i++ {
		var sum int32 = x[i]
		for j = 0; j < ord; j++ {
			sum -= MULT16_16(den[j], mem[j])
		}
		for j = ord - 1; j >= 1; j-- {
			mem[j] = mem[j-1]
		}
		mem[0] = ROUND16(sum, SIG_SHIFT)
		y[i] = sum
	}
}

// Autocorr computes the autocorrelation of x (n samples) up to lag, with an
// optional window applied to overlap samples at each end.
//
// C: void _celt_autocorr(const opus_val16 *x, opus_val32 *ac, const opus_val16 *window, int overlap, int lag, int n)
func Autocorr(x []int16, ac []int32, window []int16, overlap, lag, n int) {
	var d int32
	var i int
	xx := make([]int16, n)
	celt_assert(n > 0)
	celt_assert(overlap >= 0)
	for i = 0; i < n; i++ {
		xx[i] = x[i]
	}
	for i = 0; i < overlap; i++ {
		xx[i] = int16(MULT16_16_Q15(x[i], window[i]))
		xx[n-i-1] = int16(MULT16_16_Q15(x[n-i-1], window[i]))
	}
	{
		var ac0 int32 = 0
		var shift int
		for i = 0; i < n; i++ {
			ac0 += SHR32(MULT16_16(xx[i], xx[i]), 9)
		}
		ac0 += int32(1 + n)

		shift = int(Ilog2(ac0)) - 30 + 10
		shift = (shift + 1) / 2
		for i = 0; i < n; i++ {
			xx[i] = int16(VSHR32(xx[i], shift))
		}
	}
	for lag >= 0 {
		d = 0
		for i = lag; i < n; i++ {
			d += int32(xx[i]) * int32(xx[i-lag])
		}
		ac[lag] = d
		// printf ("%f ", ac[lag]);
		lag--
	}
	// printf ("\n");
	ac[0] += 10
}
