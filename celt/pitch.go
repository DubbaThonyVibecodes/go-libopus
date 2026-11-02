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

// PitchDownsample downsamples x by 2 (summing channels) into x_lp and applies a
// 4th-order LPC whitening filter.
//
// C: void pitch_downsample(celt_sig * OPUS_RESTRICT x[], opus_val16 * OPUS_RESTRICT x_lp, int len, int C)
func PitchDownsample(x [][]int32, x_lp []int16, length, C int) {
	var i int
	var ac [5]int32
	var tmp int16 = Q15ONE
	var lpc [4]int16
	var mem = [4]int16{0, 0, 0, 0}
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

	Autocorr(x_lp, ac[:], nil, 0, 4, length>>1)

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
	Fir(x_lp, lpc[:], x_lp, length>>1, 4, mem[:])

	mem[0] = 0
	lpc[0] = QCONST16(.8, 12)
	Fir(x_lp, lpc[:], x_lp, length>>1, 1, mem[:])
}

// C: void pitch_search(const opus_val16 * OPUS_RESTRICT x_lp, opus_val16 * OPUS_RESTRICT y, int len, int max_pitch, int *pitch)
func PitchSearch(x_lp []int16, y []int16, length, max_pitch int, pitch *int) {
	var i, j int
	var lag int
	var best_pitch = [2]int{0, 0}
	var maxcorr int32 = 1
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

	for i = 0; i < max_pitch>>2; i++ {
		var sum int32 = 0
		for j = 0; j < length>>2; j++ {
			sum = MAC16_16(sum, x_lp4[j], y_lp4[i+j])
		}
		xcorr[i] = MAX32(-1, sum)
		maxcorr = MAX32(maxcorr, sum)
	}
	find_best_pitch(xcorr, y_lp4, length>>2, max_pitch>>2, best_pitch[:], 0, maxcorr)

	// Finer search with 2x decimation
	maxcorr = 1
	for i = 0; i < max_pitch>>1; i++ {
		var sum int32 = 0
		xcorr[i] = 0
		if ABS(i-2*best_pitch[0]) > 2 && ABS(i-2*best_pitch[1]) > 2 {
			continue
		}
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

var second_check = [16]int{0, 0, 3, 2, 3, 2, 5, 2, 3, 2, 3, 2, 5, 2, 3, 2}

// RemoveDoubling checks for pitch period doubling/tripling. x is indexed so
// that x[xOff] is the C `x` pointer (the C code reads x[i-T] for negative
// offsets up to maxperiod).
//
// C: opus_val16 remove_doubling(opus_val16 *x, int maxperiod, int minperiod, int N, int *T0_, int prev_period, opus_val16 prev_gain)
func RemoveDoubling(x []int16, maxperiod, minperiod int, N int, T0_ *int, prev_period int, prev_gain int16) int16 {
	var k, i, T, T0 int
	var g, g0 int16
	var pg int16
	var xy, xx, yy int32
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
	xx = 0
	xy = 0
	yy = 0
	for i = 0; i < N; i++ {
		xy = MAC16_16(xy, x[xo+i], x[xo+i-T0])
		xx = MAC16_16(xx, x[xo+i], x[xo+i])
		yy = MAC16_16(yy, x[xo+i-T0], x[xo+i-T0])
	}
	best_xy = xy
	best_yy = yy
	{
		var x2y2 int32
		var sh, t int
		x2y2 = 1 + HALF32(MULT32_32_Q31(xx, yy))
		sh = int(Ilog2(x2y2)) >> 1
		t = int(VSHR32(x2y2, 2*(sh-7)))
		g0 = int16(VSHR32(MULT16_32_Q15(RsqrtNorm(int32(t)), xy), sh+1))
		g = g0
	}
	// Look for any pitch at T/k
	for k = 2; k <= 15; k++ {
		var T1, T1b int
		var g1 int16
		var cont int16 = 0
		T1 = (2*T0 + k) / (2 * k)
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
			T1b = (2*second_check[k]*T0 + k) / (2 * k)
		}
		xy = 0
		yy = 0
		for i = 0; i < N; i++ {
			xy = MAC16_16(xy, x[xo+i], x[xo+i-T1])
			yy = MAC16_16(yy, x[xo+i-T1], x[xo+i-T1])

			xy = MAC16_16(xy, x[xo+i], x[xo+i-T1b])
			yy = MAC16_16(yy, x[xo+i-T1b], x[xo+i-T1b])
		}
		{
			var x2y2 int32
			var sh, t int
			x2y2 = 1 + MULT32_32_Q31(xx, yy)
			sh = int(Ilog2(x2y2)) >> 1
			t = int(VSHR32(x2y2, 2*(sh-7)))
			g1 = int16(VSHR32(MULT16_32_Q15(RsqrtNorm(int32(t)), xy), sh+1))
		}
		if ABS(T1-prev_period) <= 1 {
			cont = prev_gain
		} else if ABS(T1-prev_period) <= 2 && 5*k*k < T0 {
			cont = int16(HALF32(prev_gain))
		} else {
			cont = 0
		}
		if int32(g1) > int32(QCONST16(.3, 15))+MULT16_16_Q15(QCONST16(.4, 15), g0)-int32(cont) {
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
		T1 := T + k - 1
		xy = 0
		for i = 0; i < N; i++ {
			xy = MAC16_16(xy, x[xo+i], x[xo+i-T1])
		}
		xcorr[k] = xy
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
