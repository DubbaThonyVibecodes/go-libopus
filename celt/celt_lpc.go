package celt

// C: celt/celt_lpc.h, celt/celt_lpc.c

const CELT_LPC_ORDER = 24

// Lpc computes LPC coefficients from autocorrelation values.
//
// C: void _celt_lpc(opus_val16 *_lpc, const opus_val32 *ac, int p)
func Lpc(_lpc []int16, ac []int32, p int) {
	var i, j int
	var r int32
	var error int32 = ac[0]
	var lpc [CELT_LPC_ORDER]int32

	clear(lpc[:p])
	if ac[0] != 0 {
		for i = 0; i < p; i++ {
			// Sum up this iteration's reflection coefficient
			var rr int32 = 0
			// #if defined (FIXED_POINT) && OPUS_FAST_INT64
			var acc int64 = 0
			for j = 0; j < i; j++ {
				acc += int64(lpc[j]) * int64(ac[i-j])
			}
			rr = int32(SHR64(acc, 31))
			rr += SHR32(ac[i+1], 6)
			r = -FracDiv32(SHL32(rr, 6), error)
			// Update LPC coefficients and total error
			lpc[i] = SHR32(r, 6)
			for j = 0; j < (i+1)>>1; j++ {
				var tmp1, tmp2 int32
				tmp1 = lpc[j]
				tmp2 = lpc[i-1-j]
				lpc[j] = tmp1 + MULT32_32_Q31(r, tmp2)
				lpc[i-1-j] = tmp2 + MULT32_32_Q31(r, tmp1)
			}

			error = error - MULT32_32_Q31(MULT32_32_Q31(r, r), error)
			// Bail out once we get 30 dB gain
			if error <= SHR32(ac[0], 10) {
				break
			}
		}
	}
	{
		// Convert the int32 lpcs to int16 and ensure there are no wrap-arounds.
		// This reuses the logic in silk_LPC_fit() and silk_bwexpander_32(). Any bug
		// fixes should also be applied there.
		var iter, idx int = 0, 0
		var maxabs, absval, chirp_Q16, chirp_minus_one_Q16 int32

		for iter = 0; iter < 10; iter++ {
			maxabs = 0
			for i = 0; i < p; i++ {
				absval = ABS32(lpc[i])
				if absval > maxabs {
					maxabs = absval
					idx = i
				}
			}
			maxabs = PSHR32(maxabs, 13) // Q25->Q12

			if maxabs > 32767 {
				maxabs = MIN32(maxabs, 163838)
				// (C: QCONST32(0.999, 16) with a double literal; 65470 either way.)
				chirp_Q16 = QCONST32(0.999, 16) - DIV32(SHL32(maxabs-32767, 14),
					SHR32(MULT32_32_32(maxabs, idx+1), 2))
				chirp_minus_one_Q16 = chirp_Q16 - 65536

				// Apply bandwidth expansion.
				for i = 0; i < p-1; i++ {
					lpc[i] = MULT32_32_Q16(chirp_Q16, lpc[i])
					chirp_Q16 += PSHR32(MULT32_32_32(chirp_Q16, chirp_minus_one_Q16), 16)
				}
				lpc[p-1] = MULT32_32_Q16(chirp_Q16, lpc[p-1])
			} else {
				break
			}
		}

		if iter == 10 {
			// If the coeffs still do not fit into the 16 bit range after 10 iterations,
			// fall back to the A(z)=1 filter.
			clear(lpc[:p])
			_lpc[0] = 4096 // Q12
		} else {
			for i = 0; i < p; i++ {
				_lpc[i] = EXTRACT16(PSHR32(lpc[i], 13)) // Q25->Q12
			}
		}
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
	celt_assert(&x[ord] != &y[0]) // C: celt_assert(x != y); the Go x starts ord samples before the C x
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
		y[i] = SROUND16(sum[0], SIG_SHIFT)
		y[i+1] = SROUND16(sum[1], SIG_SHIFT)
		y[i+2] = SROUND16(sum[2], SIG_SHIFT)
		y[i+3] = SROUND16(sum[3], SIG_SHIFT)
	}
	for ; i < N; i++ {
		var sum int32 = SHL32(EXTEND32(x[ord+i]), SIG_SHIFT)
		for j = 0; j < ord; j++ {
			sum = MAC16_16(sum, rnum[j], x[i+j]) // C: x[i+j-ord]
		}
		y[i] = SROUND16(sum, SIG_SHIFT)
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
// C: int _celt_autocorr(const opus_val16 *x, opus_val32 *ac, const celt_coef *window, int overlap, int lag, int n, int arch)
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
			var w int16 = COEF2VAL16(window[i])
			xx[i] = int16(MULT16_16_Q15(x[i], w))
			xx[n-i-1] = int16(MULT16_16_Q15(x[n-i-1], w))
		}
		xptr = xx
	}
	shift = 0
	{
		var ac0 int32
		var ac0_shift int = int(Ilog2(int32(n + (n >> 4))))
		ac0 = int32(1 + (n << 7))
		if n&1 != 0 {
			ac0 += SHR32(MULT16_16(xptr[0], xptr[0]), ac0_shift)
		}
		for i = (n & 1); i < n; i += 2 {
			ac0 += SHR32(MULT16_16(xptr[i], xptr[i]), ac0_shift)
			ac0 += SHR32(MULT16_16(xptr[i+1], xptr[i+1]), ac0_shift)
		}
		// Consider the effect of rounding-to-nearest when scaling the signal.
		ac0 += SHR32(ac0, 7)

		shift = int(Ilog2(ac0)) - 30 + ac0_shift + 1
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
