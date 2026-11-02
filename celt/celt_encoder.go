package celt

// C: celt/celt_encoder.c — the CELT encoder.

// CELTEncoder is the C struct OpusCustomEncoder (CELTEncoder).
type CELTEncoder struct {
	mode            *CELTMode // Mode used by the encoder
	channels        int
	stream_channels int

	force_intra int
	clip        int
	disable_pf  int
	complexity  int
	upsample    int
	start, end  int

	bitrate         int32
	vbr             int
	signalling      int
	constrained_vbr int // If zero, VBR can do whatever it likes with the rate
	loss_rate       int
	lsb_depth       int
	lfe             int
	disable_inv     int
	arch            int

	// Everything beyond this point gets cleared on a reset (ENCODER_RESET_START)
	rng             uint32
	spread_decision int
	delayedIntra    int32
	tonal_average   int
	lastCodedBands  int
	hf_average      int
	tapset_decision int

	prefilter_period int
	prefilter_gain   int16
	prefilter_tapset int
	consec_transient int
	analysis         AnalysisInfo
	silk_info        SILKInfo

	preemph_memE [2]int32
	preemph_memD [2]int32

	// VBR-related parameters
	vbr_reservoir int32
	vbr_drift     int32
	vbr_offset    int32
	vbr_count     int32
	overlap_max   int32
	stereo_saving int16
	intensity     int
	energy_mask   []int16
	spec_avg      int16

	in_mem        []int32 // Size = channels*mode->overlap
	prefilter_mem []int32 // Size = channels*COMBFILTER_MAXPERIOD
	oldBandE      []int16 // Size = channels*mode->nbEBands
	oldLogE       []int16 // Size = channels*mode->nbEBands
	oldLogE2      []int16 // Size = channels*mode->nbEBands
	energyError   []int16 // Size = channels*mode->nbEBands
}

// C: int celt_encoder_get_size(int channels)
func EncoderGetSize(channels int) int {
	mode := CustomModeCreate(48000, 960, nil)
	return CustomEncoderGetSize(mode, channels)
}

// CustomEncoderGetSize returns the C allocation size for API parity; the Go
// state is allocated with new/make.
//
// C: int opus_custom_encoder_get_size(const CELTMode *mode, int channels)
func CustomEncoderGetSize(mode *CELTMode, channels int) int {
	size := 240 + // sizeof(struct CELTEncoder) excluding in_mem[1], approximate
		channels*mode.overlap*4 + // celt_sig in_mem[channels*mode->overlap];
		channels*COMBFILTER_MAXPERIOD*4 + // celt_sig prefilter_mem[channels*COMBFILTER_MAXPERIOD];
		4*channels*mode.nbEBands*2 // opus_val16 oldBandE/oldLogE/oldLogE2/energyError[channels*mode->nbEBands];
	return size
}

// C: static int opus_custom_encoder_init_arch(CELTEncoder *st, const CELTMode *mode, int channels, int arch)
func (st *CELTEncoder) customEncoderInitArch(mode *CELTMode, channels int, arch int) int {
	if channels < 0 || channels > 2 {
		return OPUS_BAD_ARG
	}

	if st == nil || mode == nil {
		return OPUS_ALLOC_FAIL
	}

	*st = CELTEncoder{}

	st.mode = mode
	st.channels = channels
	st.stream_channels = channels

	st.in_mem = make([]int32, channels*mode.overlap)
	st.prefilter_mem = make([]int32, channels*COMBFILTER_MAXPERIOD)
	st.oldBandE = make([]int16, channels*mode.nbEBands)
	st.oldLogE = make([]int16, channels*mode.nbEBands)
	st.oldLogE2 = make([]int16, channels*mode.nbEBands)
	st.energyError = make([]int16, channels*mode.nbEBands)

	st.upsample = 1
	st.start = 0
	st.end = st.mode.effEBands
	st.signalling = 1
	st.arch = arch

	st.constrained_vbr = 1
	st.clip = 1

	st.bitrate = OPUS_BITRATE_MAX
	st.vbr = 0
	st.force_intra = 0
	st.complexity = 5
	st.lsb_depth = 24

	st.Ctl(OPUS_RESET_STATE)

	return OPUS_OK
}

// C: int celt_encoder_init(CELTEncoder *st, opus_int32 sampling_rate, int channels, int arch)
func (st *CELTEncoder) EncoderInit(sampling_rate int32, channels int, arch int) int {
	var ret int
	ret = st.customEncoderInitArch(CustomModeCreate(48000, 960, nil), channels, arch)
	if ret != OPUS_OK {
		return ret
	}
	st.upsample = resampling_factor(sampling_rate)
	return OPUS_OK
}

// Table of 6*64/x, trained on real data to minimize the average error
var transient_inv_table = [128]uint8{
	255, 255, 156, 110, 86, 70, 59, 51, 45, 40, 37, 33, 31, 28, 26, 25,
	23, 22, 21, 20, 19, 18, 17, 16, 16, 15, 15, 14, 13, 13, 12, 12,
	12, 12, 11, 11, 11, 10, 10, 10, 9, 9, 9, 9, 9, 9, 8, 8,
	8, 8, 8, 7, 7, 7, 7, 7, 7, 6, 6, 6, 6, 6, 6, 6,
	6, 6, 6, 6, 6, 6, 6, 6, 6, 5, 5, 5, 5, 5, 5, 5,
	5, 5, 5, 5, 5, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4,
	4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 2,
}

// C: static int transient_analysis(const opus_val32 * OPUS_RESTRICT in, int len, int C,
//
//	opus_val16 *tf_estimate, int *tf_chan, int allow_weak_transients,
//	int *weak_transient)
func transient_analysis(in []int32, len, C int, tf_estimate *int16, tf_chan *int, allow_weak_transients int,
	weak_transient *int) int {
	var i int
	var mem0, mem1 int32
	var is_transient int = 0
	var mask_metric int32 = 0
	var c int
	var tf_max int16
	var len2 int
	// Forward masking: 6.7 dB/ms.
	var forward_shift int = 4
	inv_table := &transient_inv_table
	tmp := make([]int16, len)

	*weak_transient = 0
	// For lower bitrates, let's be more conservative and have a forward masking
	// decay of 3.3 dB/ms. This avoids having to code transients at very low
	// bitrate (mostly for hybrid), which can result in unstable energy and/or
	// partial collapse.
	if allow_weak_transients != 0 {
		forward_shift = 5
	}
	len2 = len / 2
	for c = 0; c < C; c++ {
		var mean int32
		var unmask int32 = 0
		var norm int32
		var maxE int16
		mem0 = 0
		mem1 = 0
		// High-pass filter: (1 - 2*z^-1 + z^-2) / (1 - z^-1 + .5*z^-2)
		for i = 0; i < len; i++ {
			var x, y int32
			x = SHR32(in[i+c*len], SIG_SHIFT)
			y = ADD32(mem0, x)
			mem0 = mem1 + y - SHL32(x, 1)
			mem1 = x - SHR32(y, 1)
			tmp[i] = SROUND16(y, 2)
			// printf("%f ", tmp[i]);
		}
		// printf("\n");
		// First few samples are bad because we don't propagate the memory
		clear(tmp[:12])

		// Normalize tmp to max range
		{
			var shift int = 0
			shift = 14 - int(Ilog2(MAX16(1, Maxabs16(tmp, len))))
			if shift != 0 {
				for i = 0; i < len; i++ {
					tmp[i] = SHL16(tmp[i], shift)
				}
			}
		}

		mean = 0
		mem0 = 0
		// Grouping by two to reduce complexity
		// Forward pass to compute the post-echo threshold
		for i = 0; i < len2; i++ {
			var x2 int16 = int16(PSHR32(MULT16_16(tmp[2*i], tmp[2*i])+MULT16_16(tmp[2*i+1], tmp[2*i+1]), 16))
			mean += int32(x2)
			// FIXME: Use PSHR16() instead
			tmp[i] = int16(mem0 + PSHR32(int32(x2)-mem0, forward_shift))
			mem0 = int32(tmp[i])
		}

		mem0 = 0
		maxE = 0
		// Backward pass to compute the pre-echo threshold
		for i = len2 - 1; i >= 0; i-- {
			// Backward masking: 13.9 dB/ms.
			// FIXME: Use PSHR16() instead
			tmp[i] = int16(mem0 + PSHR32(int32(tmp[i])-mem0, 3))
			mem0 = int32(tmp[i])
			maxE = int16(MAX16(int32(maxE), mem0))
		}
		// for (i=0;i<len2;i++)printf("%f ", tmp[i]/mean);printf("\n");

		// Compute the ratio of the "frame energy" over the harmonic mean of the energy.
		// This essentially corresponds to a bitrate-normalized temporal noise-to-mask
		// ratio

		// As a compromise with the old transient detector, frame energy is the
		// geometric mean of the energy and half the max
		// Costs two sqrt() to avoid overflows
		mean = MULT16_16(Sqrt(mean), Sqrt(MULT16_16(maxE, len2>>1)))
		// Inverse of the mean energy in Q15+6
		norm = SHL32(EXTEND32(len2), 6+14) / ADD32(EPSILON, SHR32(mean, 1))
		// Compute harmonic mean discarding the unreliable boundaries
		// The data is smooth, so we only take 1/4th of the samples
		unmask = 0
		for i = 12; i < len2-5; i += 4 {
			var id int
			id = int(MAX32(0, MIN32(127, MULT16_32_Q15(int32(tmp[i])+EPSILON, norm)))) // Do not round to nearest
			unmask += int32(inv_table[id])
		}
		// printf("%d\n", unmask);
		// Normalize, compensate for the 1/4th of the sample and the factor of 6 in the inverse table
		unmask = 64 * unmask * 4 / int32(6*(len2-17))
		if unmask > mask_metric {
			*tf_chan = c
			mask_metric = unmask
		}
	}
	is_transient = b2i(mask_metric > 200)
	// For low bitrates, define "weak transients" that need to be
	// handled differently to avoid partial collapse.
	if allow_weak_transients != 0 && is_transient != 0 && mask_metric < 600 {
		is_transient = 0
		*weak_transient = 1
	}
	// Arbitrary metric for VBR boost
	tf_max = int16(MAX16(0, Sqrt(27*mask_metric)-42))
	// *tf_estimate = 1 + MIN16(1, sqrt(MAX16(0, tf_max-30))/20);
	*tf_estimate = int16(Sqrt(MAX32(0, SHL32(MULT16_16(QCONST16(0.0069, 14), MIN16(163, tf_max)), 14)-QCONST32(0.139, 28))))
	// printf("%d %f\n", tf_max, mask_metric);
	// printf("%d %f %d\n", is_transient, (float)*tf_estimate, tf_max);
	return is_transient
}

// Looks for sudden increases of energy to decide whether we need to patch
// the transient decision
//
// C: static int patch_transient_decision(opus_val16 *newE, opus_val16 *oldE, int nbEBands,
//
//	int start, int end, int C)
func patch_transient_decision(newE, oldE []int16, nbEBands int, start, end, C int) int {
	var i, c int
	var mean_diff int32 = 0
	var spread_old [26]int16
	// Apply an aggressive (-6 dB/Bark) spreading function to the old frame to
	// avoid false detection caused by irrelevant bands
	if C == 1 {
		spread_old[start] = oldE[start]
		for i = start + 1; i < end; i++ {
			spread_old[i] = MAX16(spread_old[i-1]-QCONST16(1.0, DB_SHIFT), oldE[i])
		}
	} else {
		spread_old[start] = MAX16(oldE[start], oldE[start+nbEBands])
		for i = start + 1; i < end; i++ {
			spread_old[i] = MAX16(spread_old[i-1]-QCONST16(1.0, DB_SHIFT),
				MAX16(oldE[i], oldE[i+nbEBands]))
		}
	}
	for i = end - 2; i >= start; i-- {
		spread_old[i] = MAX16(spread_old[i], spread_old[i+1]-QCONST16(1.0, DB_SHIFT))
	}
	// Compute mean increase
	c = 0
	for {
		for i = IMAX(2, start); i < end-1; i++ {
			var x1, x2 int16
			x1 = MAX16(0, newE[i+c*nbEBands])
			x2 = MAX16(0, spread_old[i])
			mean_diff = ADD32(mean_diff, EXTEND32(MAX16(0, SUB16(x1, x2))))
		}
		c++
		if c >= C {
			break
		}
	}
	mean_diff = DIV32(mean_diff, C*(end-1-IMAX(2, start)))
	// printf("%f %f %d\n", mean_diff, max_diff, count);
	return b2i(mean_diff > int32(QCONST16(1.0, DB_SHIFT)))
}

// Apply window and compute the MDCT for all sub-frames and all channels in a frame.
//
// C: static void compute_mdcts(const CELTMode *mode, int shortBlocks, celt_sig * OPUS_RESTRICT in,
//
//	celt_sig * OPUS_RESTRICT out, int C, int CC, int LM, int upsample,
//	int arch)
func compute_mdcts(mode *CELTMode, shortBlocks int, in []int32, out []int32, C, CC, LM, upsample int, arch int) {
	overlap := mode.overlap
	var N int
	var B int
	var shift int
	var i, b, c int
	if shortBlocks != 0 {
		B = shortBlocks
		N = mode.shortMdctSize
		shift = mode.maxLM
	} else {
		B = 1
		N = mode.shortMdctSize << uint(LM)
		shift = mode.maxLM - LM
	}
	c = 0
	for {
		for b = 0; b < B; b++ {
			// Interleaving the sub-frames while doing the MDCTs
			MdctForward(&mode.mdct, in[c*(B*N+overlap)+b*N:],
				out[b+c*N*B:], mode.window, overlap, shift, B,
				arch)
		}
		c++
		if c >= CC {
			break
		}
	}
	if CC == 2 && C == 1 {
		for i = 0; i < B*N; i++ {
			out[i] = ADD32(HALF32(out[i]), HALF32(out[B*N+i]))
		}
	}
	if upsample != 1 {
		c = 0
		for {
			var bound int = B * N / upsample
			for i = 0; i < bound; i++ {
				out[c*B*N+i] *= int32(upsample)
			}
			clear(out[c*B*N+bound : c*B*N+B*N])
			c++
			if c >= C {
				break
			}
		}
	}
}

// Preemphasis applies the pre-emphasis filter to one channel of interleaved
// input: pcmp[CC*i] is read (pass pcm[c:]).
//
// C: void celt_preemphasis(const opus_val16 * OPUS_RESTRICT pcmp, celt_sig * OPUS_RESTRICT inp,
//
//	int N, int CC, int upsample, const opus_val16 *coef, celt_sig *mem, int clip)
func Preemphasis(pcmp []int16, inp []int32, N, CC, upsample int, coef []int16, mem *int32, clip int) {
	var i int
	var coef0 int16
	var m int32
	var Nu int

	coef0 = coef[0]
	m = *mem

	// Fast path for the normal 48kHz case and no clipping
	if coef[1] == 0 && upsample == 1 && clip == 0 {
		for i = 0; i < N; i++ {
			var x int16
			x = SCALEIN(pcmp[CC*i])
			// Apply pre-emphasis
			inp[i] = SHL32(x, SIG_SHIFT) - m
			m = SHR32(MULT16_16(coef0, x), 15-SIG_SHIFT)
		}
		*mem = m
		return
	}

	Nu = N / upsample
	if upsample != 1 {
		clear(inp[:N])
	}
	for i = 0; i < Nu; i++ {
		inp[i*upsample] = int32(SCALEIN(pcmp[CC*i]))
	}

	_ = clip // Avoids a warning about clip being unused.
	{
		for i = 0; i < N; i++ {
			var x int16
			x = int16(inp[i])
			// Apply pre-emphasis
			inp[i] = SHL32(x, SIG_SHIFT) - m
			m = SHR32(MULT16_16(coef0, x), 15-SIG_SHIFT)
		}
	}
	*mem = m
}

// C: static opus_val32 l1_metric(const celt_norm *tmp, int N, int LM, opus_val16 bias)
func l1_metric(tmp []int16, N, LM int, bias int16) int32 {
	var i int
	var L1 int32
	L1 = 0
	for i = 0; i < N; i++ {
		L1 += EXTEND32(ABS16(tmp[i]))
	}
	// When in doubt, prefer good freq resolution
	L1 = MAC16_32_Q15(L1, LM*int(bias), L1)
	return L1

}

// C: static int tf_analysis(const CELTMode *m, int len, int isTransient,
//
//	int *tf_res, int lambda, celt_norm *X, int N0, int LM,
//	opus_val16 tf_estimate, int tf_chan)
func tf_analysis(m *CELTMode, len, isTransient int,
	tf_res []int, lambda int, X []int16, N0, LM int,
	tf_estimate int16, tf_chan int) int {
	var i int
	var cost0 int
	var cost1 int
	var sel int
	var selcost [2]int
	var tf_select int = 0
	var bias int16

	bias = int16(MULT16_16_Q14(QCONST16(.04, 15), MAX16(int32(-QCONST16(.25, 14)), int32(QCONST16(.5, 14))-int32(tf_estimate))))
	// printf("%f ", bias);

	metric := make([]int, len)
	tmp := make([]int16, int(m.eBands[len]-m.eBands[len-1])<<uint(LM))
	tmp_1 := make([]int16, int(m.eBands[len]-m.eBands[len-1])<<uint(LM))
	path0 := make([]int, len)
	path1 := make([]int, len)

	for i = 0; i < len; i++ {
		var k, N int
		var narrow int
		var L1, best_L1 int32
		var best_level int = 0
		N = int(m.eBands[i+1]-m.eBands[i]) << uint(LM)
		// band is too narrow to be split down to LM=-1
		narrow = b2i((m.eBands[i+1] - m.eBands[i]) == 1)
		copy(tmp[:N], X[tf_chan*N0+(int(m.eBands[i])<<uint(LM)):])
		// Just add the right channel if we're in stereo
		// if (C==2)
		//    for (j=0;j<N;j++)
		//       tmp[j] = ADD16(SHR16(tmp[j], 1),SHR16(X[N0+j+(m->eBands[i]<<LM)], 1));
		if isTransient != 0 {
			L1 = l1_metric(tmp, N, LM, bias)
		} else {
			L1 = l1_metric(tmp, N, 0, bias)
		}
		best_L1 = L1
		// Check the -1 case for transients
		if isTransient != 0 && narrow == 0 {
			copy(tmp_1[:N], tmp[:N])
			Haar1(tmp_1, N>>uint(LM), 1<<uint(LM))
			L1 = l1_metric(tmp_1, N, LM+1, bias)
			if L1 < best_L1 {
				best_L1 = L1
				best_level = -1
			}
		}
		// printf ("%f ", L1);
		for k = 0; k < LM+b2i(!(isTransient != 0 || narrow != 0)); k++ {
			var B int

			if isTransient != 0 {
				B = (LM - k - 1)
			} else {
				B = k + 1
			}

			Haar1(tmp, N>>uint(k), 1<<uint(k))

			L1 = l1_metric(tmp, N, B, bias)

			if L1 < best_L1 {
				best_L1 = L1
				best_level = k + 1
			}
		}
		// printf ("%d ", isTransient ? LM-best_level : best_level);
		// metric is in Q1 to be able to select the mid-point (-0.5) for narrower bands
		if isTransient != 0 {
			metric[i] = 2 * best_level
		} else {
			metric[i] = -2 * best_level
		}
		// For bands that can't be split to -1, set the metric to the half-way point to avoid
		// biasing the decision
		if narrow != 0 && (metric[i] == 0 || metric[i] == -2*LM) {
			metric[i] -= 1
		}
		// printf("%d ", metric[i]);
	}
	// printf("\n");
	// Search for the optimal tf resolution, including tf_select
	tf_select = 0
	for sel = 0; sel < 2; sel++ {
		cost0 = 0
		if isTransient != 0 {
			cost1 = 0
		} else {
			cost1 = lambda
		}
		for i = 1; i < len; i++ {
			var curr0, curr1 int
			curr0 = IMIN(cost0, cost1+lambda)
			curr1 = IMIN(cost0+lambda, cost1)
			cost0 = curr0 + ABS(metric[i]-2*int(tf_select_table[LM][4*isTransient+2*sel+0]))
			cost1 = curr1 + ABS(metric[i]-2*int(tf_select_table[LM][4*isTransient+2*sel+1]))
		}
		cost0 = IMIN(cost0, cost1)
		selcost[sel] = cost0
	}
	// For now, we're conservative and only allow tf_select=1 for transients.
	// If tests confirm it's useful for non-transients, we could allow it.
	if selcost[1] < selcost[0] && isTransient != 0 {
		tf_select = 1
	}
	cost0 = 0
	if isTransient != 0 {
		cost1 = 0
	} else {
		cost1 = lambda
	}
	// Viterbi forward pass
	for i = 1; i < len; i++ {
		var curr0, curr1 int
		var from0, from1 int

		from0 = cost0
		from1 = cost1 + lambda
		if from0 < from1 {
			curr0 = from0
			path0[i] = 0
		} else {
			curr0 = from1
			path0[i] = 1
		}

		from0 = cost0 + lambda
		from1 = cost1
		if from0 < from1 {
			curr1 = from0
			path1[i] = 0
		} else {
			curr1 = from1
			path1[i] = 1
		}
		cost0 = curr0 + ABS(metric[i]-2*int(tf_select_table[LM][4*isTransient+2*tf_select+0]))
		cost1 = curr1 + ABS(metric[i]-2*int(tf_select_table[LM][4*isTransient+2*tf_select+1]))
	}
	if cost0 < cost1 {
		tf_res[len-1] = 0
	} else {
		tf_res[len-1] = 1
	}
	// Viterbi backward pass to check the decisions
	for i = len - 2; i >= 0; i-- {
		if tf_res[i+1] == 1 {
			tf_res[i] = path1[i+1]
		} else {
			tf_res[i] = path0[i+1]
		}
	}
	// printf("%d %f\n", *tf_sum, tf_estimate);
	return tf_select
}

// C: static void tf_encode(int start, int end, int isTransient, int *tf_res, int LM, int tf_select, ec_enc *enc)
func tf_encode(start, end, isTransient int, tf_res []int, LM, tf_select int, enc *EcEnc) {
	var curr, i int
	var tf_select_rsv int
	var tf_changed int
	var logp int
	var budget uint32
	var tell uint32
	budget = enc.storage * 8
	tell = uint32(enc.Tell())
	if isTransient != 0 {
		logp = 2
	} else {
		logp = 4
	}
	// Reserve space to code the tf_select decision.
	tf_select_rsv = b2i(LM > 0 && tell+uint32(logp)+1 <= budget)
	budget -= uint32(tf_select_rsv)
	curr = 0
	tf_changed = 0
	for i = start; i < end; i++ {
		if tell+uint32(logp) <= budget {
			enc.EncBitLogp(tf_res[i]^curr, uint(logp))
			tell = uint32(enc.Tell())
			curr = tf_res[i]
			tf_changed |= curr
		} else {
			tf_res[i] = curr
		}
		if isTransient != 0 {
			logp = 4
		} else {
			logp = 5
		}
	}
	// Only code tf_select if it would actually make a difference.
	if tf_select_rsv != 0 &&
		tf_select_table[LM][4*isTransient+0+tf_changed] !=
			tf_select_table[LM][4*isTransient+2+tf_changed] {
		enc.EncBitLogp(tf_select, 1)
	} else {
		tf_select = 0
	}
	for i = start; i < end; i++ {
		tf_res[i] = int(tf_select_table[LM][4*isTransient+2*tf_select+tf_res[i]])
	}
	// for(i=0;i<end;i++)printf("%d ", isTransient ? tf_res[i] : LM+tf_res[i]);printf("\n");
}

// C: static int alloc_trim_analysis(const CELTMode *m, const celt_norm *X,
//
//	const opus_val16 *bandLogE, int end, int LM, int C, int N0,
//	AnalysisInfo *analysis, opus_val16 *stereo_saving, opus_val16 tf_estimate,
//	int intensity, opus_val16 surround_trim, opus_int32 equiv_rate, int arch)
func alloc_trim_analysis(m *CELTMode, X []int16,
	bandLogE []int16, end, LM, C, N0 int,
	analysis *AnalysisInfo, stereo_saving *int16, tf_estimate int16,
	intensity int, surround_trim int16, equiv_rate int32, arch int) int {
	var i int
	var diff int32 = 0
	var c int
	var trim_index int
	var trim int16 = QCONST16(5.0, 8)
	var logXC, logXC2 int16
	// At low bitrate, reducing the trim seems to help. At higher bitrates, it's less
	// clear what's best, so we're keeping it as it was before, at least for now.
	if equiv_rate < 64000 {
		trim = QCONST16(4.0, 8)
	} else if equiv_rate < 80000 {
		var frac int32 = (equiv_rate - 64000) >> 10
		trim = int16(int32(QCONST16(4.0, 8)) + int32(QCONST16(1.0/16.0, 8))*frac)
	}
	if C == 2 {
		var sum int16 = 0 // Q10
		var minXC int16   // Q10
		// Compute inter-channel correlation for low frequencies
		for i = 0; i < 8; i++ {
			var partial int32
			partial = InnerProd(X[int(m.eBands[i])<<uint(LM):], X[N0+(int(m.eBands[i])<<uint(LM)):],
				int(m.eBands[i+1]-m.eBands[i])<<uint(LM), arch)
			sum = ADD16(sum, EXTRACT16(SHR32(partial, 18)))
		}
		sum = int16(MULT16_16_Q15(QCONST16(1.0/8, 15), sum))
		sum = MIN16(QCONST16(1.0, 10), ABS16(sum))
		minXC = sum
		for i = 8; i < intensity; i++ {
			var partial int32
			partial = InnerProd(X[int(m.eBands[i])<<uint(LM):], X[N0+(int(m.eBands[i])<<uint(LM)):],
				int(m.eBands[i+1]-m.eBands[i])<<uint(LM), arch)
			minXC = MIN16(minXC, ABS16(EXTRACT16(SHR32(partial, 18))))
		}
		minXC = MIN16(QCONST16(1.0, 10), ABS16(minXC))
		// printf ("%f\n", sum);
		// mid-side savings estimations based on the LF average
		logXC = Log2(QCONST32(1.001, 20) - MULT16_16(sum, sum))
		// mid-side savings estimations based on min correlation
		logXC2 = int16(MAX16(HALF16(logXC), int32(Log2(QCONST32(1.001, 20)-MULT16_16(minXC, minXC)))))
		// Compensate for Q20 vs Q14 input and convert output to Q8
		logXC = int16(PSHR32(int32(logXC)-int32(QCONST16(6.0, DB_SHIFT)), DB_SHIFT-8))
		logXC2 = int16(PSHR32(int32(logXC2)-int32(QCONST16(6.0, DB_SHIFT)), DB_SHIFT-8))

		trim = int16(int32(trim) + MAX16(int32(-QCONST16(4.0, 8)), MULT16_16_Q15(QCONST16(.75, 15), logXC)))
		*stereo_saving = int16(MIN16(int32(*stereo_saving)+int32(QCONST16(0.25, 8)), -HALF16(logXC2)))
	}

	// Estimate spectral tilt
	c = 0
	for {
		for i = 0; i < end-1; i++ {
			diff += int32(bandLogE[i+c*m.nbEBands]) * int32(2+2*i-end)
		}
		c++
		if c >= C {
			break
		}
	}
	diff /= int32(C * (end - 1))
	// printf("%f\n", diff);
	trim = int16(int32(trim) - MAX32(int32(-QCONST16(2.0, 8)), MIN32(int32(QCONST16(2.0, 8)), SHR32(diff+int32(QCONST16(1.0, DB_SHIFT)), DB_SHIFT-8)/6)))
	trim = int16(int32(trim) - SHR16(surround_trim, DB_SHIFT-8))
	trim = int16(int32(trim) - 2*SHR16(tf_estimate, 14-8))
	if analysis.Valid != 0 {
		trim = int16(int32(trim) - MAX16(int32(-QCONST16(2.0, 8)), MIN16(int32(QCONST16(2.0, 8)),
			int32(int16(float32(QCONST16(2.0, 8))*(analysis.Tonality_slope+0.05))))))
	}

	trim_index = int(PSHR32(trim, 8))
	trim_index = IMAX(0, IMIN(10, trim_index))
	// printf("%d\n", trim_index);
	return trim_index
}

// C: static int stereo_analysis(const CELTMode *m, const celt_norm *X, int LM, int N0)
func stereo_analysis(m *CELTMode, X []int16, LM, N0 int) int {
	var i int
	var thetas int
	var sumLR, sumMS int32 = EPSILON, EPSILON

	// Use the L1 norm to model the entropy of the L/R signal vs the M/S signal
	for i = 0; i < 13; i++ {
		var j int
		for j = int(m.eBands[i]) << uint(LM); j < int(m.eBands[i+1])<<uint(LM); j++ {
			var L, R, M, S int32
			// We cast to 32-bit first because of the -32768 case
			L = EXTEND32(X[j])
			R = EXTEND32(X[N0+j])
			M = ADD32(L, R)
			S = SUB32(L, R)
			sumLR = ADD32(sumLR, ADD32(ABS32(L), ABS32(R)))
			sumMS = ADD32(sumMS, ADD32(ABS32(M), ABS32(S)))
		}
	}
	sumMS = MULT16_32_Q15(QCONST16(0.707107, 15), sumMS)
	thetas = 13
	// We don't need thetas for lower bands with LM<=1
	if LM <= 1 {
		thetas -= 8
	}
	return b2i(MULT16_32_Q15((int(m.eBands[13])<<uint(LM+1))+thetas, sumMS) >
		MULT16_32_Q15(int(m.eBands[13])<<uint(LM+1), sumLR))
}

// C: static opus_val16 median_of_5(const opus_val16 *x)
func median_of_5(x []int16) int16 {
	var t0, t1, t2, t3, t4 int16
	t2 = x[2]
	if x[0] > x[1] {
		t0 = x[1]
		t1 = x[0]
	} else {
		t0 = x[0]
		t1 = x[1]
	}
	if x[3] > x[4] {
		t3 = x[4]
		t4 = x[3]
	} else {
		t3 = x[3]
		t4 = x[4]
	}
	if t0 > t3 {
		t0, t3 = t3, t0 // MSWAP(t0, t3)
		t1, t4 = t4, t1 // MSWAP(t1, t4)
	}
	if t2 > t1 {
		if t1 < t3 {
			return MIN16(t2, t3)
		}
		return MIN16(t4, t1)
	}
	if t2 < t3 {
		return MIN16(t1, t3)
	}
	return MIN16(t2, t4)
}

// C: static opus_val16 median_of_3(const opus_val16 *x)
func median_of_3(x []int16) int16 {
	var t0, t1, t2 int16
	if x[0] > x[1] {
		t0 = x[1]
		t1 = x[0]
	} else {
		t0 = x[0]
		t1 = x[1]
	}
	t2 = x[2]
	if t1 < t2 {
		return t1
	} else if t0 < t2 {
		return t2
	}
	return t0
}

// C: static opus_val16 dynalloc_analysis(const opus_val16 *bandLogE, const opus_val16 *bandLogE2,
//
//	int nbEBands, int start, int end, int C, int *offsets, int lsb_depth, const opus_int16 *logN,
//	int isTransient, int vbr, int constrained_vbr, const opus_int16 *eBands, int LM,
//	int effectiveBytes, opus_int32 *tot_boost_, int lfe, opus_val16 *surround_dynalloc, AnalysisInfo *analysis)
func dynalloc_analysis(bandLogE, bandLogE2 []int16,
	nbEBands, start, end, C int, offsets []int, lsb_depth int, logN []int16,
	isTransient, vbr, constrained_vbr int, eBands []int16, LM int,
	effectiveBytes int, tot_boost_ *int32, lfe int, surround_dynalloc []int16, analysis *AnalysisInfo) int16 {
	var i, c int
	var tot_boost int32 = 0
	var maxDepth int16
	follower := make([]int16, C*nbEBands)
	noise_floor := make([]int16, C*nbEBands)
	clear(offsets[:nbEBands])
	// Dynamic allocation code
	maxDepth = -QCONST16(31.9, DB_SHIFT)
	for i = 0; i < end; i++ {
		// Noise floor must take into account eMeans, the depth, the width of the bands
		// and the preemphasis filter (approx. square of bark band ID)
		noise_floor[i] = int16(MULT16_16(QCONST16(0.0625, DB_SHIFT), logN[i]) +
			int32(QCONST16(.5, DB_SHIFT)) + int32(SHL16(9-lsb_depth, DB_SHIFT)) - int32(SHL16(eMeans[i], 6)) +
			MULT16_16(QCONST16(.0062, DB_SHIFT), (i+5)*(i+5)))
	}
	c = 0
	for {
		for i = 0; i < end; i++ {
			maxDepth = MAX16(maxDepth, bandLogE[c*nbEBands+i]-noise_floor[i])
		}
		c++
		if c >= C {
			break
		}
	}
	// Make sure that dynamic allocation can't make us bust the budget
	if effectiveBytes > 50 && LM >= 1 && lfe == 0 {
		var last int = 0
		c = 0
		for {
			var offset int16
			var tmp int16
			var f []int16
			f = follower[c*nbEBands:]
			f[0] = bandLogE2[c*nbEBands]
			for i = 1; i < end; i++ {
				// The last band to be at least 3 dB higher than the previous one
				// is the last we'll consider. Otherwise, we run into problems on
				// bandlimited signals.
				if bandLogE2[c*nbEBands+i] > bandLogE2[c*nbEBands+i-1]+QCONST16(.5, DB_SHIFT) {
					last = i
				}
				f[i] = MIN16(f[i-1]+QCONST16(1.5, DB_SHIFT), bandLogE2[c*nbEBands+i])
			}
			for i = last - 1; i >= 0; i-- {
				f[i] = MIN16(f[i], MIN16(f[i+1]+QCONST16(2.0, DB_SHIFT), bandLogE2[c*nbEBands+i]))
			}

			// Combine with a median filter to avoid dynalloc triggering unnecessarily.
			// The "offset" value controls how conservative we are -- a higher offset
			// reduces the impact of the median filter and makes dynalloc use more bits.
			offset = QCONST16(1.0, DB_SHIFT)
			for i = 2; i < end-2; i++ {
				f[i] = MAX16(f[i], median_of_5(bandLogE2[c*nbEBands+i-2:])-offset)
			}
			tmp = median_of_3(bandLogE2[c*nbEBands:]) - offset
			f[0] = MAX16(f[0], tmp)
			f[1] = MAX16(f[1], tmp)
			tmp = median_of_3(bandLogE2[c*nbEBands+end-3:]) - offset
			f[end-2] = MAX16(f[end-2], tmp)
			f[end-1] = MAX16(f[end-1], tmp)

			for i = 0; i < end; i++ {
				f[i] = MAX16(f[i], noise_floor[i])
			}
			c++
			if c >= C {
				break
			}
		}
		if C == 2 {
			for i = start; i < end; i++ {
				// Consider 24 dB "cross-talk"
				follower[nbEBands+i] = MAX16(follower[nbEBands+i], follower[i]-QCONST16(4.0, DB_SHIFT))
				follower[i] = MAX16(follower[i], follower[nbEBands+i]-QCONST16(4.0, DB_SHIFT))
				follower[i] = int16(HALF16(int32(MAX16(0, bandLogE[i]-follower[i])) + int32(MAX16(0, bandLogE[nbEBands+i]-follower[nbEBands+i]))))
			}
		} else {
			for i = start; i < end; i++ {
				follower[i] = MAX16(0, bandLogE[i]-follower[i])
			}
		}
		for i = start; i < end; i++ {
			follower[i] = MAX16(follower[i], surround_dynalloc[i])
		}
		// For non-transient CBR/CVBR frames, halve the dynalloc contribution
		if (vbr == 0 || constrained_vbr != 0) && isTransient == 0 {
			for i = start; i < end; i++ {
				follower[i] = int16(HALF16(follower[i]))
			}
		}
		for i = start; i < end; i++ {
			if i < 8 {
				follower[i] *= 2
			}
			if i >= 12 {
				follower[i] = int16(HALF16(follower[i]))
			}
		}
		if analysis.Valid != 0 {
			for i = start; i < IMIN(LEAK_BANDS, end); i++ {
				follower[i] = int16(int32(follower[i]) + int32(QCONST16(1.0/64.0, DB_SHIFT))*int32(analysis.Leak_boost[i]))
			}
		}
		for i = start; i < end; i++ {
			var width int
			var boost int
			var boost_bits int

			follower[i] = MIN16(follower[i], QCONST16(4, DB_SHIFT))

			width = C * int(eBands[i+1]-eBands[i]) << uint(LM)
			if width < 6 {
				boost = int(SHR32(EXTEND32(follower[i]), DB_SHIFT))
				boost_bits = boost * width << BITRES
			} else if width > 48 {
				boost = int(SHR32(EXTEND32(follower[i])*8, DB_SHIFT))
				boost_bits = (boost * width << BITRES) / 8
			} else {
				boost = int(SHR32(EXTEND32(follower[i])*int32(width)/6, DB_SHIFT))
				boost_bits = boost * 6 << BITRES
			}
			// For CBR and non-transient CVBR frames, limit dynalloc to 2/3 of the bits
			if (vbr == 0 || (constrained_vbr != 0 && isTransient == 0)) &&
				(tot_boost+int32(boost_bits))>>BITRES>>3 > int32(2*effectiveBytes/3) {
				var cap int32 = int32((2 * effectiveBytes / 3) << BITRES << 3)
				offsets[i] = int(cap - tot_boost)
				tot_boost = cap
				break
			} else {
				offsets[i] = boost
				tot_boost += int32(boost_bits)
			}
		}
	}
	*tot_boost_ = tot_boost
	return maxDepth
}

// C: static int run_prefilter(CELTEncoder *st, celt_sig *in, celt_sig *prefilter_mem, int CC, int N,
//
//	int prefilter_tapset, int *pitch, opus_val16 *gain, int *qgain, int enabled, int nbAvailableBytes)
func run_prefilter(st *CELTEncoder, in []int32, prefilter_mem []int32, CC, N int,
	prefilter_tapset int, pitch *int, gain *int16, qgain *int, enabled int, nbAvailableBytes int) int {
	var c int
	var pre [2][]int32
	var mode *CELTMode
	var pitch_index int
	var gain1 int16
	var pf_threshold int16
	var pf_on int
	var qg int
	var overlap int

	mode = st.mode
	overlap = mode.overlap
	_pre := make([]int32, CC*(N+COMBFILTER_MAXPERIOD))

	pre[0] = _pre
	pre[1] = _pre[(N + COMBFILTER_MAXPERIOD):]

	c = 0
	for {
		copy(pre[c][:COMBFILTER_MAXPERIOD], prefilter_mem[c*COMBFILTER_MAXPERIOD:])
		copy(pre[c][COMBFILTER_MAXPERIOD:COMBFILTER_MAXPERIOD+N], in[c*(N+overlap)+overlap:])
		c++
		if c >= CC {
			break
		}
	}

	if enabled != 0 {
		pitch_buf := make([]int16, (COMBFILTER_MAXPERIOD+N)>>1)

		PitchDownsample(pre[:], pitch_buf, COMBFILTER_MAXPERIOD+N, CC, st.arch)
		// Don't search for the fir last 1.5 octave of the range because
		// there's too many false-positives due to short-term correlation
		PitchSearch(pitch_buf[(COMBFILTER_MAXPERIOD>>1):], pitch_buf, N,
			COMBFILTER_MAXPERIOD-3*COMBFILTER_MINPERIOD, &pitch_index,
			st.arch)
		pitch_index = COMBFILTER_MAXPERIOD - pitch_index

		gain1 = RemoveDoubling(pitch_buf, COMBFILTER_MAXPERIOD, COMBFILTER_MINPERIOD,
			N, &pitch_index, st.prefilter_period, st.prefilter_gain, st.arch)
		if pitch_index > COMBFILTER_MAXPERIOD-2 {
			pitch_index = COMBFILTER_MAXPERIOD - 2
		}
		gain1 = int16(MULT16_16_Q15(QCONST16(.7, 15), gain1))
		// printf("%d %d %f %f\n", pitch_change, pitch_index, gain1, st->analysis.tonality);
		if st.loss_rate > 2 {
			gain1 = int16(HALF32(gain1))
		}
		if st.loss_rate > 4 {
			gain1 = int16(HALF32(gain1))
		}
		if st.loss_rate > 8 {
			gain1 = 0
		}
	} else {
		gain1 = 0
		pitch_index = COMBFILTER_MINPERIOD
	}

	// Gain threshold for enabling the prefilter/postfilter
	pf_threshold = QCONST16(.2, 15)

	// Adjusting the threshold based on rate and continuity
	if ABS(pitch_index-st.prefilter_period)*10 > pitch_index {
		pf_threshold += QCONST16(.2, 15)
	}
	if nbAvailableBytes < 25 {
		pf_threshold += QCONST16(.1, 15)
	}
	if nbAvailableBytes < 35 {
		pf_threshold += QCONST16(.1, 15)
	}
	if st.prefilter_gain > QCONST16(.4, 15) {
		pf_threshold -= QCONST16(.1, 15)
	}
	if st.prefilter_gain > QCONST16(.55, 15) {
		pf_threshold -= QCONST16(.1, 15)
	}

	// Hard threshold at 0.2
	pf_threshold = MAX16(pf_threshold, QCONST16(.2, 15))
	if gain1 < pf_threshold {
		gain1 = 0
		pf_on = 0
		qg = 0
	} else {
		// This block is not gated by a total bits check only because
		// of the nbAvailableBytes check above.
		if ABS16(int32(gain1)-int32(st.prefilter_gain)) < int32(QCONST16(.1, 15)) {
			gain1 = st.prefilter_gain
		}

		qg = ((int(gain1)+1536)>>10)/3 - 1
		qg = IMAX(0, IMIN(7, qg))
		gain1 = int16(int32(QCONST16(0.09375, 15)) * int32(qg+1))
		pf_on = 1
	}
	// printf("%d %f\n", pitch_index, gain1);

	c = 0
	for {
		var offset int = mode.shortMdctSize - overlap
		st.prefilter_period = IMAX(st.prefilter_period, COMBFILTER_MINPERIOD)
		copy(in[c*(N+overlap):c*(N+overlap)+overlap], st.in_mem[c*(overlap):])
		if offset != 0 {
			comb_filter(in, c*(N+overlap)+overlap, pre[c], COMBFILTER_MAXPERIOD,
				st.prefilter_period, st.prefilter_period, offset, -st.prefilter_gain, -st.prefilter_gain,
				st.prefilter_tapset, st.prefilter_tapset, nil, 0, st.arch)
		}

		comb_filter(in, c*(N+overlap)+overlap+offset, pre[c], COMBFILTER_MAXPERIOD+offset,
			st.prefilter_period, pitch_index, N-offset, -st.prefilter_gain, -gain1,
			st.prefilter_tapset, prefilter_tapset, mode.window, overlap, st.arch)
		copy(st.in_mem[c*(overlap):c*(overlap)+overlap], in[c*(N+overlap)+N:])

		if N > COMBFILTER_MAXPERIOD {
			copy(prefilter_mem[c*COMBFILTER_MAXPERIOD:c*COMBFILTER_MAXPERIOD+COMBFILTER_MAXPERIOD], pre[c][N:])
		} else {
			copy(prefilter_mem[c*COMBFILTER_MAXPERIOD:c*COMBFILTER_MAXPERIOD+COMBFILTER_MAXPERIOD-N], prefilter_mem[c*COMBFILTER_MAXPERIOD+N:])
			copy(prefilter_mem[c*COMBFILTER_MAXPERIOD+COMBFILTER_MAXPERIOD-N:c*COMBFILTER_MAXPERIOD+COMBFILTER_MAXPERIOD], pre[c][COMBFILTER_MAXPERIOD:])
		}
		c++
		if c >= CC {
			break
		}
	}

	*gain = gain1
	*pitch = pitch_index
	*qgain = qg
	return pf_on
}

// C: static int compute_vbr(const CELTMode *mode, AnalysisInfo *analysis, opus_int32 base_target,
//
//	int LM, opus_int32 bitrate, int lastCodedBands, int C, int intensity,
//	int constrained_vbr, opus_val16 stereo_saving, int tot_boost,
//	opus_val16 tf_estimate, int pitch_change, opus_val16 maxDepth,
//	int lfe, int has_surround_mask, opus_val16 surround_masking,
//	opus_val16 temporal_vbr)
func compute_vbr(mode *CELTMode, analysis *AnalysisInfo, base_target int32,
	LM int, bitrate int32, lastCodedBands, C, intensity int,
	constrained_vbr int, stereo_saving int16, tot_boost int,
	tf_estimate int16, pitch_change int, maxDepth int16,
	lfe, has_surround_mask int, surround_masking int16,
	temporal_vbr int16) int32 {
	// The target rate in 8th bits per frame
	var target int32
	var coded_bins int
	var coded_bands int
	var tf_calibration int16
	var nbEBands int
	var eBands []int16

	nbEBands = mode.nbEBands
	eBands = mode.eBands

	if lastCodedBands != 0 {
		coded_bands = lastCodedBands
	} else {
		coded_bands = nbEBands
	}
	coded_bins = int(eBands[coded_bands]) << uint(LM)
	if C == 2 {
		coded_bins += int(eBands[IMIN(intensity, coded_bands)]) << uint(LM)
	}

	target = base_target

	// printf("%f %f %f %f %d %d ", st->analysis.activity, st->analysis.tonality, tf_estimate, st->stereo_saving, tot_boost, coded_bands);
	if analysis.Valid != 0 && float64(analysis.Activity) < .4 {
		target -= int32(float32(coded_bins<<BITRES) * (0.4 - analysis.Activity))
	}
	// Stereo savings
	if C == 2 {
		var coded_stereo_bands int
		var coded_stereo_dof int
		var max_frac int16
		coded_stereo_bands = IMIN(intensity, coded_bands)
		coded_stereo_dof = (int(eBands[coded_stereo_bands]) << uint(LM)) - coded_stereo_bands
		// Maximum fraction of the bits we can save if the signal is mono.
		max_frac = DIV32_16(MULT16_16(QCONST16(0.8, 15), coded_stereo_dof), coded_bins)
		stereo_saving = MIN16(stereo_saving, QCONST16(1.0, 8))
		// printf("%d %d %d ", coded_stereo_dof, coded_bins, tot_boost);
		target -= int32(MIN32(MULT16_32_Q15(max_frac, target),
			SHR32(MULT16_16(int32(stereo_saving)-int32(QCONST16(0.1, 8)), (coded_stereo_dof<<BITRES)), 8)))
	}
	// Boost the rate according to dynalloc (minus the dynalloc average for calibration).
	target += int32(tot_boost - (19 << uint(LM)))
	// Apply transient boost, compensating for average boost.
	tf_calibration = QCONST16(0.044, 14)
	target += int32(SHL32(MULT16_32_Q15(int32(tf_estimate)-int32(tf_calibration), target), 1))

	// Apply tonality boost
	if analysis.Valid != 0 && lfe == 0 {
		var tonal_target int32
		var tonal float32

		// Tonality boost (compensating for the average).
		tonal = max(float32(0.0), analysis.Tonality-0.15) - 0.12
		tonal_target = target + int32(float32(coded_bins<<BITRES)*1.2*tonal)
		if pitch_change != 0 {
			tonal_target += int32(float32(coded_bins<<BITRES) * .8)
		}
		// printf("%f %f ", analysis->tonality, tonal);
		target = tonal_target
	}

	if has_surround_mask != 0 && lfe == 0 {
		var surround_target int32 = target + int32(SHR32(MULT16_16(surround_masking, coded_bins<<BITRES), DB_SHIFT))
		// printf("%f %d %d %d %d %d %d ", surround_masking, coded_bins, st->end, st->intensity, surround_target, target, st->bitrate);
		target = IMAX(target/4, surround_target)
	}

	{
		var floor_depth int32
		var bins int
		bins = int(eBands[nbEBands-2]) << uint(LM)
		// floor_depth = SHR32(MULT16_16((C*bins<<BITRES),celt_log2(SHL32(MAX16(1,sample_max),13))), DB_SHIFT);
		floor_depth = int32(SHR32(MULT16_16((C*bins<<BITRES), maxDepth), DB_SHIFT))
		floor_depth = IMAX(floor_depth, target>>2)
		target = IMIN(target, floor_depth)
		// printf("%f %d\n", maxDepth, floor_depth);
	}

	// Make VBR less aggressive for constrained VBR because we can't keep a higher bitrate
	// for long. Needs tuning.
	if (has_surround_mask == 0 || lfe != 0) && constrained_vbr != 0 {
		target = base_target + int32(MULT16_32_Q15(QCONST16(0.67, 15), target-base_target))
	}

	if has_surround_mask == 0 && tf_estimate < QCONST16(.2, 14) {
		var amount int16
		var tvbr_factor int16
		amount = int16(MULT16_16_Q15(QCONST16(.0000031, 30), IMAX(0, IMIN(32000, 96000-bitrate))))
		tvbr_factor = int16(SHR32(MULT16_16(temporal_vbr, amount), DB_SHIFT))
		target += int32(MULT16_32_Q15(tvbr_factor, target))
	}

	// Don't allow more than doubling the rate
	target = IMIN(2*base_target, target)

	return target
}

var intensity_thresholds = [21]int16{
	// 0  1  2  3  4  5  6  7  8  9 10 11 12 13 14 15 16 17 18 19  20  off
	1, 2, 3, 4, 5, 6, 7, 8, 16, 24, 36, 44, 50, 56, 62, 67, 72, 79, 88, 106, 134}
var intensity_histeresis = [21]int16{
	1, 1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 2, 2, 2, 3, 3, 4, 5, 6, 8, 8}

// EncodeWithEc encodes one frame. Returns the number of bytes written to
// compressed or a negative error code.
//
// C: int celt_encode_with_ec(CELTEncoder * OPUS_RESTRICT st, const opus_val16 * pcm, int frame_size, unsigned char *compressed, int nbCompressedBytes, ec_enc *enc)
func (st *CELTEncoder) EncodeWithEc(pcm []int16, frame_size int, compressed []byte, nbCompressedBytes int, enc *EcEnc) int {
	var i, c, N int
	var bits int32
	var _enc EcEnc
	var prefilter_mem []int32
	var oldBandE, oldLogE, oldLogE2, energyError []int16
	var shortBlocks int = 0
	var isTransient int = 0
	CC := st.channels
	C := st.stream_channels
	var LM, M int
	var tf_select int
	var nbFilledBytes, nbAvailableBytes int
	var start int
	var end int
	var effEnd int
	var codedBands int
	var alloc_trim int
	var pitch_index int = COMBFILTER_MINPERIOD
	var gain1 int16 = 0
	var dual_stereo int = 0
	var effectiveBytes int
	var dynalloc_logp int
	var vbr_rate int32
	var total_bits int32
	var total_boost int32
	var balance int32
	var tell int32
	var tell0_frac int32
	var prefilter_tapset int = 0
	var pf_on int
	var anti_collapse_rsv int
	var anti_collapse_on int = 0
	var silence int = 0
	var tf_chan int = 0
	var tf_estimate int16
	var pitch_change int = 0
	var tot_boost int32
	var sample_max int32
	var maxDepth int16
	var mode *CELTMode
	var nbEBands int
	var overlap int
	var eBands []int16
	var secondMdct int
	var signalBandwidth int
	var transient_got_disabled int = 0
	var surround_masking int16 = 0
	var temporal_vbr int16 = 0
	var surround_trim int16 = 0
	var equiv_rate int32
	var hybrid int
	var weak_transient int = 0

	mode = st.mode
	nbEBands = mode.nbEBands
	overlap = mode.overlap
	eBands = mode.eBands
	start = st.start
	end = st.end
	hybrid = b2i(start != 0)
	tf_estimate = 0
	if nbCompressedBytes < 2 || pcm == nil {
		return OPUS_BAD_ARG
	}

	frame_size *= st.upsample
	for LM = 0; LM <= mode.maxLM; LM++ {
		if mode.shortMdctSize<<uint(LM) == frame_size {
			break
		}
	}
	if LM > mode.maxLM {
		return OPUS_BAD_ARG
	}
	M = 1 << uint(LM)
	N = M * mode.shortMdctSize

	prefilter_mem = st.prefilter_mem
	oldBandE = st.oldBandE
	oldLogE = st.oldLogE
	oldLogE2 = st.oldLogE2
	energyError = st.energyError

	if enc == nil {
		tell = 1
		tell0_frac = tell
		nbFilledBytes = 0
	} else {
		tell = int32(enc.TellFrac())
		tell0_frac = tell
		tell = int32(enc.Tell())
		nbFilledBytes = int((tell + 4) >> 3)
	}

	celt_assert(st.signalling == 0)

	// Can't produce more than 1275 output bytes
	nbCompressedBytes = IMIN(nbCompressedBytes, 1275)
	nbAvailableBytes = nbCompressedBytes - nbFilledBytes

	if st.vbr != 0 && st.bitrate != OPUS_BITRATE_MAX {
		var den int32 = mode.Fs >> BITRES
		vbr_rate = (st.bitrate*int32(frame_size) + (den >> 1)) / den
		effectiveBytes = int(vbr_rate >> (3 + BITRES))
	} else {
		var tmp int32
		vbr_rate = 0
		tmp = st.bitrate * int32(frame_size)
		if tell > 1 {
			tmp += tell
		}
		if st.bitrate != OPUS_BITRATE_MAX {
			nbCompressedBytes = IMAX(2, IMIN(nbCompressedBytes,
				int((tmp+4*mode.Fs)/(8*mode.Fs))-b2i(st.signalling != 0)))
		}
		effectiveBytes = nbCompressedBytes - nbFilledBytes
	}
	equiv_rate = (int32(nbCompressedBytes) * 8 * 50 >> uint(3-LM)) - int32((40*C+20)*((400>>uint(LM))-50))
	if st.bitrate != OPUS_BITRATE_MAX {
		equiv_rate = IMIN(equiv_rate, st.bitrate-int32((40*C+20)*((400>>uint(LM))-50)))
	}

	if enc == nil {
		_enc.EncInit(compressed, uint32(nbCompressedBytes))
		enc = &_enc
	}

	if vbr_rate > 0 {
		// Computes the max bit-rate allowed in VBR mode to avoid violating the
		// target rate and buffering.
		// We must do this up front so that bust-prevention logic triggers
		// correctly if we don't have enough bits.
		if st.constrained_vbr != 0 {
			var vbr_bound int32
			var max_allowed int32
			// We could use any multiple of vbr_rate as bound (depending on the
			// delay).
			// This is clamped to ensure we use at least two bytes if the encoder
			// was entirely empty, but to allow 0 in hybrid mode.
			vbr_bound = vbr_rate
			var minBytes int32
			if tell == 1 {
				minBytes = 2
			}
			max_allowed = IMIN(IMAX(minBytes,
				(vbr_rate+vbr_bound-st.vbr_reservoir)>>(BITRES+3)),
				int32(nbAvailableBytes))
			if max_allowed < int32(nbAvailableBytes) {
				nbCompressedBytes = nbFilledBytes + int(max_allowed)
				nbAvailableBytes = int(max_allowed)
				enc.EncShrink(uint32(nbCompressedBytes))
			}
		}
	}
	total_bits = int32(nbCompressedBytes * 8)

	effEnd = end
	if effEnd > mode.effEBands {
		effEnd = mode.effEBands
	}

	in := make([]int32, CC*(N+overlap))

	sample_max = MAX32(st.overlap_max, Maxabs16(pcm, C*(N-overlap)/st.upsample))
	st.overlap_max = Maxabs16(pcm[C*(N-overlap)/st.upsample:], C*overlap/st.upsample)
	sample_max = MAX32(sample_max, st.overlap_max)
	silence = b2i(sample_max == 0)
	if tell == 1 {
		enc.EncBitLogp(silence, 15)
	} else {
		silence = 0
	}
	if silence != 0 {
		// In VBR mode there is no need to send more than the minimum.
		if vbr_rate > 0 {
			nbCompressedBytes = IMIN(nbCompressedBytes, nbFilledBytes+2)
			effectiveBytes = nbCompressedBytes
			total_bits = int32(nbCompressedBytes * 8)
			nbAvailableBytes = 2
			enc.EncShrink(uint32(nbCompressedBytes))
		}
		// Pretend we've filled all the remaining bits with zeros
		// (that's what the initialiser did anyway)
		tell = int32(nbCompressedBytes * 8)
		enc.nbits_total += int(tell) - enc.Tell()
	}
	c = 0
	for {
		var need_clip int = 0
		Preemphasis(pcm[c:], in[c*(N+overlap)+overlap:], N, CC, st.upsample,
			mode.preemph[:], &st.preemph_memE[c], need_clip)
		c++
		if c >= CC {
			break
		}
	}

	// Find pitch period and gain
	{
		var enabled int
		var qg int
		enabled = b2i(((st.lfe != 0 && nbAvailableBytes > 3) || nbAvailableBytes > 12*C) && hybrid == 0 && silence == 0 && st.disable_pf == 0 &&
			st.complexity >= 5)

		prefilter_tapset = st.tapset_decision
		pf_on = run_prefilter(st, in, prefilter_mem, CC, N, prefilter_tapset, &pitch_index, &gain1, &qg, enabled, nbAvailableBytes)
		if (gain1 > QCONST16(.4, 15) || st.prefilter_gain > QCONST16(.4, 15)) && (st.analysis.Valid == 0 || float64(st.analysis.Tonality) > .3) &&
			(float64(pitch_index) > 1.26*float64(st.prefilter_period) || float64(pitch_index) < .79*float64(st.prefilter_period)) {
			pitch_change = 1
		}
		if pf_on == 0 {
			if hybrid == 0 && tell+16 <= total_bits {
				enc.EncBitLogp(0, 1)
			}
		} else {
			// This block is not gated by a total bits check only because
			// of the nbAvailableBytes check above.
			var octave int
			enc.EncBitLogp(1, 1)
			pitch_index += 1
			octave = EC_ILOG(uint32(pitch_index)) - 5
			enc.EncUint(uint32(octave), 6)
			enc.EncBits(uint32(pitch_index-(16<<uint(octave))), uint(4+octave))
			pitch_index -= 1
			enc.EncBits(uint32(qg), 3)
			enc.EncIcdf(prefilter_tapset, tapset_icdf[:], 2)
		}
	}

	isTransient = 0
	shortBlocks = 0
	if st.complexity >= 1 && st.lfe == 0 {
		// Reduces the likelihood of energy instability on fricatives at low bitrate
		// in hybrid mode. It seems like we still want to have real transients on vowels
		// though (small SILK quantization offset value).
		var allow_weak_transients int = b2i(hybrid != 0 && effectiveBytes < 15 && st.silk_info.Offset >= 100)
		isTransient = transient_analysis(in, N+overlap, CC,
			&tf_estimate, &tf_chan, allow_weak_transients, &weak_transient)
	}
	if LM > 0 && enc.Tell()+3 <= int(total_bits) {
		if isTransient != 0 {
			shortBlocks = M
		}
	} else {
		isTransient = 0
		transient_got_disabled = 1
	}

	freq := make([]int32, CC*N) // Interleaved signal MDCTs
	bandE := make([]int32, nbEBands*CC)
	bandLogE := make([]int16, nbEBands*CC)

	secondMdct = b2i(shortBlocks != 0 && st.complexity >= 8)
	bandLogE2 := make([]int16, C*nbEBands)
	if secondMdct != 0 {
		compute_mdcts(mode, 0, in, freq, C, CC, LM, st.upsample, st.arch)
		ComputeBandEnergies(mode, freq, bandE, effEnd, C, LM, st.arch)
		Amp2Log2(mode, effEnd, end, bandE, bandLogE2, C)
		for i = 0; i < C*nbEBands; i++ {
			bandLogE2[i] += int16(HALF16(SHL16(LM, DB_SHIFT)))
		}
	}

	compute_mdcts(mode, shortBlocks, in, freq, C, CC, LM, st.upsample, st.arch)
	if CC == 2 && C == 1 {
		tf_chan = 0
	}
	ComputeBandEnergies(mode, freq, bandE, effEnd, C, LM, st.arch)

	if st.lfe != 0 {
		for i = 2; i < end; i++ {
			bandE[i] = IMIN(bandE[i], MULT16_32_Q15(QCONST16(1e-4, 15), bandE[0]))
			bandE[i] = MAX32(bandE[i], EPSILON)
		}
	}
	Amp2Log2(mode, effEnd, end, bandE, bandLogE, C)

	surround_dynalloc := make([]int16, C*nbEBands)
	clear(surround_dynalloc[:end])
	// This computes how much masking takes place between surround channels
	if hybrid == 0 && st.energy_mask != nil && st.lfe == 0 {
		var mask_end int
		var midband int
		var count_dynalloc int
		var mask_avg int32 = 0
		var diff int32 = 0
		var count int = 0
		mask_end = IMAX(2, st.lastCodedBands)
		for c = 0; c < C; c++ {
			for i = 0; i < mask_end; i++ {
				var mask int16
				mask = MAX16(MIN16(st.energy_mask[nbEBands*c+i],
					QCONST16(.25, DB_SHIFT)), -QCONST16(2.0, DB_SHIFT))
				if mask > 0 {
					mask = int16(HALF16(mask))
				}
				mask_avg += MULT16_16(mask, eBands[i+1]-eBands[i])
				count += int(eBands[i+1] - eBands[i])
				diff += MULT16_16(mask, 1+2*i-mask_end)
			}
		}
		celt_assert(count > 0)
		mask_avg = int32(DIV32_16(mask_avg, count))
		mask_avg += int32(QCONST16(.2, DB_SHIFT))
		diff = diff * 6 / int32(C*(mask_end-1)*(mask_end+1)*mask_end)
		// Again, being conservative
		diff = HALF32(diff)
		diff = MAX32(MIN32(diff, QCONST32(.031, DB_SHIFT)), -QCONST32(.031, DB_SHIFT))
		// Find the band that's in the middle of the coded spectrum
		for midband = 0; eBands[midband+1] < eBands[mask_end]/2; midband++ {
		}
		count_dynalloc = 0
		for i = 0; i < mask_end; i++ {
			var lin int32
			var unmask int16
			lin = mask_avg + diff*int32(i-midband)
			if C == 2 {
				unmask = MAX16(st.energy_mask[i], st.energy_mask[nbEBands+i])
			} else {
				unmask = st.energy_mask[i]
			}
			unmask = MIN16(unmask, QCONST16(.0, DB_SHIFT))
			unmask = int16(int32(unmask) - lin)
			if unmask > QCONST16(.25, DB_SHIFT) {
				surround_dynalloc[i] = unmask - QCONST16(.25, DB_SHIFT)
				count_dynalloc++
			}
		}
		if count_dynalloc >= 3 {
			// If we need dynalloc in many bands, it's probably because our
			// initial masking rate was too low.
			mask_avg += int32(QCONST16(.25, DB_SHIFT))
			if mask_avg > 0 {
				// Something went really wrong in the original calculations,
				// disabling masking.
				mask_avg = 0
				diff = 0
				clear(surround_dynalloc[:mask_end])
			} else {
				for i = 0; i < mask_end; i++ {
					surround_dynalloc[i] = MAX16(0, surround_dynalloc[i]-QCONST16(.25, DB_SHIFT))
				}
			}
		}
		mask_avg += int32(QCONST16(.2, DB_SHIFT))
		// Convert to 1/64th units used for the trim
		surround_trim = int16(64 * diff)
		// printf("%d %d ", mask_avg, surround_trim);
		surround_masking = int16(mask_avg)
	}
	// Temporal VBR (but not for LFE)
	if st.lfe == 0 {
		var follow int16 = -QCONST16(10.0, DB_SHIFT)
		var frame_avg int32 = 0
		var offset int16
		if shortBlocks != 0 {
			offset = int16(HALF16(SHL16(LM, DB_SHIFT)))
		} else {
			offset = 0
		}
		for i = start; i < end; i++ {
			follow = MAX16(follow-QCONST16(1.0, DB_SHIFT), bandLogE[i]-offset)
			if C == 2 {
				follow = MAX16(follow, bandLogE[i+nbEBands]-offset)
			}
			frame_avg += int32(follow)
		}
		frame_avg /= int32(end - start)
		temporal_vbr = int16(SUB16(frame_avg, st.spec_avg))
		temporal_vbr = MIN16(QCONST16(3.0, DB_SHIFT), MAX16(-QCONST16(1.5, DB_SHIFT), temporal_vbr))
		st.spec_avg += int16(MULT16_16_Q15(QCONST16(.02, 15), temporal_vbr))
	}
	// for (i=0;i<21;i++)
	//    printf("%f ", bandLogE[i]);
	// printf("\n");

	if secondMdct == 0 {
		copy(bandLogE2[:C*nbEBands], bandLogE[:C*nbEBands])
	}

	// Last chance to catch any transient we might have missed in the
	// time-domain analysis
	if LM > 0 && enc.Tell()+3 <= int(total_bits) && isTransient == 0 && st.complexity >= 5 && st.lfe == 0 && hybrid == 0 {
		if patch_transient_decision(bandLogE, oldBandE, nbEBands, start, end, C) != 0 {
			isTransient = 1
			shortBlocks = M
			compute_mdcts(mode, shortBlocks, in, freq, C, CC, LM, st.upsample, st.arch)
			ComputeBandEnergies(mode, freq, bandE, effEnd, C, LM, st.arch)
			Amp2Log2(mode, effEnd, end, bandE, bandLogE, C)
			// Compensate for the scaling of short vs long mdcts
			for i = 0; i < C*nbEBands; i++ {
				bandLogE2[i] += int16(HALF16(SHL16(LM, DB_SHIFT)))
			}
			tf_estimate = QCONST16(.2, 14)
		}
	}

	if LM > 0 && enc.Tell()+3 <= int(total_bits) {
		enc.EncBitLogp(isTransient, 3)
	}

	X := make([]int16, C*N) // Interleaved normalised MDCTs

	// Band normalisation
	NormaliseBands(mode, freq, X, bandE, effEnd, C, M)

	tf_res := make([]int, nbEBands)
	// Disable variable tf resolution for hybrid and at very low bitrate
	if effectiveBytes >= 15*C && hybrid == 0 && st.complexity >= 2 && st.lfe == 0 {
		var lambda int
		lambda = IMAX(5, 1280/effectiveBytes+2)
		tf_select = tf_analysis(mode, effEnd, isTransient, tf_res, lambda, X, N, LM, tf_estimate, tf_chan)
		for i = effEnd; i < end; i++ {
			tf_res[i] = tf_res[effEnd-1]
		}
	} else if hybrid != 0 && weak_transient != 0 {
		// For weak transients, we rely on the fact that improving time resolution using
		// TF on a long window is imperfect and will not result in an energy collapse at
		// low bitrate.
		for i = 0; i < end; i++ {
			tf_res[i] = 1
		}
		tf_select = 0
	} else if hybrid != 0 && effectiveBytes < 15 {
		// For low bitrate hybrid, we force temporal resolution to 5 ms rather than 2.5 ms.
		for i = 0; i < end; i++ {
			tf_res[i] = 0
		}
		tf_select = isTransient
	} else {
		for i = 0; i < end; i++ {
			tf_res[i] = isTransient
		}
		tf_select = 0
	}

	error := make([]int16, C*nbEBands)
	c = 0
	for {
		for i = start; i < end; i++ {
			// When the energy is stable, slightly bias energy quantization towards
			// the previous error to make the gain more stable (a constant offset is
			// better than fluctuations).
			if ABS32(SUB32(bandLogE[i+c*nbEBands], oldBandE[i+c*nbEBands])) < int32(QCONST16(2.0, DB_SHIFT)) {
				bandLogE[i+c*nbEBands] = int16(int32(bandLogE[i+c*nbEBands]) - MULT16_16_Q15(energyError[i+c*nbEBands], QCONST16(0.25, 15)))
			}
		}
		c++
		if c >= C {
			break
		}
	}
	QuantCoarseEnergy(mode, start, end, effEnd, bandLogE,
		oldBandE, uint32(total_bits), error, enc,
		C, LM, nbAvailableBytes, st.force_intra,
		&st.delayedIntra, b2i(st.complexity >= 4), st.loss_rate, st.lfe)

	tf_encode(start, end, isTransient, tf_res, LM, tf_select, enc)

	if enc.Tell()+4 <= int(total_bits) {
		if st.lfe != 0 {
			st.tapset_decision = 0
			st.spread_decision = SPREAD_NORMAL
		} else if hybrid != 0 {
			if st.complexity == 0 {
				st.spread_decision = SPREAD_NONE
			} else if isTransient != 0 {
				st.spread_decision = SPREAD_NORMAL
			} else {
				st.spread_decision = SPREAD_AGGRESSIVE
			}
		} else if shortBlocks != 0 || st.complexity < 3 || nbAvailableBytes < 10*C {
			if st.complexity == 0 {
				st.spread_decision = SPREAD_NONE
			} else {
				st.spread_decision = SPREAD_NORMAL
			}
		} else {
			// Disable new spreading+tapset estimator until we can show it works
			// better than the old one. So far it seems like spreading_decision()
			// works best.
			{
				st.spread_decision = SpreadingDecision(mode, X,
					&st.tonal_average, st.spread_decision, &st.hf_average,
					&st.tapset_decision, b2i(pf_on != 0 && shortBlocks == 0), effEnd, C, M)
			}
			// printf("%d %d\n", st->tapset_decision, st->spread_decision);
			// printf("%f %d %f %d\n\n", st->analysis.tonality, st->spread_decision, st->analysis.tonality_slope, st->tapset_decision);
		}
		enc.EncIcdf(st.spread_decision, spread_icdf[:], 5)
	}

	offsets := make([]int, nbEBands)

	maxDepth = dynalloc_analysis(bandLogE, bandLogE2, nbEBands, start, end, C, offsets,
		st.lsb_depth, mode.logN, isTransient, st.vbr, st.constrained_vbr,
		eBands, LM, effectiveBytes, &tot_boost, st.lfe, surround_dynalloc, &st.analysis)
	// For LFE, everything interesting is in the first band
	if st.lfe != 0 {
		offsets[0] = IMIN(8, effectiveBytes/3)
	}
	cap := make([]int, nbEBands)
	init_caps(mode, cap, LM, C)

	dynalloc_logp = 6
	total_bits <<= BITRES
	total_boost = 0
	tell = int32(enc.TellFrac())
	for i = start; i < end; i++ {
		var width, quanta int
		var dynalloc_loop_logp int
		var boost int
		var j int
		width = C * int(eBands[i+1]-eBands[i]) << uint(LM)
		// quanta is 6 bits, but no more than 1 bit/sample
		// and no less than 1/8 bit/sample
		quanta = IMIN(width<<BITRES, IMAX(6<<BITRES, width))
		dynalloc_loop_logp = dynalloc_logp
		boost = 0
		for j = 0; tell+int32(dynalloc_loop_logp<<BITRES) < total_bits-total_boost &&
			boost < cap[i]; j++ {
			var flag int
			flag = b2i(j < offsets[i])
			enc.EncBitLogp(flag, uint(dynalloc_loop_logp))
			tell = int32(enc.TellFrac())
			if flag == 0 {
				break
			}
			boost += quanta
			total_boost += int32(quanta)
			dynalloc_loop_logp = 1
		}
		// Making dynalloc more likely
		if j != 0 {
			dynalloc_logp = IMAX(2, dynalloc_logp-1)
		}
		offsets[i] = boost
	}

	if C == 2 {
		// Always use MS for 2.5 ms frames until we can do a better analysis
		if LM != 0 {
			dual_stereo = stereo_analysis(mode, X, LM, N)
		}

		st.intensity = HysteresisDecision(int16(equiv_rate/1000),
			intensity_thresholds[:], intensity_histeresis[:], 21, st.intensity)
		st.intensity = IMIN(end, IMAX(start, st.intensity))
	}

	alloc_trim = 5
	if tell+(6<<BITRES) <= total_bits-total_boost {
		if start > 0 || st.lfe != 0 {
			st.stereo_saving = 0
			alloc_trim = 5
		} else {
			alloc_trim = alloc_trim_analysis(mode, X, bandLogE,
				end, LM, C, N, &st.analysis, &st.stereo_saving, tf_estimate,
				st.intensity, surround_trim, equiv_rate, st.arch)
		}
		enc.EncIcdf(alloc_trim, trim_icdf[:], 7)
		tell = int32(enc.TellFrac())
	}

	// Variable bitrate
	if vbr_rate > 0 {
		var alpha int16
		var delta int32
		// The target rate in 8th bits per frame
		var target, base_target int32
		var min_allowed int32
		var lm_diff int = mode.maxLM - LM

		// Don't attempt to use more than 510 kb/s, even for frames smaller than 20 ms.
		// The CELT allocator will just not be able to use more than that anyway.
		nbCompressedBytes = IMIN(nbCompressedBytes, 1275>>uint(3-LM))
		if hybrid == 0 {
			base_target = vbr_rate - int32((40*C+20)<<BITRES)
		} else {
			base_target = IMAX(0, vbr_rate-int32((9*C+4)<<BITRES))
		}

		if st.constrained_vbr != 0 {
			base_target += (st.vbr_offset >> uint(lm_diff))
		}

		if hybrid == 0 {
			target = compute_vbr(mode, &st.analysis, base_target, LM, equiv_rate,
				st.lastCodedBands, C, st.intensity, st.constrained_vbr,
				st.stereo_saving, int(tot_boost), tf_estimate, pitch_change, maxDepth,
				st.lfe, b2i(st.energy_mask != nil), surround_masking,
				temporal_vbr)
		} else {
			target = base_target
			// Tonal frames (offset<100) need more bits than noisy (offset>100) ones.
			if st.silk_info.Offset < 100 {
				target += 12 << BITRES >> uint(3-LM)
			}
			if st.silk_info.Offset > 100 {
				target -= 18 << BITRES >> uint(3-LM)
			}
			// Boosting bitrate on transients and vowels with significant temporal
			// spikes.
			target += int32(MULT16_16_Q14(int32(tf_estimate)-int32(QCONST16(.25, 14)), (50 << BITRES)))
			// If we have a strong transient, let's make sure it has enough bits to code
			// the first two bands, so that it can use folding rather than noise.
			if tf_estimate > QCONST16(.7, 14) {
				target = IMAX(target, 50<<BITRES)
			}
		}
		// The current offset is removed from the target and the space used
		// so far is added
		target = target + tell
		// In VBR mode the frame size must not be reduced so much that it would
		// result in the encoder running out of bits.
		// The margin of 2 bytes ensures that none of the bust-prevention logic
		// in the decoder will have triggered so far.
		min_allowed = ((tell + total_boost + (1 << (BITRES + 3)) - 1) >> (BITRES + 3)) + 2
		// Take into account the 37 bits we need to have left in the packet to
		// signal a redundant frame in hybrid mode. Creating a shorter packet would
		// create an entropy coder desync.
		if hybrid != 0 {
			min_allowed = IMAX(min_allowed, (tell0_frac+(37<<BITRES)+total_boost+(1<<(BITRES+3))-1)>>(BITRES+3))
		}

		nbAvailableBytes = int((target + (1 << (BITRES + 2))) >> (BITRES + 3))
		nbAvailableBytes = IMAX(int(min_allowed), nbAvailableBytes)
		nbAvailableBytes = IMIN(nbCompressedBytes, nbAvailableBytes)

		// By how much did we "miss" the target on that frame
		delta = target - vbr_rate

		target = int32(nbAvailableBytes) << (BITRES + 3)

		// If the frame is silent we don't adjust our drift, otherwise
		// the encoder will shoot to very high rates after hitting a
		// span of silence, but we do allow the bitres to refill.
		// This means that we'll undershoot our target in CVBR/VBR modes
		// on files with lots of silence.
		if silence != 0 {
			nbAvailableBytes = 2
			target = 2 * 8 << BITRES
			delta = 0
		}

		if st.vbr_count < 970 {
			st.vbr_count++
			alpha = int16(Rcp(SHL32(EXTEND32(st.vbr_count+20), 16)))
		} else {
			alpha = QCONST16(.001, 15)
		}
		// How many bits have we used in excess of what we're allowed
		if st.constrained_vbr != 0 {
			st.vbr_reservoir += target - vbr_rate
		}
		// printf ("%d\n", st->vbr_reservoir);

		// Compute the offset we need to apply in order to reach the target
		if st.constrained_vbr != 0 {
			st.vbr_drift += int32(MULT16_32_Q15(alpha, (delta*(1<<uint(lm_diff)))-st.vbr_offset-st.vbr_drift))
			st.vbr_offset = -st.vbr_drift
		}
		// printf ("%d\n", st->vbr_drift);

		if st.constrained_vbr != 0 && st.vbr_reservoir < 0 {
			// We're under the min value -- increase rate
			var adjust int = int((-st.vbr_reservoir) / (8 << BITRES))
			// Unless we're just coding silence
			if silence == 0 {
				nbAvailableBytes += adjust
			}
			st.vbr_reservoir = 0
			// printf ("+%d\n", adjust);
		}
		nbCompressedBytes = IMIN(nbCompressedBytes, nbAvailableBytes)
		// printf("%d\n", nbCompressedBytes*50*8);
		// This moves the raw bits to take into account the new compressed size
		enc.EncShrink(uint32(nbCompressedBytes))
	}

	// Bit allocation
	fine_quant := make([]int, nbEBands)
	pulses := make([]int, nbEBands)
	fine_priority := make([]int, nbEBands)

	// bits =           packet size                    - where we are - safety
	bits = ((int32(nbCompressedBytes) * 8) << BITRES) - int32(enc.TellFrac()) - 1
	if isTransient != 0 && LM >= 2 && bits >= int32((LM+2)<<BITRES) {
		anti_collapse_rsv = 1 << BITRES
	} else {
		anti_collapse_rsv = 0
	}
	bits -= int32(anti_collapse_rsv)
	signalBandwidth = end - 1
	if st.analysis.Valid != 0 {
		var min_bandwidth int
		if equiv_rate < int32(32000)*int32(C) {
			min_bandwidth = 13
		} else if equiv_rate < int32(48000)*int32(C) {
			min_bandwidth = 16
		} else if equiv_rate < int32(60000)*int32(C) {
			min_bandwidth = 18
		} else if equiv_rate < int32(80000)*int32(C) {
			min_bandwidth = 19
		} else {
			min_bandwidth = 20
		}
		signalBandwidth = IMAX(st.analysis.Bandwidth, min_bandwidth)
	}
	if st.lfe != 0 {
		signalBandwidth = 1
	}
	codedBands = ComputeAllocation(mode, start, end, offsets, cap,
		alloc_trim, &st.intensity, &dual_stereo, bits, &balance, pulses,
		fine_quant, fine_priority, C, LM, enc, 1, st.lastCodedBands, signalBandwidth)
	if st.lastCodedBands != 0 {
		st.lastCodedBands = IMIN(st.lastCodedBands+1, IMAX(st.lastCodedBands-1, codedBands))
	} else {
		st.lastCodedBands = codedBands
	}

	QuantFineEnergy(mode, start, end, oldBandE, error, fine_quant, enc, C)

	// Residual quantisation
	collapse_masks := make([]uint8, C*nbEBands)
	var Y []int16
	if C == 2 {
		Y = X[N:]
	}
	QuantAllBands(1, mode, start, end, X, Y, collapse_masks,
		bandE, pulses, shortBlocks, st.spread_decision,
		dual_stereo, st.intensity, tf_res, int32(nbCompressedBytes*(8<<BITRES)-anti_collapse_rsv),
		balance, enc, LM, codedBands, &st.rng, st.complexity, st.arch, st.disable_inv)

	if anti_collapse_rsv > 0 {
		anti_collapse_on = b2i(st.consec_transient < 2)
		enc.EncBits(uint32(anti_collapse_on), 1)
	}
	QuantEnergyFinalise(mode, start, end, oldBandE, error, fine_quant, fine_priority, nbCompressedBytes*8-enc.Tell(), enc, C)
	clear(energyError[:nbEBands*CC])
	c = 0
	for {
		for i = start; i < end; i++ {
			energyError[i+c*nbEBands] = MAX16(-QCONST16(0.5, 15), MIN16(QCONST16(0.5, 15), error[i+c*nbEBands]))
		}
		c++
		if c >= C {
			break
		}
	}

	if silence != 0 {
		for i = 0; i < C*nbEBands; i++ {
			oldBandE[i] = -QCONST16(28.0, DB_SHIFT)
		}
	}

	st.prefilter_period = pitch_index
	st.prefilter_gain = gain1
	st.prefilter_tapset = prefilter_tapset

	if CC == 2 && C == 1 {
		copy(oldBandE[nbEBands:nbEBands+nbEBands], oldBandE[:nbEBands])
	}

	if isTransient == 0 {
		copy(oldLogE2[:CC*nbEBands], oldLogE[:CC*nbEBands])
		copy(oldLogE[:CC*nbEBands], oldBandE[:CC*nbEBands])
	} else {
		for i = 0; i < CC*nbEBands; i++ {
			oldLogE[i] = MIN16(oldLogE[i], oldBandE[i])
		}
	}
	// In case start or end were to change
	c = 0
	for {
		for i = 0; i < start; i++ {
			oldBandE[c*nbEBands+i] = 0
			oldLogE[c*nbEBands+i] = -QCONST16(28.0, DB_SHIFT)
			oldLogE2[c*nbEBands+i] = -QCONST16(28.0, DB_SHIFT)
		}
		for i = end; i < nbEBands; i++ {
			oldBandE[c*nbEBands+i] = 0
			oldLogE[c*nbEBands+i] = -QCONST16(28.0, DB_SHIFT)
			oldLogE2[c*nbEBands+i] = -QCONST16(28.0, DB_SHIFT)
		}
		c++
		if c >= CC {
			break
		}
	}

	if isTransient != 0 || transient_got_disabled != 0 {
		st.consec_transient++
	} else {
		st.consec_transient = 0
	}
	st.rng = enc.rng

	// If there's any room left (can only happen for very high rates),
	// it's already filled with zeros
	enc.EncDone()

	if enc.GetError() != 0 {
		return OPUS_INTERNAL_ERROR
	}
	return nbCompressedBytes
}

// Ctl is the C opus_custom_encoder_ctl / celt_encoder_ctl. Value arguments
// are int32 (int is accepted), output arguments are *int32, *uint32 or
// **CELTMode; CELT_SET_ANALYSIS takes *AnalysisInfo and OPUS_SET_ENERGY_MASK
// takes []int16 (nil for NULL), CELT_SET_SILK_INFO takes *SILKInfo.
//
// C: int opus_custom_encoder_ctl(CELTEncoder * OPUS_RESTRICT st, int request, ...)
func (st *CELTEncoder) Ctl(request int, args ...any) int {
	switch request {
	case OPUS_SET_COMPLEXITY_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value < 0 || value > 10 {
			return OPUS_BAD_ARG
		}
		st.complexity = int(value)
	case CELT_SET_START_BAND_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value < 0 || int(value) >= st.mode.nbEBands {
			return OPUS_BAD_ARG
		}
		st.start = int(value)
	case CELT_SET_END_BAND_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value < 1 || int(value) > st.mode.nbEBands {
			return OPUS_BAD_ARG
		}
		st.end = int(value)
	case CELT_SET_PREDICTION_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value < 0 || value > 2 {
			return OPUS_BAD_ARG
		}
		st.disable_pf = b2i(value <= 1)
		st.force_intra = b2i(value == 0)
	case OPUS_SET_PACKET_LOSS_PERC_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value < 0 || value > 100 {
			return OPUS_BAD_ARG
		}
		st.loss_rate = int(value)
	case OPUS_SET_VBR_CONSTRAINT_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok {
			return OPUS_BAD_ARG
		}
		st.constrained_vbr = int(value)
	case OPUS_SET_VBR_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok {
			return OPUS_BAD_ARG
		}
		st.vbr = int(value)
	case OPUS_SET_BITRATE_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || (value <= 500 && value != OPUS_BITRATE_MAX) {
			return OPUS_BAD_ARG
		}
		value = IMIN(value, int32(260000*st.channels))
		st.bitrate = value
	case CELT_SET_CHANNELS_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value < 1 || value > 2 {
			return OPUS_BAD_ARG
		}
		st.stream_channels = int(value)
	case OPUS_SET_LSB_DEPTH_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value < 8 || value > 24 {
			return OPUS_BAD_ARG
		}
		st.lsb_depth = int(value)
	case OPUS_GET_LSB_DEPTH_REQUEST:
		value, ok := args[0].(*int32)
		if !ok {
			return OPUS_BAD_ARG
		}
		*value = int32(st.lsb_depth)
	case OPUS_SET_PHASE_INVERSION_DISABLED_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value < 0 || value > 1 {
			return OPUS_BAD_ARG
		}
		st.disable_inv = int(value)
	case OPUS_GET_PHASE_INVERSION_DISABLED_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = int32(st.disable_inv)
	case OPUS_RESET_STATE:
		var i int
		// OPUS_CLEAR from ENCODER_RESET_START (rng) to the end of the state.
		st.rng = 0
		st.spread_decision = 0
		st.delayedIntra = 0
		st.tonal_average = 0
		st.lastCodedBands = 0
		st.hf_average = 0
		st.tapset_decision = 0
		st.prefilter_period = 0
		st.prefilter_gain = 0
		st.prefilter_tapset = 0
		st.consec_transient = 0
		st.analysis = AnalysisInfo{}
		st.silk_info = SILKInfo{}
		st.preemph_memE = [2]int32{}
		st.preemph_memD = [2]int32{}
		st.vbr_reservoir = 0
		st.vbr_drift = 0
		st.vbr_offset = 0
		st.vbr_count = 0
		st.overlap_max = 0
		st.stereo_saving = 0
		st.intensity = 0
		st.energy_mask = nil
		st.spec_avg = 0
		clear(st.in_mem)
		clear(st.prefilter_mem)
		clear(st.oldBandE)
		clear(st.oldLogE)
		clear(st.oldLogE2)
		clear(st.energyError)
		for i = 0; i < st.channels*st.mode.nbEBands; i++ {
			st.oldLogE[i] = -QCONST16(28.0, DB_SHIFT)
			st.oldLogE2[i] = -QCONST16(28.0, DB_SHIFT)
		}
		st.vbr_offset = 0
		st.delayedIntra = 1
		st.spread_decision = SPREAD_NORMAL
		st.tonal_average = 256
		st.hf_average = 0
		st.tapset_decision = 0
	case CELT_SET_SIGNALLING_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok {
			return OPUS_BAD_ARG
		}
		st.signalling = int(value)
	case CELT_SET_ANALYSIS_REQUEST:
		info, ok := args[0].(*AnalysisInfo)
		if !ok {
			return OPUS_BAD_ARG
		}
		if info != nil {
			st.analysis = *info
		}
	case CELT_SET_SILK_INFO_REQUEST:
		info, ok := args[0].(*SILKInfo)
		if !ok {
			return OPUS_BAD_ARG
		}
		if info != nil {
			st.silk_info = *info
		}
	case CELT_GET_MODE_REQUEST:
		value, ok := args[0].(**CELTMode)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = st.mode
	case OPUS_GET_FINAL_RANGE_REQUEST:
		value, ok := args[0].(*uint32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = st.rng
	case OPUS_SET_LFE_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok {
			return OPUS_BAD_ARG
		}
		st.lfe = int(value)
	case OPUS_SET_ENERGY_MASK_REQUEST:
		if args[0] == nil {
			st.energy_mask = nil
		} else {
			value, ok := args[0].([]int16)
			if !ok {
				return OPUS_BAD_ARG
			}
			st.energy_mask = value
		}
	default:
		return OPUS_UNIMPLEMENTED
	}
	return OPUS_OK
}
