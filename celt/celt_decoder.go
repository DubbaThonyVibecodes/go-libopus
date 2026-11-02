package celt

// C: celt/celt_decoder.c — the CELT decoder.

const DECODE_BUFFER_SIZE = 2048

// The maximum pitch lag to allow in the pitch-based PLC. It's possible to save
// CPU time in the PLC pitch search by making this smaller than MAX_PERIOD. The
// current value corresponds to a pitch of 66.67 Hz.
const PLC_PITCH_LAG_MAX = 720

// The minimum pitch lag to allow in the pitch-based PLC. This corresponds to a
// pitch of 480 Hz.
const PLC_PITCH_LAG_MIN = 100

const PLC_UPDATE_FRAMES = 4

// PLC_UPDATE_SAMPLES (PLC_UPDATE_FRAMES*FRAME_SIZE) is only used with
// ENABLE_DEEP_PLC; FRAME_SIZE comes from dnn/lpcnet.h, which is not ported.

// CELTDecoder is the C struct OpusCustomDecoder (CELTDecoder).
type CELTDecoder struct {
	mode            *CELTMode
	overlap         int
	channels        int
	stream_channels int

	downsample  int
	start, end  int
	signalling  int
	disable_inv int
	complexity  int
	arch        int

	// Everything beyond this point gets cleared on a reset (DECODER_RESET_START)
	rng                   uint32
	error                 int
	last_pitch_index      int
	loss_duration         int
	skip_plc              int
	postfilter_period     int
	postfilter_period_old int
	postfilter_gain       int16
	postfilter_gain_old   int16
	postfilter_tapset     int
	postfilter_tapset_old int
	prefilter_and_fold    int

	preemph_memD [2]int32

	_decode_mem    []int32 // Size = channels*(DECODE_BUFFER_SIZE+mode->overlap)
	lpc            []int16 // Size = channels*CELT_LPC_ORDER
	oldEBands      []int16 // Size = 2*mode->nbEBands
	oldLogE        []int16 // Size = 2*mode->nbEBands
	oldLogE2       []int16 // Size = 2*mode->nbEBands
	backgroundLogE []int16 // Size = 2*mode->nbEBands
}

// Make basic checks on the CELT state to ensure we don't end
// up writing all over memory.
// (ENABLE_HARDENING is defined, so this is compiled; it only asserts.)
//
// C: void validate_celt_decoder(CELTDecoder *st)
func validate_celt_decoder(st *CELTDecoder) {
	// CUSTOM_MODES is not defined.
	celt_assert(st.mode == CustomModeCreate(48000, 960, nil))
	celt_assert(st.overlap == 120)
	celt_assert(st.end <= 21)
	celt_assert(st.channels == 1 || st.channels == 2)
	celt_assert(st.stream_channels == 1 || st.stream_channels == 2)
	celt_assert(st.downsample > 0)
	celt_assert(st.start == 0 || st.start == 17)
	celt_assert(st.start < st.end)
	// OPUS_ARCHMASK is not defined.
	celt_assert(st.last_pitch_index <= PLC_PITCH_LAG_MAX)
	celt_assert(st.last_pitch_index >= PLC_PITCH_LAG_MIN || st.last_pitch_index == 0)
	celt_assert(st.postfilter_period < MAX_PERIOD)
	celt_assert(st.postfilter_period >= COMBFILTER_MINPERIOD || st.postfilter_period == 0)
	celt_assert(st.postfilter_period_old < MAX_PERIOD)
	celt_assert(st.postfilter_period_old >= COMBFILTER_MINPERIOD || st.postfilter_period_old == 0)
	celt_assert(st.postfilter_tapset <= 2)
	celt_assert(st.postfilter_tapset >= 0)
	celt_assert(st.postfilter_tapset_old <= 2)
	celt_assert(st.postfilter_tapset_old >= 0)
}

// C: #define VALIDATE_CELT_DECODER(st) validate_celt_decoder(st)
func VALIDATE_CELT_DECODER(st *CELTDecoder) { validate_celt_decoder(st) }

// C: int celt_decoder_get_size(int channels)
func DecoderGetSize(channels int) int {
	mode := CustomModeCreate(48000, 960, nil)
	return CustomDecoderGetSize(mode, channels)
}

// C: int opus_custom_decoder_get_size(const CELTMode *mode, int channels)
func CustomDecoderGetSize(mode *CELTMode, channels int) int {
	size := 128 + // sizeof(struct CELTDecoder) excluding _decode_mem[1], approximate
		channels*(DECODE_BUFFER_SIZE+mode.overlap)*4 +
		channels*CELT_LPC_ORDER*2 +
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
	st.lpc = make([]int16, channels*CELT_LPC_ORDER)
	st.oldEBands = make([]int16, 2*mode.nbEBands)
	st.oldLogE = make([]int16, 2*mode.nbEBands)
	st.oldLogE2 = make([]int16, 2*mode.nbEBands)
	st.backgroundLogE = make([]int16, 2*mode.nbEBands)

	st.downsample = 1
	st.start = 0
	st.end = st.mode.effEBands
	st.signalling = 1
	// DISABLE_UPDATE_DRAFT is not defined (RFC 8251 behaviour).
	st.disable_inv = b2i(channels == 1)
	st.arch = SelectArch()

	st.Ctl(OPUS_RESET_STATE)

	return OPUS_OK
}

// Special case for stereo with no downsampling and no accumulation. This is
// quite common and we can make it faster by processing both channels in the
// same loop, reducing overhead due to the dependency loop in the IIR filter.
//
// C: static void deemphasis_stereo_simple(celt_sig *in[], opus_val16 *pcm, int N, const opus_val16 coef0,
//
//	celt_sig *mem)
func deemphasis_stereo_simple(in [][]int32, pcm []int16, N int, coef0 int16,
	mem []int32) {
	var x0, x1 []int32
	var m0, m1 int32
	var j int
	x0 = in[0]
	x1 = in[1]
	m0 = mem[0]
	m1 = mem[1]
	for j = 0; j < N; j++ {
		var tmp0, tmp1 int32
		// Add VERY_SMALL to x[] first to reduce dependency chain.
		tmp0 = x0[j] + VERY_SMALL + m0
		tmp1 = x1[j] + VERY_SMALL + m1
		m0 = MULT16_32_Q15(coef0, tmp0)
		m1 = MULT16_32_Q15(coef0, tmp1)
		pcm[2*j] = SCALEOUT(SIG2WORD16(tmp0))
		pcm[2*j+1] = SCALEOUT(SIG2WORD16(tmp1))
	}
	mem[0] = m0
	mem[1] = m1
}

// C: static void deemphasis(celt_sig *in[], opus_val16 *pcm, int N, int C, int downsample, const opus_val16 *coef,
//
//	celt_sig *mem, int accum)
func deemphasis(in [][]int32, pcm []int16, N, C, downsample int, coef []int16,
	mem []int32, accum int) {
	var c int
	var Nd int
	var apply_downsampling int = 0
	var coef0 int16
	// Short version for common case.
	if downsample == 1 && C == 2 && accum == 0 {
		deemphasis_stereo_simple(in, pcm, N, coef[0], mem)
		return
	}
	scratch := make([]int32, N)
	coef0 = coef[0]
	Nd = N / downsample
	c = 0
	for {
		var j int
		var x []int32
		var y []int16
		var m int32 = mem[c]
		x = in[c]
		y = pcm[c:]
		if downsample > 1 {
			// Shortcut for the standard (non-custom modes) case
			for j = 0; j < N; j++ {
				var tmp int32 = x[j] + VERY_SMALL + m
				m = MULT16_32_Q15(coef0, tmp)
				scratch[j] = tmp
			}
			apply_downsampling = 1
		} else {
			// Shortcut for the standard (non-custom modes) case
			if accum != 0 {
				for j = 0; j < N; j++ {
					var tmp int32 = x[j] + VERY_SMALL + m
					m = MULT16_32_Q15(coef0, tmp)
					y[j*C] = SAT16(ADD32(y[j*C], SCALEOUT(SIG2WORD16(tmp))))
				}
			} else {
				for j = 0; j < N; j++ {
					var tmp int32 = x[j] + VERY_SMALL + m
					m = MULT16_32_Q15(coef0, tmp)
					y[j*C] = SCALEOUT(SIG2WORD16(tmp))
				}
			}
		}
		mem[c] = m

		if apply_downsampling != 0 {
			// Perform down-sampling
			if accum != 0 {
				for j = 0; j < Nd; j++ {
					y[j*C] = SAT16(ADD32(y[j*C], SCALEOUT(SIG2WORD16(scratch[j*downsample]))))
				}
			} else {
				for j = 0; j < Nd; j++ {
					y[j*C] = SCALEOUT(SIG2WORD16(scratch[j*downsample]))
				}
			}
		}
		c++
		if c >= C {
			break
		}
	}
}

// C: static void celt_synthesis(const CELTMode *mode, celt_norm *X, celt_sig * out_syn[],
//
//	opus_val16 *oldBandE, int start, int effEnd, int C, int CC,
//	int isTransient, int LM, int downsample,
//	int silence, int arch)
func celt_synthesis(mode *CELTMode, X []int16, out_syn [][]int32,
	oldBandE []int16, start, effEnd, C, CC int,
	isTransient, LM, downsample int,
	silence int, arch int) {
	var c, i int
	var M int
	var b int
	var B int
	var N, NB int
	var shift int
	var nbEBands int
	var overlap int

	overlap = mode.overlap
	nbEBands = mode.nbEBands
	N = mode.shortMdctSize << uint(LM)
	freq := make([]int32, N) // Interleaved signal MDCTs
	M = 1 << uint(LM)

	if isTransient != 0 {
		B = M
		NB = mode.shortMdctSize
		shift = mode.maxLM
	} else {
		B = 1
		NB = mode.shortMdctSize << uint(LM)
		shift = mode.maxLM - LM
	}

	if CC == 2 && C == 1 {
		// Copying a mono streams to two channels
		var freq2 []int32
		DenormaliseBands(mode, X, freq, oldBandE, start, effEnd, M,
			downsample, silence)
		// Store a temporary copy in the output buffer because the IMDCT destroys its input.
		freq2 = out_syn[1][overlap/2:]
		copy(freq2[:N], freq[:N])
		for b = 0; b < B; b++ {
			MdctBackward(&mode.mdct, freq2[b:], out_syn[0][NB*b:], mode.window, overlap, shift, B, arch)
		}
		for b = 0; b < B; b++ {
			MdctBackward(&mode.mdct, freq[b:], out_syn[1][NB*b:], mode.window, overlap, shift, B, arch)
		}
	} else if CC == 1 && C == 2 {
		// Downmixing a stereo stream to mono
		var freq2 []int32
		freq2 = out_syn[0][overlap/2:]
		DenormaliseBands(mode, X, freq, oldBandE, start, effEnd, M,
			downsample, silence)
		// Use the output buffer as temp array before downmixing.
		DenormaliseBands(mode, X[N:], freq2, oldBandE[nbEBands:], start, effEnd, M,
			downsample, silence)
		for i = 0; i < N; i++ {
			freq[i] = ADD32(HALF32(freq[i]), HALF32(freq2[i]))
		}
		for b = 0; b < B; b++ {
			MdctBackward(&mode.mdct, freq[b:], out_syn[0][NB*b:], mode.window, overlap, shift, B, arch)
		}
	} else {
		// Normal case (mono or stereo)
		c = 0
		for {
			DenormaliseBands(mode, X[c*N:], freq, oldBandE[c*nbEBands:], start, effEnd, M,
				downsample, silence)
			for b = 0; b < B; b++ {
				MdctBackward(&mode.mdct, freq[b:], out_syn[c][NB*b:], mode.window, overlap, shift, B, arch)
			}
			c++
			if c >= CC {
				break
			}
		}
	}
	// Saturate IMDCT output so that we can't overflow in the pitch postfilter
	// or in the
	c = 0
	for {
		for i = 0; i < N; i++ {
			out_syn[c][i] = SATURATE(out_syn[c][i], SIG_SAT)
		}
		c++
		if c >= CC {
			break
		}
	}
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

// C: static int celt_plc_pitch_search(celt_sig *decode_mem[2], int C, int arch)
func celt_plc_pitch_search(decode_mem [][]int32, C int, arch int) int {
	var pitch_index int
	lp_pitch_buf := make([]int16, DECODE_BUFFER_SIZE>>1)
	PitchDownsample(decode_mem, lp_pitch_buf,
		DECODE_BUFFER_SIZE, C, arch)
	PitchSearch(lp_pitch_buf[(PLC_PITCH_LAG_MAX>>1):], lp_pitch_buf,
		DECODE_BUFFER_SIZE-PLC_PITCH_LAG_MAX,
		PLC_PITCH_LAG_MAX-PLC_PITCH_LAG_MIN, &pitch_index, arch)
	pitch_index = PLC_PITCH_LAG_MAX - pitch_index
	return pitch_index
}

// prefilter_and_fold is a plain function (not a method) because the
// CELTDecoder field of the same name would clash with a method name.
//
// C: static void prefilter_and_fold(CELTDecoder * OPUS_RESTRICT st, int N)
func prefilter_and_fold(st *CELTDecoder, N int) {
	var c int
	var CC int
	var i int
	var overlap int
	var decode_mem [2][]int32
	var mode *CELTMode
	mode = st.mode
	overlap = st.overlap
	CC = st.channels
	etmp := make([]int32, overlap)
	c = 0
	for {
		decode_mem[c] = st._decode_mem[c*(DECODE_BUFFER_SIZE+overlap) : (c+1)*(DECODE_BUFFER_SIZE+overlap)]
		c++
		if c >= CC {
			break
		}
	}

	c = 0
	for {
		// Apply the pre-filter to the MDCT overlap for the next frame because
		// the post-filter will be re-applied in the decoder after the MDCT
		// overlap.
		comb_filter(etmp, 0, decode_mem[c], DECODE_BUFFER_SIZE-N,
			st.postfilter_period_old, st.postfilter_period, overlap,
			-st.postfilter_gain_old, -st.postfilter_gain,
			st.postfilter_tapset_old, st.postfilter_tapset, nil, 0, st.arch)

		// Simulate TDAC on the concealed audio so that it blends with the
		// MDCT of the next frame.
		for i = 0; i < overlap/2; i++ {
			decode_mem[c][DECODE_BUFFER_SIZE-N+i] =
				MULT16_32_Q15(mode.window[i], etmp[overlap-1-i]) +
					MULT16_32_Q15(mode.window[overlap-i-1], etmp[i])
		}
		c++
		if c >= CC {
			break
		}
	}
}

// ENABLE_DEEP_PLC is not defined: update_plc_state() and the LPCNetPLCState
// parameter of celt_decode_lost() are not ported.
//
// C: static void celt_decode_lost(CELTDecoder * OPUS_RESTRICT st, int N, int LM)
func (st *CELTDecoder) decodeLost(N, LM int) {
	var c int
	var i int
	C := st.channels
	var decode_mem [2][]int32
	var out_syn [2][]int32
	var lpc []int16
	var oldBandE, oldLogE, oldLogE2, backgroundLogE []int16
	var mode *CELTMode
	var nbEBands int
	var overlap int
	var start int
	var loss_duration int
	var noise_based int
	var eBands []int16

	mode = st.mode
	nbEBands = mode.nbEBands
	overlap = mode.overlap
	eBands = mode.eBands

	c = 0
	for {
		decode_mem[c] = st._decode_mem[c*(DECODE_BUFFER_SIZE+overlap) : (c+1)*(DECODE_BUFFER_SIZE+overlap)]
		out_syn[c] = decode_mem[c][DECODE_BUFFER_SIZE-N:]
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

	loss_duration = st.loss_duration
	start = st.start
	// ENABLE_DEEP_PLC is not defined.
	noise_based = b2i(loss_duration >= 40 || start != 0 || st.skip_plc != 0)
	if noise_based != 0 {
		// Noise-based PLC/CNG
		var seed uint32
		var end int
		var effEnd int
		var decay int16
		end = st.end
		effEnd = IMAX(start, IMIN(end, mode.effEBands))

		X := make([]int16, C*N) // Interleaved normalised MDCTs
		c = 0
		for {
			copy(decode_mem[c][:DECODE_BUFFER_SIZE-N+overlap], decode_mem[c][N:])
			c++
			if c >= C {
				break
			}
		}

		if st.prefilter_and_fold != 0 {
			prefilter_and_fold(st, N)
		}

		// Energy decay
		if loss_duration == 0 {
			decay = QCONST16(1.5, DB_SHIFT)
		} else {
			decay = QCONST16(.5, DB_SHIFT)
		}
		c = 0
		for {
			for i = start; i < end; i++ {
				oldBandE[c*nbEBands+i] = MAX16(backgroundLogE[c*nbEBands+i], oldBandE[c*nbEBands+i]-decay)
			}
			c++
			if c >= C {
				break
			}
		}
		seed = st.rng
		for c = 0; c < C; c++ {
			for i = start; i < effEnd; i++ {
				var j int
				var boffs int
				var blen int
				boffs = N*c + (int(eBands[i]) << uint(LM))
				blen = int(eBands[i+1]-eBands[i]) << uint(LM)
				for j = 0; j < blen; j++ {
					seed = LcgRand(seed)
					X[boffs+j] = int16(int32(seed) >> 20)
				}
				RenormaliseVector(X[boffs:], blen, Q15ONE, st.arch)
			}
		}
		st.rng = seed

		celt_synthesis(mode, X, out_syn[:], oldBandE, start, effEnd, C, C, 0, LM, st.downsample, 0, st.arch)
		st.prefilter_and_fold = 0
		// Skip regular PLC until we get two consecutive packets.
		st.skip_plc = 1
	} else {
		var exc_length int
		// Pitch-based PLC
		var window []int16
		var fade int16 = Q15ONE
		var pitch_index int

		if loss_duration == 0 {
			// ENABLE_DEEP_PLC is not defined: no update_plc_state().
			pitch_index = celt_plc_pitch_search(decode_mem[:], C, st.arch)
			st.last_pitch_index = pitch_index
		} else {
			pitch_index = st.last_pitch_index
			fade = QCONST16(.8, 15)
		}

		// We want the excitation for 2 pitch periods in order to look for a
		// decaying signal, but we can't get more than MAX_PERIOD.
		exc_length = IMIN(2*pitch_index, MAX_PERIOD)

		_exc := make([]int16, MAX_PERIOD+CELT_LPC_ORDER)
		fir_tmp := make([]int16, exc_length)
		exc := _exc[CELT_LPC_ORDER:]
		window = mode.window
		c = 0
		for {
			var decay int16
			var attenuation int16
			var S1 int32 = 0
			var buf []int32
			var extrapolation_offset int
			var extrapolation_len int
			var j int

			buf = decode_mem[c]
			// (C: exc[i-CELT_LPC_ORDER]; exc = _exc+CELT_LPC_ORDER.)
			for i = 0; i < MAX_PERIOD+CELT_LPC_ORDER; i++ {
				_exc[i] = SROUND16(buf[DECODE_BUFFER_SIZE-MAX_PERIOD-CELT_LPC_ORDER+i], SIG_SHIFT)
			}

			if loss_duration == 0 {
				var ac [CELT_LPC_ORDER + 1]int32
				// Compute LPC coefficients for the last MAX_PERIOD samples before
				// the first loss so we can work in the excitation-filter domain.
				Autocorr(exc, ac[:], window, overlap,
					CELT_LPC_ORDER, MAX_PERIOD, st.arch)
				// Add a noise floor of -40 dB.
				ac[0] += SHR32(ac[0], 13)
				// Use lag windowing to stabilize the Levinson-Durbin recursion.
				for i = 1; i <= CELT_LPC_ORDER; i++ {
					// ac[i] *= exp(-.5*(2*M_PI*.002*i)*(2*M_PI*.002*i));
					ac[i] -= MULT16_32_Q15(2*i*i, ac[i])
				}
				Lpc(lpc[c*CELT_LPC_ORDER:], ac[:], CELT_LPC_ORDER)
				// For fixed-point, apply bandwidth expansion until we can guarantee that
				// no overflow can happen in the IIR filter. This means:
				// 32768*sum(abs(filter)) < 2^31
				for {
					var tmp int16 = Q15ONE
					var sum int32 = int32(QCONST16(1., SIG_SHIFT))
					for i = 0; i < CELT_LPC_ORDER; i++ {
						sum += int32(ABS16(lpc[c*CELT_LPC_ORDER+i]))
					}
					if sum < 65535 {
						break
					}
					for i = 0; i < CELT_LPC_ORDER; i++ {
						tmp = int16(MULT16_16_Q15(QCONST16(.99, 15), tmp))
						lpc[c*CELT_LPC_ORDER+i] = int16(MULT16_16_Q15(lpc[c*CELT_LPC_ORDER+i], tmp))
					}
				}
			}
			// Initialize the LPC history with the samples just before the start
			// of the region for which we're computing the excitation.
			{
				// Compute the excitation for exc_length samples before the loss. We need the copy
				// because celt_fir() cannot filter in-place.
				// (Fir reads CELT_LPC_ORDER samples behind its input, so the input
				// slice starts at exc+MAX_PERIOD-exc_length-CELT_LPC_ORDER.)
				Fir(_exc[MAX_PERIOD-exc_length:], lpc[c*CELT_LPC_ORDER:],
					fir_tmp, exc_length, CELT_LPC_ORDER, st.arch)
				copy(exc[MAX_PERIOD-exc_length:MAX_PERIOD], fir_tmp[:exc_length])
			}

			// Check if the waveform is decaying, and if so how fast.
			// We do this to avoid adding energy when concealing in a segment
			// with decaying energy.
			{
				var E1, E2 int32 = 1, 1
				var decay_length int
				var shift int = IMAX(0, 2*int(Zlog2(Maxabs16(exc[MAX_PERIOD-exc_length:], exc_length)))-20)
				decay_length = exc_length >> 1
				for i = 0; i < decay_length; i++ {
					var e int16
					e = exc[MAX_PERIOD-decay_length+i]
					E1 += SHR32(MULT16_16(e, e), shift)
					e = exc[MAX_PERIOD-2*decay_length+i]
					E2 += SHR32(MULT16_16(e, e), shift)
				}
				E1 = MIN32(E1, E2)
				decay = int16(Sqrt(FracDiv32(SHR32(E1, 1), E2)))
			}

			// Move the decoder memory one frame to the left to give us room to
			// add the data for the new frame. We ignore the overlap that extends
			// past the end of the buffer, because we aren't going to use it.
			copy(buf[:DECODE_BUFFER_SIZE-N], buf[N:DECODE_BUFFER_SIZE])

			// Extrapolate from the end of the excitation with a period of
			// "pitch_index", scaling down each period by an additional factor of
			// "decay".
			extrapolation_offset = MAX_PERIOD - pitch_index
			// We need to extrapolate enough samples to cover a complete MDCT
			// window (including overlap/2 samples on both sides).
			extrapolation_len = N + overlap
			// We also apply fading if this is not the first loss.
			attenuation = int16(MULT16_16_Q15(fade, decay))
			i = 0
			j = 0
			for ; i < extrapolation_len; i, j = i+1, j+1 {
				var tmp int16
				if j >= pitch_index {
					j -= pitch_index
					attenuation = int16(MULT16_16_Q15(attenuation, decay))
				}
				buf[DECODE_BUFFER_SIZE-N+i] =
					SHL32(EXTEND32(MULT16_16_Q15(attenuation,
						exc[extrapolation_offset+j])), SIG_SHIFT)
				// Compute the energy of the previously decoded signal whose
				// excitation we're copying.
				tmp = SROUND16(
					buf[DECODE_BUFFER_SIZE-MAX_PERIOD-N+extrapolation_offset+j],
					SIG_SHIFT)
				S1 += SHR32(MULT16_16(tmp, tmp), 10)
			}
			{
				var lpc_mem [CELT_LPC_ORDER]int16
				// Copy the last decoded samples (prior to the overlap region) to
				// synthesis filter memory so we can have a continuous signal.
				for i = 0; i < CELT_LPC_ORDER; i++ {
					lpc_mem[i] = SROUND16(buf[DECODE_BUFFER_SIZE-N-1-i], SIG_SHIFT)
				}
				// Apply the synthesis filter to convert the excitation back into
				// the signal domain.
				Iir(buf[DECODE_BUFFER_SIZE-N:], lpc[c*CELT_LPC_ORDER:],
					buf[DECODE_BUFFER_SIZE-N:], extrapolation_len, CELT_LPC_ORDER,
					lpc_mem[:], st.arch)
				for i = 0; i < extrapolation_len; i++ {
					buf[DECODE_BUFFER_SIZE-N+i] = SATURATE(buf[DECODE_BUFFER_SIZE-N+i], SIG_SAT)
				}
			}

			// Check if the synthesis energy is higher than expected, which can
			// happen with the signal changes during our window. If so,
			// attenuate.
			{
				var S2 int32 = 0
				for i = 0; i < extrapolation_len; i++ {
					var tmp int16 = SROUND16(buf[DECODE_BUFFER_SIZE-N+i], SIG_SHIFT)
					S2 += SHR32(MULT16_16(tmp, tmp), 10)
				}
				// This checks for an "explosion" in the synthesis.
				if !(S1 > SHR32(S2, 2)) {
					for i = 0; i < extrapolation_len; i++ {
						buf[DECODE_BUFFER_SIZE-N+i] = 0
					}
				} else if S1 < S2 {
					var ratio int16 = int16(Sqrt(FracDiv32(SHR32(S1, 1)+1, S2+1)))
					for i = 0; i < overlap; i++ {
						var tmp_g int16 = int16(int32(Q15ONE) -
							MULT16_16_Q15(window[i], int32(Q15ONE)-int32(ratio)))
						buf[DECODE_BUFFER_SIZE-N+i] =
							MULT16_32_Q15(tmp_g, buf[DECODE_BUFFER_SIZE-N+i])
					}
					for i = overlap; i < extrapolation_len; i++ {
						buf[DECODE_BUFFER_SIZE-N+i] =
							MULT16_32_Q15(ratio, buf[DECODE_BUFFER_SIZE-N+i])
					}
				}
			}
			c++
			if c >= C {
				break
			}
		}

		// ENABLE_DEEP_PLC is not defined: the neural PLC block is not ported.
		st.prefilter_and_fold = 1
	}

	// Saturate to soemthing large to avoid wrap-around.
	st.loss_duration = IMIN(10000, loss_duration+(1<<uint(LM)))
}

// DecodeWithEcDred decodes one frame. data==nil or len<=1 triggers packet loss
// concealment. Returns the number of samples per channel or a negative error.
// (ENABLE_DEEP_PLC is not defined, so there is no LPCNetPLCState parameter.)
//
// C: int celt_decode_with_ec_dred(CELTDecoder * OPUS_RESTRICT st, const unsigned char *data,
//
//	int len, opus_val16 * OPUS_RESTRICT pcm, int frame_size, ec_dec *dec, int accum)
func (st *CELTDecoder) DecodeWithEcDred(data []byte, length int, pcm []int16, frame_size int, dec *EcDec, accum int) int {
	var c, i, N int
	var spread_decision int
	var bits int32
	var _dec EcDec
	var decode_mem [2][]int32
	var out_syn [2][]int32
	var lpc []int16
	var oldBandE, oldLogE, oldLogE2, backgroundLogE []int16

	var shortBlocks int
	var isTransient int
	var max_background_increase int16
	var intra_ener int
	CC := st.channels
	var LM, M int
	var start int
	var end int
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
	var mode *CELTMode
	var nbEBands int
	var overlap int
	var eBands []int16

	VALIDATE_CELT_DECODER(st)
	mode = st.mode
	nbEBands = mode.nbEBands
	overlap = mode.overlap
	eBands = mode.eBands
	start = st.start
	end = st.end
	frame_size *= st.downsample

	lpc = st.lpc
	oldBandE = st.oldEBands
	oldLogE = st.oldLogE
	oldLogE2 = st.oldLogE2
	backgroundLogE = st.backgroundLogE
	_ = lpc

	{
		for LM = 0; LM <= mode.maxLM; LM++ {
			if mode.shortMdctSize<<uint(LM) == frame_size {
				break
			}
		}
		if LM > mode.maxLM {
			return OPUS_BAD_ARG
		}
	}
	M = 1 << uint(LM)

	if length < 0 || length > 1275 || pcm == nil {
		return OPUS_BAD_ARG
	}

	N = M * mode.shortMdctSize
	c = 0
	for {
		decode_mem[c] = st._decode_mem[c*(DECODE_BUFFER_SIZE+overlap) : (c+1)*(DECODE_BUFFER_SIZE+overlap)]
		out_syn[c] = decode_mem[c][DECODE_BUFFER_SIZE-N:]
		c++
		if c >= CC {
			break
		}
	}

	effEnd = end
	if effEnd > mode.effEBands {
		effEnd = mode.effEBands
	}

	if data == nil || length <= 1 {
		st.decodeLost(N, LM)
		deemphasis(out_syn[:], pcm, N, CC, st.downsample, mode.preemph[:], st.preemph_memD[:], accum)
		return frame_size / st.downsample
	}
	// ENABLE_DEEP_PLC is not defined: no lpcnet->blend reset.

	// Check if there are at least two packets received consecutively before
	// turning on the pitch-based PLC
	if st.loss_duration == 0 {
		st.skip_plc = 0
	}

	if dec == nil {
		_dec.DecInit(data, uint32(length))
		dec = &_dec
	}

	if C == 1 {
		for i = 0; i < nbEBands; i++ {
			oldBandE[i] = MAX16(oldBandE[i], oldBandE[nbEBands+i])
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
	if start == 0 && tell+16 <= total_bits {
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
	// If recovering from packet loss, make sure we make the energy prediction safe to reduce the
	// risk of getting loud artifacts.
	if intra_ener == 0 && st.loss_duration != 0 {
		c = 0
		for {
			var safety int16 = 0
			var missing int = IMIN(10, st.loss_duration>>uint(LM))
			if LM == 0 {
				safety = QCONST16(1.5, DB_SHIFT)
			} else if LM == 1 {
				safety = QCONST16(.5, DB_SHIFT)
			}
			for i = start; i < end; i++ {
				if oldBandE[c*nbEBands+i] < MAX16(oldLogE[c*nbEBands+i], oldLogE2[c*nbEBands+i]) {
					// If energy is going down already, continue the trend.
					var slope int32
					var E0, E1, E2 int32
					E0 = int32(oldBandE[c*nbEBands+i])
					E1 = int32(oldLogE[c*nbEBands+i])
					E2 = int32(oldLogE2[c*nbEBands+i])
					slope = MAX32(E1-E0, HALF32(E2-E0))
					E0 -= MAX32(0, int32(1+missing)*slope)
					oldBandE[c*nbEBands+i] = int16(MAX32(-int32(QCONST16(20., DB_SHIFT)), E0))
				} else {
					// Otherwise take the min of the last frames.
					oldBandE[c*nbEBands+i] = MIN16(MIN16(oldBandE[c*nbEBands+i], oldLogE[c*nbEBands+i]), oldLogE2[c*nbEBands+i])
				}
				// Shorter frames have more natural fluctuations -- play it safe.
				oldBandE[c*nbEBands+i] -= safety
			}
			c++
			if c >= 2 {
				break
			}
		}
	}
	// Get band energies
	UnquantCoarseEnergy(mode, start, end, oldBandE,
		intra_ener, dec, C, LM)

	tf_res := make([]int, nbEBands)
	tf_decode(start, end, isTransient, tf_res, LM, dec)

	tell = int32(dec.Tell())
	spread_decision = SPREAD_NORMAL
	if tell+4 <= total_bits {
		spread_decision = dec.DecIcdf(spread_icdf[:], 5)
	}

	cap := make([]int, nbEBands)

	init_caps(mode, cap, LM, C)

	offsets := make([]int, nbEBands)

	dynalloc_logp = 6
	total_bits <<= BITRES
	tell = int32(dec.TellFrac())
	for i = start; i < end; i++ {
		var width, quanta int
		var dynalloc_loop_logp int
		var boost int
		width = C * int(eBands[i+1]-eBands[i]) << uint(LM)
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

	fine_quant := make([]int, nbEBands)
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

	pulses := make([]int, nbEBands)
	fine_priority := make([]int, nbEBands)

	codedBands = CltComputeAllocation(mode, start, end, offsets, cap,
		alloc_trim, &intensity, &dual_stereo, bits, &balance, pulses,
		fine_quant, fine_priority, C, LM, dec, 0, 0, 0)

	UnquantFineEnergy(mode, start, end, oldBandE, fine_quant, dec, C)

	c = 0
	for {
		copy(decode_mem[c][:DECODE_BUFFER_SIZE-N+overlap], decode_mem[c][N:])
		c++
		if c >= CC {
			break
		}
	}

	// Decode fixed codebook
	collapse_masks := make([]uint8, C*nbEBands)

	X := make([]int16, C*N) // Interleaved normalised MDCTs

	var Y []int16
	if C == 2 {
		Y = X[N:]
	}
	QuantAllBands(0, mode, start, end, X, Y, collapse_masks,
		nil, pulses, shortBlocks, spread_decision, dual_stereo, intensity, tf_res,
		int32(length*(8<<BITRES)-anti_collapse_rsv), balance, dec, LM, codedBands, &st.rng, 0,
		st.arch, st.disable_inv)

	if anti_collapse_rsv > 0 {
		anti_collapse_on = int(dec.DecBits(1))
	}

	UnquantEnergyFinalise(mode, start, end, oldBandE,
		fine_quant, fine_priority, length*8-dec.Tell(), dec, C)

	if anti_collapse_on != 0 {
		AntiCollapse(mode, X, collapse_masks, LM, C, N,
			start, end, oldBandE, oldLogE, oldLogE2, pulses, st.rng, st.arch)
	}

	if silence != 0 {
		for i = 0; i < C*nbEBands; i++ {
			oldBandE[i] = -QCONST16(28.0, DB_SHIFT)
		}
	}
	if st.prefilter_and_fold != 0 {
		prefilter_and_fold(st, N)
	}
	celt_synthesis(mode, X, out_syn[:], oldBandE, start, effEnd,
		C, CC, isTransient, LM, st.downsample, silence, st.arch)

	c = 0
	for {
		st.postfilter_period = IMAX(st.postfilter_period, COMBFILTER_MINPERIOD)
		st.postfilter_period_old = IMAX(st.postfilter_period_old, COMBFILTER_MINPERIOD)
		so := DECODE_BUFFER_SIZE - N // index of out_syn[c][0] in decode_mem[c]
		comb_filter(decode_mem[c], so, decode_mem[c], so, st.postfilter_period_old, st.postfilter_period, mode.shortMdctSize,
			st.postfilter_gain_old, st.postfilter_gain, st.postfilter_tapset_old, st.postfilter_tapset,
			mode.window, overlap, st.arch)
		if LM != 0 {
			comb_filter(decode_mem[c], so+mode.shortMdctSize, decode_mem[c], so+mode.shortMdctSize, st.postfilter_period, postfilter_pitch, N-mode.shortMdctSize,
				st.postfilter_gain, postfilter_gain, st.postfilter_tapset, postfilter_tapset,
				mode.window, overlap, st.arch)
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
		copy(oldBandE[nbEBands:nbEBands+nbEBands], oldBandE[:nbEBands])
	}

	if isTransient == 0 {
		copy(oldLogE2[:2*nbEBands], oldLogE[:2*nbEBands])
		copy(oldLogE[:2*nbEBands], oldBandE[:2*nbEBands])
	} else {
		for i = 0; i < 2*nbEBands; i++ {
			oldLogE[i] = MIN16(oldLogE[i], oldBandE[i])
		}
	}
	// In normal circumstances, we only allow the noise floor to increase by
	// up to 2.4 dB/second, but when we're in DTX we give the weight of
	// all missing packets to the update packet.
	max_background_increase = int16(IMIN(160, st.loss_duration+M) * int(QCONST16(0.001, DB_SHIFT)))
	for i = 0; i < 2*nbEBands; i++ {
		backgroundLogE[i] = MIN16(backgroundLogE[i]+max_background_increase, oldBandE[i])
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
		if c >= 2 {
			break
		}
	}
	st.rng = dec.rng

	deemphasis(out_syn[:], pcm, N, CC, st.downsample, mode.preemph[:], st.preemph_memD[:], accum)
	st.loss_duration = 0
	st.prefilter_and_fold = 0
	if dec.Tell() > 8*length {
		return OPUS_INTERNAL_ERROR
	}
	if dec.GetError() != 0 {
		st.error = 1
	}
	return frame_size / st.downsample
}

// DecodeWithEc decodes one frame; it is a wrapper around DecodeWithEcDred
// (with a NULL LPCNetPLCState when ENABLE_DEEP_PLC is defined, which it is not).
//
// C: int celt_decode_with_ec(CELTDecoder * OPUS_RESTRICT st, const unsigned char *data,
//
//	int len, opus_val16 * OPUS_RESTRICT pcm, int frame_size, ec_dec *dec, int accum)
func (st *CELTDecoder) DecodeWithEc(data []byte, length int, pcm []int16, frame_size int, dec *EcDec, accum int) int {
	return st.DecodeWithEcDred(data, length, pcm, frame_size, dec, accum)
}

// Ctl is the C opus_custom_decoder_ctl / celt_decoder_ctl.
//
// C: int opus_custom_decoder_ctl(CELTDecoder * OPUS_RESTRICT st, int request, ...)
func (st *CELTDecoder) Ctl(request int, args ...any) int {
	switch request {
	case OPUS_SET_COMPLEXITY_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value < 0 || value > 10 {
			return OPUS_BAD_ARG
		}
		st.complexity = int(value)
	case OPUS_GET_COMPLEXITY_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = int32(st.complexity)
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
		st.loss_duration = 0
		st.skip_plc = 0
		st.postfilter_period = 0
		st.postfilter_period_old = 0
		st.postfilter_gain = 0
		st.postfilter_gain_old = 0
		st.postfilter_tapset = 0
		st.postfilter_tapset_old = 0
		st.prefilter_and_fold = 0
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
		st.skip_plc = 1
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
	default:
		return OPUS_UNIMPLEMENTED
	}
	return OPUS_OK
}
