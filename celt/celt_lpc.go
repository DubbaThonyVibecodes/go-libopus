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

	clear(lpc[:p])
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

// C: void celt_fir_c(const opus_val16 *x, const opus_val16 *num, opus_val16 *y, int N, int ord, int arch)
// C: #define celt_fir(x, num, y, N, ord, arch) (celt_fir_c(x, num, y, N, ord, arch))
//
// The C reads ord samples *behind* x (x[i+j-ord]), so the Go slice x starts
// ord samples *before* the C x pointer: the caller passes the C pointer
// minus ord, and the C x[k] is the Go x[ord+k].
func Fir(x []int16, num []int16, y []int16, N, ord int, arch int) {
	var i, j int
	rnum := make([]int16, ord)
	for i = 0; i < ord; i++ {
		rnum[i] = num[ord-i-1]
	}
	for i = 0; i < N-3; i += 4 {
		var sum [4]int32
		sum[0] = SHL32(EXTEND32(x[ord+i]), SIG_SHIFT)
		sum[1] = SHL32(EXTEND32(x[ord+i+1]), SIG_SHIFT)
		sum[2] = SHL32(EXTEND32(x[ord+i+2]), SIG_SHIFT)
		sum[3] = SHL32(EXTEND32(x[ord+i+3]), SIG_SHIFT)
		XcorrKernel(rnum, x[i:], sum[:], ord, arch)
		y[i] = ROUND16(sum[0], SIG_SHIFT)
		y[i+1] = ROUND16(sum[1], SIG_SHIFT)
		y[i+2] = ROUND16(sum[2], SIG_SHIFT)
		y[i+3] = ROUND16(sum[3], SIG_SHIFT)
	}
	for ; i < N; i++ {
		var sum int32 = SHL32(EXTEND32(x[ord+i]), SIG_SHIFT)
		for j = 0; j < ord; j++ {
			sum = MAC16_16(sum, rnum[j], x[i+j]) // C: x[i+j-ord]
		}
		y[i] = ROUND16(sum, SIG_SHIFT)
	}
}

// C: void celt_iir(const opus_val32 *_x, const opus_val16 *den, opus_val32 *_y, int N, int ord, opus_val16 *mem, int arch)
func Iir(_x []int32, den []int16, _y []int32, N, ord int, mem []int16, arch int) {
	var i, j int

	celt_assert((ord & 3) == 0)
	rden := make([]int16, ord)
	y := make([]int16, N+ord)
	for i = 0; i < ord; i++ {
		rden[i] = den[ord-i-1]
	}
	for i = 0; i < ord; i++ {
		y[i] = -mem[ord-i-1]
	}
	for ; i < N+ord; i++ {
		y[i] = 0
	}
	for i = 0; i < N-3; i += 4 {
		// Unroll by 4 as if it were an FIR filter
		var sum [4]int32
		sum[0] = _x[i]
		sum[1] = _x[i+1]
		sum[2] = _x[i+2]
		sum[3] = _x[i+3]
		XcorrKernel(rden, y[i:], sum[:], ord, arch)

		// Patch up the result to compensate for the fact that this is an IIR
		y[i+ord] = -SROUND16(sum[0], SIG_SHIFT)
		_y[i] = sum[0]
		sum[1] = MAC16_16(sum[1], y[i+ord], den[0])
		y[i+ord+1] = -SROUND16(sum[1], SIG_SHIFT)
		_y[i+1] = sum[1]
		sum[2] = MAC16_16(sum[2], y[i+ord+1], den[0])
		sum[2] = MAC16_16(sum[2], y[i+ord], den[1])
		y[i+ord+2] = -SROUND16(sum[2], SIG_SHIFT)
		_y[i+2] = sum[2]

		sum[3] = MAC16_16(sum[3], y[i+ord+2], den[0])
		sum[3] = MAC16_16(sum[3], y[i+ord+1], den[1])
		sum[3] = MAC16_16(sum[3], y[i+ord], den[2])
		y[i+ord+3] = -SROUND16(sum[3], SIG_SHIFT)
		_y[i+3] = sum[3]
	}
	for ; i < N; i++ {
		var sum int32 = _x[i]
		for j = 0; j < ord; j++ {
			sum -= MULT16_16(rden[j], y[i+j])
		}
		y[i+ord] = SROUND16(sum, SIG_SHIFT)
		_y[i] = sum
	}
	for i = 0; i < ord; i++ {
		mem[i] = int16(_y[N-i-1])
	}
}

// Autocorr computes the autocorrelation of x (n samples) up to lag, with an
// optional window applied to overlap samples at each end. It returns the
// shift that was applied.
//
// C: int _celt_autocorr(const opus_val16 *x, opus_val32 *ac, const opus_val16 *window, int overlap, int lag, int n, int arch)
func Autocorr(x []int16, ac []int32, window []int16, overlap, lag, n int, arch int) int {
	var d int32
	var i, k int
	var fastN int = n - lag
	var shift int
	var xptr []int16
	xx := make([]int16, n)
	celt_assert(n > 0)
	celt_assert(overlap >= 0)
	if overlap == 0 {
		xptr = x
	} else {
		for i = 0; i < n; i++ {
			xx[i] = x[i]
		}
		for i = 0; i < overlap; i++ {
			xx[i] = int16(MULT16_16_Q15(x[i], window[i]))
			xx[n-i-1] = int16(MULT16_16_Q15(x[n-i-1], window[i]))
		}
		xptr = xx
	}
	shift = 0
	{
		var ac0 int32
		ac0 = int32(1 + (n << 7))
		if n&1 != 0 {
			ac0 += SHR32(MULT16_16(xptr[0], xptr[0]), 9)
		}
		for i = (n & 1); i < n; i += 2 {
			ac0 += SHR32(MULT16_16(xptr[i], xptr[i]), 9)
			ac0 += SHR32(MULT16_16(xptr[i+1], xptr[i+1]), 9)
		}

		shift = int(Ilog2(ac0)) - 30 + 10
		shift = (shift) / 2
		if shift > 0 {
			for i = 0; i < n; i++ {
				xx[i] = int16(PSHR32(xptr[i], shift))
			}
			xptr = xx
		} else {
			shift = 0
		}
	}
	PitchXcorr(xptr, xptr, ac, fastN, lag+1, arch)
	for k = 0; k <= lag; k++ {
		d = 0
		for i = k + fastN; i < n; i++ {
			d = MAC16_16(d, xptr[i], xptr[i-k])
		}
		ac[k] += d
	}
	shift = 2 * shift
	if shift <= 0 {
		ac[0] += SHL32(int32(1), -shift)
	}
	if ac[0] < 268435456 {
		var shift2 int = 29 - EC_ILOG(uint32(ac[0]))
		for i = 0; i <= lag; i++ {
			ac[i] = SHL32(ac[i], shift2)
		}
		shift -= shift2
	} else if ac[0] >= 536870912 {
		var shift2 int = 1
		if ac[0] >= 1073741824 {
			shift2++
		}
		for i = 0; i <= lag; i++ {
			ac[i] = SHR32(ac[i], shift2)
		}
		shift += shift2
	}

	return shift
}
