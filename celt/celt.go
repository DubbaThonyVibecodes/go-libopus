package celt

// C: celt/celt.h, celt/celt.c — the CELT encoder and decoder.

// PACKAGE_VERSION mirrors the upstream version this port tracks.
const PACKAGE_VERSION = "1.0.3"

var trim_icdf = [11]uint8{126, 124, 119, 109, 87, 41, 19, 9, 4, 2, 0}

// Probs: NONE: 21.875%, LIGHT: 6.25%, NORMAL: 65.625%, AGGRESSIVE: 6.25%
var spread_icdf = [4]uint8{25, 23, 2, 0}

var tapset_icdf = [3]uint8{2, 1, 0}

const (
	COMBFILTER_MAXPERIOD = 1024
	COMBFILTER_MINPERIOD = 15
)

// C: static int resampling_factor(opus_int32 rate)
func resampling_factor(rate int32) int {
	var ret int
	switch rate {
	case 48000:
		ret = 1
	case 24000:
		ret = 2
	case 16000:
		ret = 3
	case 12000:
		ret = 4
	case 8000:
		ret = 6
	default:
		celt_assert(false)
		ret = 0
	}
	return ret
}

// CELTEncoder is the C struct OpusCustomEncoder (CELTEncoder).
type CELTEncoder struct {
	mode            *CELTMode // Mode used by the encoder
	overlap         int
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

	preemph_memE [2]int32
	preemph_memD [2]int32

	// VBR-related parameters
	vbr_reservoir int32
	vbr_drift     int32
	vbr_offset    int32
	vbr_count     int32

	in_mem        []int32 // Size = channels*mode->overlap
	prefilter_mem []int32 // Size = channels*COMBFILTER_MAXPERIOD
	oldBandE      []int16 // Size = channels*mode->nbEBands
	oldLogE       []int16 // Size = channels*mode->nbEBands
	oldLogE2      []int16 // Size = channels*mode->nbEBands
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
	size := 200 + // sizeof(struct CELTEncoder) excluding in_mem[1], approximate
		channels*mode.overlap*4 + // celt_sig in_mem[channels*mode->overlap];
		channels*COMBFILTER_MAXPERIOD*4 + // celt_sig prefilter_mem[channels*COMBFILTER_MAXPERIOD];
		3*channels*mode.nbEBands*2 // opus_val16 oldBandE/oldLogE/oldLogE2[channels*mode->nbEBands];
	return size
}

// C: int celt_encoder_init(CELTEncoder *st, opus_int32 sampling_rate, int channels)
func (st *CELTEncoder) EncoderInit(sampling_rate int32, channels int) int {
	var ret int
	ret = st.CustomEncoderInit(CustomModeCreate(48000, 960, nil), channels)
	if ret != OPUS_OK {
		return ret
	}
	st.upsample = resampling_factor(sampling_rate)
	return OPUS_OK
}

// C: int opus_custom_encoder_init(CELTEncoder *st, const CELTMode *mode, int channels)
func (st *CELTEncoder) CustomEncoderInit(mode *CELTMode, channels int) int {
	if channels < 0 || channels > 2 {
		return OPUS_BAD_ARG
	}

	if st == nil || mode == nil {
		return OPUS_ALLOC_FAIL
	}

	*st = CELTEncoder{}

	st.mode = mode
	st.overlap = mode.overlap
	st.channels = channels
	st.stream_channels = channels

	st.in_mem = make([]int32, channels*mode.overlap)
	st.prefilter_mem = make([]int32, channels*COMBFILTER_MAXPERIOD)
	st.oldBandE = make([]int16, channels*mode.nbEBands)
	st.oldLogE = make([]int16, channels*mode.nbEBands)
	st.oldLogE2 = make([]int16, channels*mode.nbEBands)

	st.upsample = 1
	st.start = 0
	st.end = st.mode.effEBands
	st.signalling = 1

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

// C: static inline opus_val16 SIG2WORD16(celt_sig x)
func SIG2WORD16(x int32) int16 {
	x = PSHR32(x, SIG_SHIFT)
	x = MAX32(x, -32768)
	x = MIN32(x, 32767)
	return EXTRACT16(x)
}

// C: static int transient_analysis(const opus_val32 * OPUS_RESTRICT in, int len, int C, int overlap)
func transient_analysis(in []int32, length, C int, overlap int) int {
	var i int
	var mem0, mem1 int32 = 0, 0
	var is_transient int = 0
	var block, N int
	tmp := make([]int16, length)

	block = overlap / 2
	N = length / block
	bins := make([]int16, N)
	if C == 1 {
		for i = 0; i < length; i++ {
			tmp[i] = int16(SHR32(in[i], SIG_SHIFT))
		}
	} else {
		for i = 0; i < length; i++ {
			tmp[i] = int16(SHR32(ADD32(in[i], in[i+length]), SIG_SHIFT+1))
		}
	}

	// High-pass filter: (1 - 2*z^-1 + z^-2) / (1 - z^-1 + .5*z^-2)
	for i = 0; i < length; i++ {
		var x, y int32
		x = int32(tmp[i])
		y = ADD32(mem0, x)
		mem0 = mem1 + y - SHL32(x, 1)
		mem1 = x - SHR32(y, 1)
		tmp[i] = EXTRACT16(SHR32(y, 2))
	}
	// First few samples are bad because we don't propagate the memory
	for i = 0; i < 12; i++ {
		tmp[i] = 0
	}

	for i = 0; i < N; i++ {
		var j int
		var max_abs int16 = 0
		for j = 0; j < block; j++ {
			max_abs = MAX16(max_abs, ABS16(tmp[i*block+j]))
		}
		bins[i] = max_abs
	}
	for i = 0; i < N; i++ {
		var j int
		var conseq int = 0
		var t1, t2, t3 int16

		t1 = int16(MULT16_16_Q15(QCONST16(.15, 15), bins[i]))
		t2 = int16(MULT16_16_Q15(QCONST16(.4, 15), bins[i]))
		t3 = int16(MULT16_16_Q15(QCONST16(.15, 15), bins[i]))
		for j = 0; j < i; j++ {
			if bins[j] < t1 {
				conseq++
			}
			if bins[j] < t2 {
				conseq++
			} else {
				conseq = 0
			}
		}
		if conseq >= 3 {
			is_transient = 1
		}
		conseq = 0
		for j = i + 1; j < N; j++ {
			if bins[j] < t3 {
				conseq++
			} else {
				conseq = 0
			}
		}
		if conseq >= 7 {
			is_transient = 1
		}
	}
	return is_transient
}

// Apply window and compute the MDCT for all sub-frames and all channels in a frame.
//
// C: static void compute_mdcts(const CELTMode *mode, int shortBlocks, celt_sig * OPUS_RESTRICT in, celt_sig * OPUS_RESTRICT out, int C, int LM)
func compute_mdcts(mode *CELTMode, shortBlocks int, in []int32, out []int32, C, LM int) {
	if C == 1 && shortBlocks == 0 {
		overlap := OVERLAP(mode)
		MdctForward(&mode.mdct, in, out, mode.window, overlap, mode.maxLM-LM, 1)
	} else {
		overlap := OVERLAP(mode)
		N := mode.shortMdctSize << uint(LM)
		B := 1
		var b, c int
		if shortBlocks != 0 {
			N = mode.shortMdctSize
			B = shortBlocks
		}
		c = 0
		for {
			for b = 0; b < B; b++ {
				// Interleaving the sub-frames while doing the MDCTs
				shift := mode.maxLM - LM
				if shortBlocks != 0 {
					shift = mode.maxLM
				}
				MdctForward(&mode.mdct, in[c*(B*N+overlap)+b*N:], out[b+c*N*B:], mode.window, overlap, shift, B)
			}
			c++
			if c >= C {
				break
			}
		}
	}
}

// Compute the IMDCT and apply window for all sub-frames and all channels in a frame.
//
// C: static void compute_inv_mdcts(const CELTMode *mode, int shortBlocks, celt_sig *X,
//
//	celt_sig * OPUS_RESTRICT out_mem[], celt_sig * OPUS_RESTRICT overlap_mem[], int C, int LM)
func compute_inv_mdcts(mode *CELTMode, shortBlocks int, X []int32, out_mem [][]int32, overlap_mem [][]int32, C, LM int) {
	var c int
	N := mode.shortMdctSize << uint(LM)
	overlap := OVERLAP(mode)
	x := make([]int32, N+overlap)
	c = 0
	for {
		var j, b int
		N2 := N
		B := 1

		if shortBlocks != 0 {
			N2 = mode.shortMdctSize
			B = shortBlocks
		}
		// Prevents problems from the imdct doing the overlap-add
		clear(x[:overlap])

		for b = 0; b < B; b++ {
			// IMDCT on the interleaved the sub-frames
			shift := mode.maxLM - LM
			if shortBlocks != 0 {
				shift = mode.maxLM
			}
			MdctBackward(&mode.mdct, X[b+c*N2*B:], x, N2*b, mode.window, overlap, shift, B)
		}

		for j = 0; j < overlap; j++ {
			out_mem[c][j] = x[j] + overlap_mem[c][j]
		}
		for ; j < N; j++ {
			out_mem[c][j] = x[j]
		}
		for j = 0; j < overlap; j++ {
			overlap_mem[c][j] = x[N+j]
		}
		c++
		if c >= C {
			break
		}
	}
}

// C: static void deemphasis(celt_sig *in[], opus_val16 *pcm, int N, int C, int downsample, const opus_val16 *coef, celt_sig *mem)
func deemphasis(in [][]int32, pcm []int16, N, C, downsample int, coef []int16, mem []int32) {
	var c int
	var count int = 0
	c = 0
	for {
		var j int
		m := mem[c]
		x := in[c]
		y := c
		for j = 0; j < N; j++ {
			tmp := x[j] + m
			m = MULT16_32_Q15(coef[0], tmp) - MULT16_32_Q15(coef[1], x[j])
			tmp = SHL32(MULT16_32_Q15(coef[3], tmp), 2)
			// Technically the store could be moved outside of the if because
			// the stores we don't want will just be overwritten
			if count == 0 {
				pcm[y] = SCALEOUT(SIG2WORD16(tmp))
			}
			count++
			if count == downsample {
				y += C
				count = 0
			}
		}
		mem[c] = m
		c++
		if c >= C {
			break
		}
	}
}

var comb_gains = [3][3]int16{
	{QCONST16(0.3066406250, 15), QCONST16(0.2170410156, 15), QCONST16(0.1296386719, 15)},
	{QCONST16(0.4638671875, 15), QCONST16(0.2680664062, 15), QCONST16(0.0, 15)},
	{QCONST16(0.7998046875, 15), QCONST16(0.1000976562, 15), QCONST16(0.0, 15)},
}

// comb_filter applies the pitch pre/post-filter. y[yOff:] is the output and
// x[xOff:] the input; the input is read back to xOff-T-2, so the C pointer
// arithmetic is preserved by passing base slices with offsets.
//
// C: static void comb_filter(opus_val32 *y, opus_val32 *x, int T0, int T1, int N,
//
//	opus_val16 g0, opus_val16 g1, int tapset0, int tapset1,
//	const opus_val16 *window, int overlap)
func comb_filter(y []int32, yOff int, x []int32, xOff int, T0, T1, N int,
	g0, g1 int16, tapset0, tapset1 int,
	window []int16, overlap int) {
	var i int
	// printf ("%d %d %f %f\n", T0, T1, g0, g1);
	var g00, g01, g02, g10, g11, g12 int16
	gains := &comb_gains
	g00 = int16(MULT16_16_Q15(g0, gains[tapset0][0]))
	g01 = int16(MULT16_16_Q15(g0, gains[tapset0][1]))
	g02 = int16(MULT16_16_Q15(g0, gains[tapset0][2]))
	g10 = int16(MULT16_16_Q15(g1, gains[tapset1][0]))
	g11 = int16(MULT16_16_Q15(g1, gains[tapset1][1]))
	g12 = int16(MULT16_16_Q15(g1, gains[tapset1][2]))
	for i = 0; i < overlap; i++ {
		var f int16
		f = int16(MULT16_16_Q15(window[i], window[i]))
		y[yOff+i] = x[xOff+i] +
			MULT16_32_Q15(MULT16_16_Q15(int32(Q15ONE)-int32(f), g00), x[xOff+i-T0]) +
			MULT16_32_Q15(MULT16_16_Q15(int32(Q15ONE)-int32(f), g01), x[xOff+i-T0-1]) +
			MULT16_32_Q15(MULT16_16_Q15(int32(Q15ONE)-int32(f), g01), x[xOff+i-T0+1]) +
			MULT16_32_Q15(MULT16_16_Q15(int32(Q15ONE)-int32(f), g02), x[xOff+i-T0-2]) +
			MULT16_32_Q15(MULT16_16_Q15(int32(Q15ONE)-int32(f), g02), x[xOff+i-T0+2]) +
			MULT16_32_Q15(MULT16_16_Q15(f, g10), x[xOff+i-T1]) +
			MULT16_32_Q15(MULT16_16_Q15(f, g11), x[xOff+i-T1-1]) +
			MULT16_32_Q15(MULT16_16_Q15(f, g11), x[xOff+i-T1+1]) +
			MULT16_32_Q15(MULT16_16_Q15(f, g12), x[xOff+i-T1-2]) +
			MULT16_32_Q15(MULT16_16_Q15(f, g12), x[xOff+i-T1+2])
	}
	for i = overlap; i < N; i++ {
		y[yOff+i] = x[xOff+i] +
			MULT16_32_Q15(g10, x[xOff+i-T1]) +
			MULT16_32_Q15(g11, x[xOff+i-T1-1]) +
			MULT16_32_Q15(g11, x[xOff+i-T1+1]) +
			MULT16_32_Q15(g12, x[xOff+i-T1-2]) +
			MULT16_32_Q15(g12, x[xOff+i-T1+2])
	}
}

var tf_select_table = [4][8]int8{
	{0, -1, 0, -1, 0, -1, 0, -1},
	{0, -1, 0, -2, 1, 0, 1, -1},
	{0, -2, 0, -3, 2, 0, 1, -1},
	{0, -2, 0, -3, 3, 0, 1, -1},
}

// C: static opus_val32 l1_metric(const celt_norm *tmp, int N, int LM, int width)
func l1_metric(tmp []int16, N, LM, width int) int32 {
	var i, j int
	var sqrtM_1 = [4]int16{Q15ONE, QCONST16(.70710678, 15), QCONST16(0.5, 15), QCONST16(0.35355339, 15)}
	var L1 int32
	var bias int16
	L1 = 0
	for i = 0; i < 1<<uint(LM); i++ {
		var L2 int32 = 0
		for j = 0; j < N>>uint(LM); j++ {
			L2 = MAC16_16(L2, tmp[(j<<uint(LM))+i], tmp[(j<<uint(LM))+i])
		}
		L1 += Sqrt(L2)
	}
	L1 = MULT16_32_Q15(sqrtM_1[LM], L1)
	if width == 1 {
		bias = int16(int32(QCONST16(.12, 15)) * int32(LM))
	} else if width == 2 {
		bias = int16(int32(QCONST16(.05, 15)) * int32(LM))
	} else {
		bias = int16(int32(QCONST16(.02, 15)) * int32(LM))
	}
	L1 = MAC16_32_Q15(L1, bias, L1)
	return L1
}

// C: static int tf_analysis(const CELTMode *m, int len, int C, int isTransient,
//
//	int *tf_res, int nbCompressedBytes, celt_norm *X, int N0, int LM,
//	int start, int *tf_sum)
func tf_analysis(m *CELTMode, length, C, isTransient int,
	tf_res []int, nbCompressedBytes int, X []int16, N0, LM int,
	start int, tf_sum *int) int {
	var i int
	var cost0, cost1 int
	var lambda int
	var tf_select int = 0

	if nbCompressedBytes < 15*C || start != 0 {
		*tf_sum = 0
		for i = 0; i < length; i++ {
			tf_res[i] = isTransient
		}
		return 0
	}
	if nbCompressedBytes < 40 {
		lambda = 12
	} else if nbCompressedBytes < 60 {
		lambda = 6
	} else if nbCompressedBytes < 100 {
		lambda = 4
	} else {
		lambda = 3
	}

	metric := make([]int, length)
	tmp := make([]int16, int(m.eBands[length]-m.eBands[length-1])<<uint(LM))
	path0 := make([]int, length)
	path1 := make([]int, length)

	*tf_sum = 0
	for i = 0; i < length; i++ {
		var j, k, N int
		var L1, best_L1 int32
		var best_level int = 0
		N = int(m.eBands[i+1]-m.eBands[i]) << uint(LM)
		for j = 0; j < N; j++ {
			tmp[j] = X[j+(int(m.eBands[i])<<uint(LM))]
		}
		// Just add the right channel if we're in stereo
		if C == 2 {
			for j = 0; j < N; j++ {
				tmp[j] = ADD16(SHR16(tmp[j], 1), SHR16(X[N0+j+(int(m.eBands[i])<<uint(LM))], 1))
			}
		}
		lm := 0
		if isTransient != 0 {
			lm = LM
		}
		L1 = l1_metric(tmp, N, lm, N>>uint(LM))
		best_L1 = L1
		// printf ("%f ", L1);
		for k = 0; k < LM; k++ {
			var B int

			if isTransient != 0 {
				B = (LM - k - 1)
			} else {
				B = k + 1
			}

			if isTransient != 0 {
				Haar1(tmp, N>>uint(LM-k), 1<<uint(LM-k))
			} else {
				Haar1(tmp, N>>uint(k), 1<<uint(k))
			}

			L1 = l1_metric(tmp, N, B, N>>uint(LM))

			if L1 < best_L1 {
				best_L1 = L1
				best_level = k + 1
			}
		}
		// printf ("%d ", isTransient ? LM-best_level : best_level);
		if isTransient != 0 {
			metric[i] = best_level
		} else {
			metric[i] = -best_level
		}
		*tf_sum += metric[i]
	}
	// printf("\n");
	// NOTE: Future optimized implementations could detect extreme transients and set
	// tf_select = 1 but so far we have not found a reliable way of making this useful
	tf_select = 0

	cost0 = 0
	if isTransient != 0 {
		cost1 = 0
	} else {
		cost1 = lambda
	}
	// Viterbi forward pass
	for i = 1; i < length; i++ {
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
		cost0 = curr0 + ABS(metric[i]-int(tf_select_table[LM][4*isTransient+2*tf_select+0]))
		cost1 = curr1 + ABS(metric[i]-int(tf_select_table[LM][4*isTransient+2*tf_select+1]))
	}
	if cost0 < cost1 {
		tf_res[length-1] = 0
	} else {
		tf_res[length-1] = 1
	}
	// Viterbi backward pass to check the decisions
	for i = length - 2; i >= 0; i-- {
		if tf_res[i+1] == 1 {
			tf_res[i] = path1[i+1]
		} else {
			tf_res[i] = path0[i+1]
		}
	}
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
	// printf("%d %d ", isTransient, tf_select); for(i=0;i<end;i++)printf("%d ", tf_res[i]);printf("\n");
}

// C: static void tf_decode(int start, int end, int isTransient, int *tf_res, int LM, ec_dec *dec)
func tf_decode(start, end, isTransient int, tf_res []int, LM int, dec *EcDec) {
	var i, curr, tf_select int
	var tf_select_rsv int
	var tf_changed int
	var logp int
	var budget uint32
	var tell uint32

	budget = dec.storage * 8
	tell = uint32(dec.Tell())
	if isTransient != 0 {
		logp = 2
	} else {
		logp = 4
	}
	tf_select_rsv = b2i(LM > 0 && tell+uint32(logp)+1 <= budget)
	budget -= uint32(tf_select_rsv)
	tf_changed = 0
	curr = 0
	for i = start; i < end; i++ {
		if tell+uint32(logp) <= budget {
			curr ^= dec.DecBitLogp(uint(logp))
			tell = uint32(dec.Tell())
			tf_changed |= curr
		}
		tf_res[i] = curr
		if isTransient != 0 {
			logp = 4
		} else {
			logp = 5
		}
	}
	tf_select = 0
	if tf_select_rsv != 0 &&
		tf_select_table[LM][4*isTransient+0+tf_changed] !=
			tf_select_table[LM][4*isTransient+2+tf_changed] {
		tf_select = dec.DecBitLogp(1)
	}
	for i = start; i < end; i++ {
		tf_res[i] = int(tf_select_table[LM][4*isTransient+2*tf_select+tf_res[i]])
	}
}

// C: static void init_caps(const CELTMode *m,int *cap,int LM,int C)
func init_caps(m *CELTMode, cap []int, LM, C int) {
	var i int
	for i = 0; i < m.nbEBands; i++ {
		var N int
		N = int(m.eBands[i+1]-m.eBands[i]) << uint(LM)
		cap[i] = (int(m.cache.caps[m.nbEBands*(2*LM+C-1)+i]) + 64) * C * N >> 2
	}
}

// C: static int alloc_trim_analysis(const CELTMode *m, const celt_norm *X,
//
//	const opus_val16 *bandLogE, int end, int LM, int C, int N0)
func alloc_trim_analysis(m *CELTMode, X []int16, bandLogE []int16, end, LM, C, N0 int) int {
	var i int
	var diff int32 = 0
	var c int
	var trim_index int = 5
	if C == 2 {
		var sum int16 = 0 // Q10
		// Compute inter-channel correlation for low frequencies
		for i = 0; i < 8; i++ {
			var j int
			var partial int32 = 0
			for j = int(m.eBands[i]) << uint(LM); j < int(m.eBands[i+1])<<uint(LM); j++ {
				partial = MAC16_16(partial, X[j], X[N0+j])
			}
			sum = ADD16(sum, EXTRACT16(SHR32(partial, 18)))
		}
		sum = int16(MULT16_16_Q15(QCONST16(1.0/8, 15), sum))
		// printf ("%f\n", sum);
		if sum > QCONST16(.995, 10) {
			trim_index -= 4
		} else if sum > QCONST16(.92, 10) {
			trim_index -= 3
		} else if sum > QCONST16(.85, 10) {
			trim_index -= 2
		} else if sum > QCONST16(.8, 10) {
			trim_index -= 1
		}
	}

	// Estimate spectral tilt
	c = 0
	for {
		for i = 0; i < end-1; i++ {
			diff += int32(bandLogE[i+c*m.nbEBands]) * int32(2+2*i-m.nbEBands)
		}
		c++
		if c >= C {
			break
		}
	}
	// We divide by two here to avoid making the tilt larger for stereo as a
	// result of a bug in the loop above
	diff /= int32(2 * C * (end - 1))
	// printf("%f\n", diff);
	if diff > int32(QCONST16(2.0, DB_SHIFT)) {
		trim_index--
	}
	if diff > int32(QCONST16(8.0, DB_SHIFT)) {
		trim_index--
	}
	if diff < -int32(QCONST16(4.0, DB_SHIFT)) {
		trim_index++
	}
	if diff < -int32(QCONST16(10.0, DB_SHIFT)) {
		trim_index++
	}

	if trim_index < 0 {
		trim_index = 0
	}
	if trim_index > 10 {
		trim_index = 10
	}
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

// EncodeWithEc encodes one frame. If enc is nil a range encoder is created
// on compressed. Returns the number of bytes written or a negative error.
//
// C: int celt_encode_with_ec(CELTEncoder * OPUS_RESTRICT st, const opus_val16 * pcm, int frame_size, unsigned char *compressed, int nbCompressedBytes, ec_enc *enc)
func (st *CELTEncoder) EncodeWithEc(pcm []int16, frame_size int, compressed []byte, nbCompressedBytes int, enc *EcEnc) int {
	var i, c, N int
	var bits int32
	var _enc EcEnc
	var prefilter_mem []int32
	var oldBandE, oldLogE, oldLogE2 []int16
	var shortBlocks int = 0
	var isTransient int = 0
	CC := st.channels
	C := st.stream_channels
	var LM, M int
	var tf_select int
	var nbFilledBytes, nbAvailableBytes int
	var effEnd int
	var codedBands int
	var tf_sum int
	var alloc_trim int
	var pitch_index int = COMBFILTER_MINPERIOD
	var gain1 int16 = 0
	var intensity int = 0
	var dual_stereo int = 0
	var effectiveBytes int
	var pf_threshold int16
	var dynalloc_logp int
	var vbr_rate int32
	var total_bits int32
	var total_boost int32
	var balance int32
	var tell int32
	var prefilter_tapset int = 0
	var pf_on int
	var anti_collapse_rsv int
	var anti_collapse_on int = 0
	var silence int = 0

	if nbCompressedBytes < 2 || pcm == nil {
		return OPUS_BAD_ARG
	}

	frame_size *= st.upsample
	for LM = 0; LM <= st.mode.maxLM; LM++ {
		if st.mode.shortMdctSize<<uint(LM) == frame_size {
			break
		}
	}
	if LM > st.mode.maxLM {
		return OPUS_BAD_ARG
	}
	M = 1 << uint(LM)
	N = M * st.mode.shortMdctSize

	prefilter_mem = st.prefilter_mem
	oldBandE = st.oldBandE
	oldLogE = st.oldLogE
	oldLogE2 = st.oldLogE2

	if enc == nil {
		tell = 1
		nbFilledBytes = 0
	} else {
		tell = int32(enc.Tell())
		nbFilledBytes = int(tell+4) >> 3
	}

	celt_assert(st.signalling == 0)

	// Can't produce more than 1275 output bytes
	nbCompressedBytes = IMIN(nbCompressedBytes, 1275)
	nbAvailableBytes = nbCompressedBytes - nbFilledBytes

	if st.vbr != 0 && st.bitrate != OPUS_BITRATE_MAX {
		var den int32 = st.mode.Fs >> BITRES
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
				int((tmp+4*st.mode.Fs)/(8*st.mode.Fs))-b2i(st.signalling != 0)))
		}
		effectiveBytes = nbCompressedBytes
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
			var lo int32
			if tell == 1 {
				lo = 2
			}
			max_allowed = IMIN(IMAX(lo,
				(vbr_rate+vbr_bound-st.vbr_reservoir)>>(BITRES+3)),
				int32(nbAvailableBytes))
			if int(max_allowed) < nbAvailableBytes {
				nbCompressedBytes = nbFilledBytes + int(max_allowed)
				nbAvailableBytes = int(max_allowed)
				enc.EncShrink(uint32(nbCompressedBytes))
			}
		}
	}
	total_bits = int32(nbCompressedBytes * 8)

	effEnd = st.end
	if effEnd > st.mode.effEBands {
		effEnd = st.mode.effEBands
	}

	in := make([]int32, CC*(N+st.overlap))

	// Find pitch period and gain
	{
		_pre := make([]int32, CC*(N+COMBFILTER_MAXPERIOD))
		var pre [2][]int32

		pre[0] = _pre
		pre[1] = _pre[(N + COMBFILTER_MAXPERIOD):]

		silence = 1
		c = 0
		for {
			var count int = 0
			pcmp := c
			inp := c*(N+st.overlap) + st.overlap

			for i = 0; i < N; i++ {
				var x, tmp int32

				x = int32(SCALEIN(pcm[pcmp]))
				count++
				if count == st.upsample {
					count = 0
					pcmp += CC
				} else {
					x = 0
				}
				// Apply pre-emphasis
				tmp = MULT16_16(st.mode.preemph[2], x)
				in[inp] = tmp + st.preemph_memE[c]
				st.preemph_memE[c] = MULT16_32_Q15(st.mode.preemph[1], in[inp]) -
					MULT16_32_Q15(st.mode.preemph[0], tmp)
				if in[inp] != 0 {
					silence = 0
				}
				inp++
			}
			copy(pre[c][:COMBFILTER_MAXPERIOD], prefilter_mem[c*COMBFILTER_MAXPERIOD:])
			copy(pre[c][COMBFILTER_MAXPERIOD:COMBFILTER_MAXPERIOD+N], in[c*(N+st.overlap)+st.overlap:])
			c++
			if c >= CC {
				break
			}
		}

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
		if nbAvailableBytes > 12*C && st.start == 0 && silence == 0 && st.disable_pf == 0 && st.complexity >= 5 {
			pitch_buf := make([]int16, (COMBFILTER_MAXPERIOD+N)>>1)

			PitchDownsample(pre[:], pitch_buf, COMBFILTER_MAXPERIOD+N, CC)
			PitchSearch(pitch_buf[(COMBFILTER_MAXPERIOD>>1):], pitch_buf, N,
				COMBFILTER_MAXPERIOD-COMBFILTER_MINPERIOD, &pitch_index)
			pitch_index = COMBFILTER_MAXPERIOD - pitch_index

			gain1 = RemoveDoubling(pitch_buf, COMBFILTER_MAXPERIOD, COMBFILTER_MINPERIOD,
				N, &pitch_index, st.prefilter_period, st.prefilter_gain)
			if pitch_index > COMBFILTER_MAXPERIOD-2 {
				pitch_index = COMBFILTER_MAXPERIOD - 2
			}
			gain1 = int16(MULT16_16_Q15(QCONST16(.7, 15), gain1))
			if st.loss_rate > 2 {
				gain1 = int16(HALF32(gain1))
			}
			if st.loss_rate > 4 {
				gain1 = int16(HALF32(gain1))
			}
			if st.loss_rate > 8 {
				gain1 = 0
			}
			prefilter_tapset = st.tapset_decision
		} else {
			gain1 = 0
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
			if st.start == 0 && tell+16 <= total_bits {
				enc.EncBitLogp(0, 1)
			}
			gain1 = 0
			pf_on = 0
		} else {
			// This block is not gated by a total bits check only because
			// of the nbAvailableBytes check above.
			var qg int
			var octave int

			if ABS16(int32(gain1)-int32(st.prefilter_gain)) < int32(QCONST16(.1, 15)) {
				gain1 = st.prefilter_gain
			}

			qg = ((int(gain1)+1536)>>10)/3 - 1
			qg = IMAX(0, IMIN(7, qg))
			enc.EncBitLogp(1, 1)
			pitch_index += 1
			octave = EC_ILOG(uint32(pitch_index)) - 5
			enc.EncUint(uint32(octave), 6)
			enc.EncBits(uint32(pitch_index-(16<<uint(octave))), uint(4+octave))
			pitch_index -= 1
			enc.EncBits(uint32(qg), 3)
			if enc.Tell()+2 <= int(total_bits) {
				enc.EncIcdf(prefilter_tapset, tapset_icdf[:], 2)
			} else {
				prefilter_tapset = 0
			}
			gain1 = int16(int32(QCONST16(0.09375, 15)) * int32(qg+1))
			pf_on = 1
		}
		// printf("%d %f\n", pitch_index, gain1);

		c = 0
		for {
			offset := st.mode.shortMdctSize - st.mode.overlap
			st.prefilter_period = IMAX(st.prefilter_period, COMBFILTER_MINPERIOD)
			copy(in[c*(N+st.overlap):c*(N+st.overlap)+st.overlap], st.in_mem[c*(st.overlap):])
			if offset != 0 {
				comb_filter(in, c*(N+st.overlap)+st.overlap, pre[c], COMBFILTER_MAXPERIOD,
					st.prefilter_period, st.prefilter_period, offset, -st.prefilter_gain, -st.prefilter_gain,
					st.prefilter_tapset, st.prefilter_tapset, nil, 0)
			}

			comb_filter(in, c*(N+st.overlap)+st.overlap+offset, pre[c], COMBFILTER_MAXPERIOD+offset,
				st.prefilter_period, pitch_index, N-offset, -st.prefilter_gain, -gain1,
				st.prefilter_tapset, prefilter_tapset, st.mode.window, st.mode.overlap)
			copy(st.in_mem[c*(st.overlap):(c+1)*st.overlap], in[c*(N+st.overlap)+N:])

			if N > COMBFILTER_MAXPERIOD {
				copy(prefilter_mem[c*COMBFILTER_MAXPERIOD:(c+1)*COMBFILTER_MAXPERIOD], pre[c][N:])
			} else {
				copy(prefilter_mem[c*COMBFILTER_MAXPERIOD:c*COMBFILTER_MAXPERIOD+COMBFILTER_MAXPERIOD-N], prefilter_mem[c*COMBFILTER_MAXPERIOD+N:])
				copy(prefilter_mem[c*COMBFILTER_MAXPERIOD+COMBFILTER_MAXPERIOD-N:(c+1)*COMBFILTER_MAXPERIOD], pre[c][COMBFILTER_MAXPERIOD:])
			}
			c++
			if c >= CC {
				break
			}
		}
	}

	isTransient = 0
	shortBlocks = 0
	if LM > 0 && enc.Tell()+3 <= int(total_bits) {
		if st.complexity > 1 {
			isTransient = transient_analysis(in, N+st.overlap, CC, st.overlap)
			if isTransient != 0 {
				shortBlocks = M
			}
		}
		enc.EncBitLogp(isTransient, 3)
	}

	freq := make([]int32, CC*N)                    // Interleaved signal MDCTs
	bandE := make([]int32, st.mode.nbEBands*CC)    // band energies
	bandLogE := make([]int16, st.mode.nbEBands*CC) // log band energies
	// Compute MDCTs
	compute_mdcts(st.mode, shortBlocks, in, freq, CC, LM)

	if CC == 2 && C == 1 {
		for i = 0; i < N; i++ {
			freq[i] = ADD32(HALF32(freq[i]), HALF32(freq[N+i]))
		}
	}
	if st.upsample != 1 {
		c = 0
		for {
			bound := N / st.upsample
			for i = 0; i < bound; i++ {
				freq[c*N+i] *= int32(st.upsample)
			}
			for ; i < N; i++ {
				freq[c*N+i] = 0
			}
			c++
			if c >= C {
				break
			}
		}
	}
	X := make([]int16, C*N) // Interleaved normalised MDCTs

	ComputeBandEnergies(st.mode, freq, bandE, effEnd, C, M)

	Amp2Log2(st.mode, effEnd, st.end, bandE, bandLogE, C)

	// Band normalisation
	NormaliseBands(st.mode, freq, X, bandE, effEnd, C, M)

	tf_res := make([]int, st.mode.nbEBands)
	tf_select = tf_analysis(st.mode, effEnd, C, isTransient, tf_res, effectiveBytes, X, N, LM, st.start, &tf_sum)
	for i = effEnd; i < st.end; i++ {
		tf_res[i] = tf_res[effEnd-1]
	}

	error := make([]int16, C*st.mode.nbEBands)
	QuantCoarseEnergy(st.mode, st.start, st.end, effEnd, bandLogE,
		oldBandE, uint32(total_bits), error, enc,
		C, LM, nbAvailableBytes, st.force_intra,
		&st.delayedIntra, b2i(st.complexity >= 4), st.loss_rate)

	tf_encode(st.start, st.end, isTransient, tf_res, LM, tf_select, enc)

	if enc.Tell()+4 <= int(total_bits) {
		if shortBlocks != 0 || st.complexity < 3 ||
			nbAvailableBytes < 10*C || st.start != 0 {
			if st.complexity == 0 {
				st.spread_decision = SPREAD_NONE
			} else {
				st.spread_decision = SPREAD_NORMAL
			}
		} else {
			st.spread_decision = SpreadingDecision(st.mode, X,
				&st.tonal_average, st.spread_decision, &st.hf_average,
				&st.tapset_decision, b2i(pf_on != 0 && shortBlocks == 0), effEnd, C, M)
		}
		enc.EncIcdf(st.spread_decision, spread_icdf[:], 5)
	}

	cap := make([]int, st.mode.nbEBands)
	offsets := make([]int, st.mode.nbEBands)

	init_caps(st.mode, cap, LM, C)
	for i = 0; i < st.mode.nbEBands; i++ {
		offsets[i] = 0
	}
	// Dynamic allocation code
	// Make sure that dynamic allocation can't make us bust the budget
	if effectiveBytes > 50 && LM >= 1 {
		var t1, t2 int
		if LM <= 1 {
			t1 = 3
			t2 = 5
		} else {
			t1 = 2
			t2 = 4
		}
		for i = st.start + 1; i < st.end-1; i++ {
			var d2 int32
			d2 = 2*int32(bandLogE[i]) - int32(bandLogE[i-1]) - int32(bandLogE[i+1])
			if C == 2 {
				d2 = HALF32(d2 + 2*int32(bandLogE[i+st.mode.nbEBands]) -
					int32(bandLogE[i-1+st.mode.nbEBands]) - int32(bandLogE[i+1+st.mode.nbEBands]))
			}
			if d2 > int32(SHL16(t1, DB_SHIFT)) {
				offsets[i] += 1
			}
			if d2 > int32(SHL16(t2, DB_SHIFT)) {
				offsets[i] += 1
			}
		}
	}
	dynalloc_logp = 6
	total_bits <<= BITRES
	total_boost = 0
	tell = int32(enc.TellFrac())
	for i = st.start; i < st.end; i++ {
		var width, quanta int
		var dynalloc_loop_logp int
		var boost int
		var j int
		width = C * int(st.mode.eBands[i+1]-st.mode.eBands[i]) << uint(LM)
		// quanta is 6 bits, but no more than 1 bit/sample
		// and no less than 1/8 bit/sample
		quanta = IMIN(width<<BITRES, IMAX(6<<BITRES, width))
		dynalloc_loop_logp = dynalloc_logp
		boost = 0
		for j = 0; tell+int32(dynalloc_loop_logp<<BITRES) < total_bits-total_boost && boost < cap[i]; j++ {
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
	alloc_trim = 5
	if tell+(6<<BITRES) <= total_bits-total_boost {
		alloc_trim = alloc_trim_analysis(st.mode, X, bandLogE, st.end, LM, C, N)
		enc.EncIcdf(alloc_trim, trim_icdf[:], 7)
		tell = int32(enc.TellFrac())
	}

	// Variable bitrate
	if vbr_rate > 0 {
		var alpha int16
		var delta int32
		// The target rate in 8th bits per frame
		var target int32
		var min_allowed int32
		lm_diff := st.mode.maxLM - LM

		// Don't attempt to use more than 510 kb/s, even for frames smaller than 20 ms.
		// The CELT allocator will just not be able to use more than that anyway.
		nbCompressedBytes = IMIN(nbCompressedBytes, 1275>>uint(3-LM))
		target = vbr_rate + (st.vbr_offset >> uint(lm_diff)) - int32((40*C+20)<<BITRES)

		// Shortblocks get a large boost in bitrate, but since they
		// are uncommon long blocks are not greatly affected
		if shortBlocks != 0 || tf_sum < -2*(st.end-st.start) {
			target = 7 * target / 4
		} else if tf_sum < -(st.end - st.start) {
			target = 3 * target / 2
		} else if M > 1 {
			target -= (target + 14) / 28
		}

		// The current offset is removed from the target and the space used
		// so far is added
		target = target + tell

		// In VBR mode the frame size must not be reduced so much that it would
		// result in the encoder running out of bits.
		// The margin of 2 bytes ensures that none of the bust-prevention logic
		// in the decoder will have triggered so far.
		min_allowed = ((tell + total_boost + (1 << (BITRES + 3)) - 1) >> (BITRES + 3)) + 2 - int32(nbFilledBytes)

		nbAvailableBytes = int((target + (1 << (BITRES + 2))) >> (BITRES + 3))
		nbAvailableBytes = IMAX(int(min_allowed), nbAvailableBytes)
		nbAvailableBytes = IMIN(nbCompressedBytes, nbAvailableBytes+nbFilledBytes) - nbFilledBytes

		// By how much did we "miss" the target on that frame
		delta = target - vbr_rate

		target = int32(nbAvailableBytes << (BITRES + 3))

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
		st.vbr_drift += MULT16_32_Q15(alpha, (delta*(1<<uint(lm_diff)))-st.vbr_offset-st.vbr_drift)
		st.vbr_offset = -st.vbr_drift
		// printf ("%d\n", st->vbr_drift);

		if st.constrained_vbr != 0 && st.vbr_reservoir < 0 {
			// We're under the min value -- increase rate
			adjust := int((-st.vbr_reservoir) / (8 << BITRES))
			// Unless we're just coding silence
			if silence == 0 {
				nbAvailableBytes += adjust
			}
			st.vbr_reservoir = 0
			// printf ("+%d\n", adjust);
		}
		nbCompressedBytes = IMIN(nbCompressedBytes, nbAvailableBytes+nbFilledBytes)
		// This moves the raw bits to take into account the new compressed size
		enc.EncShrink(uint32(nbCompressedBytes))
	}
	if C == 2 {
		var effectiveRate int

		// Always use MS for 2.5 ms frames until we can do a better analysis
		if LM != 0 {
			dual_stereo = stereo_analysis(st.mode, X, LM, N)
		}

		// Account for coarse energy
		effectiveRate = (8*effectiveBytes - 80) >> uint(LM)

		// effectiveRate in kb/s
		effectiveRate = 2 * effectiveRate / 5
		if effectiveRate < 35 {
			intensity = 8
		} else if effectiveRate < 50 {
			intensity = 12
		} else if effectiveRate < 68 {
			intensity = 16
		} else if effectiveRate < 84 {
			intensity = 18
		} else if effectiveRate < 102 {
			intensity = 19
		} else if effectiveRate < 130 {
			intensity = 20
		} else {
			intensity = 100
		}
		intensity = IMIN(st.end, IMAX(st.start, intensity))
	}

	// Bit allocation
	fine_quant := make([]int, st.mode.nbEBands)
	pulses := make([]int, st.mode.nbEBands)
	fine_priority := make([]int, st.mode.nbEBands)

	// bits =           packet size                    - where we are - safety
	bits = ((int32(nbCompressedBytes) * 8) << BITRES) - int32(enc.TellFrac()) - 1
	if isTransient != 0 && LM >= 2 && bits >= int32((LM+2)<<BITRES) {
		anti_collapse_rsv = 1 << BITRES
	} else {
		anti_collapse_rsv = 0
	}
	bits -= int32(anti_collapse_rsv)
	codedBands = ComputeAllocation(st.mode, st.start, st.end, offsets, cap,
		alloc_trim, &intensity, &dual_stereo, bits, &balance, pulses,
		fine_quant, fine_priority, C, LM, enc, 1, st.lastCodedBands)
	st.lastCodedBands = codedBands

	QuantFineEnergy(st.mode, st.start, st.end, oldBandE, error, fine_quant, enc, C)

	// Residual quantisation
	collapse_masks := make([]uint8, C*st.mode.nbEBands)
	var Y []int16
	if C == 2 {
		Y = X[N:]
	}
	QuantAllBands(1, st.mode, st.start, st.end, X, Y, collapse_masks,
		bandE, pulses, shortBlocks, st.spread_decision, dual_stereo, intensity, tf_res,
		int32(nbCompressedBytes*(8<<BITRES)-anti_collapse_rsv), balance, enc, LM, codedBands, &st.rng)

	if anti_collapse_rsv > 0 {
		anti_collapse_on = b2i(st.consec_transient < 2)
		enc.EncBits(uint32(anti_collapse_on), 1)
	}
	QuantEnergyFinalise(st.mode, st.start, st.end, oldBandE, error, fine_quant, fine_priority, nbCompressedBytes*8-enc.Tell(), enc, C)

	if silence != 0 {
		for i = 0; i < C*st.mode.nbEBands; i++ {
			oldBandE[i] = -QCONST16(28.0, DB_SHIFT)
		}
	}

	st.prefilter_period = pitch_index
	st.prefilter_gain = gain1
	st.prefilter_tapset = prefilter_tapset

	if CC == 2 && C == 1 {
		for i = 0; i < st.mode.nbEBands; i++ {
			oldBandE[st.mode.nbEBands+i] = oldBandE[i]
		}
	}

	if isTransient == 0 {
		for i = 0; i < CC*st.mode.nbEBands; i++ {
			oldLogE2[i] = oldLogE[i]
		}
		for i = 0; i < CC*st.mode.nbEBands; i++ {
			oldLogE[i] = oldBandE[i]
		}
	} else {
		for i = 0; i < CC*st.mode.nbEBands; i++ {
			oldLogE[i] = MIN16(oldLogE[i], oldBandE[i])
		}
	}
	// In case start or end were to change
	c = 0
	for {
		for i = 0; i < st.start; i++ {
			oldBandE[c*st.mode.nbEBands+i] = 0
			oldLogE[c*st.mode.nbEBands+i] = -QCONST16(28.0, DB_SHIFT)
			oldLogE2[c*st.mode.nbEBands+i] = -QCONST16(28.0, DB_SHIFT)
		}
		for i = st.end; i < st.mode.nbEBands; i++ {
			oldBandE[c*st.mode.nbEBands+i] = 0
			oldLogE[c*st.mode.nbEBands+i] = -QCONST16(28.0, DB_SHIFT)
			oldLogE2[c*st.mode.nbEBands+i] = -QCONST16(28.0, DB_SHIFT)
		}
		c++
		if c >= CC {
			break
		}
	}

	if isTransient != 0 {
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

// argInt32 extracts an opus_int32 value argument from a Ctl call.
func argInt32(args []any, i int) (int32, bool) {
	if i >= len(args) {
		return 0, false
	}
	switch v := args[i].(type) {
	case int32:
		return v, true
	case int:
		return int32(v), true
	case int64:
		return int32(v), true
	case uint32:
		return int32(v), true
	}
	return 0, false
}

// Ctl is the C opus_custom_encoder_ctl / celt_encoder_ctl. Value arguments
// are int32 (int is accepted), output arguments are *int32, *uint32 or **CELTMode.
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
		st.preemph_memE = [2]int32{}
		st.preemph_memD = [2]int32{}
		st.vbr_reservoir = 0
		st.vbr_drift = 0
		st.vbr_offset = 0
		st.vbr_count = 0
		clear(st.in_mem)
		clear(st.prefilter_mem)
		clear(st.oldBandE)
		clear(st.oldLogE)
		clear(st.oldLogE2)
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
	default:
		return OPUS_UNIMPLEMENTED
	}
	return OPUS_OK
}

/**********************************************************************/
/*                             DECODER                                */
/**********************************************************************/

const DECODE_BUFFER_SIZE = 2048

// CELTDecoder is the C struct OpusCustomDecoder (CELTDecoder).
type CELTDecoder struct {
	mode            *CELTMode
	overlap         int
	channels        int
	stream_channels int

	downsample int
	start, end int
	signalling int

	// Everything beyond this point gets cleared on a reset (DECODER_RESET_START)
	rng                   uint32
	error                 int
	last_pitch_index      int
	loss_count            int
	postfilter_period     int
	postfilter_period_old int
	postfilter_gain       int16
	postfilter_gain_old   int16
	postfilter_tapset     int
	postfilter_tapset_old int

	preemph_memD [2]int32

	_decode_mem    []int32 // Size = channels*(DECODE_BUFFER_SIZE+mode->overlap)
	lpc            []int16 // Size = channels*LPC_ORDER
	oldEBands      []int16 // Size = 2*mode->nbEBands
	oldLogE        []int16 // Size = 2*mode->nbEBands
	oldLogE2       []int16 // Size = 2*mode->nbEBands
	backgroundLogE []int16 // Size = 2*mode->nbEBands
}

// C: int celt_decoder_get_size(int channels)
func DecoderGetSize(channels int) int {
	mode := CustomModeCreate(48000, 960, nil)
	return CustomDecoderGetSize(mode, channels)
}

// C: int opus_custom_decoder_get_size(const CELTMode *mode, int channels)
func CustomDecoderGetSize(mode *CELTMode, channels int) int {
	size := 120 + // sizeof(struct CELTDecoder) excluding _decode_mem[1], approximate
		channels*(DECODE_BUFFER_SIZE+mode.overlap)*4 +
		channels*LPC_ORDER*2 +
		4*2*mode.nbEBands*2
	return size
}

// C: int celt_decoder_init(CELTDecoder *st, opus_int32 sampling_rate, int channels)
func (st *CELTDecoder) DecoderInit(sampling_rate int32, channels int) int {
	var ret int
	ret = st.CustomDecoderInit(CustomModeCreate(48000, 960, nil), channels)
	if ret != OPUS_OK {
		return ret
	}
	st.downsample = resampling_factor(sampling_rate)
	if st.downsample == 0 {
		return OPUS_BAD_ARG
	}
	return OPUS_OK
}

// C: int opus_custom_decoder_init(CELTDecoder *st, const CELTMode *mode, int channels)
func (st *CELTDecoder) CustomDecoderInit(mode *CELTMode, channels int) int {
	if channels < 0 || channels > 2 {
		return OPUS_BAD_ARG
	}

	if st == nil {
		return OPUS_ALLOC_FAIL
	}

	*st = CELTDecoder{}

	st.mode = mode
	st.overlap = mode.overlap
	st.channels = channels
	st.stream_channels = channels

	st._decode_mem = make([]int32, channels*(DECODE_BUFFER_SIZE+mode.overlap))
	st.lpc = make([]int16, channels*LPC_ORDER)
	st.oldEBands = make([]int16, 2*mode.nbEBands)
	st.oldLogE = make([]int16, 2*mode.nbEBands)
	st.oldLogE2 = make([]int16, 2*mode.nbEBands)
	st.backgroundLogE = make([]int16, 2*mode.nbEBands)

	st.downsample = 1
	st.start = 0
	st.end = st.mode.effEBands
	st.signalling = 1

	st.loss_count = 0

	st.Ctl(OPUS_RESET_STATE)

	return OPUS_OK
}

// C: static void celt_decode_lost(CELTDecoder * OPUS_RESTRICT st, opus_val16 * OPUS_RESTRICT pcm, int N, int LM)
func (st *CELTDecoder) decodeLost(pcm []int16, N, LM int) {
	var c int
	var pitch_index int
	var fade int16 = Q15ONE
	var i, length int
	C := st.channels
	var offset int
	var decode_mem [2][]int32
	var overlap_mem [2][]int32
	var out_syn [2][]int32
	// out_mem[c] == decode_mem[c][outMemOff:], but is indexed with negative
	// offsets in C so we keep the offset explicit.
	const outMemOff = DECODE_BUFFER_SIZE - MAX_PERIOD
	var lpc []int16
	var oldBandE, oldLogE, oldLogE2, backgroundLogE []int16
	var mode *CELTMode
	var nbEBands int
	var overlap int
	var eBands []int16

	mode = st.mode
	nbEBands = mode.nbEBands
	overlap = mode.overlap
	eBands = mode.eBands

	c = 0
	for {
		decode_mem[c] = st._decode_mem[c*(DECODE_BUFFER_SIZE+st.overlap):]
		overlap_mem[c] = decode_mem[c][DECODE_BUFFER_SIZE:]
		c++
		if c >= C {
			break
		}
	}
	lpc = st.lpc
	oldBandE = st.oldEBands
	oldLogE = st.oldLogE
	oldLogE2 = st.oldLogE2
	backgroundLogE = st.backgroundLogE
	_ = oldLogE
	_ = oldLogE2

	c = 0
	for {
		out_syn[c] = decode_mem[c][outMemOff+MAX_PERIOD-N:]
		c++
		if c >= C {
			break
		}
	}

	length = N + overlap

	if st.loss_count >= 5 || st.start != 0 {
		// Noise-based PLC/CNG
		var seed uint32
		var effEnd int

		effEnd = st.end
		if effEnd > mode.effEBands {
			effEnd = mode.effEBands
		}

		freq := make([]int32, C*N) // Interleaved signal MDCTs
		X := make([]int16, C*N)    // Interleaved normalised MDCTs
		bandE := make([]int32, nbEBands*C)

		if st.loss_count >= 5 {
			Log2Amp(mode, st.start, st.end, bandE, backgroundLogE, C)
		} else {
			// Energy decay
			var decay int16
			if st.loss_count == 0 {
				decay = QCONST16(1.5, DB_SHIFT)
			} else {
				decay = QCONST16(.5, DB_SHIFT)
			}
			c = 0
			for {
				for i = st.start; i < st.end; i++ {
					oldBandE[c*nbEBands+i] -= decay
				}
				c++
				if c >= C {
					break
				}
			}
			Log2Amp(mode, st.start, st.end, bandE, oldBandE, C)
		}
		seed = st.rng
		for c = 0; c < C; c++ {
			for i = 0; i < (int(st.mode.eBands[st.start]) << uint(LM)); i++ {
				X[c*N+i] = 0
			}
			for i = st.start; i < mode.effEBands; i++ {
				var j int
				var boffs int
				var blen int
				boffs = N*c + (int(eBands[i]) << uint(LM))
				blen = int(eBands[i+1]-eBands[i]) << uint(LM)
				for j = 0; j < blen; j++ {
					seed = LcgRand(seed)
					X[boffs+j] = int16(int32(seed) >> 20)
				}
				RenormaliseVector(X[boffs:], blen, Q15ONE)
			}
			for i = (int(st.mode.eBands[st.end]) << uint(LM)); i < N; i++ {
				X[c*N+i] = 0
			}
		}
		st.rng = seed

		DenormaliseBands(mode, X, freq, bandE, mode.effEBands, C, 1<<uint(LM))

		c = 0
		for {
			for i = 0; i < int(st.mode.eBands[st.start])<<uint(LM); i++ {
				freq[c*N+i] = 0
			}
			c++
			if c >= C {
				break
			}
		}
		c = 0
		for {
			bound := int(eBands[effEnd]) << uint(LM)
			if st.downsample != 1 {
				bound = IMIN(bound, N/st.downsample)
			}
			for i = bound; i < N; i++ {
				freq[c*N+i] = 0
			}
			c++
			if c >= C {
				break
			}
		}
		c = 0
		for {
			copy(decode_mem[c][:DECODE_BUFFER_SIZE-N+overlap], decode_mem[c][N:])
			c++
			if c >= C {
				break
			}
		}
		compute_inv_mdcts(mode, 0, freq, out_syn[:], overlap_mem[:], C, LM)
	} else {
		// Pitch-based PLC
		if st.loss_count == 0 {
			pitch_buf := make([]int16, DECODE_BUFFER_SIZE>>1)
			// Corresponds to a min pitch of 67 Hz. It's possible to save CPU in this
			// search by using only part of the decode buffer
			poffset := 720
			PitchDownsample(decode_mem[:], pitch_buf, DECODE_BUFFER_SIZE, C)
			// Max pitch is 100 samples (480 Hz)
			PitchSearch(pitch_buf[(poffset)>>1:], pitch_buf, DECODE_BUFFER_SIZE-poffset,
				poffset-100, &pitch_index)
			pitch_index = poffset - pitch_index
			st.last_pitch_index = pitch_index
		} else {
			pitch_index = st.last_pitch_index
			fade = QCONST16(.8, 15)
		}

		etmp := make([]int32, overlap)
		c = 0
		for {
			var exc [MAX_PERIOD]int16
			var ac [LPC_ORDER + 1]int32
			var decay int16
			var attenuation int16
			var S1 int32 = 0
			var mem [LPC_ORDER]int16
			e := out_syn[c]
			dm := decode_mem[c]
			om := outMemOff // index of out_mem[c][0] within dm

			offset = MAX_PERIOD - pitch_index
			for i = 0; i < MAX_PERIOD; i++ {
				exc[i] = ROUND16(dm[om+i], SIG_SHIFT)
			}

			// Compute LPC coefficients for the last MAX_PERIOD samples before the loss so we can
			// work in the excitation-filter domain
			if st.loss_count == 0 {
				Autocorr(exc[:], ac[:], mode.window, overlap, LPC_ORDER, MAX_PERIOD)

				// Noise floor -40 dB
				ac[0] += SHR32(ac[0], 13)
				// Lag windowing
				for i = 1; i <= LPC_ORDER; i++ {
					// ac[i] *= exp(-.5*(2*M_PI*.002*i)*(2*M_PI*.002*i));
					ac[i] -= MULT16_32_Q15(2*i*i, ac[i])
				}

				Lpc(lpc[c*LPC_ORDER:], ac[:], LPC_ORDER)
			}
			// Samples just before the beginning of exc
			for i = 0; i < LPC_ORDER; i++ {
				mem[i] = ROUND16(dm[om-1-i], SIG_SHIFT)
			}
			// Compute the excitation for MAX_PERIOD samples before the loss
			Fir(exc[:], lpc[c*LPC_ORDER:], exc[:], MAX_PERIOD, LPC_ORDER, mem[:])

			// Check if the waveform is decaying (and if so how fast)
			// We do this to avoid adding energy when concealing in a segment
			// with decaying energy
			{
				var E1, E2 int32 = 1, 1
				var period int
				var shift int

				if pitch_index <= MAX_PERIOD/2 {
					period = pitch_index
				} else {
					period = MAX_PERIOD / 2
				}
				shift = IMAX(0, 2*int(Zlog2(Maxabs16(exc[MAX_PERIOD-2*period:], 2*period)))-20)
				for i = 0; i < period; i++ {
					E1 += SHR32(MULT16_16(exc[MAX_PERIOD-period+i], exc[MAX_PERIOD-period+i]), shift)
					E2 += SHR32(MULT16_16(exc[MAX_PERIOD-2*period+i], exc[MAX_PERIOD-2*period+i]), shift)
				}
				if E1 > E2 {
					E1 = E2
				}
				decay = int16(Sqrt(FracDiv32(SHR32(E1, 1), E2)))
				attenuation = decay
			}

			// Move memory one frame to the left
			copy(dm[:DECODE_BUFFER_SIZE-N+overlap], dm[N:])

			// Extrapolate excitation with the right period, taking decay into account
			for i = 0; i < length; i++ {
				var tmp int16
				if offset+i >= MAX_PERIOD {
					offset -= pitch_index
					attenuation = int16(MULT16_16_Q15(attenuation, decay))
				}
				e[i] = SHL32(EXTEND32(MULT16_16_Q15(attenuation, exc[offset+i])), SIG_SHIFT)
				// Compute the energy of the previously decoded signal whose
				// excitation we're copying
				tmp = ROUND16(dm[om-N+offset+i], SIG_SHIFT)
				S1 += SHR32(MULT16_16(tmp, tmp), 8)
			}

			// Copy the last decoded samples (prior to the overlap region) to
			// synthesis filter memory so we can have a continuous signal.
			for i = 0; i < LPC_ORDER; i++ {
				mem[i] = ROUND16(dm[om+MAX_PERIOD-N-1-i], SIG_SHIFT)
			}
			// Apply the fading if not the first loss
			for i = 0; i < length; i++ {
				e[i] = MULT16_32_Q15(fade, e[i])
			}
			// Synthesis filter -- back in the signal domain
			Iir(e, lpc[c*LPC_ORDER:], e, length, LPC_ORDER, mem[:])

			// Check if the synthesis energy is higher than expected, which can
			// happen with the signal changes during our window. If so, attenuate.
			{
				var S2 int32 = 0
				for i = 0; i < length; i++ {
					var tmp int16 = ROUND16(e[i], SIG_SHIFT)
					S2 += SHR32(MULT16_16(tmp, tmp), 8)
				}
				// This checks for an "explosion" in the synthesis
				if !(S1 > SHR32(S2, 2)) {
					for i = 0; i < length; i++ {
						e[i] = 0
					}
				} else if S1 < S2 {
					var ratio int16 = int16(Sqrt(FracDiv32(SHR32(S1, 1)+1, S2+1)))
					for i = 0; i < overlap; i++ {
						var tmp_g int16 = int16(int32(Q15ONE) - MULT16_16_Q15(mode.window[i], int32(Q15ONE)-int32(ratio)))
						e[i] = MULT16_32_Q15(tmp_g, e[i])
					}
					for i = overlap; i < length; i++ {
						e[i] = MULT16_32_Q15(ratio, e[i])
					}
				}
			}

			// Apply pre-filter to the MDCT overlap for the next frame because the
			// post-filter will be re-applied in the decoder after the MDCT overlap
			comb_filter(etmp, 0, dm, om+MAX_PERIOD, st.postfilter_period, st.postfilter_period, st.overlap,
				-st.postfilter_gain, -st.postfilter_gain, st.postfilter_tapset, st.postfilter_tapset,
				nil, 0)

			// Simulate TDAC on the concealed audio so that it blends with the
			// MDCT of next frames.
			for i = 0; i < overlap/2; i++ {
				var tmp int32
				tmp = MULT16_32_Q15(mode.window[i], etmp[overlap-1-i]) +
					MULT16_32_Q15(mode.window[overlap-i-1], etmp[i])
				dm[om+MAX_PERIOD+i] = MULT16_32_Q15(mode.window[overlap-i-1], tmp)
				dm[om+MAX_PERIOD+overlap-i-1] = MULT16_32_Q15(mode.window[i], tmp)
			}
			c++
			if c >= C {
				break
			}
		}
	}

	deemphasis(out_syn[:], pcm, N, C, st.downsample, mode.preemph[:], st.preemph_memD[:])

	st.loss_count++
}

// DecodeWithEc decodes one frame. data==nil or len<=1 triggers packet loss
// concealment. Returns the number of samples per channel or a negative error.
//
// C: int celt_decode_with_ec(CELTDecoder * OPUS_RESTRICT st, const unsigned char *data, int len, opus_val16 * OPUS_RESTRICT pcm, int frame_size, ec_dec *dec)
func (st *CELTDecoder) DecodeWithEc(data []byte, length int, pcm []int16, frame_size int, dec *EcDec) int {
	var c, i, N int
	var spread_decision int
	var bits int32
	var _dec EcDec
	var decode_mem [2][]int32
	var overlap_mem [2][]int32
	var out_syn [2][]int32
	const outMemOff = DECODE_BUFFER_SIZE - MAX_PERIOD
	var oldBandE, oldLogE, oldLogE2, backgroundLogE []int16

	var shortBlocks int
	var isTransient int
	var intra_ener int
	CC := st.channels
	var LM, M int
	var effEnd int
	var codedBands int
	var alloc_trim int
	var postfilter_pitch int
	var postfilter_gain int16
	var intensity int = 0
	var dual_stereo int = 0
	var total_bits int32
	var balance int32
	var tell int32
	var dynalloc_logp int
	var postfilter_tapset int
	var anti_collapse_rsv int
	var anti_collapse_on int = 0
	var silence int
	C := st.stream_channels

	frame_size *= st.downsample

	c = 0
	for {
		decode_mem[c] = st._decode_mem[c*(DECODE_BUFFER_SIZE+st.overlap):]
		overlap_mem[c] = decode_mem[c][DECODE_BUFFER_SIZE:]
		c++
		if c >= CC {
			break
		}
	}
	oldBandE = st.oldEBands
	oldLogE = st.oldLogE
	oldLogE2 = st.oldLogE2
	backgroundLogE = st.backgroundLogE

	{
		for LM = 0; LM <= st.mode.maxLM; LM++ {
			if st.mode.shortMdctSize<<uint(LM) == frame_size {
				break
			}
		}
		if LM > st.mode.maxLM {
			return OPUS_BAD_ARG
		}
	}
	M = 1 << uint(LM)

	if length < 0 || length > 1275 || pcm == nil {
		return OPUS_BAD_ARG
	}

	N = M * st.mode.shortMdctSize

	effEnd = st.end
	if effEnd > st.mode.effEBands {
		effEnd = st.mode.effEBands
	}

	if data == nil || length <= 1 {
		st.decodeLost(pcm, N, LM)
		return frame_size / st.downsample
	}

	freq := make([]int32, IMAX(CC, C)*N) // Interleaved signal MDCTs
	X := make([]int16, C*N)              // Interleaved normalised MDCTs
	bandE := make([]int32, st.mode.nbEBands*C)
	c = 0
	for {
		for i = 0; i < M*int(st.mode.eBands[st.start]); i++ {
			X[c*N+i] = 0
		}
		c++
		if c >= C {
			break
		}
	}
	c = 0
	for {
		for i = M * int(st.mode.eBands[effEnd]); i < N; i++ {
			X[c*N+i] = 0
		}
		c++
		if c >= C {
			break
		}
	}

	if dec == nil {
		_dec.DecInit(data, uint32(length))
		dec = &_dec
	}

	if C == 1 {
		for i = 0; i < st.mode.nbEBands; i++ {
			oldBandE[i] = MAX16(oldBandE[i], oldBandE[st.mode.nbEBands+i])
		}
	}

	total_bits = int32(length * 8)
	tell = int32(dec.Tell())

	if tell >= total_bits {
		silence = 1
	} else if tell == 1 {
		silence = dec.DecBitLogp(15)
	} else {
		silence = 0
	}
	if silence != 0 {
		// Pretend we've read all the remaining bits
		tell = int32(length * 8)
		dec.nbits_total += int(tell) - dec.Tell()
	}

	postfilter_gain = 0
	postfilter_pitch = 0
	postfilter_tapset = 0
	if st.start == 0 && tell+16 <= total_bits {
		if dec.DecBitLogp(1) != 0 {
			var qg, octave int
			octave = int(dec.DecUint(6))
			postfilter_pitch = (16 << uint(octave)) + int(dec.DecBits(uint(4+octave))) - 1
			qg = int(dec.DecBits(3))
			if dec.Tell()+2 <= int(total_bits) {
				postfilter_tapset = dec.DecIcdf(tapset_icdf[:], 2)
			}
			postfilter_gain = int16(int32(QCONST16(.09375, 15)) * int32(qg+1))
		}
		tell = int32(dec.Tell())
	}

	if LM > 0 && tell+3 <= total_bits {
		isTransient = dec.DecBitLogp(3)
		tell = int32(dec.Tell())
	} else {
		isTransient = 0
	}

	if isTransient != 0 {
		shortBlocks = M
	} else {
		shortBlocks = 0
	}

	// Decode the global flags (first symbols in the stream)
	if tell+3 <= total_bits {
		intra_ener = dec.DecBitLogp(3)
	} else {
		intra_ener = 0
	}
	// Get band energies
	UnquantCoarseEnergy(st.mode, st.start, st.end, oldBandE,
		intra_ener, dec, C, LM)

	tf_res := make([]int, st.mode.nbEBands)
	tf_decode(st.start, st.end, isTransient, tf_res, LM, dec)

	tell = int32(dec.Tell())
	spread_decision = SPREAD_NORMAL
	if tell+4 <= total_bits {
		spread_decision = dec.DecIcdf(spread_icdf[:], 5)
	}

	pulses := make([]int, st.mode.nbEBands)
	cap := make([]int, st.mode.nbEBands)
	offsets := make([]int, st.mode.nbEBands)
	fine_priority := make([]int, st.mode.nbEBands)

	init_caps(st.mode, cap, LM, C)

	dynalloc_logp = 6
	total_bits <<= BITRES
	tell = int32(dec.TellFrac())
	for i = st.start; i < st.end; i++ {
		var width, quanta int
		var dynalloc_loop_logp int
		var boost int
		width = C * int(st.mode.eBands[i+1]-st.mode.eBands[i]) << uint(LM)
		// quanta is 6 bits, but no more than 1 bit/sample
		// and no less than 1/8 bit/sample
		quanta = IMIN(width<<BITRES, IMAX(6<<BITRES, width))
		dynalloc_loop_logp = dynalloc_logp
		boost = 0
		for tell+int32(dynalloc_loop_logp<<BITRES) < total_bits && boost < cap[i] {
			var flag int
			flag = dec.DecBitLogp(uint(dynalloc_loop_logp))
			tell = int32(dec.TellFrac())
			if flag == 0 {
				break
			}
			boost += quanta
			total_bits -= int32(quanta)
			dynalloc_loop_logp = 1
		}
		offsets[i] = boost
		// Making dynalloc more likely
		if boost > 0 {
			dynalloc_logp = IMAX(2, dynalloc_logp-1)
		}
	}

	fine_quant := make([]int, st.mode.nbEBands)
	if tell+(6<<BITRES) <= total_bits {
		alloc_trim = dec.DecIcdf(trim_icdf[:], 7)
	} else {
		alloc_trim = 5
	}

	bits = ((int32(length) * 8) << BITRES) - int32(dec.TellFrac()) - 1
	if isTransient != 0 && LM >= 2 && bits >= int32((LM+2)<<BITRES) {
		anti_collapse_rsv = 1 << BITRES
	} else {
		anti_collapse_rsv = 0
	}
	bits -= int32(anti_collapse_rsv)
	codedBands = ComputeAllocation(st.mode, st.start, st.end, offsets, cap,
		alloc_trim, &intensity, &dual_stereo, bits, &balance, pulses,
		fine_quant, fine_priority, C, LM, dec, 0, 0)

	UnquantFineEnergy(st.mode, st.start, st.end, oldBandE, fine_quant, dec, C)

	// Decode fixed codebook
	collapse_masks := make([]uint8, C*st.mode.nbEBands)
	var Y []int16
	if C == 2 {
		Y = X[N:]
	}
	QuantAllBands(0, st.mode, st.start, st.end, X, Y, collapse_masks,
		nil, pulses, shortBlocks, spread_decision, dual_stereo, intensity, tf_res,
		int32(length*(8<<BITRES)-anti_collapse_rsv), balance, dec, LM, codedBands, &st.rng)

	if anti_collapse_rsv > 0 {
		anti_collapse_on = int(dec.DecBits(1))
	}

	UnquantEnergyFinalise(st.mode, st.start, st.end, oldBandE,
		fine_quant, fine_priority, length*8-dec.Tell(), dec, C)

	if anti_collapse_on != 0 {
		AntiCollapse(st.mode, X, collapse_masks, LM, C, N,
			st.start, st.end, oldBandE, oldLogE, oldLogE2, pulses, st.rng)
	}

	Log2Amp(st.mode, st.start, st.end, bandE, oldBandE, C)

	if silence != 0 {
		for i = 0; i < C*st.mode.nbEBands; i++ {
			bandE[i] = 0
			oldBandE[i] = -QCONST16(28.0, DB_SHIFT)
		}
	}
	// Synthesis
	DenormaliseBands(st.mode, X, freq, bandE, effEnd, C, M)

	copy(decode_mem[0][:DECODE_BUFFER_SIZE-N], decode_mem[0][N:])
	if CC == 2 {
		copy(decode_mem[1][:DECODE_BUFFER_SIZE-N], decode_mem[1][N:])
	}

	c = 0
	for {
		for i = 0; i < M*int(st.mode.eBands[st.start]); i++ {
			freq[c*N+i] = 0
		}
		c++
		if c >= C {
			break
		}
	}
	c = 0
	for {
		bound := M * int(st.mode.eBands[effEnd])
		if st.downsample != 1 {
			bound = IMIN(bound, N/st.downsample)
		}
		for i = bound; i < N; i++ {
			freq[c*N+i] = 0
		}
		c++
		if c >= C {
			break
		}
	}

	out_syn[0] = decode_mem[0][outMemOff+MAX_PERIOD-N:]
	if CC == 2 {
		out_syn[1] = decode_mem[1][outMemOff+MAX_PERIOD-N:]
	}

	if CC == 2 && C == 1 {
		for i = 0; i < N; i++ {
			freq[N+i] = freq[i]
		}
	}
	if CC == 1 && C == 2 {
		for i = 0; i < N; i++ {
			freq[i] = HALF32(ADD32(freq[i], freq[N+i]))
		}
	}

	// Compute inverse MDCTs
	compute_inv_mdcts(st.mode, shortBlocks, freq, out_syn[:], overlap_mem[:], CC, LM)

	c = 0
	for {
		st.postfilter_period = IMAX(st.postfilter_period, COMBFILTER_MINPERIOD)
		st.postfilter_period_old = IMAX(st.postfilter_period_old, COMBFILTER_MINPERIOD)
		so := outMemOff + MAX_PERIOD - N // index of out_syn[c][0] in decode_mem[c]
		comb_filter(decode_mem[c], so, decode_mem[c], so, st.postfilter_period_old, st.postfilter_period, st.mode.shortMdctSize,
			st.postfilter_gain_old, st.postfilter_gain, st.postfilter_tapset_old, st.postfilter_tapset,
			st.mode.window, st.overlap)
		if LM != 0 {
			comb_filter(decode_mem[c], so+st.mode.shortMdctSize, decode_mem[c], so+st.mode.shortMdctSize, st.postfilter_period, postfilter_pitch, N-st.mode.shortMdctSize,
				st.postfilter_gain, postfilter_gain, st.postfilter_tapset, postfilter_tapset,
				st.mode.window, st.mode.overlap)
		}
		c++
		if c >= CC {
			break
		}
	}
	st.postfilter_period_old = st.postfilter_period
	st.postfilter_gain_old = st.postfilter_gain
	st.postfilter_tapset_old = st.postfilter_tapset
	st.postfilter_period = postfilter_pitch
	st.postfilter_gain = postfilter_gain
	st.postfilter_tapset = postfilter_tapset
	if LM != 0 {
		st.postfilter_period_old = st.postfilter_period
		st.postfilter_gain_old = st.postfilter_gain
		st.postfilter_tapset_old = st.postfilter_tapset
	}

	if C == 1 {
		for i = 0; i < st.mode.nbEBands; i++ {
			oldBandE[st.mode.nbEBands+i] = oldBandE[i]
		}
	}

	// In case start or end were to change
	if isTransient == 0 {
		for i = 0; i < 2*st.mode.nbEBands; i++ {
			oldLogE2[i] = oldLogE[i]
		}
		for i = 0; i < 2*st.mode.nbEBands; i++ {
			oldLogE[i] = oldBandE[i]
		}
		for i = 0; i < 2*st.mode.nbEBands; i++ {
			backgroundLogE[i] = MIN16(int16(int32(backgroundLogE[i])+int32(M)*int32(QCONST16(0.001, DB_SHIFT))), oldBandE[i])
		}
	} else {
		for i = 0; i < 2*st.mode.nbEBands; i++ {
			oldLogE[i] = MIN16(oldLogE[i], oldBandE[i])
		}
	}
	c = 0
	for {
		for i = 0; i < st.start; i++ {
			oldBandE[c*st.mode.nbEBands+i] = 0
			oldLogE[c*st.mode.nbEBands+i] = -QCONST16(28.0, DB_SHIFT)
			oldLogE2[c*st.mode.nbEBands+i] = -QCONST16(28.0, DB_SHIFT)
		}
		for i = st.end; i < st.mode.nbEBands; i++ {
			oldBandE[c*st.mode.nbEBands+i] = 0
			oldLogE[c*st.mode.nbEBands+i] = -QCONST16(28.0, DB_SHIFT)
			oldLogE2[c*st.mode.nbEBands+i] = -QCONST16(28.0, DB_SHIFT)
		}
		c++
		if c >= 2 {
			break
		}
	}
	st.rng = dec.rng

	deemphasis(out_syn[:], pcm, N, CC, st.downsample, st.mode.preemph[:], st.preemph_memD[:])
	st.loss_count = 0
	if dec.Tell() > 8*length {
		return OPUS_INTERNAL_ERROR
	}
	if dec.GetError() != 0 {
		st.error = 1
	}
	return frame_size / st.downsample
}

// Ctl is the C opus_custom_decoder_ctl / celt_decoder_ctl.
//
// C: int opus_custom_decoder_ctl(CELTDecoder * OPUS_RESTRICT st, int request, ...)
func (st *CELTDecoder) Ctl(request int, args ...any) int {
	switch request {
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
	case CELT_SET_CHANNELS_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value < 1 || value > 2 {
			return OPUS_BAD_ARG
		}
		st.stream_channels = int(value)
	case CELT_GET_AND_CLEAR_ERROR_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = int32(st.error)
		st.error = 0
	case OPUS_GET_LOOKAHEAD_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = int32(st.overlap / st.downsample)
	case OPUS_RESET_STATE:
		var i int
		// OPUS_CLEAR from DECODER_RESET_START (rng) to the end of the state.
		st.rng = 0
		st.error = 0
		st.last_pitch_index = 0
		st.loss_count = 0
		st.postfilter_period = 0
		st.postfilter_period_old = 0
		st.postfilter_gain = 0
		st.postfilter_gain_old = 0
		st.postfilter_tapset = 0
		st.postfilter_tapset_old = 0
		st.preemph_memD = [2]int32{}
		clear(st._decode_mem)
		clear(st.lpc)
		clear(st.oldEBands)
		clear(st.oldLogE)
		clear(st.oldLogE2)
		clear(st.backgroundLogE)
		for i = 0; i < 2*st.mode.nbEBands; i++ {
			st.oldLogE[i] = -QCONST16(28.0, DB_SHIFT)
			st.oldLogE2[i] = -QCONST16(28.0, DB_SHIFT)
		}
	case OPUS_GET_PITCH_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = int32(st.postfilter_period)
	case CELT_GET_MODE_REQUEST:
		value, ok := args[0].(**CELTMode)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = st.mode
	case CELT_SET_SIGNALLING_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok {
			return OPUS_BAD_ARG
		}
		st.signalling = int(value)
	case OPUS_GET_FINAL_RANGE_REQUEST:
		value, ok := args[0].(*uint32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = st.rng
	default:
		return OPUS_UNIMPLEMENTED
	}
	return OPUS_OK
}

var error_strings = [8]string{
	"success",
	"invalid argument",
	"buffer too small",
	"internal error",
	"corrupted stream",
	"request not implemented",
	"invalid state",
	"memory allocation failed",
}

// C: const char *opus_strerror(int error)
func Strerror(error int) string {
	if error > 0 || error < -7 {
		return "unknown error"
	}
	return error_strings[-error]
}

// C: const char *opus_get_version_string(void)
func GetVersionString() string {
	return "libopus " + PACKAGE_VERSION + "-fixed"
}
