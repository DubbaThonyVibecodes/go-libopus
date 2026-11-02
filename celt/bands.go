package celt

// C: celt/bands.h, celt/bands.c

const (
	SPREAD_NONE       = 0
	SPREAD_LIGHT      = 1
	SPREAD_NORMAL     = 2
	SPREAD_AGGRESSIVE = 3
)

// C: opus_uint32 celt_lcg_rand(opus_uint32 seed)
func LcgRand(seed uint32) uint32 {
	return 1664525*seed + 1013904223
}

// This is a cos() approximation designed to be bit-exact on any platform. Bit exactness
// with this approximation is important because it has an impact on the bit allocation.
//
// C: static opus_int16 bitexact_cos(opus_int16 x)
func bitexact_cos(x int16) int16 {
	var tmp int32
	var x2 int16
	tmp = (4096 + int32(x)*int32(x)) >> 13
	celt_assert(tmp <= 32767)
	x2 = int16(tmp)
	x2 = int16((32767 - int32(x2)) + FRAC_MUL16(x2, (-7651+FRAC_MUL16(x2, (8277+FRAC_MUL16(-626, x2))))))
	celt_assert(x2 <= 32766)
	return 1 + x2
}

// C: static int bitexact_log2tan(int isin,int icos)
func bitexact_log2tan(isin, icos int) int {
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
// C: void compute_band_energies(const CELTMode *m, const celt_sig *X, celt_ener *bandE, int end, int C, int M)
func ComputeBandEnergies(m *CELTMode, X []int32, bandE []int32, end, C, M int) {
	var i, c, N int
	eBands := m.eBands
	N = M * m.shortMdctSize
	c = 0
	for {
		for i = 0; i < end; i++ {
			var j int
			var maxval int32 = 0
			var sum int32 = 0

			j = M * int(eBands[i])
			for {
				maxval = MAX32(maxval, X[j+c*N])
				maxval = MAX32(maxval, -X[j+c*N])
				j++
				if j >= M*int(eBands[i+1]) {
					break
				}
			}

			if maxval > 0 {
				shift := int(Ilog2(maxval)) - 10
				j = M * int(eBands[i])
				for {
					sum = MAC16_16(sum, EXTRACT16(VSHR32(X[j+c*N], shift)),
						EXTRACT16(VSHR32(X[j+c*N], shift)))
					j++
					if j >= M*int(eBands[i+1]) {
						break
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
// C: void denormalise_bands(const CELTMode *m, const celt_norm * OPUS_RESTRICT X, celt_sig * OPUS_RESTRICT freq, const celt_ener *bandE, int end, int C, int M)
func DenormaliseBands(m *CELTMode, X []int16, freq []int32, bandE []int32, end, C, M int) {
	var i, c, N int
	eBands := m.eBands
	N = M * m.shortMdctSize
	celt_assert2(C <= 2, "denormalise_bands() not implemented for >2 channels")
	c = 0
	for {
		f := freq[c*N:]
		x := X[c*N:]
		fi, xi := 0, 0
		for i = 0; i < end; i++ {
			var j, band_end int
			var g int32 = SHR32(bandE[i+c*m.nbEBands], 1)
			j = M * int(eBands[i])
			band_end = M * int(eBands[i+1])
			for {
				f[fi] = SHL32(MULT16_32_Q15(x[xi], g), 2)
				fi++
				xi++
				j++
				if j >= band_end {
					break
				}
			}
		}
		for i = M * int(eBands[end]); i < N; i++ {
			f[fi] = 0
			fi++
		}
		c++
		if c >= C {
			break
		}
	}
}

// AntiCollapse prevents energy collapse for transients with multiple short MDCTs.
//
// C: void anti_collapse(const CELTMode *m, celt_norm *X_, unsigned char *collapse_masks, int LM, int C, int size,
//
//	int start, int end, opus_val16 *logE, opus_val16 *prev1logE,
//	opus_val16 *prev2logE, int *pulses, opus_uint32 seed)
func AntiCollapse(m *CELTMode, X_ []int16, collapse_masks []uint8, LM, C, size int,
	start, end int, logE, prev1logE, prev2logE []int16, pulses []int, seed uint32) {
	var c, i, j, k int
	for i = start; i < end; i++ {
		var N0 int
		var thresh, sqrt_1 int16
		var depth int
		var shift int
		var thresh32 int32

		N0 = int(m.eBands[i+1] - m.eBands[i])
		// depth in 1/8 bits
		depth = (1 + pulses[i]) / (int(m.eBands[i+1]-m.eBands[i]) << uint(LM))

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
				RenormaliseVector(X, N0<<uint(LM), Q15ONE)
			}
			c++
			if c >= C {
				break
			}
		}
	}
}

// C: static void intensity_stereo(const CELTMode *m, celt_norm *X, celt_norm *Y, const celt_ener *bandE, int bandID, int N)
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
		X[j] = int16(MULT16_16_Q14(a1, l) + MULT16_16_Q14(a2, r))
		// Side is not encoded, no need to calculate
	}
}

// C: static void stereo_split(celt_norm *X, celt_norm *Y, int N)
func stereo_split(X, Y []int16, N int) {
	var j int
	for j = 0; j < N; j++ {
		var r, l int16
		l = int16(MULT16_16_Q15(QCONST16(.70710678, 15), X[j]))
		r = int16(MULT16_16_Q15(QCONST16(.70710678, 15), Y[j]))
		X[j] = l + r
		Y[j] = r - l
	}
}

// C: static void stereo_merge(celt_norm *X, celt_norm *Y, opus_val16 mid, int N)
func stereo_merge(X, Y []int16, mid int16, N int) {
	var j int
	var xp, side int32 = 0, 0
	var El, Er int32
	var mid2 int16
	var kl, kr int
	var t, lgain, rgain int32

	// Compute the norm of X+Y and X-Y as |X|^2 + |Y|^2 +/- sum(xy)
	for j = 0; j < N; j++ {
		xp = MAC16_16(xp, X[j], Y[j])
		side = MAC16_16(side, Y[j], Y[j])
	}
	// Compensating for the mid normalization
	xp = MULT16_32_Q15(mid, xp)
	// mid and side are in Q15, not Q14 like X and Y
	mid2 = int16(SHR32(mid, 1))
	El = MULT16_16(mid2, mid2) + side - 2*xp
	Er = MULT16_16(mid2, mid2) + side + 2*xp
	if Er < QCONST32(6e-4, 28) || El < QCONST32(6e-4, 28) {
		for j = 0; j < N; j++ {
			Y[j] = X[j]
		}
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
		l = int16(MULT16_16_Q15(mid, X[j]))
		r = Y[j]
		X[j] = EXTRACT16(PSHR32(MULT16_16(lgain, SUB16(l, r)), kl+1))
		Y[j] = EXTRACT16(PSHR32(MULT16_16(rgain, ADD16(l, r)), kr+1))
	}
}

// SpreadingDecision decides whether we should spread the pulses in the current frame.
//
// C: int spreading_decision(const CELTMode *m, celt_norm *X, int *average,
//
//	int last_decision, int *hf_average, int *tapset_decision, int update_hf,
//	int end, int C, int M)
func SpreadingDecision(m *CELTMode, X []int16, average *int,
	last_decision int, hf_average, tapset_decision *int, update_hf int,
	end, C, M int) int {
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
				hf_sum += 32 * (tcount[1] + tcount[0]) / N
			}
			tmp = b2i(2*tcount[2] >= N) + b2i(2*tcount[1] >= N) + b2i(2*tcount[0] >= N)
			sum += tmp * 256
			nbBands++
		}
		c++
		if c >= C {
			break
		}
	}

	if update_hf != 0 {
		if hf_sum != 0 {
			hf_sum /= C * (4 - m.nbEBands + end)
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
	celt_assert(nbBands > 0) // M*(eBands[end]-eBands[end-1]) <= 8 assures this
	sum /= nbBands
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
	for j = 0; j < N; j++ {
		X[j] = tmp[j]
	}
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
	for j = 0; j < N; j++ {
		X[j] = tmp[j]
	}
}

// C: void haar1(celt_norm *X, int N0, int stride)
func Haar1(X []int16, N0, stride int) {
	var i, j int
	N0 >>= 1
	for i = 0; i < stride; i++ {
		for j = 0; j < N0; j++ {
			var tmp1, tmp2 int16
			tmp1 = int16(MULT16_16_Q15(QCONST16(.70710678, 15), X[stride*2*j+i]))
			tmp2 = int16(MULT16_16_Q15(QCONST16(.70710678, 15), X[stride*(2*j+1)+i]))
			X[stride*2*j+i] = tmp1 + tmp2
			X[stride*(2*j+1)+i] = tmp1 - tmp2
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
	qb = IMIN(b-pulse_cap-(4<<BITRES), (b+N2*offset)/N2)

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

var bit_interleave_table = [16]uint8{
	0, 1, 1, 1, 2, 3, 3, 3, 2, 3, 3, 3, 2, 3, 3, 3,
}

var bit_deinterleave_table = [16]uint8{
	0x00, 0x03, 0x0C, 0x0F, 0x30, 0x33, 0x3C, 0x3F,
	0xC0, 0xC3, 0xCC, 0xCF, 0xF0, 0xF3, 0xFC, 0xFF,
}

// This function is responsible for encoding and decoding a band for both
// the mono and stereo case. Even in the mono case, it can split the band
// in two and transmit the energy difference with the two half-bands. It
// can be called recursively so bands can end up being split in 8 parts.
//
// C: static unsigned quant_band(int encode, const CELTMode *m, int i, celt_norm *X, celt_norm *Y,
//
//	int N, int b, int spread, int B, int intensity, int tf_change, celt_norm *lowband, ec_ctx *ec,
//	opus_int32 *remaining_bits, int LM, celt_norm *lowband_out, const celt_ener *bandE, int level,
//	opus_uint32 *seed, opus_val16 gain, celt_norm *lowband_scratch, int fill)
func quant_band(encode int, m *CELTMode, i int, X, Y []int16,
	N, b, spread, B, intensity, tf_change int, lowband []int16, ec *EcCtx,
	remaining_bits *int32, LM int, lowband_out []int16, bandE []int32, level int,
	seed *uint32, gain int16, lowband_scratch []int16, fill int) uint32 {
	var cache []uint8
	var q, curr_bits int
	var stereo, split int
	var imid, iside int = 0, 0
	N0 := N
	N_B := N
	var N_B0 int
	B0 := B
	var time_divide int = 0
	var recombine int = 0
	var inv int = 0
	var mid, side int16 = 0, 0
	var longBlocks int
	var cm uint32 = 0
	resynth := b2i(encode == 0)

	longBlocks = b2i(B0 == 1)

	N_B /= B
	N_B0 = N_B

	stereo = b2i(Y != nil)
	split = stereo

	// Special case for one sample
	if N == 1 {
		var c int
		x := X
		c = 0
		for {
			var sign int = 0
			if *remaining_bits >= 1<<BITRES {
				if encode != 0 {
					sign = b2i(x[0] < 0)
					ec.EncBits(uint32(sign), 1)
				} else {
					sign = int(ec.DecBits(1))
				}
				*remaining_bits -= 1 << BITRES
				b -= 1 << BITRES
			}
			if resynth != 0 {
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

	if stereo == 0 && level == 0 {
		var k int
		if tf_change > 0 {
			recombine = tf_change
		}
		// Band recombining to increase frequency resolution

		if lowband != nil && (recombine != 0 || ((N_B&1) == 0 && tf_change < 0) || B0 > 1) {
			var j int
			for j = 0; j < N; j++ {
				lowband_scratch[j] = lowband[j]
			}
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
	}

	// If we need 1.5 more bit than we can produce, split the band in two.
	cache = m.cache.bits[m.cache.index[(LM+1)*m.nbEBands+i]:]
	if stereo == 0 && LM != -1 && b > int(cache[cache[0]])+12 && N > 2 {
		N >>= 1
		Y = X[N:]
		split = 1
		LM -= 1
		if B == 1 {
			fill = (fill & 1) | (fill << 1)
		}
		B = (B + 1) >> 1
	}

	if split != 0 {
		var qn int
		var itheta int = 0
		var mbits, sbits, delta int
		var qalloc int
		var pulse_cap int
		var offset int
		var orig_fill int
		var tell int32

		// Decide on the resolution to give to the split parameter theta
		pulse_cap = int(m.logN[i]) + LM*(1<<BITRES)
		if stereo != 0 && N == 2 {
			offset = (pulse_cap >> 1) - QTHETA_OFFSET_TWOPHASE
		} else {
			offset = (pulse_cap >> 1) - QTHETA_OFFSET
		}
		qn = compute_qn(N, b, offset, pulse_cap, stereo)
		if stereo != 0 && i >= intensity {
			qn = 1
		}
		if encode != 0 {
			// theta is the atan() of the ratio between the (normalized)
			// side and mid. With just that parameter, we can re-scale both
			// mid and side because we know that 1) they have unit norm and
			// 2) they are orthogonal.
			itheta = StereoItheta(X, Y, stereo, N)
		}
		tell = int32(ec.TellFrac())
		if qn != 1 {
			if encode != 0 {
				itheta = (itheta*qn + 8192) >> 14
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
			itheta = int(int32(itheta) * 16384 / int32(qn))
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
				inv = b2i(itheta > 8192)
				if inv != 0 {
					var j int
					for j = 0; j < N; j++ {
						Y[j] = -Y[j]
					}
				}
				intensity_stereo(m, X, Y, bandE, i, N)
			}
			if b > 2<<BITRES && *remaining_bits > 2<<BITRES {
				if encode != 0 {
					ec.EncBitLogp(inv, 2)
				} else {
					inv = ec.DecBitLogp(2)
				}
			} else {
				inv = 0
			}
			itheta = 0
		}
		qalloc = int(ec.TellFrac()) - int(tell)
		b -= qalloc

		orig_fill = fill
		if itheta == 0 {
			imid = 32767
			iside = 0
			fill &= (1 << uint(B)) - 1
			delta = -16384
		} else if itheta == 16384 {
			imid = 0
			iside = 32767
			fill &= ((1 << uint(B)) - 1) << uint(B)
			delta = 16384
		} else {
			imid = int(bitexact_cos(int16(itheta)))
			iside = int(bitexact_cos(int16(16384 - itheta)))
			// This is the mid vs side allocation that minimizes squared error
			// in that band.
			delta = int(FRAC_MUL16((N-1)<<7, bitexact_log2tan(iside, imid)))
		}

		mid = int16(imid)
		side = int16(iside)

		// This is a special case for N=2 that only works for stereo and takes
		// advantage of the fact that mid and side are orthogonal to encode
		// the side with just one bit.
		if N == 2 && stereo != 0 {
			var c int
			var sign int = 0
			var x2, y2 []int16
			mbits = b
			sbits = 0
			// Only need one bit for the side
			if itheta != 0 && itheta != 16384 {
				sbits = 1 << BITRES
			}
			mbits -= sbits
			c = b2i(itheta > 8192)
			*remaining_bits -= int32(qalloc + sbits)

			if c != 0 {
				x2 = Y
				y2 = X
			} else {
				x2 = X
				y2 = Y
			}
			if sbits != 0 {
				if encode != 0 {
					// Here we only need to encode a sign for the side
					sign = b2i(int32(x2[0])*int32(y2[1])-int32(x2[1])*int32(y2[0]) < 0)
					ec.EncBits(uint32(sign), 1)
				} else {
					sign = int(ec.DecBits(1))
				}
			}
			sign = 1 - 2*sign
			// We use orig_fill here because we want to fold the side, but if
			// itheta==16384, we'll have cleared the low bits of fill.
			cm = quant_band(encode, m, i, x2, nil, N, mbits, spread, B, intensity, tf_change, lowband, ec, remaining_bits, LM, lowband_out, nil, level, seed, gain, lowband_scratch, orig_fill)
			// We don't split N=2 bands, so cm is either 1 or 0 (for a fold-collapse),
			// and there's no need to worry about mixing with the other channel.
			y2[0] = int16(-sign) * x2[1]
			y2[1] = int16(sign) * x2[0]
			if resynth != 0 {
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
			var next_lowband2 []int16 = nil
			var next_lowband_out1 []int16 = nil
			var next_level int = 0
			var rebalance int32

			// Give more bits to low-energy MDCTs than they would otherwise deserve
			if B0 > 1 && stereo == 0 && (itheta&0x3fff) != 0 {
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
			*remaining_bits -= int32(qalloc)

			if lowband != nil && stereo == 0 {
				next_lowband2 = lowband[N:] // >32-bit split case
			}

			// Only stereo needs to pass on lowband_out. Otherwise, it's
			// handled at the end
			if stereo != 0 {
				next_lowband_out1 = lowband_out
			} else {
				next_level = level + 1
			}

			var midGain int16
			if stereo != 0 {
				midGain = Q15ONE
			} else {
				midGain = int16(MULT16_16_P15(gain, mid))
			}
			sideGain := int16(MULT16_16_P15(gain, side))
			// cm |= ... << ((B0>>1)&(stereo-1))
			shiftY := uint((B0 >> 1) & (stereo - 1))

			rebalance = *remaining_bits
			if mbits >= sbits {
				// In stereo mode, we do not apply a scaling to the mid because we need the normalized
				// mid for folding later
				cm = quant_band(encode, m, i, X, nil, N, mbits, spread, B, intensity, tf_change,
					lowband, ec, remaining_bits, LM, next_lowband_out1,
					nil, next_level, seed, midGain, lowband_scratch, fill)
				rebalance = int32(mbits) - (rebalance - *remaining_bits)
				if rebalance > 3<<BITRES && itheta != 0 {
					sbits += int(rebalance) - (3 << BITRES)
				}

				// For a stereo split, the high bits of fill are always zero, so no
				// folding will be done to the side.
				cm |= quant_band(encode, m, i, Y, nil, N, sbits, spread, B, intensity, tf_change,
					next_lowband2, ec, remaining_bits, LM, nil,
					nil, next_level, seed, sideGain, nil, fill>>uint(B)) << shiftY
			} else {
				// For a stereo split, the high bits of fill are always zero, so no
				// folding will be done to the side.
				cm = quant_band(encode, m, i, Y, nil, N, sbits, spread, B, intensity, tf_change,
					next_lowband2, ec, remaining_bits, LM, nil,
					nil, next_level, seed, sideGain, nil, fill>>uint(B)) << shiftY
				rebalance = int32(sbits) - (rebalance - *remaining_bits)
				if rebalance > 3<<BITRES && itheta != 16384 {
					mbits += int(rebalance) - (3 << BITRES)
				}
				// In stereo mode, we do not apply a scaling to the mid because we need the normalized
				// mid for folding later
				cm |= quant_band(encode, m, i, X, nil, N, mbits, spread, B, intensity, tf_change,
					lowband, ec, remaining_bits, LM, next_lowband_out1,
					nil, next_level, seed, midGain, lowband_scratch, fill)
			}
		}

	} else {
		// This is the basic no-split case
		q = bits2pulses(m, i, LM, b)
		curr_bits = pulses2bits(m, i, LM, q)
		*remaining_bits -= int32(curr_bits)

		// Ensures we can never bust the budget
		for *remaining_bits < 0 && q > 0 {
			*remaining_bits += int32(curr_bits)
			q--
			curr_bits = pulses2bits(m, i, LM, q)
			*remaining_bits -= int32(curr_bits)
		}

		if q != 0 {
			K := get_pulses(q)

			// Finally do the actual quantization
			if encode != 0 {
				cm = AlgQuant(X, N, K, spread, B, ec)
			} else {
				cm = AlgUnquant(X, N, K, spread, B, ec, gain)
			}
		} else {
			// If there's no pulse, fill the band anyway
			var j int
			if resynth != 0 {
				var cm_mask uint32
				// B can be as large as 16, so this shift might overflow an int on a
				// 16-bit platform; use a long to get defined behavior.
				cm_mask = uint32(1<<uint(B)) - 1
				fill &= int(cm_mask)
				if fill == 0 {
					for j = 0; j < N; j++ {
						X[j] = 0
					}
				} else {
					if lowband == nil {
						// Noise
						for j = 0; j < N; j++ {
							*seed = LcgRand(*seed)
							X[j] = int16(int32(*seed) >> 20)
						}
						cm = cm_mask
					} else {
						// Folded spectrum
						for j = 0; j < N; j++ {
							var tmp int16
							*seed = LcgRand(*seed)
							// About 48 dB below the "normal" folding level
							tmp = QCONST16(1.0/256, 10)
							if (*seed)&0x8000 == 0 {
								tmp = -tmp
							}
							X[j] = lowband[j] + tmp
						}
						cm = uint32(fill)
					}
					RenormaliseVector(X, N, gain)
				}
			}
		}
	}

	// This code is used by the decoder and by the resynthesis-enabled encoder
	if resynth != 0 {
		if stereo != 0 {
			if N != 2 {
				stereo_merge(X, Y, mid, N)
			}
			if inv != 0 {
				var j int
				for j = 0; j < N; j++ {
					Y[j] = -Y[j]
				}
			}
		} else if level == 0 {
			var k int

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
	}
	return cm
}

// QuantAllBands performs quantisation/encoding (or decoding) of the residual spectrum.
//
// C: void quant_all_bands(int encode, const CELTMode *m, int start, int end,
//
//	celt_norm *X_, celt_norm *Y_, unsigned char *collapse_masks, const celt_ener *bandE, int *pulses,
//	int shortBlocks, int spread, int dual_stereo, int intensity, int *tf_res,
//	opus_int32 total_bits, opus_int32 balance, ec_ctx *ec, int LM, int codedBands, opus_uint32 *seed)
func QuantAllBands(encode int, m *CELTMode, start, end int,
	X_, Y_ []int16, collapse_masks []uint8, bandE []int32, pulses []int,
	shortBlocks, spread, dual_stereo, intensity int, tf_res []int,
	total_bits, balance int32, ec *EcCtx, LM, codedBands int, seed *uint32) {
	var i int
	var remaining_bits int32
	eBands := m.eBands
	var norm, norm2 []int16
	var B, M int
	var lowband_offset int
	var update_lowband int = 1
	var C int = 1
	if Y_ != nil {
		C = 2
	}
	resynth := b2i(encode == 0)

	M = 1 << uint(LM)
	if shortBlocks != 0 {
		B = M
	} else {
		B = 1
	}
	_norm := make([]int16, C*M*int(eBands[m.nbEBands]))
	lowband_scratch := make([]int16, M*int(eBands[m.nbEBands]-eBands[m.nbEBands-1]))
	norm = _norm
	norm2 = norm[M*int(eBands[m.nbEBands]):]

	lowband_offset = 0
	for i = start; i < end; i++ {
		var tell int32
		var b, N int
		var curr_balance int32
		var effective_lowband int = -1
		var X, Y []int16
		var tf_change int = 0
		var x_cm, y_cm uint32

		X = X_[M*int(eBands[i]):]
		if Y_ != nil {
			Y = Y_[M*int(eBands[i]):]
		} else {
			Y = nil
		}
		N = M*int(eBands[i+1]) - M*int(eBands[i])
		tell = int32(ec.TellFrac())

		// Compute how many bits we want to allocate to this band
		if i != start {
			balance -= tell
		}
		remaining_bits = total_bits - tell - 1
		if i <= codedBands-1 {
			curr_balance = balance / int32(IMIN(3, codedBands-i))
			b = IMAX(0, IMIN(16383, IMIN(int(remaining_bits)+1, pulses[i]+int(curr_balance))))
		} else {
			b = 0
		}

		if resynth != 0 && M*int(eBands[i])-N >= M*int(eBands[start]) && (update_lowband != 0 || lowband_offset == 0) {
			lowband_offset = i
		}

		tf_change = tf_res[i]
		if i >= m.effEBands {
			X = norm
			if Y_ != nil {
				Y = norm
			}
		}

		// Get a conservative estimate of the collapse_mask's for the bands we're
		// going to be folding from.
		if lowband_offset != 0 && (spread != SPREAD_AGGRESSIVE || B > 1 || tf_change < 0) {
			var fold_start, fold_end, fold_i int
			// This ensures we never repeat spectral content within one band
			effective_lowband = IMAX(M*int(eBands[start]), M*int(eBands[lowband_offset])-N)
			fold_start = lowband_offset
			for {
				fold_start--
				if M*int(eBands[fold_start]) <= effective_lowband {
					break
				}
			}
			fold_end = lowband_offset - 1
			for {
				fold_end++
				if M*int(eBands[fold_end]) >= effective_lowband+N {
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

			// Switch off dual stereo to do intensity
			dual_stereo = 0
			if resynth != 0 {
				for j = M * int(eBands[start]); j < M*int(eBands[i]); j++ {
					norm[j] = int16(HALF32(int32(norm[j]) + int32(norm2[j])))
				}
			}
		}
		var lb, lb2 []int16
		if effective_lowband != -1 {
			lb = norm[effective_lowband:]
			if dual_stereo != 0 { // norm2 is empty for mono (C pointer arithmetic is harmless there)
				lb2 = norm2[effective_lowband:]
			}
		}
		if dual_stereo != 0 {
			x_cm = quant_band(encode, m, i, X, nil, N, b/2, spread, B, intensity, tf_change,
				lb, ec, &remaining_bits, LM,
				norm[M*int(eBands[i]):], bandE, 0, seed, Q15ONE, lowband_scratch, int(x_cm))
			y_cm = quant_band(encode, m, i, Y, nil, N, b/2, spread, B, intensity, tf_change,
				lb2, ec, &remaining_bits, LM,
				norm2[M*int(eBands[i]):], bandE, 0, seed, Q15ONE, lowband_scratch, int(y_cm))
		} else {
			x_cm = quant_band(encode, m, i, X, Y, N, b, spread, B, intensity, tf_change,
				lb, ec, &remaining_bits, LM,
				norm[M*int(eBands[i]):], bandE, 0, seed, Q15ONE, lowband_scratch, int(x_cm|y_cm))
			y_cm = x_cm
		}
		collapse_masks[i*C+0] = uint8(x_cm)
		collapse_masks[i*C+C-1] = uint8(y_cm)
		balance += int32(pulses[i]) + tell

		// Update the folding position only as long as we have 1 bit/sample depth
		update_lowband = b2i(b > (N << BITRES))
	}
}
