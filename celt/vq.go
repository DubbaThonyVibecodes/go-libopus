package celt

// C: celt/vq.h, celt/vq.c — vector quantisation of the residual.

// C: static void exp_rotation1(celt_norm *X, int len, int stride, opus_val16 c, opus_val16 s)
func exp_rotation1(X []int16, length, stride int, c, s int16) {
	var i int
	var ms int16
	Xptr := X
	ms = NEG16(s)
	for i = 0; i < length-stride; i++ {
		var x1, x2 int16
		x1 = Xptr[0]
		x2 = Xptr[stride]
		Xptr[stride] = EXTRACT16(PSHR32(MAC16_16(MULT16_16(c, x2), s, x1), 15))
		Xptr[0] = EXTRACT16(PSHR32(MAC16_16(MULT16_16(c, x1), ms, x2), 15))
		Xptr = Xptr[1:]
	}
	p := length - 2*stride - 1
	for i = length - 2*stride - 1; i >= 0; i-- {
		var x1, x2 int16
		x1 = X[p]
		x2 = X[p+stride]
		X[p+stride] = EXTRACT16(PSHR32(MAC16_16(MULT16_16(c, x2), s, x1), 15))
		X[p] = EXTRACT16(PSHR32(MAC16_16(MULT16_16(c, x1), ms, x2), 15))
		p--
	}
}

// C: void exp_rotation(celt_norm *X, int len, int dir, int stride, int K, int spread)
func ExpRotation(X []int16, length, dir, stride, K, spread int) {
	var SPREAD_FACTOR = [3]int{15, 10, 5}
	var i int
	var c, s int16
	var gain, theta int16
	var stride2 int = 0
	var factor int

	if 2*K >= length || spread == SPREAD_NONE {
		return
	}
	factor = SPREAD_FACTOR[spread-1]

	gain = int16(Div(MULT16_16(Q15_ONE, length), int32(length+factor*K)))
	theta = int16(HALF16(MULT16_16_Q15(gain, gain)))

	c = CosNorm(EXTEND32(theta))
	s = CosNorm(EXTEND32(SUB16(Q15ONE, theta))) // sin(theta)

	if length >= 8*stride {
		stride2 = 1
		// This is just a simple (equivalent) way of computing sqrt(len/stride) with rounding.
		// It's basically incrementing long as (stride2+0.5)^2 < len/stride.
		for (stride2*stride2+stride2)*stride+(stride>>2) < length {
			stride2++
		}
	}
	// NOTE: As a minor optimization, we could be passing around log2(B), not B, for both this and for
	// extract_collapse_mask().
	length = int(Udiv(uint32(length), uint32(stride)))
	for i = 0; i < stride; i++ {
		if dir < 0 {
			if stride2 != 0 {
				exp_rotation1(X[i*length:], length, stride2, s, c)
			}
			exp_rotation1(X[i*length:], length, 1, c, s)
		} else {
			exp_rotation1(X[i*length:], length, 1, c, -s)
			if stride2 != 0 {
				exp_rotation1(X[i*length:], length, stride2, s, -c)
			}
		}
	}
}

// Takes the pitch vector and the decoded residual vector, computes the gain
// that will give ||p+g*y||=1 and mixes the residual with the pitch.
//
// C: static void normalise_residual(int * OPUS_RESTRICT iy, celt_norm * OPUS_RESTRICT X, int N, opus_val32 Ryy, opus_val16 gain)
func normalise_residual(iy []int, X []int16, N int, Ryy int32, gain int16) {
	var i, k int
	var t int32
	var g int16

	k = int(Ilog2(Ryy)) >> 1
	t = VSHR32(Ryy, 2*(k-7))
	g = int16(MULT16_16_P15(RsqrtNorm(t), gain))

	i = 0
	for {
		X[i] = EXTRACT16(PSHR32(MULT16_16(g, iy[i]), k+1))
		i++
		if i >= N {
			break
		}
	}
}

// C: static unsigned extract_collapse_mask(int *iy, int N, int B)
func extract_collapse_mask(iy []int, N, B int) uint32 {
	var collapse_mask uint32
	var N0, i int
	if B <= 1 {
		return 1
	}
	// NOTE: As a minor optimization, we could be passing around log2(B), not B, for both this and for
	// exp_rotation().
	N0 = int(Udiv(uint32(N), uint32(B)))
	collapse_mask = 0
	i = 0
	for {
		var j int
		var tmp uint32 = 0
		j = 0
		for {
			tmp |= uint32(iy[i*N0+j])
			j++
			if j >= N0 {
				break
			}
		}
		if tmp != 0 {
			collapse_mask |= 1 << uint(i)
		}
		i++
		if i >= B {
			break
		}
	}
	return collapse_mask
}

// C: opus_val16 op_pvq_search_c(celt_norm *X, int *iy, int K, int N, int arch)
// C: #define op_pvq_search(x, iy, K, N, arch) (op_pvq_search_c(x, iy, K, N, arch))
func OpPvqSearch(X []int16, iy []int, K, N int, arch int) int16 {
	var i, j int
	var pulsesLeft int
	var sum, xy int32
	var yy int16

	_ = arch
	y := make([]int16, N)
	signx := make([]int, N)

	// Get rid of the sign
	sum = 0
	j = 0
	for {
		if X[j] < 0 {
			signx[j] = 1
		} else {
			signx[j] = 0
		}
		// OPT: Make sure the compiler doesn't use a branch on ABS16().
		X[j] = ABS16(X[j])
		iy[j] = 0
		y[j] = 0
		j++
		if j >= N {
			break
		}
	}

	xy = 0
	yy = 0

	pulsesLeft = K

	// Do a pre-search by projecting on the pyramid
	if K > (N >> 1) {
		var rcp int16
		j = 0
		for {
			sum += int32(X[j])
			j++
			if j >= N {
				break
			}
		}
		// If X is too small, just replace it with a pulse at 0
		if sum <= int32(K) {
			X[0] = QCONST16(1.0, 14)
			j = 1
			for {
				X[j] = 0
				j++
				if j >= N {
					break
				}
			}
			sum = int32(QCONST16(1.0, 14))
		}
		rcp = EXTRACT16(MULT16_32_Q16(K, Rcp(sum)))
		j = 0
		for {
			// It's really important to round *towards zero* here
			iy[j] = int(MULT16_16_Q15(X[j], rcp))
			y[j] = int16(iy[j])
			yy = int16(MAC16_16(yy, y[j], y[j]))
			xy = MAC16_16(xy, X[j], y[j])
			y[j] *= 2
			pulsesLeft -= iy[j]
			j++
			if j >= N {
				break
			}
		}
	}
	celt_assert2(pulsesLeft >= 0, "Allocated too many pulses in the quick pass")

	// This should never happen, but just in case it does (e.g. on silence)
	// we fill the first bin with pulses.
	if pulsesLeft > N+3 {
		var tmp int16 = int16(pulsesLeft)
		yy = int16(MAC16_16(yy, tmp, tmp))
		yy = int16(MAC16_16(yy, tmp, y[0]))
		iy[0] += pulsesLeft
		pulsesLeft = 0
	}

	for i = 0; i < pulsesLeft; i++ {
		var Rxy, Ryy int16
		var best_id int
		var best_num int32
		var best_den int16
		var rshift int
		rshift = 1 + int(Ilog2(int32(K-pulsesLeft+i+1)))
		best_id = 0
		// The squared magnitude term gets added anyway, so we might as well
		// add it outside the loop
		yy = ADD16(yy, 1)

		// Calculations for position 0 are out of the loop, in part to reduce
		// mispredicted branches (since the if condition is usually false)
		// in the loop.
		// Temporary sums of the new pulse(s)
		Rxy = EXTRACT16(SHR32(ADD32(xy, EXTEND32(X[0])), rshift))
		// We're multiplying y[j] by two so we don't have to do it here
		Ryy = ADD16(yy, y[0])

		// Approximate score: we maximise Rxy/sqrt(Ryy) (we're guaranteed that
		// Rxy is positive because the sign is pre-computed)
		Rxy = int16(MULT16_16_Q15(Rxy, Rxy))
		best_den = Ryy
		best_num = int32(Rxy)
		j = 1
		for {
			// Temporary sums of the new pulse(s)
			Rxy = EXTRACT16(SHR32(ADD32(xy, EXTEND32(X[j])), rshift))
			// We're multiplying y[j] by two so we don't have to do it here
			Ryy = ADD16(yy, y[j])

			// Approximate score: we maximise Rxy/sqrt(Ryy) (we're guaranteed that
			// Rxy is positive because the sign is pre-computed)
			Rxy = int16(MULT16_16_Q15(Rxy, Rxy))
			// The idea is to check for num/den >= best_num/best_den, but that way
			// we can do it without any division
			// OPT: It's not clear whether a cmov is faster than a branch here
			// since the condition is more often false than true and using
			// a cmov introduces data dependencies across iterations. The optimal
			// choice may be architecture-dependent.
			if MULT16_16(best_den, Rxy) > MULT16_16(Ryy, best_num) {
				best_den = Ryy
				best_num = int32(Rxy)
				best_id = j
			}
			j++
			if j >= N {
				break
			}
		}

		// Updating the sums of the new pulse(s)
		xy = ADD32(xy, EXTEND32(X[best_id]))
		// We're multiplying y[j] by two so we don't have to do it here
		yy = ADD16(yy, y[best_id])

		// Only now that we've made the final choice, update y/iy
		// Multiplying y[j] by 2 so we don't have to do it everywhere else
		y[best_id] += 2
		iy[best_id]++
	}

	// Put the original sign back
	j = 0
	for {
		// iy[j] = signx[j] ? -iy[j] : iy[j];
		// OPT: The is more likely to be compiled without a branch than the code above
		// but has the same performance otherwise.
		iy[j] = (iy[j] ^ -signx[j]) + signx[j]
		j++
		if j >= N {
			break
		}
	}
	return yy
}

// AlgQuant is the algebraic pulse-vector quantiser. The signal x is replaced by
// the sum of the pitch and a combination of pulses such that its norm is still
// equal to 1. This is the function that will typically require the most CPU.
// Returns a mask indicating which blocks in the band received pulses.
//
// C: unsigned alg_quant(celt_norm *X, int N, int K, int spread, int B, ec_enc *enc, opus_val16 gain, int resynth, int arch)
func AlgQuant(X []int16, N, K, spread, B int, enc *EcEnc, gain int16, resynth int, arch int) uint32 {
	var yy int16
	var collapse_mask uint32

	celt_assert2(K > 0, "alg_quant() needs at least one pulse")
	celt_assert2(N > 1, "alg_quant() needs at least two dimensions")

	// Covers vectorization by up to 4.
	iy := make([]int, N+3)

	ExpRotation(X, N, 1, B, K, spread)

	yy = OpPvqSearch(X, iy, K, N, arch)

	EncodePulses(iy, N, K, enc)

	if resynth != 0 {
		normalise_residual(iy, X, N, int32(yy), gain)
		ExpRotation(X, N, -1, B, K, spread)
	}

	collapse_mask = extract_collapse_mask(iy, N, B)
	return collapse_mask
}

// AlgUnquant decodes a pulse vector and combines the result with the pitch
// vector to produce the final normalised signal in the current band.
//
// C: unsigned alg_unquant(celt_norm *X, int N, int K, int spread, int B, ec_dec *dec, opus_val16 gain)
func AlgUnquant(X []int16, N, K, spread, B int, dec *EcDec, gain int16) uint32 {
	var Ryy int32
	var collapse_mask uint32
	celt_assert2(K > 0, "alg_unquant() needs at least one pulse")
	celt_assert2(N > 1, "alg_unquant() needs at least two dimensions")
	iy := make([]int, N)
	Ryy = DecodePulses(iy, N, K, dec)
	normalise_residual(iy, X, N, Ryy, gain)
	ExpRotation(X, N, -1, B, K, spread)
	collapse_mask = extract_collapse_mask(iy, N, B)
	return collapse_mask
}

// C: void renormalise_vector(celt_norm *X, int N, opus_val16 gain, int arch)
func RenormaliseVector(X []int16, N int, gain int16, arch int) {
	var i, k int
	var E int32
	var g int16
	var t int32
	E = EPSILON + InnerProd(X, X, N, arch)
	k = int(Ilog2(E)) >> 1
	t = VSHR32(E, 2*(k-7))
	g = int16(MULT16_16_P15(RsqrtNorm(t), gain))

	for i = 0; i < N; i++ {
		X[i] = EXTRACT16(PSHR32(MULT16_16(g, X[i]), k+1))
	}
	// return celt_sqrt(E);
}

// C: int stereo_itheta(const celt_norm *X, const celt_norm *Y, int stereo, int N, int arch)
func StereoItheta(X, Y []int16, stereo, N int, arch int) int {
	var i, itheta int
	var mid, side int16
	var Emid, Eside int32

	Emid = EPSILON
	Eside = EPSILON
	if stereo != 0 {
		for i = 0; i < N; i++ {
			var m, s int16
			m = ADD16(SHR16(X[i], 1), SHR16(Y[i], 1))
			s = int16(SUB16(SHR16(X[i], 1), SHR16(Y[i], 1)))
			Emid = MAC16_16(Emid, m, m)
			Eside = MAC16_16(Eside, s, s)
		}
	} else {
		Emid += InnerProd(X, X, N, arch)
		Eside += InnerProd(Y, Y, N, arch)
	}
	mid = int16(Sqrt(Emid))
	side = int16(Sqrt(Eside))
	// 0.63662 = 2/pi
	itheta = int(MULT16_16_Q15(QCONST16(0.63662, 15), Atan2p(side, mid)))

	return itheta
}
