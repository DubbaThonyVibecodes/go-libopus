package celt

// C: celt/pitch.h, celt/pitch.c — pitch analysis.

// C: static void find_best_pitch(opus_val32 *xcorr, opus_val16 *y, int len, int max_pitch, int *best_pitch, int yshift, opus_val32 maxcorr)
func find_best_pitch(xcorr []int32, y []int16, length, max_pitch int, best_pitch []int, yshift int, maxcorr int32) {
	var i, j int
	var Syy int32 = 1
	var best_num [2]int16
	var best_den [2]int32
	var xshift int

	xshift = int(Ilog2(maxcorr)) - 14

	best_num[0] = -1
	best_num[1] = -1
	best_den[0] = 0
	best_den[1] = 0
	best_pitch[0] = 0
	best_pitch[1] = 1
	for j = 0; j < length; j++ {
		Syy = ADD32(Syy, SHR32(MULT16_16(y[j], y[j]), yshift))
	}
	for i = 0; i < max_pitch; i++ {
		if xcorr[i] > 0 {
			var num int16
			var xcorr16 int32
			xcorr16 = int32(EXTRACT16(VSHR32(xcorr[i], xshift)))
			num = int16(MULT16_16_Q15(xcorr16, xcorr16))
			if MULT16_32_Q15(num, best_den[1]) > MULT16_32_Q15(best_num[1], Syy) {
				if MULT16_32_Q15(num, best_den[0]) > MULT16_32_Q15(best_num[0], Syy) {
					best_num[1] = best_num[0]
					best_den[1] = best_den[0]
					best_pitch[1] = best_pitch[0]
					best_num[0] = num
					best_den[0] = Syy
					best_pitch[0] = i
				} else {
					best_num[1] = num
					best_den[1] = Syy
					best_pitch[1] = i
				}
			}
		}
		Syy += SHR32(MULT16_16(y[i+length], y[i+length]), yshift) - SHR32(MULT16_16(y[i], y[i]), yshift)
		Syy = MAX32(1, Syy)
	}
}

// C: static void celt_fir5(const opus_val16 *x, const opus_val16 *num, opus_val16 *y, int N, opus_val16 *mem)
func celt_fir5(x []int16, num []int16, y []int16, N int, mem []int16) {
	var i int
	var num0, num1, num2, num3, num4 int16
	var mem0, mem1, mem2, mem3, mem4 int32
	num0 = num[0]
	num1 = num[1]
	num2 = num[2]
	num3 = num[3]
	num4 = num[4]
	mem0 = int32(mem[0])
	mem1 = int32(mem[1])
	mem2 = int32(mem[2])
	mem3 = int32(mem[3])
	mem4 = int32(mem[4])
	for i = 0; i < N; i++ {
		var sum int32 = SHL32(EXTEND32(x[i]), SIG_SHIFT)
		sum = MAC16_16(sum, num0, mem0)
		sum = MAC16_16(sum, num1, mem1)
		sum = MAC16_16(sum, num2, mem2)
		sum = MAC16_16(sum, num3, mem3)
		sum = MAC16_16(sum, num4, mem4)
		mem4 = mem3
		mem3 = mem2
		mem2 = mem1
		mem1 = mem0
		mem0 = int32(x[i])
		y[i] = ROUND16(sum, SIG_SHIFT)
	}
	mem[0] = int16(mem0)
	mem[1] = int16(mem1)
	mem[2] = int16(mem2)
	mem[3] = int16(mem3)
	mem[4] = int16(mem4)
}

// PitchDownsample downsamples x by 2 (summing channels) into x_lp and applies a
// 4th-order LPC whitening filter.
//
// C: void pitch_downsample(celt_sig * OPUS_RESTRICT x[], opus_val16 * OPUS_RESTRICT x_lp, int len, int C, int arch)
func PitchDownsample(x [][]int32, x_lp []int16, length, C int, arch int) {
	var i int
	var ac [5]int32
	var tmp int16 = Q15ONE
	var lpc [4]int16
	var mem = [5]int16{0, 0, 0, 0, 0}
	var lpc2 [5]int16
	var c1 int16 = QCONST16(.8, 15)
	var shift int
	var maxabs int32 = Maxabs32(x[0], length)
	if C == 2 {
		var maxabs_1 int32 = Maxabs32(x[1], length)
		maxabs = MAX32(maxabs, maxabs_1)
	}
	if maxabs < 1 {
		maxabs = 1
	}
	shift = int(Ilog2(maxabs)) - 10
	if shift < 0 {
		shift = 0
	}
	if C == 2 {
		shift++
	}
	for i = 1; i < length>>1; i++ {
		x_lp[i] = int16(SHR32(HALF32(HALF32(x[0][(2*i-1)]+x[0][(2*i+1)])+x[0][2*i]), shift))
	}
	x_lp[0] = int16(SHR32(HALF32(HALF32(x[0][1])+x[0][0]), shift))
	if C == 2 {
		for i = 1; i < length>>1; i++ {
			x_lp[i] += int16(SHR32(HALF32(HALF32(x[1][(2*i-1)]+x[1][(2*i+1)])+x[1][2*i]), shift))
		}
		x_lp[0] += int16(SHR32(HALF32(HALF32(x[1][1])+x[1][0]), shift))
	}

	Autocorr(x_lp, ac[:], nil, 0,
		4, length>>1, arch)

	// Noise floor -40 dB
	ac[0] += SHR32(ac[0], 13)
	// Lag windowing
	for i = 1; i <= 4; i++ {
		// ac[i] *= exp(-.5*(2*M_PI*.002*i)*(2*M_PI*.002*i));
		ac[i] -= MULT16_32_Q15(2*i*i, ac[i])
	}

	Lpc(lpc[:], ac[:], 4)
	for i = 0; i < 4; i++ {
		tmp = int16(MULT16_16_Q15(QCONST16(.9, 15), tmp))
		lpc[i] = int16(MULT16_16_Q15(lpc[i], tmp))
	}
	// Add a zero
	lpc2[0] = int16(int32(lpc[0]) + int32(QCONST16(.8, SIG_SHIFT)))
	lpc2[1] = int16(int32(lpc[1]) + MULT16_16_Q15(c1, lpc[0]))
	lpc2[2] = int16(int32(lpc[2]) + MULT16_16_Q15(c1, lpc[1]))
	lpc2[3] = int16(int32(lpc[3]) + MULT16_16_Q15(c1, lpc[2]))
	lpc2[4] = int16(MULT16_16_Q15(c1, lpc[3]))
	celt_fir5(x_lp, lpc2[:], x_lp, length>>1, mem[:])
}

// XcorrKernel is the kernel you really want to optimize. It gets used a lot
// by the prefilter and by the PLC. sum has 4 entries.
//
// C: static OPUS_INLINE void xcorr_kernel_c(const opus_val16 * x, const opus_val16 * y, opus_val32 sum[4], int len)
// C: #define xcorr_kernel(x, y, sum, len, arch) ((void)(arch),xcorr_kernel_c(x, y, sum, len))
func XcorrKernel(x, y []int16, sum []int32, len int, arch int) {
	_ = arch
	var j int
	var y_0, y_1, y_2, y_3 int16
	celt_assert(len >= 3)
	y_3 = 0 // gcc doesn't realize that y_3 can't be used uninitialized
	xi, yi := 0, 0
	y_0 = y[yi]
	yi++
	y_1 = y[yi]
	yi++
	y_2 = y[yi]
	yi++
	for j = 0; j < len-3; j += 4 {
		var tmp int16
		tmp = x[xi]
		xi++
		y_3 = y[yi]
		yi++
		sum[0] = MAC16_16(sum[0], tmp, y_0)
		sum[1] = MAC16_16(sum[1], tmp, y_1)
		sum[2] = MAC16_16(sum[2], tmp, y_2)
		sum[3] = MAC16_16(sum[3], tmp, y_3)
		tmp = x[xi]
		xi++
		y_0 = y[yi]
		yi++
		sum[0] = MAC16_16(sum[0], tmp, y_1)
		sum[1] = MAC16_16(sum[1], tmp, y_2)
		sum[2] = MAC16_16(sum[2], tmp, y_3)
		sum[3] = MAC16_16(sum[3], tmp, y_0)
		tmp = x[xi]
		xi++
		y_1 = y[yi]
		yi++
		sum[0] = MAC16_16(sum[0], tmp, y_2)
		sum[1] = MAC16_16(sum[1], tmp, y_3)
		sum[2] = MAC16_16(sum[2], tmp, y_0)
		sum[3] = MAC16_16(sum[3], tmp, y_1)
		tmp = x[xi]
		xi++
		y_2 = y[yi]
		yi++
		sum[0] = MAC16_16(sum[0], tmp, y_3)
		sum[1] = MAC16_16(sum[1], tmp, y_0)
		sum[2] = MAC16_16(sum[2], tmp, y_1)
		sum[3] = MAC16_16(sum[3], tmp, y_2)
	}
	if j < len {
		j++
		var tmp int16 = x[xi]
		xi++
		y_3 = y[yi]
		yi++
		sum[0] = MAC16_16(sum[0], tmp, y_0)
		sum[1] = MAC16_16(sum[1], tmp, y_1)
		sum[2] = MAC16_16(sum[2], tmp, y_2)
		sum[3] = MAC16_16(sum[3], tmp, y_3)
	} else {
		j++
	}
	if j < len {
		j++
		var tmp int16 = x[xi]
		xi++
		y_0 = y[yi]
		yi++
		sum[0] = MAC16_16(sum[0], tmp, y_1)
		sum[1] = MAC16_16(sum[1], tmp, y_2)
		sum[2] = MAC16_16(sum[2], tmp, y_3)
		sum[3] = MAC16_16(sum[3], tmp, y_0)
	} else {
		j++
	}
	if j < len {
		var tmp int16 = x[xi]
		xi++
		y_1 = y[yi]
		yi++
		sum[0] = MAC16_16(sum[0], tmp, y_2)
		sum[1] = MAC16_16(sum[1], tmp, y_3)
		sum[2] = MAC16_16(sum[2], tmp, y_0)
		sum[3] = MAC16_16(sum[3], tmp, y_1)
	}
}

// C: static OPUS_INLINE void dual_inner_prod_c(const opus_val16 *x, const opus_val16 *y01, const opus_val16 *y02, int N, opus_val32 *xy1, opus_val32 *xy2)
// C: #define dual_inner_prod(x, y01, y02, N, xy1, xy2, arch) ((void)(arch),dual_inner_prod_c(x, y01, y02, N, xy1, xy2))
func DualInnerProd(x, y01, y02 []int16, N int, xy1, xy2 *int32, arch int) {
	_ = arch
	var i int
	var xy01 int32 = 0
	var xy02 int32 = 0
	for i = 0; i < N; i++ {
		xy01 = MAC16_16(xy01, x[i], y01[i])
		xy02 = MAC16_16(xy02, x[i], y02[i])
	}
	*xy1 = xy01
	*xy2 = xy02
}

// We make sure a C version is always available for cases where the overhead of
// vectorization and passing around an arch flag aren't worth it.
//
// C: static OPUS_INLINE opus_val32 celt_inner_prod_c(const opus_val16 *x, const opus_val16 *y, int N)
// C: #define celt_inner_prod(x, y, N, arch) ((void)(arch),celt_inner_prod_c(x, y, N))
func InnerProd(x, y []int16, N int, arch int) int32 {
	_ = arch
	var i int
	var xy int32 = 0
	for i = 0; i < N; i++ {
		xy = MAC16_16(xy, x[i], y[i])
	}
	return xy
}

// PitchXcorr is the pure C implementation (unrolled version of the pitch
// correlation). It returns the maximum correlation (fixed-point build).
//
// C: opus_val32 celt_pitch_xcorr_c(const opus_val16 *_x, const opus_val16 *_y, opus_val32 *xcorr, int len, int max_pitch, int arch)
// C: #define celt_pitch_xcorr celt_pitch_xcorr_c
func PitchXcorr(_x, _y []int16, xcorr []int32, len int, max_pitch int, arch int) int32 {
	var i int
	// The EDSP version requires that max_pitch is at least 1, and that _x is
	// 32-bit aligned.
	// Since it's hard to put asserts in assembly, put them here.
	var maxcorr int32 = 1
	celt_assert(max_pitch > 0)
	for i = 0; i < max_pitch-3; i += 4 {
		var sum = [4]int32{0, 0, 0, 0}
		XcorrKernel(_x, _y[i:], sum[:], len, arch)
		xcorr[i] = sum[0]
		xcorr[i+1] = sum[1]
		xcorr[i+2] = sum[2]
		xcorr[i+3] = sum[3]
		sum[0] = MAX32(sum[0], sum[1])
		sum[2] = MAX32(sum[2], sum[3])
		sum[0] = MAX32(sum[0], sum[2])
		maxcorr = MAX32(maxcorr, sum[0])
	}
	// In case max_pitch isn't a multiple of 4, do non-unrolled version.
	for ; i < max_pitch; i++ {
		var sum int32
		sum = InnerProd(_x, _y[i:], len, arch)
		xcorr[i] = sum
		maxcorr = MAX32(maxcorr, sum)
	}
	return maxcorr
}

// C: void pitch_search(const opus_val16 * OPUS_RESTRICT x_lp, opus_val16 * OPUS_RESTRICT y, int len, int max_pitch, int *pitch, int arch)
func PitchSearch(x_lp []int16, y []int16, length, max_pitch int, pitch *int, arch int) {
	var i, j int
	var lag int
	var best_pitch = [2]int{0, 0}
	var maxcorr int32
	var xmax, ymax int32
	var shift int = 0
	var offset int

	celt_assert(length > 0)
	celt_assert(max_pitch > 0)
	lag = length + max_pitch

	x_lp4 := make([]int16, length>>2)
	y_lp4 := make([]int16, lag>>2)
	xcorr := make([]int32, max_pitch>>1)

	// Downsample by 2 again
	for j = 0; j < length>>2; j++ {
		x_lp4[j] = x_lp[2*j]
	}
	for j = 0; j < lag>>2; j++ {
		y_lp4[j] = y[2*j]
	}

	xmax = Maxabs16(x_lp4, length>>2)
	ymax = Maxabs16(y_lp4, lag>>2)
	shift = int(Ilog2(MAX32(1, MAX32(xmax, ymax)))) - 11
	if shift > 0 {
		for j = 0; j < length>>2; j++ {
			x_lp4[j] = int16(SHR16(x_lp4[j], shift))
		}
		for j = 0; j < lag>>2; j++ {
			y_lp4[j] = int16(SHR16(y_lp4[j], shift))
		}
		// Use double the shift for a MAC
		shift *= 2
	} else {
		shift = 0
	}

	// Coarse search with 4x decimation

	maxcorr = PitchXcorr(x_lp4, y_lp4, xcorr, length>>2, max_pitch>>2, arch)

	find_best_pitch(xcorr, y_lp4, length>>2, max_pitch>>2, best_pitch[:], 0, maxcorr)

	// Finer search with 2x decimation
	maxcorr = 1
	for i = 0; i < max_pitch>>1; i++ {
		var sum int32
		xcorr[i] = 0
		if ABS(i-2*best_pitch[0]) > 2 && ABS(i-2*best_pitch[1]) > 2 {
			continue
		}
		sum = 0
		for j = 0; j < length>>1; j++ {
			sum += SHR32(MULT16_16(x_lp[j], y[i+j]), shift)
		}
		xcorr[i] = MAX32(-1, sum)
		maxcorr = MAX32(maxcorr, sum)
	}
	find_best_pitch(xcorr, y, length>>1, max_pitch>>1, best_pitch[:], shift+1, maxcorr)

	// Refine by pseudo-interpolation
	if best_pitch[0] > 0 && best_pitch[0] < (max_pitch>>1)-1 {
		var a, b, c int32
		a = xcorr[best_pitch[0]-1]
		b = xcorr[best_pitch[0]]
		c = xcorr[best_pitch[0]+1]
		if (c - a) > MULT16_32_Q15(QCONST16(.7, 15), b-a) {
			offset = 1
		} else if (a - c) > MULT16_32_Q15(QCONST16(.7, 15), b-c) {
			offset = -1
		} else {
			offset = 0
		}
	} else {
		offset = 0
	}
	*pitch = 2*best_pitch[0] - offset
}

// C: static opus_val16 compute_pitch_gain(opus_val32 xy, opus_val32 xx, opus_val32 yy)
func compute_pitch_gain(xy, xx, yy int32) int16 {
	var x2y2 int32
	var sx, sy, shift int
	var g int32
	var den int16
	if xy == 0 || xx == 0 || yy == 0 {
		return 0
	}
	sx = int(Ilog2(xx)) - 14
	sy = int(Ilog2(yy)) - 14
	shift = sx + sy
	x2y2 = SHR32(MULT16_16(VSHR32(xx, sx), VSHR32(yy, sy)), 14)
	if shift&1 != 0 {
		if x2y2 < 32768 {
			x2y2 <<= 1
			shift--
		} else {
			x2y2 >>= 1
			shift++
		}
	}
	den = RsqrtNorm(x2y2)
	g = MULT16_32_Q15(den, xy)
	g = VSHR32(g, (shift>>1)-1)
	return EXTRACT16(MIN32(g, int32(Q15ONE)))
}

var second_check = [16]int{0, 0, 3, 2, 3, 2, 5, 2, 3, 2, 3, 2, 5, 2, 3, 2}

// RemoveDoubling checks for pitch period doubling/tripling. x is indexed so
// that x[xOff] is the C `x` pointer (the C code reads x[i-T] for negative
// offsets up to maxperiod).
//
// C: opus_val16 remove_doubling(opus_val16 *x, int maxperiod, int minperiod, int N, int *T0_, int prev_period, opus_val16 prev_gain, int arch)
func RemoveDoubling(x []int16, maxperiod, minperiod int, N int, T0_ *int, prev_period int, prev_gain int16, arch int) int16 {
	var k, i, T, T0 int
	var g, g0 int16
	var pg int16
	var xy, xx, yy, xy2 int32
	var xcorr [3]int32
	var best_xy, best_yy int32
	var offset int
	var minperiod0 int

	minperiod0 = minperiod
	maxperiod /= 2
	minperiod /= 2
	*T0_ /= 2
	prev_period /= 2
	N /= 2
	xo := maxperiod // x += maxperiod
	if *T0_ >= maxperiod {
		*T0_ = maxperiod - 1
	}

	T = *T0_
	T0 = T
	yy_lookup := make([]int32, maxperiod+1)
	DualInnerProd(x[xo:], x[xo:], x[xo-T0:], N, &xx, &xy, arch)
	yy_lookup[0] = xx
	yy = xx
	for i = 1; i <= maxperiod; i++ {
		yy = yy + MULT16_16(x[xo-i], x[xo-i]) - MULT16_16(x[xo+N-i], x[xo+N-i])
		yy_lookup[i] = MAX32(0, yy)
	}
	yy = yy_lookup[T0]
	best_xy = xy
	best_yy = yy
	g0 = compute_pitch_gain(xy, xx, yy)
	g = g0
	// Look for any pitch at T/k
	for k = 2; k <= 15; k++ {
		var T1, T1b int
		var g1 int16
		var cont int16 = 0
		var thresh int16
		T1 = int(Udiv(uint32(2*T0+k), uint32(2*k)))
		if T1 < minperiod {
			break
		}
		// Look for another strong correlation at T1b
		if k == 2 {
			if T1+T0 > maxperiod {
				T1b = T0
			} else {
				T1b = T0 + T1
			}
		} else {
			T1b = int(Udiv(uint32(2*second_check[k]*T0+k), uint32(2*k)))
		}
		DualInnerProd(x[xo:], x[xo-T1:], x[xo-T1b:], N, &xy, &xy2, arch)
		xy = HALF32(xy + xy2)
		yy = HALF32(yy_lookup[T1] + yy_lookup[T1b])
		g1 = compute_pitch_gain(xy, xx, yy)
		if ABS(T1-prev_period) <= 1 {
			cont = prev_gain
		} else if ABS(T1-prev_period) <= 2 && 5*k*k < T0 {
			cont = int16(HALF16(prev_gain))
		} else {
			cont = 0
		}
		thresh = int16(MAX16(int32(QCONST16(.3, 15)), MULT16_16_Q15(QCONST16(.7, 15), g0)-int32(cont)))
		// Bias against very high pitch (very short period) to avoid false-positives
		// due to short-term correlation
		if T1 < 3*minperiod {
			thresh = int16(MAX16(int32(QCONST16(.4, 15)), MULT16_16_Q15(QCONST16(.85, 15), g0)-int32(cont)))
		} else if T1 < 2*minperiod {
			thresh = int16(MAX16(int32(QCONST16(.5, 15)), MULT16_16_Q15(QCONST16(.9, 15), g0)-int32(cont)))
		}
		if g1 > thresh {
			best_xy = xy
			best_yy = yy
			T = T1
			g = g1
		}
	}
	best_xy = MAX32(0, best_xy)
	if best_yy <= best_xy {
		pg = Q15ONE
	} else {
		pg = int16(SHR32(FracDiv32(best_xy, best_yy+1), 16))
	}

	for k = 0; k < 3; k++ {
		xcorr[k] = InnerProd(x[xo:], x[xo-(T+k-1):], N, arch)
	}
	if (xcorr[2] - xcorr[0]) > MULT16_32_Q15(QCONST16(.7, 15), xcorr[1]-xcorr[0]) {
		offset = 1
	} else if (xcorr[0] - xcorr[2]) > MULT16_32_Q15(QCONST16(.7, 15), xcorr[1]-xcorr[2]) {
		offset = -1
	} else {
		offset = 0
	}
	if pg > g {
		pg = g
	}
	*T0_ = 2*T + offset

	if *T0_ < minperiod0 {
		*T0_ = minperiod0
	}
	return pg
}
