package celt

// C: celt/bands.h, celt/bands.c

const (
	SPREAD_NONE       = 0
	SPREAD_LIGHT      = 1
	SPREAD_NORMAL     = 2
	SPREAD_AGGRESSIVE = 3
)

// C: int hysteresis_decision(opus_val16 val, const opus_val16 *thresholds, const opus_val16 *hysteresis, int N, int prev)
func HysteresisDecision(val int16, thresholds []int16, hysteresis []int16, N int, prev int) int {
	var i int
	for i = 0; i < N; i++ {
		if val < thresholds[i] {
			break
		}
	}
	if i > prev && int32(val) < int32(thresholds[prev])+int32(hysteresis[prev]) {
		i = prev
	}
	if i < prev && int32(val) > int32(thresholds[prev-1])-int32(hysteresis[prev-1]) {
		i = prev
	}
	return i
}

// C: opus_uint32 celt_lcg_rand(opus_uint32 seed)
func LcgRand(seed uint32) uint32 {
	return 1664525*seed + 1013904223
}

// This is a cos() approximation designed to be bit-exact on any platform. Bit exactness
// with this approximation is important because it has an impact on the bit allocation.
//
// C: opus_int16 bitexact_cos(opus_int16 x)
func BitexactCos(x int16) int16 {
	var tmp int32
	var x2 int16
	tmp = (4096 + int32(x)*int32(x)) >> 13
	celt_sig_assert(tmp <= 32767)
	x2 = int16(tmp)
	x2 = int16((32767 - int32(x2)) + FRAC_MUL16(x2, (-7651+FRAC_MUL16(x2, (8277+FRAC_MUL16(-626, x2))))))
	celt_sig_assert(x2 <= 32766)
	return 1 + x2
}

// C: int bitexact_log2tan(int isin,int icos)
func BitexactLog2tan(isin, icos int) int {
	var lc, ls int
	lc = EC_ILOG(uint32(icos))
	ls = EC_ILOG(uint32(isin))
	icos <<= uint(15 - lc)
	isin <<= uint(15 - ls)
	return (ls-lc)*(1<<11) +
		int(FRAC_MUL16(isin, FRAC_MUL16(isin, -2597)+7932)) -
		int(FRAC_MUL16(icos, FRAC_MUL16(icos, -2597)+7932))
}

// ComputeBandEnergies computes the amplitude (sqrt energy) in each of the bands.
//
// C: void compute_band_energies(const CELTMode *m, const celt_sig *X, celt_ener *bandE, int end, int C, int LM, int arch)
func ComputeBandEnergies(m *CELTMode, X []int32, bandE []int32, end, C, LM int, arch int) {
	var i, c, N int
	eBands := m.eBands
	_ = arch
	N = m.shortMdctSize << uint(LM)
	c = 0
	for {
		for i = 0; i < end; i++ {
			var j int
			var maxval int32 = 0
			var sum int32 = 0

			maxval = Maxabs32(X[c*N+(int(eBands[i])<<uint(LM)):], int(eBands[i+1]-eBands[i])<<uint(LM))
			if maxval > 0 {
				shift := int(Ilog2(maxval)) - 14 + (((int(m.logN[i]) >> BITRES) + LM + 1) >> 1)
				j = int(eBands[i]) << uint(LM)
				if shift > 0 {
					for {
						sum = MAC16_16(sum, EXTRACT16(SHR32(X[j+c*N], shift)),
							EXTRACT16(SHR32(X[j+c*N], shift)))
						j++
						if j >= int(eBands[i+1])<<uint(LM) {
							break
						}
					}
				} else {
					for {
						sum = MAC16_16(sum, EXTRACT16(SHL32(X[j+c*N], -shift)),
							EXTRACT16(SHL32(X[j+c*N], -shift)))
						j++
						if j >= int(eBands[i+1])<<uint(LM) {
							break
						}
					}
				}
				// We're adding one here to ensure the normalized band isn't larger than unity norm
				bandE[i+c*m.nbEBands] = EPSILON + VSHR32(EXTEND32(Sqrt(sum)), -shift)
			} else {
				bandE[i+c*m.nbEBands] = EPSILON
			}
			// printf ("%f ", bandE[i+c*m->nbEBands]);
		}
		c++
		if c >= C {
			break
		}
	}
	// printf ("\n");
}

// NormaliseBands normalises each band such that the energy is one.
//
// C: void normalise_bands(const CELTMode *m, const celt_sig * OPUS_RESTRICT freq, celt_norm * OPUS_RESTRICT X, const celt_ener *bandE, int end, int C, int M)
func NormaliseBands(m *CELTMode, freq []int32, X []int16, bandE []int32, end, C, M int) {
	var i, c, N int
	eBands := m.eBands
	N = M * m.shortMdctSize
	c = 0
	for {
		i = 0
		for {
			var g int16
			var j, shift int
			var E int16
			shift = int(Zlog2(bandE[i+c*m.nbEBands])) - 13
			E = int16(VSHR32(bandE[i+c*m.nbEBands], shift))
			g = EXTRACT16(Rcp(SHL32(E, 3)))
			j = M * int(eBands[i])
			for {
				X[j+c*N] = int16(MULT16_16_Q15(VSHR32(freq[j+c*N], shift-1), g))
				j++
				if j >= M*int(eBands[i+1]) {
					break
				}
			}
			i++
			if i >= end {
				break
			}
		}
		c++
		if c >= C {
			break
		}
	}
}

// DenormaliseBands de-normalises the energy to produce the synthesis from the unit-energy bands.
//
// C: void denormalise_bands(const CELTMode *m, const celt_norm * OPUS_RESTRICT X,
//
//	celt_sig * OPUS_RESTRICT freq, const opus_val16 *bandLogE, int start,
//	int end, int M, int downsample, int silence)
func DenormaliseBands(m *CELTMode, X []int16, freq []int32, bandLogE []int16, start, end, M, downsample, silence int) {
	var i, N int
	var bound int
	var f []int32
	var x []int16
	eBands := m.eBands
	N = M * m.shortMdctSize
	bound = M * int(eBands[end])
	if downsample != 1 {
		bound = IMIN(bound, N/downsample)
	}
	if silence != 0 {
		bound = 0
		start = 0
		end = 0
	}
	f = freq
	x = X[M*int(eBands[start]):]
	fi, xi := 0, 0
	for i = 0; i < M*int(eBands[start]); i++ {
		f[fi] = 0
		fi++
	}
	for i = start; i < end; i++ {
		var j, band_end int
		var g int16
		var lg int16
		var shift int
		j = M * int(eBands[i])
		band_end = M * int(eBands[i+1])
		lg = SATURATE16(ADD32(bandLogE[i], SHL32(int32(eMeans[i]), 6)))
		// Handle the integer part of the log energy
		shift = 16 - int(lg>>DB_SHIFT)
		if shift > 31 {
			shift = 0
			g = 0
		} else {
			// Handle the fractional part.
			g = int16(Exp2Frac(lg & ((1 << DB_SHIFT) - 1)))
		}
		// Handle extreme gains with negative shift.
		if shift < 0 {
			// For shift <= -2 and g > 16384 we'd be likely to overflow, so we're
			// capping the gain here, which is equivalent to a cap of 18 on lg.
			// This shouldn't trigger unless the bitstream is already corrupted.
			if shift <= -2 {
				g = 16384
				shift = -2
			}
			for {
				f[fi] = SHL32(MULT16_16(x[xi], g), -shift)
				fi++
				xi++
				j++
				if j >= band_end {
					break
				}
			}
		} else {
			// Be careful of the fixed-point "else" just above when changing this code
			for {
				f[fi] = SHR32(MULT16_16(x[xi], g), shift)
				fi++
				xi++
				j++
				if j >= band_end {
					break
				}
			}
		}
	}
	celt_assert(start <= end)
	clear(freq[bound:N])
}

// AntiCollapse prevents energy collapse for transients with multiple short MDCTs.
//
// C: void anti_collapse(const CELTMode *m, celt_norm *X_,
//
//	unsigned char *collapse_masks, int LM, int C, int size, int start,
//	int end, const opus_val16 *logE, const opus_val16 *prev1logE,
//	const opus_val16 *prev2logE, const int *pulses, opus_uint32 seed,
//	int arch)
func AntiCollapse(m *CELTMode, X_ []int16, collapse_masks []uint8, LM, C, size int,
	start, end int, logE, prev1logE, prev2logE []int16, pulses []int, seed uint32, arch int) {
	var c, i, j, k int
	for i = start; i < end; i++ {
		var N0 int
		var thresh, sqrt_1 int16
		var depth int
		var shift int
		var thresh32 int32

		N0 = int(m.eBands[i+1] - m.eBands[i])
		// depth in 1/8 bits
		celt_sig_assert(pulses[i] >= 0)
		depth = int(Udiv(uint32(1+pulses[i]), uint32(m.eBands[i+1]-m.eBands[i]))) >> uint(LM)

		thresh32 = SHR32(Exp2(-SHL16(depth, 10-BITRES)), 1)
		thresh = int16(MULT16_32_Q15(QCONST16(0.5, 15), MIN32(32767, thresh32)))
		{
			var t int32
			t = int32(N0 << uint(LM))
			shift = int(Ilog2(t)) >> 1
			t = SHL32(t, (7-shift)<<1)
			sqrt_1 = RsqrtNorm(t)
		}

		c = 0
		for {
			var X []int16
			var prev1, prev2 int16
			var Ediff int32
			var r int16
			var renormalize int = 0
			prev1 = prev1logE[c*m.nbEBands+i]
			prev2 = prev2logE[c*m.nbEBands+i]
			if C == 1 {
				prev1 = MAX16(prev1, prev1logE[m.nbEBands+i])
				prev2 = MAX16(prev2, prev2logE[m.nbEBands+i])
			}
			Ediff = EXTEND32(logE[c*m.nbEBands+i]) - EXTEND32(MIN16(prev1, prev2))
			Ediff = MAX32(0, Ediff)

			if Ediff < 16384 {
				var r32 int32 = SHR32(Exp2(-EXTRACT16(Ediff)), 1)
				r = int16(2 * MIN16(16383, r32))
			} else {
				r = 0
			}
			if LM == 3 {
				r = int16(MULT16_16_Q14(23170, MIN32(23169, int32(r))))
			}
			r = int16(SHR16(MIN16(thresh, r), 1))
			r = int16(SHR32(MULT16_16_Q15(sqrt_1, r), shift))

			X = X_[c*size+(int(m.eBands[i])<<uint(LM)):]
			for k = 0; k < 1<<uint(LM); k++ {
				// Detect collapse
				if collapse_masks[i*C+c]&(1<<uint(k)) == 0 {
					// Fill with noise
					for j = 0; j < N0; j++ {
						seed = LcgRand(seed)
						if seed&0x8000 != 0 {
							X[(j<<uint(LM))+k] = r
						} else {
							X[(j<<uint(LM))+k] = -r
						}
					}
					renormalize = 1
				}
			}
			// We just added some energy, so we need to renormalise
			if renormalize != 0 {
				RenormaliseVector(X, N0<<uint(LM), Q15ONE, arch)
			}
			c++
			if c >= C {
				break
			}
		}
	}
}

// Compute the weights to use for optimizing normalized distortion across
// channels. We use the amplitude to weight square distortion, which means
// that we use the square root of the value we would have been using if we
// wanted to minimize the MSE in the non-normalized domain. This roughly
// corresponds to some quick-and-dirty perceptual experiments I ran to
// measure inter-aural masking (there doesn't seem to be any published data
// on the topic).
//
// C: static void compute_channel_weights(celt_ener Ex, celt_ener Ey, opus_val16 w[2])
func compute_channel_weights(Ex, Ey int32, w *[2]int16) {
	var minE int32
	var shift int
	minE = MIN32(Ex, Ey)
	// Adjustment to make the weights a bit more conservative.
	Ex = ADD32(Ex, minE/3)
	Ey = ADD32(Ey, minE/3)
	shift = int(Ilog2(EPSILON+MAX32(Ex, Ey))) - 14
	w[0] = int16(VSHR32(Ex, shift))
	w[1] = int16(VSHR32(Ey, shift))
}

// C: static void intensity_stereo(const CELTMode *m, celt_norm * OPUS_RESTRICT X, const celt_norm * OPUS_RESTRICT Y, const celt_ener *bandE, int bandID, int N)
func intensity_stereo(m *CELTMode, X, Y []int16, bandE []int32, bandID, N int) {
	i := bandID
	var j int
	var a1, a2 int16
	var left, right int16
	var norm int16
	shift := int(Zlog2(MAX32(bandE[i], bandE[i+m.nbEBands]))) - 13
	left = int16(VSHR32(bandE[i], shift))
	right = int16(VSHR32(bandE[i+m.nbEBands], shift))
	norm = int16(EPSILON + Sqrt(EPSILON+MULT16_16(left, left)+MULT16_16(right, right)))
	a1 = DIV32_16(SHL32(EXTEND32(left), 14), norm)
	a2 = DIV32_16(SHL32(EXTEND32(right), 14), norm)
	for j = 0; j < N; j++ {
		var r, l int16
		l = X[j]
		r = Y[j]
		X[j] = EXTRACT16(SHR32(MAC16_16(MULT16_16(a1, l), a2, r), 14))
		// Side is not encoded, no need to calculate
	}
}

// C: static void stereo_split(celt_norm * OPUS_RESTRICT X, celt_norm * OPUS_RESTRICT Y, int N)
func stereo_split(X, Y []int16, N int) {
	var j int
	for j = 0; j < N; j++ {
		var r, l int32
		l = MULT16_16(QCONST16(.70710678, 15), X[j])
		r = MULT16_16(QCONST16(.70710678, 15), Y[j])
		X[j] = EXTRACT16(SHR32(ADD32(l, r), 15))
		Y[j] = EXTRACT16(SHR32(SUB32(r, l), 15))
	}
}

// C: static void stereo_merge(celt_norm * OPUS_RESTRICT X, celt_norm * OPUS_RESTRICT Y, opus_val16 mid, int N, int arch)
func stereo_merge(X, Y []int16, mid int16, N int, arch int) {
	var j int
	var xp, side int32 = 0, 0
	var El, Er int32
	var mid2 int16
	var kl, kr int
	var t, lgain, rgain int32

	// Compute the norm of X+Y and X-Y as |X|^2 + |Y|^2 +/- sum(xy)
	DualInnerProd(Y, X, Y, N, &xp, &side, arch)
	// Compensating for the mid normalization
	xp = MULT16_32_Q15(mid, xp)
	// mid and side are in Q15, not Q14 like X and Y
	mid2 = int16(SHR16(mid, 1))
	El = MULT16_16(mid2, mid2) + side - 2*xp
	Er = MULT16_16(mid2, mid2) + side + 2*xp
	if Er < QCONST32(6e-4, 28) || El < QCONST32(6e-4, 28) {
		copy(Y[:N], X[:N])
		return
	}

	kl = int(Ilog2(El)) >> 1
	kr = int(Ilog2(Er)) >> 1
	t = VSHR32(El, (kl-7)<<1)
	lgain = int32(RsqrtNorm(t))
	t = VSHR32(Er, (kr-7)<<1)
	rgain = int32(RsqrtNorm(t))

	if kl < 7 {
		kl = 7
	}
	if kr < 7 {
		kr = 7
	}

	for j = 0; j < N; j++ {
		var r, l int16
		// Apply mid scaling (side is already scaled)
		l = int16(MULT16_16_P15(mid, X[j]))
		r = Y[j]
		X[j] = EXTRACT16(PSHR32(MULT16_16(lgain, SUB16(l, r)), kl+1))
		Y[j] = EXTRACT16(PSHR32(MULT16_16(rgain, ADD16(l, r)), kr+1))
	}
}

// SpreadingDecision decides whether we should spread the pulses in the current frame.
//
// C: int spreading_decision(const CELTMode *m, const celt_norm *X, int *average,
//
//	int last_decision, int *hf_average, int *tapset_decision, int update_hf,
//	int end, int C, int M, const int *spread_weight)
func SpreadingDecision(m *CELTMode, X []int16, average *int,
	last_decision int, hf_average, tapset_decision *int, update_hf int,
	end, C, M int, spread_weight []int) int {
	var i, c, N0 int
	var sum, nbBands int = 0, 0
	eBands := m.eBands
	var decision int
	var hf_sum int = 0

	celt_assert(end > 0)

	N0 = M * m.shortMdctSize

	if M*int(eBands[end]-eBands[end-1]) <= 8 {
		return SPREAD_NONE
	}
	c = 0
	for {
		for i = 0; i < end; i++ {
			var j, N, tmp int
			var tcount = [3]int{0, 0, 0}
			x := X[M*int(eBands[i])+c*N0:]
			N = M * int(eBands[i+1]-eBands[i])
			if N <= 8 {
				continue
			}
			// Compute rough CDF of |x[j]|
			for j = 0; j < N; j++ {
				var x2N int32 // Q13

				x2N = MULT16_16(MULT16_16_Q15(x[j], x[j]), N)
				if x2N < int32(QCONST16(0.25, 13)) {
					tcount[0]++
				}
				if x2N < int32(QCONST16(0.0625, 13)) {
					tcount[1]++
				}
				if x2N < int32(QCONST16(0.015625, 13)) {
					tcount[2]++
				}
			}

			// Only include four last bands (8 kHz and up)
			if i > m.nbEBands-4 {
				hf_sum += int(Udiv(uint32(32*(tcount[1]+tcount[0])), uint32(N)))
			}
			tmp = b2i(2*tcount[2] >= N) + b2i(2*tcount[1] >= N) + b2i(2*tcount[0] >= N)
			sum += tmp * spread_weight[i]
			nbBands += spread_weight[i]
		}
		c++
		if c >= C {
			break
		}
	}

	if update_hf != 0 {
		if hf_sum != 0 {
			hf_sum = int(Udiv(uint32(hf_sum), uint32(C*(4-m.nbEBands+end))))
		}
		*hf_average = (*hf_average + hf_sum) >> 1
		hf_sum = *hf_average
		if *tapset_decision == 2 {
			hf_sum += 4
		} else if *tapset_decision == 0 {
			hf_sum -= 4
		}
		if hf_sum > 22 {
			*tapset_decision = 2
		} else if hf_sum > 18 {
			*tapset_decision = 1
		} else {
			*tapset_decision = 0
		}
	}
	// printf("%d %d %d\n", hf_sum, *hf_average, *tapset_decision);
	celt_assert(nbBands > 0) // end has to be non-zero
	celt_assert(sum >= 0)
	sum = int(Udiv(uint32(int32(sum)<<8), uint32(nbBands)))
	// Recursive averaging
	sum = (sum + *average) >> 1
	*average = sum
	// Hysteresis
	sum = (3*sum + (((3 - last_decision) << 7) + 64) + 2) >> 2
	if sum < 80 {
		decision = SPREAD_AGGRESSIVE
	} else if sum < 256 {
		decision = SPREAD_NORMAL
	} else if sum < 384 {
		decision = SPREAD_LIGHT
	} else {
		decision = SPREAD_NONE
	}
	return decision
}

// b2i converts a C boolean expression to int.
func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Indexing table for converting from natural Hadamard to ordery Hadamard
// This is essentially a bit-reversed Gray, on top of which we've added
// an inversion of the order because we want the DC at the end rather than
// the beginning. The lines are for N=2, 4, 8, 16
var ordery_table = []int{
	1, 0,
	3, 0, 2, 1,
	7, 0, 4, 3, 6, 1, 5, 2,
	15, 0, 8, 7, 12, 3, 11, 4, 14, 1, 9, 6, 13, 2, 10, 5,
}

// C: static void deinterleave_hadamard(celt_norm *X, int N0, int stride, int hadamard)
func deinterleave_hadamard(X []int16, N0, stride, hadamard int) {
	var i, j, N int
	N = N0 * stride
	tmp := make([]int16, N)
	celt_assert(stride > 0)
	if hadamard != 0 {
		ordery := ordery_table[stride-2:]
		for i = 0; i < stride; i++ {
			for j = 0; j < N0; j++ {
				tmp[ordery[i]*N0+j] = X[j*stride+i]
			}
		}
	} else {
		for i = 0; i < stride; i++ {
			for j = 0; j < N0; j++ {
				tmp[i*N0+j] = X[j*stride+i]
			}
		}
	}
	copy(X[:N], tmp)
}

// C: static void interleave_hadamard(celt_norm *X, int N0, int stride, int hadamard)
func interleave_hadamard(X []int16, N0, stride, hadamard int) {
	var i, j, N int
	N = N0 * stride
	tmp := make([]int16, N)
	if hadamard != 0 {
		ordery := ordery_table[stride-2:]
		for i = 0; i < stride; i++ {
			for j = 0; j < N0; j++ {
				tmp[j*stride+i] = X[ordery[i]*N0+j]
			}
		}
	} else {
		for i = 0; i < stride; i++ {
			for j = 0; j < N0; j++ {
				tmp[j*stride+i] = X[i*N0+j]
			}
		}
	}
	copy(X[:N], tmp)
}

// C: void haar1(celt_norm *X, int N0, int stride)
func Haar1(X []int16, N0, stride int) {
	var i, j int
	N0 >>= 1
	for i = 0; i < stride; i++ {
		for j = 0; j < N0; j++ {
			var tmp1, tmp2 int32
			tmp1 = MULT16_16(QCONST16(.70710678, 15), X[stride*2*j+i])
			tmp2 = MULT16_16(QCONST16(.70710678, 15), X[stride*(2*j+1)+i])
			X[stride*2*j+i] = EXTRACT16(PSHR32(ADD32(tmp1, tmp2), 15))
			X[stride*(2*j+1)+i] = EXTRACT16(PSHR32(SUB32(tmp1, tmp2), 15))
		}
	}
}

// C: static int compute_qn(int N, int b, int offset, int pulse_cap, int stereo)
func compute_qn(N, b, offset, pulse_cap, stereo int) int {
	var exp2_table8 = [8]int16{16384, 17866, 19483, 21247, 23170, 25267, 27554, 30048}
	var qn, qb int
	N2 := 2*N - 1
	if stereo != 0 && N == 2 {
		N2--
	}
	// The upper limit ensures that in a stereo split with itheta==16384, we'll
	// always have enough bits left over to code at least one pulse in the
	// side; otherwise it would collapse, since it doesn't get folded.
	qb = int(Sudiv(int32(b+N2*offset), int32(N2)))
	qb = IMIN(b-pulse_cap-(4<<BITRES), qb)

	qb = IMIN(8<<BITRES, qb)

	if qb < (1 << BITRES >> 1) {
		qn = 1
	} else {
		qn = int(exp2_table8[qb&0x7]) >> uint(14-(qb>>BITRES))
		qn = (qn + 1) >> 1 << 1
	}
	celt_assert(qn <= 256)
	return qn
}

// C: struct band_ctx
type band_ctx struct {
	encode            int
	resynth           int
	m                 *CELTMode
	i                 int
	intensity         int
	spread            int
	tf_change         int
	ec                *EcCtx
	remaining_bits    int32
	bandE             []int32
	seed              uint32
	arch              int
	theta_round       int
	disable_inv       int
	avoid_split_noise int
}

// C: struct split_ctx
type split_ctx struct {
	inv    int
	imid   int
	iside  int
	delta  int
	itheta int
	qalloc int
}

// C: static void compute_theta(struct band_ctx *ctx, struct split_ctx *sctx,
//
//	celt_norm *X, celt_norm *Y, int N, int *b, int B, int B0,
//	int LM,
//	int stereo, int *fill)
func compute_theta(ctx *band_ctx, sctx *split_ctx,
	X, Y []int16, N int, b *int, B, B0 int,
	LM int,
	stereo int, fill *int) {
	var qn int
	var itheta int = 0
	var delta int
	var imid, iside int
	var qalloc int
	var pulse_cap int
	var offset int
	var tell int32
	var inv int = 0
	var encode int
	var m *CELTMode
	var i int
	var intensity int
	var ec *EcCtx
	var bandE []int32

	encode = ctx.encode
	m = ctx.m
	i = ctx.i
	intensity = ctx.intensity
	ec = ctx.ec
	bandE = ctx.bandE

	// Decide on the resolution to give to the split parameter theta
	pulse_cap = int(m.logN[i]) + LM*(1<<BITRES)
	if stereo != 0 && N == 2 {
		offset = (pulse_cap >> 1) - QTHETA_OFFSET_TWOPHASE
	} else {
		offset = (pulse_cap >> 1) - QTHETA_OFFSET
	}
	qn = compute_qn(N, *b, offset, pulse_cap, stereo)
	if stereo != 0 && i >= intensity {
		qn = 1
	}
	if encode != 0 {
		// theta is the atan() of the ratio between the (normalized)
		// side and mid. With just that parameter, we can re-scale both
		// mid and side because we know that 1) they have unit norm and
		// 2) they are orthogonal.
		itheta = StereoItheta(X, Y, stereo, N, ctx.arch)
	}
	tell = int32(ec.TellFrac())
	if qn != 1 {
		if encode != 0 {
			if stereo == 0 || ctx.theta_round == 0 {
				itheta = int((int32(itheta)*int32(qn) + 8192) >> 14)
				if stereo == 0 && ctx.avoid_split_noise != 0 && itheta > 0 && itheta < qn {
					// Check if the selected value of theta will cause the bit allocation
					// to inject noise on one side. If so, make sure the energy of that side
					// is zero.
					var unquantized int = int(Udiv(uint32(int32(itheta)*16384), uint32(qn)))
					imid = int(BitexactCos(int16(unquantized)))
					iside = int(BitexactCos(int16(16384 - unquantized)))
					delta = int(FRAC_MUL16((N-1)<<7, BitexactLog2tan(iside, imid)))
					if delta > *b {
						itheta = qn
					} else if delta < -*b {
						itheta = 0
					}
				}
			} else {
				var down int
				// Bias quantization towards itheta=0 and itheta=16384.
				var bias int
				if itheta > 8192 {
					bias = 32767 / qn
				} else {
					bias = -32767 / qn
				}
				down = IMIN(qn-1, IMAX(0, int((int32(itheta)*int32(qn)+int32(bias))>>14)))
				if ctx.theta_round < 0 {
					itheta = down
				} else {
					itheta = down + 1
				}
			}
		}
		// Entropy coding of the angle. We use a uniform pdf for the
		// time split, a step for stereo, and a triangular one for the rest.
		if stereo != 0 && N > 2 {
			p0 := 3
			x := itheta
			x0 := qn / 2
			ft := p0*(x0+1) + x0
			// Use a probability of p0 up to itheta=8192 and then use 1 after
			if encode != 0 {
				var fl, fh int
				if x <= x0 {
					fl = p0 * x
					fh = p0 * (x + 1)
				} else {
					fl = (x - 1 - x0) + (x0+1)*p0
					fh = (x - x0) + (x0+1)*p0
				}
				ec.Encode(uint32(fl), uint32(fh), uint32(ft))
			} else {
				var fs int
				fs = int(ec.Decode(uint32(ft)))
				if fs < (x0+1)*p0 {
					x = fs / p0
				} else {
					x = x0 + 1 + (fs - (x0+1)*p0)
				}
				var fl, fh int
				if x <= x0 {
					fl = p0 * x
					fh = p0 * (x + 1)
				} else {
					fl = (x - 1 - x0) + (x0+1)*p0
					fh = (x - x0) + (x0+1)*p0
				}
				ec.DecUpdate(uint32(fl), uint32(fh), uint32(ft))
				itheta = x
			}
		} else if B0 > 1 || stereo != 0 {
			// Uniform pdf
			if encode != 0 {
				ec.EncUint(uint32(itheta), uint32(qn+1))
			} else {
				itheta = int(ec.DecUint(uint32(qn + 1)))
			}
		} else {
			var fs, ft int = 1, 0
			ft = ((qn >> 1) + 1) * ((qn >> 1) + 1)
			if encode != 0 {
				var fl int

				if itheta <= (qn >> 1) {
					fs = itheta + 1
				} else {
					fs = qn + 1 - itheta
				}
				if itheta <= (qn >> 1) {
					fl = itheta * (itheta + 1) >> 1
				} else {
					fl = ft - ((qn + 1 - itheta) * (qn + 2 - itheta) >> 1)
				}

				ec.Encode(uint32(fl), uint32(fl+fs), uint32(ft))
			} else {
				// Triangular pdf
				var fl int = 0
				var fm int
				fm = int(ec.Decode(uint32(ft)))

				if fm < ((qn >> 1) * ((qn >> 1) + 1) >> 1) {
					itheta = (int(Isqrt32(8*uint32(fm)+1)) - 1) >> 1
					fs = itheta + 1
					fl = itheta * (itheta + 1) >> 1
				} else {
					itheta = (2*(qn+1) - int(Isqrt32(8*uint32(ft-fm-1)+1))) >> 1
					fs = qn + 1 - itheta
					fl = ft - ((qn + 1 - itheta) * (qn + 2 - itheta) >> 1)
				}

				ec.DecUpdate(uint32(fl), uint32(fl+fs), uint32(ft))
			}
		}
		celt_assert(itheta >= 0)
		itheta = int(Udiv(uint32(int32(itheta)*16384), uint32(qn)))
		if encode != 0 && stereo != 0 {
			if itheta == 0 {
				intensity_stereo(m, X, Y, bandE, i, N)
			} else {
				stereo_split(X, Y, N)
			}
		}
		// NOTE: Renormalising X and Y *may* help fixed-point a bit at very high rate.
		// Let's do that at higher complexity
	} else if stereo != 0 {
		if encode != 0 {
			inv = b2i(itheta > 8192 && ctx.disable_inv == 0)
			if inv != 0 {
				var j int
				for j = 0; j < N; j++ {
					Y[j] = -Y[j]
				}
			}
			intensity_stereo(m, X, Y, bandE, i, N)
		}
		if *b > 2<<BITRES && ctx.remaining_bits > 2<<BITRES {
			if encode != 0 {
				ec.EncBitLogp(inv, 2)
			} else {
				inv = ec.DecBitLogp(2)
			}
		} else {
			inv = 0
		}
		// inv flag override to avoid problems with downmixing.
		if ctx.disable_inv != 0 {
			inv = 0
		}
		itheta = 0
	}
	qalloc = int(ec.TellFrac()) - int(tell)
	*b -= qalloc

	if itheta == 0 {
		imid = 32767
		iside = 0
		*fill &= (1 << uint(B)) - 1
		delta = -16384
	} else if itheta == 16384 {
		imid = 0
		iside = 32767
		*fill &= ((1 << uint(B)) - 1) << uint(B)
		delta = 16384
	} else {
		imid = int(BitexactCos(int16(itheta)))
		iside = int(BitexactCos(int16(16384 - itheta)))
		// This is the mid vs side allocation that minimizes squared error
		// in that band.
		delta = int(FRAC_MUL16((N-1)<<7, BitexactLog2tan(iside, imid)))
	}

	sctx.inv = inv
	sctx.imid = imid
	sctx.iside = iside
	sctx.delta = delta
	sctx.itheta = itheta
	sctx.qalloc = qalloc
}

// C: static unsigned quant_band_n1(struct band_ctx *ctx, celt_norm *X, celt_norm *Y, celt_norm *lowband_out)
func quant_band_n1(ctx *band_ctx, X, Y []int16, lowband_out []int16) uint32 {
	var c int
	var stereo int
	x := X
	var encode int
	var ec *EcCtx

	encode = ctx.encode
	ec = ctx.ec

	stereo = b2i(Y != nil)
	c = 0
	for {
		var sign int = 0
		if ctx.remaining_bits >= 1<<BITRES {
			if encode != 0 {
				sign = b2i(x[0] < 0)
				ec.EncBits(uint32(sign), 1)
			} else {
				sign = int(ec.DecBits(1))
			}
			ctx.remaining_bits -= 1 << BITRES
		}
		if ctx.resynth != 0 {
			if sign != 0 {
				x[0] = -NORM_SCALING
			} else {
				x[0] = NORM_SCALING
			}
		}
		x = Y
		c++
		if c >= 1+stereo {
			break
		}
	}
	if lowband_out != nil {
		lowband_out[0] = int16(SHR16(X[0], 4))
	}
	return 1
}

// This function is responsible for encoding and decoding a mono partition.
// It can split the band in two and transmit the energy difference with
// the two half-bands. It can be called recursively so bands can end up being
// split in 8 parts.
//
// C: static unsigned quant_partition(struct band_ctx *ctx, celt_norm *X,
//
//	int N, int b, int B, celt_norm *lowband,
//	int LM,
//	opus_val16 gain, int fill)
func quant_partition(ctx *band_ctx, X []int16,
	N, b, B int, lowband []int16,
	LM int,
	gain int16, fill int) uint32 {
	var cache []uint8
	var q int
	var curr_bits int
	var imid, iside int = 0, 0
	B0 := B
	var mid, side int16 = 0, 0
	var cm uint32 = 0
	var Y []int16 = nil
	var encode int
	var m *CELTMode
	var i int
	var spread int
	var ec *EcCtx

	encode = ctx.encode
	m = ctx.m
	i = ctx.i
	spread = ctx.spread
	ec = ctx.ec

	// If we need 1.5 more bit than we can produce, split the band in two.
	cache = m.cache.bits[m.cache.index[(LM+1)*m.nbEBands+i]:]
	if LM != -1 && b > int(cache[cache[0]])+12 && N > 2 {
		var mbits, sbits, delta int
		var itheta int
		var qalloc int
		var sctx split_ctx
		var next_lowband2 []int16 = nil
		var rebalance int32

		N >>= 1
		Y = X[N:]
		LM -= 1
		if B == 1 {
			fill = (fill & 1) | (fill << 1)
		}
		B = (B + 1) >> 1

		compute_theta(ctx, &sctx, X, Y, N, &b, B, B0, LM, 0, &fill)
		imid = sctx.imid
		iside = sctx.iside
		delta = sctx.delta
		itheta = sctx.itheta
		qalloc = sctx.qalloc
		mid = int16(imid)
		side = int16(iside)

		// Give more bits to low-energy MDCTs than they would otherwise deserve
		if B0 > 1 && (itheta&0x3fff) != 0 {
			if itheta > 8192 {
				// Rough approximation for pre-echo masking
				delta -= delta >> uint(4-LM)
			} else {
				// Corresponds to a forward-masking slope of 1.5 dB per 10 ms
				delta = IMIN(0, delta+(N<<BITRES>>uint(5-LM)))
			}
		}
		mbits = IMAX(0, IMIN(b, (b-delta)/2))
		sbits = b - mbits
		ctx.remaining_bits -= int32(qalloc)

		if lowband != nil {
			next_lowband2 = lowband[N:] // >32-bit split case
		}

		rebalance = ctx.remaining_bits
		if mbits >= sbits {
			cm = quant_partition(ctx, X, N, mbits, B, lowband, LM,
				int16(MULT16_16_P15(gain, mid)), fill)
			rebalance = int32(mbits) - (rebalance - ctx.remaining_bits)
			if rebalance > 3<<BITRES && itheta != 0 {
				sbits += int(rebalance) - (3 << BITRES)
			}
			cm |= quant_partition(ctx, Y, N, sbits, B, next_lowband2, LM,
				int16(MULT16_16_P15(gain, side)), fill>>uint(B)) << uint(B0>>1)
		} else {
			cm = quant_partition(ctx, Y, N, sbits, B, next_lowband2, LM,
				int16(MULT16_16_P15(gain, side)), fill>>uint(B)) << uint(B0>>1)
			rebalance = int32(sbits) - (rebalance - ctx.remaining_bits)
			if rebalance > 3<<BITRES && itheta != 16384 {
				mbits += int(rebalance) - (3 << BITRES)
			}
			cm |= quant_partition(ctx, X, N, mbits, B, lowband, LM,
				int16(MULT16_16_P15(gain, mid)), fill)
		}
	} else {
		// This is the basic no-split case
		q = bits2pulses(m, i, LM, b)
		curr_bits = pulses2bits(m, i, LM, q)
		ctx.remaining_bits -= int32(curr_bits)

		// Ensures we can never bust the budget
		for ctx.remaining_bits < 0 && q > 0 {
			ctx.remaining_bits += int32(curr_bits)
			q--
			curr_bits = pulses2bits(m, i, LM, q)
			ctx.remaining_bits -= int32(curr_bits)
		}

		if q != 0 {
			K := get_pulses(q)

			// Finally do the actual quantization
			if encode != 0 {
				cm = AlgQuant(X, N, K, spread, B, ec, gain, ctx.resynth, ctx.arch)
			} else {
				cm = AlgUnquant(X, N, K, spread, B, ec, gain)
			}
		} else {
			// If there's no pulse, fill the band anyway
			var j int
			if ctx.resynth != 0 {
				var cm_mask uint32
				// B can be as large as 16, so this shift might overflow an int on a
				// 16-bit platform; use a long to get defined behavior.
				cm_mask = uint32(uint64(1)<<uint(B)) - 1
				fill &= int(cm_mask)
				if fill == 0 {
					clear(X[:N])
				} else {
					if lowband == nil {
						// Noise
						for j = 0; j < N; j++ {
							ctx.seed = LcgRand(ctx.seed)
							X[j] = int16(int32(ctx.seed) >> 20)
						}
						cm = cm_mask
					} else {
						// Folded spectrum
						for j = 0; j < N; j++ {
							var tmp int16
							ctx.seed = LcgRand(ctx.seed)
							// About 48 dB below the "normal" folding level
							tmp = QCONST16(1.0/256, 10)
							if (ctx.seed)&0x8000 == 0 {
								tmp = -tmp
							}
							X[j] = lowband[j] + tmp
						}
						cm = uint32(fill)
					}
					RenormaliseVector(X, N, gain, ctx.arch)
				}
			}
		}
	}

	return cm
}

// This function is responsible for encoding and decoding a band for the mono case.
//
// C: static unsigned quant_band(struct band_ctx *ctx, celt_norm *X,
//
//	int N, int b, int B, celt_norm *lowband,
//	int LM, celt_norm *lowband_out,
//	opus_val16 gain, celt_norm *lowband_scratch, int fill)
func quant_band(ctx *band_ctx, X []int16,
	N, b, B int, lowband []int16,
	LM int, lowband_out []int16,
	gain int16, lowband_scratch []int16, fill int) uint32 {
	N0 := N
	N_B := N
	var N_B0 int
	B0 := B
	var time_divide int = 0
	var recombine int = 0
	var longBlocks int
	var cm uint32 = 0
	var k int
	var encode int
	var tf_change int

	encode = ctx.encode
	tf_change = ctx.tf_change

	longBlocks = b2i(B0 == 1)

	N_B = int(Udiv(uint32(N_B), uint32(B)))

	// Special case for one sample
	if N == 1 {
		return quant_band_n1(ctx, X, nil, lowband_out)
	}

	if tf_change > 0 {
		recombine = tf_change
	}
	// Band recombining to increase frequency resolution

	if lowband_scratch != nil && lowband != nil && (recombine != 0 || ((N_B&1) == 0 && tf_change < 0) || B0 > 1) {
		copy(lowband_scratch[:N], lowband[:N])
		lowband = lowband_scratch
	}

	for k = 0; k < recombine; k++ {
		if encode != 0 {
			Haar1(X, N>>uint(k), 1<<uint(k))
		}
		if lowband != nil {
			Haar1(lowband, N>>uint(k), 1<<uint(k))
		}
		fill = int(bit_interleave_table[fill&0xF]) | int(bit_interleave_table[fill>>4])<<2
	}
	B >>= uint(recombine)
	N_B <<= uint(recombine)

	// Increasing the time resolution
	for (N_B&1) == 0 && tf_change < 0 {
		if encode != 0 {
			Haar1(X, N_B, B)
		}
		if lowband != nil {
			Haar1(lowband, N_B, B)
		}
		fill |= fill << uint(B)
		B <<= 1
		N_B >>= 1
		time_divide++
		tf_change++
	}
	B0 = B
	N_B0 = N_B

	// Reorganize the samples in time order instead of frequency order
	if B0 > 1 {
		if encode != 0 {
			deinterleave_hadamard(X, N_B>>uint(recombine), B0<<uint(recombine), longBlocks)
		}
		if lowband != nil {
			deinterleave_hadamard(lowband, N_B>>uint(recombine), B0<<uint(recombine), longBlocks)
		}
	}

	cm = quant_partition(ctx, X, N, b, B, lowband, LM, gain, fill)

	// This code is used by the decoder and by the resynthesis-enabled encoder
	if ctx.resynth != 0 {
		// Undo the sample reorganization going from time order to frequency order
		if B0 > 1 {
			interleave_hadamard(X, N_B>>uint(recombine), B0<<uint(recombine), longBlocks)
		}

		// Undo time-freq changes that we did earlier
		N_B = N_B0
		B = B0
		for k = 0; k < time_divide; k++ {
			B >>= 1
			N_B <<= 1
			cm |= cm >> uint(B)
			Haar1(X, N_B, B)
		}

		for k = 0; k < recombine; k++ {
			cm = uint32(bit_deinterleave_table[cm])
			Haar1(X, N0>>uint(k), 1<<uint(k))
		}
		B <<= uint(recombine)

		// Scale output for later folding
		if lowband_out != nil {
			var j int
			var n int16
			n = int16(Sqrt(SHL32(EXTEND32(N0), 22)))
			for j = 0; j < N0; j++ {
				lowband_out[j] = int16(MULT16_16_Q15(n, X[j]))
			}
		}
		cm &= (1 << uint(B)) - 1
	}
	return cm
}

var bit_interleave_table = [16]uint8{
	0, 1, 1, 1, 2, 3, 3, 3, 2, 3, 3, 3, 2, 3, 3, 3,
}

var bit_deinterleave_table = [16]uint8{
	0x00, 0x03, 0x0C, 0x0F, 0x30, 0x33, 0x3C, 0x3F,
	0xC0, 0xC3, 0xCC, 0xCF, 0xF0, 0xF3, 0xFC, 0xFF,
}

// This function is responsible for encoding and decoding a band for the stereo case.
//
// C: static unsigned quant_band_stereo(struct band_ctx *ctx, celt_norm *X, celt_norm *Y,
//
//	int N, int b, int B, celt_norm *lowband,
//	int LM, celt_norm *lowband_out,
//	celt_norm *lowband_scratch, int fill)
func quant_band_stereo(ctx *band_ctx, X, Y []int16,
	N, b, B int, lowband []int16,
	LM int, lowband_out []int16,
	lowband_scratch []int16, fill int) uint32 {
	var imid, iside int = 0, 0
	var inv int = 0
	var mid, side int16 = 0, 0
	var cm uint32 = 0
	var mbits, sbits, delta int
	var itheta int
	var qalloc int
	var sctx split_ctx
	var orig_fill int
	var encode int
	var ec *EcCtx

	encode = ctx.encode
	ec = ctx.ec

	// Special case for one sample
	if N == 1 {
		return quant_band_n1(ctx, X, Y, lowband_out)
	}

	orig_fill = fill

	compute_theta(ctx, &sctx, X, Y, N, &b, B, B, LM, 1, &fill)
	inv = sctx.inv
	imid = sctx.imid
	iside = sctx.iside
	delta = sctx.delta
	itheta = sctx.itheta
	qalloc = sctx.qalloc
	mid = int16(imid)
	side = int16(iside)

	// This is a special case for N=2 that only works for stereo and takes
	// advantage of the fact that mid and side are orthogonal to encode
	// the side with just one bit.
	if N == 2 {
		var c int
		var sign int = 0
		var x2, y2 []int16
		mbits = b
		sbits = 0
		// Only need one bit for the side.
		if itheta != 0 && itheta != 16384 {
			sbits = 1 << BITRES
		}
		mbits -= sbits
		c = b2i(itheta > 8192)
		ctx.remaining_bits -= int32(qalloc + sbits)

		if c != 0 {
			x2 = Y
			y2 = X
		} else {
			x2 = X
			y2 = Y
		}
		if sbits != 0 {
			if encode != 0 {
				// Here we only need to encode a sign for the side.
				sign = b2i(int32(x2[0])*int32(y2[1])-int32(x2[1])*int32(y2[0]) < 0)
				ec.EncBits(uint32(sign), 1)
			} else {
				sign = int(ec.DecBits(1))
			}
		}
		sign = 1 - 2*sign
		// We use orig_fill here because we want to fold the side, but if
		// itheta==16384, we'll have cleared the low bits of fill.
		cm = quant_band(ctx, x2, N, mbits, B, lowband, LM, lowband_out, Q15ONE,
			lowband_scratch, orig_fill)
		// We don't split N=2 bands, so cm is either 1 or 0 (for a fold-collapse),
		// and there's no need to worry about mixing with the other channel.
		y2[0] = int16(-sign) * x2[1]
		y2[1] = int16(sign) * x2[0]
		if ctx.resynth != 0 {
			var tmp int16
			X[0] = int16(MULT16_16_Q15(mid, X[0]))
			X[1] = int16(MULT16_16_Q15(mid, X[1]))
			Y[0] = int16(MULT16_16_Q15(side, Y[0]))
			Y[1] = int16(MULT16_16_Q15(side, Y[1]))
			tmp = X[0]
			X[0] = int16(SUB16(tmp, Y[0]))
			Y[0] = ADD16(tmp, Y[0])
			tmp = X[1]
			X[1] = int16(SUB16(tmp, Y[1]))
			Y[1] = ADD16(tmp, Y[1])
		}
	} else {
		// "Normal" split code
		var rebalance int32

		mbits = IMAX(0, IMIN(b, (b-delta)/2))
		sbits = b - mbits
		ctx.remaining_bits -= int32(qalloc)

		rebalance = ctx.remaining_bits
		if mbits >= sbits {
			// In stereo mode, we do not apply a scaling to the mid because we need the normalized
			// mid for folding later.
			cm = quant_band(ctx, X, N, mbits, B, lowband, LM, lowband_out, Q15ONE,
				lowband_scratch, fill)
			rebalance = int32(mbits) - (rebalance - ctx.remaining_bits)
			if rebalance > 3<<BITRES && itheta != 0 {
				sbits += int(rebalance) - (3 << BITRES)
			}

			// For a stereo split, the high bits of fill are always zero, so no
			// folding will be done to the side.
			cm |= quant_band(ctx, Y, N, sbits, B, nil, LM, nil, side, nil, fill>>uint(B))
		} else {
			// For a stereo split, the high bits of fill are always zero, so no
			// folding will be done to the side.
			cm = quant_band(ctx, Y, N, sbits, B, nil, LM, nil, side, nil, fill>>uint(B))
			rebalance = int32(sbits) - (rebalance - ctx.remaining_bits)
			if rebalance > 3<<BITRES && itheta != 16384 {
				mbits += int(rebalance) - (3 << BITRES)
			}
			// In stereo mode, we do not apply a scaling to the mid because we need the normalized
			// mid for folding later.
			cm |= quant_band(ctx, X, N, mbits, B, lowband, LM, lowband_out, Q15ONE,
				lowband_scratch, fill)
		}
	}

	// This code is used by the decoder and by the resynthesis-enabled encoder
	if ctx.resynth != 0 {
		if N != 2 {
			stereo_merge(X, Y, mid, N, ctx.arch)
		}
		if inv != 0 {
			var j int
			for j = 0; j < N; j++ {
				Y[j] = -Y[j]
			}
		}
	}
	return cm
}

// C: static void special_hybrid_folding(const CELTMode *m, celt_norm *norm, celt_norm *norm2, int start, int M, int dual_stereo)
// (1.4: inside #ifndef DISABLE_UPDATE_DRAFT, which is not defined, so it is compiled.)
func special_hybrid_folding(m *CELTMode, norm, norm2 []int16, start, M, dual_stereo int) {
	var n1, n2 int
	eBands := m.eBands
	n1 = M * int(eBands[start+1]-eBands[start])
	n2 = M * int(eBands[start+2]-eBands[start+1])
	// Duplicate enough of the first band folding data to be able to fold the second band.
	// Copies no data for CELT-only mode.
	copy(norm[n1:n1+(n2-n1)], norm[2*n1-n2:2*n1-n2+(n2-n1)])
	if dual_stereo != 0 {
		copy(norm2[n1:n1+(n2-n1)], norm2[2*n1-n2:2*n1-n2+(n2-n1)])
	}
}

// QuantAllBands performs quantisation/encoding (or decoding) of the residual spectrum.
//
// C: void quant_all_bands(int encode, const CELTMode *m, int start, int end,
//
//	celt_norm *X_, celt_norm *Y_, unsigned char *collapse_masks,
//	const celt_ener *bandE, int *pulses, int shortBlocks, int spread,
//	int dual_stereo, int intensity, int *tf_res, opus_int32 total_bits,
//	opus_int32 balance, ec_ctx *ec, int LM, int codedBands,
//	opus_uint32 *seed, int complexity, int arch, int disable_inv)
func QuantAllBands(encode int, m *CELTMode, start, end int,
	X_, Y_ []int16, collapse_masks []uint8,
	bandE []int32, pulses []int, shortBlocks, spread int,
	dual_stereo, intensity int, tf_res []int, total_bits int32,
	balance int32, ec *EcCtx, LM, codedBands int,
	seed *uint32, complexity int, arch int, disable_inv int) {
	var i int
	var remaining_bits int32
	eBands := m.eBands
	var norm, norm2 []int16
	var X_save, Y_save, X_save2, Y_save2, norm_save2 []int16
	var resynth_alloc int
	var lowband_scratch []int16
	var B int
	var M int
	var lowband_offset int
	var update_lowband int = 1
	var C int = 1
	if Y_ != nil {
		C = 2
	}
	var norm_offset int
	theta_rdo := b2i(encode != 0 && Y_ != nil && dual_stereo == 0 && complexity >= 8)
	resynth := b2i(encode == 0 || theta_rdo != 0)
	var ctx band_ctx

	M = 1 << uint(LM)
	if shortBlocks != 0 {
		B = M
	} else {
		B = 1
	}
	norm_offset = M * int(eBands[start])
	// No need to allocate norm for the last band because we don't need an
	// output in that band.
	_norm := make([]int16, C*(M*int(eBands[m.nbEBands-1])-norm_offset))
	norm = _norm
	norm2 = norm[M*int(eBands[m.nbEBands-1])-norm_offset:]

	// For decoding, we can use the last band as scratch space because we don't need that
	// scratch space for the last band and we don't care about the data there until we're
	// decoding the last band.
	if encode != 0 && resynth != 0 {
		resynth_alloc = M * int(eBands[m.nbEBands]-eBands[m.nbEBands-1])
	} else {
		resynth_alloc = 0 // ALLOC_NONE
	}
	_lowband_scratch := make([]int16, resynth_alloc)
	if encode != 0 && resynth != 0 {
		lowband_scratch = _lowband_scratch
	} else {
		lowband_scratch = X_[M*int(eBands[m.nbEBands-1]):]
	}
	X_save = make([]int16, resynth_alloc)
	Y_save = make([]int16, resynth_alloc)
	X_save2 = make([]int16, resynth_alloc)
	Y_save2 = make([]int16, resynth_alloc)
	norm_save2 = make([]int16, resynth_alloc)

	lowband_offset = 0
	ctx.bandE = bandE
	ctx.ec = ec
	ctx.encode = encode
	ctx.intensity = intensity
	ctx.m = m
	ctx.seed = *seed
	ctx.spread = spread
	ctx.arch = arch
	ctx.disable_inv = disable_inv
	ctx.resynth = resynth
	ctx.theta_round = 0
	// Avoid injecting noise in the first band on transients.
	ctx.avoid_split_noise = b2i(B > 1)
	for i = start; i < end; i++ {
		var tell int32
		var b int
		var N int
		var curr_balance int32
		var effective_lowband int = -1
		var X, Y []int16
		var tf_change int = 0
		var x_cm uint32
		var y_cm uint32
		var last int

		ctx.i = i
		last = b2i(i == end-1)

		X = X_[M*int(eBands[i]):]
		if Y_ != nil {
			Y = Y_[M*int(eBands[i]):]
		} else {
			Y = nil
		}
		N = M*int(eBands[i+1]) - M*int(eBands[i])
		celt_assert(N > 0)
		tell = int32(ec.TellFrac())

		// Compute how many bits we want to allocate to this band
		if i != start {
			balance -= tell
		}
		remaining_bits = total_bits - tell - 1
		ctx.remaining_bits = remaining_bits
		if i <= codedBands-1 {
			curr_balance = Sudiv(balance, int32(IMIN(3, codedBands-i)))
			b = IMAX(0, IMIN(16383, IMIN(int(remaining_bits)+1, pulses[i]+int(curr_balance))))
		} else {
			b = 0
		}

		// DISABLE_UPDATE_DRAFT is not defined (RFC 8251 behaviour).
		if resynth != 0 && (M*int(eBands[i])-N >= M*int(eBands[start]) || i == start+1) && (update_lowband != 0 || lowband_offset == 0) {
			lowband_offset = i
		}
		if i == start+1 {
			special_hybrid_folding(m, norm, norm2, start, M, dual_stereo)
		}

		tf_change = tf_res[i]
		ctx.tf_change = tf_change
		if i >= m.effEBands {
			X = norm
			if Y_ != nil {
				Y = norm
			}
			lowband_scratch = nil
		}
		if last != 0 && theta_rdo == 0 {
			lowband_scratch = nil
		}

		// Get a conservative estimate of the collapse_mask's for the bands we're
		// going to be folding from.
		if lowband_offset != 0 && (spread != SPREAD_AGGRESSIVE || B > 1 || tf_change < 0) {
			var fold_start int
			var fold_end int
			var fold_i int
			// This ensures we never repeat spectral content within one band
			effective_lowband = IMAX(0, M*int(eBands[lowband_offset])-norm_offset-N)
			fold_start = lowband_offset
			for {
				fold_start--
				if !(M*int(eBands[fold_start]) > effective_lowband+norm_offset) {
					break
				}
			}
			fold_end = lowband_offset - 1
			// DISABLE_UPDATE_DRAFT is not defined (RFC 8251 behaviour).
			for {
				fold_end++
				if !(fold_end < i && M*int(eBands[fold_end]) < effective_lowband+norm_offset+N) {
					break
				}
			}
			x_cm = 0
			y_cm = 0
			fold_i = fold_start
			for {
				x_cm |= uint32(collapse_masks[fold_i*C+0])
				y_cm |= uint32(collapse_masks[fold_i*C+C-1])
				fold_i++
				if fold_i >= fold_end {
					break
				}
			}
		} else {
			// Otherwise, we'll be using the LCG to fold, so all blocks will (almost
			// always) be non-zero.
			x_cm = (1 << uint(B)) - 1
			y_cm = x_cm
		}

		if dual_stereo != 0 && i == intensity {
			var j int

			// Switch off dual stereo to do intensity.
			dual_stereo = 0
			if resynth != 0 {
				for j = 0; j < M*int(eBands[i])-norm_offset; j++ {
					norm[j] = int16(HALF32(int32(norm[j]) + int32(norm2[j])))
				}
			}
		}
		var lb, lb2, lo, lo2 []int16
		if effective_lowband != -1 {
			lb = norm[effective_lowband:]
			if C == 2 {
				lb2 = norm2[effective_lowband:]
			}
		}
		if last == 0 {
			lo = norm[M*int(eBands[i])-norm_offset:]
			if C == 2 {
				lo2 = norm2[M*int(eBands[i])-norm_offset:]
			}
		}
		if dual_stereo != 0 {
			x_cm = quant_band(&ctx, X, N, b/2, B,
				lb, LM,
				lo, Q15ONE, lowband_scratch, int(x_cm))
			y_cm = quant_band(&ctx, Y, N, b/2, B,
				lb2, LM,
				lo2, Q15ONE, lowband_scratch, int(y_cm))
		} else {
			if Y != nil {
				if theta_rdo != 0 && i < intensity {
					var ec_save, ec_save2 EcCtx
					var ctx_save, ctx_save2 band_ctx
					var dist0, dist1 int32
					var cm, cm2 uint32
					var nstart_bytes, nend_bytes, save_bytes int
					var bytes_buf []byte
					var bytes_save [1275]byte
					var w [2]int16
					compute_channel_weights(bandE[i], bandE[i+m.nbEBands], &w)
					// Make a copy.
					cm = x_cm | y_cm
					ec_save = *ec
					ctx_save = ctx
					copy(X_save[:N], X[:N])
					copy(Y_save[:N], Y[:N])
					// Encode and round down.
					ctx.theta_round = -1
					x_cm = quant_band_stereo(&ctx, X, Y, N, b, B,
						lb, LM,
						lo, lowband_scratch, int(cm))
					dist0 = MULT16_32_Q15(w[0], InnerProd(X_save, X, N, arch)) + MULT16_32_Q15(w[1], InnerProd(Y_save, Y, N, arch))

					// Save first result.
					cm2 = x_cm
					ec_save2 = *ec
					ctx_save2 = ctx
					copy(X_save2[:N], X[:N])
					copy(Y_save2[:N], Y[:N])
					if last == 0 {
						copy(norm_save2[:N], norm[M*int(eBands[i])-norm_offset:][:N])
					}
					nstart_bytes = int(ec_save.offs)
					nend_bytes = int(ec_save.storage)
					bytes_buf = ec_save.buf[nstart_bytes:]
					save_bytes = nend_bytes - nstart_bytes
					copy(bytes_save[:save_bytes], bytes_buf[:save_bytes])

					// Restore
					*ec = ec_save
					ctx = ctx_save
					copy(X[:N], X_save[:N])
					copy(Y[:N], Y_save[:N])
					// DISABLE_UPDATE_DRAFT is not defined (RFC 8251 behaviour).
					if i == start+1 {
						special_hybrid_folding(m, norm, norm2, start, M, dual_stereo)
					}
					// Encode and round up.
					ctx.theta_round = 1
					x_cm = quant_band_stereo(&ctx, X, Y, N, b, B,
						lb, LM,
						lo, lowband_scratch, int(cm))
					dist1 = MULT16_32_Q15(w[0], InnerProd(X_save, X, N, arch)) + MULT16_32_Q15(w[1], InnerProd(Y_save, Y, N, arch))
					if dist0 >= dist1 {
						x_cm = cm2
						*ec = ec_save2
						ctx = ctx_save2
						copy(X[:N], X_save2[:N])
						copy(Y[:N], Y_save2[:N])
						if last == 0 {
							copy(norm[M*int(eBands[i])-norm_offset:][:N], norm_save2[:N])
						}
						copy(bytes_buf[:save_bytes], bytes_save[:save_bytes])
					}
				} else {
					ctx.theta_round = 0
					x_cm = quant_band_stereo(&ctx, X, Y, N, b, B,
						lb, LM,
						lo, lowband_scratch, int(x_cm|y_cm))
				}
			} else {
				x_cm = quant_band(&ctx, X, N, b, B,
					lb, LM,
					lo, Q15ONE, lowband_scratch, int(x_cm|y_cm))
			}
			y_cm = x_cm
		}
		collapse_masks[i*C+0] = uint8(x_cm)
		collapse_masks[i*C+C-1] = uint8(y_cm)
		balance += int32(pulses[i]) + tell

		// Update the folding position only as long as we have 1 bit/sample depth.
		update_lowband = b2i(b > (N << BITRES))
		// We only need to avoid noise on a split for the first band. After that, we
		// have folding.
		ctx.avoid_split_noise = 0
	}
	*seed = ctx.seed
}
