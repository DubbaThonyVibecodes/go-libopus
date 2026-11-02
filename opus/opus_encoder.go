package opus

import (
	"math"

	"github.com/DubbaThonyVibecodes/go-libopus/celt"
	"github.com/DubbaThonyVibecodes/go-libopus/silk"
)

// C: src/opus_encoder.c

const MAX_ENCODER_BUFFER = 480

// StereoWidthState is the C struct StereoWidthState.
type StereoWidthState struct {
	XX, XY, YY     int32
	smoothed_width int16
	max_follower   int16
}

// OpusEncoder is the C struct OpusEncoder. The SILK and CELT states that C
// places after the struct in the same allocation are embedded fields.
type OpusEncoder struct {
	silk_mode          silk.EncControlStruct
	application        int
	channels           int
	delay_compensation int
	force_channels     int
	signal_type        int
	user_bandwidth     int
	max_bandwidth      int
	user_forced_mode   int
	voice_ratio        int
	Fs                 int32
	use_vbr            int
	vbr_constraint     int
	variable_duration  int
	bitrate_bps        int32
	user_bitrate_bps   int32
	lsb_depth          int
	encoder_buffer     int
	lfe                int
	arch               int
	analysis           TonalityAnalysisState

	// Everything beyond this point gets cleared on a reset (OPUS_ENCODER_RESET_START)
	stream_channels         int
	hybrid_stereo_width_Q14 int16
	variable_HP_smth2_Q15   int32
	prev_HB_gain            int16
	hp_mem                  [4]int32
	mode                    int
	prev_mode               int
	prev_channels           int
	prev_framesize          int
	bandwidth               int
	silk_bw_switch          int
	// Sampling rate (at the API level)
	first              int
	energy_masking     []int16
	width_mem          StereoWidthState
	delay_buffer       [MAX_ENCODER_BUFFER * 2]int16
	detected_bandwidth int
	rangeFinal         uint32

	silk_enc silk.Encoder
	celt_enc celt.CELTEncoder
}

// Transition tables for the voice and music. First column is the
// middle (memoriless) threshold. The second column is the hysteresis
// (difference with the middle)
var mono_voice_bandwidth_thresholds = [8]int32{
	11000, 1000, // NB<->MB
	14000, 1000, // MB<->WB
	17000, 1000, // WB<->SWB
	21000, 2000, // SWB<->FB
}
var mono_music_bandwidth_thresholds = [8]int32{
	12000, 1000, // NB<->MB
	15000, 1000, // MB<->WB
	18000, 2000, // WB<->SWB
	22000, 2000, // SWB<->FB
}
var stereo_voice_bandwidth_thresholds = [8]int32{
	11000, 1000, // NB<->MB
	14000, 1000, // MB<->WB
	21000, 2000, // WB<->SWB
	28000, 2000, // SWB<->FB
}
var stereo_music_bandwidth_thresholds = [8]int32{
	12000, 1000, // NB<->MB
	18000, 2000, // MB<->WB
	21000, 2000, // WB<->SWB
	30000, 2000, // SWB<->FB
}

// Threshold bit-rates for switching between mono and stereo
const stereo_voice_threshold int32 = 30000
const stereo_music_threshold int32 = 30000

// Threshold bit-rate for switching between SILK/hybrid and CELT-only
var mode_thresholds = [2][2]int32{
	// voice   music
	{64000, 16000}, // mono
	{36000, 16000}, // stereo
}

// C: int opus_encoder_get_size(int channels)
func EncoderGetSize(channels int) int {
	var silkEncSizeBytes, celtEncSizeBytes int
	var ret int
	if channels < 1 || channels > 2 {
		return 0
	}
	ret = silk.GetEncoderSize(&silkEncSizeBytes)
	if ret != 0 {
		return 0
	}
	silkEncSizeBytes = align(silkEncSizeBytes)
	celtEncSizeBytes = celt.EncoderGetSize(channels)
	return align(2200) + silkEncSizeBytes + celtEncSizeBytes
}

// C: int opus_encoder_init(OpusEncoder* st, opus_int32 Fs, int channels, int application)
func (st *OpusEncoder) EncoderInit(Fs int32, channels int, application int) int {
	var err int
	var ret int

	if (Fs != 48000 && Fs != 24000 && Fs != 16000 && Fs != 12000 && Fs != 8000) || (channels != 1 && channels != 2) ||
		(application != OPUS_APPLICATION_VOIP && application != OPUS_APPLICATION_AUDIO &&
			application != OPUS_APPLICATION_RESTRICTED_LOWDELAY) {
		return OPUS_BAD_ARG
	}

	*st = OpusEncoder{}
	// Create SILK encoder
	silk_enc := &st.silk_enc
	celt_enc := &st.celt_enc

	st.stream_channels = channels
	st.channels = channels

	st.Fs = Fs

	st.arch = celt.SelectArch()

	ret = silk_enc.InitEncoder(st.arch, &st.silk_mode)
	if ret != 0 {
		return OPUS_INTERNAL_ERROR
	}

	// default SILK parameters
	st.silk_mode.NChannelsAPI = int32(channels)
	st.silk_mode.NChannelsInternal = int32(channels)
	st.silk_mode.API_sampleRate = st.Fs
	st.silk_mode.MaxInternalSampleRate = 16000
	st.silk_mode.MinInternalSampleRate = 8000
	st.silk_mode.DesiredInternalSampleRate = 16000
	st.silk_mode.PayloadSize_ms = 20
	st.silk_mode.BitRate = 25000
	st.silk_mode.PacketLossPercentage = 0
	st.silk_mode.Complexity = 9
	st.silk_mode.UseInBandFEC = 0
	st.silk_mode.UseDTX = 0
	st.silk_mode.UseCBR = 0
	st.silk_mode.ReducedDependency = 0

	// Create CELT encoder
	// Initialize CELT encoder
	err = celt_enc.EncoderInit(Fs, channels, st.arch)
	if err != OPUS_OK {
		return OPUS_INTERNAL_ERROR
	}

	celt_enc.Ctl(celt.CELT_SET_SIGNALLING_REQUEST, int32(0))
	celt_enc.Ctl(celt.OPUS_SET_COMPLEXITY_REQUEST, int32(st.silk_mode.Complexity))

	st.use_vbr = 1
	// Makes constrained VBR the default (safer for real-time use)
	st.vbr_constraint = 1
	st.user_bitrate_bps = OPUS_AUTO
	st.bitrate_bps = 3000 + Fs*int32(channels)
	st.application = application
	st.signal_type = OPUS_AUTO
	st.user_bandwidth = OPUS_AUTO
	st.max_bandwidth = OPUS_BANDWIDTH_FULLBAND
	st.force_channels = OPUS_AUTO
	st.user_forced_mode = OPUS_AUTO
	st.voice_ratio = -1
	st.encoder_buffer = int(st.Fs / 100)
	st.lsb_depth = 24
	st.variable_duration = OPUS_FRAMESIZE_ARG

	// Delay compensation of 4 ms (2.5 ms for SILK's extra look-ahead
	// + 1.5 ms for SILK resamplers and stereo prediction)
	st.delay_compensation = int(st.Fs / 250)

	st.hybrid_stereo_width_Q14 = 1 << 14
	st.prev_HB_gain = celt.Q15ONE
	st.variable_HP_smth2_Q15 = silk.LSHIFT(silk.Lin2log(silk.VARIABLE_HP_MIN_CUTOFF_HZ), 8)
	st.first = 1
	st.mode = MODE_HYBRID
	st.bandwidth = OPUS_BANDWIDTH_FULLBAND

	st.analysis.TonalityAnalysisInit()

	return OPUS_OK
}

// C: static unsigned char gen_toc(int mode, int framerate, int bandwidth, int channels)
func gen_toc(mode int, framerate int, bandwidth int, channels int) byte {
	var period int
	var toc byte
	period = 0
	for framerate < 400 {
		framerate <<= 1
		period++
	}
	if mode == MODE_SILK_ONLY {
		toc = byte((bandwidth - OPUS_BANDWIDTH_NARROWBAND) << 5)
		toc |= byte((period - 2) << 3)
	} else if mode == MODE_CELT_ONLY {
		tmp := bandwidth - OPUS_BANDWIDTH_MEDIUMBAND
		if tmp < 0 {
			tmp = 0
		}
		toc = 0x80
		toc |= byte(tmp << 5)
		toc |= byte(period << 3)
	} else { // Hybrid
		toc = 0x60
		toc |= byte((bandwidth - OPUS_BANDWIDTH_SUPERWIDEBAND) << 4)
		toc |= byte((period - 2) << 3)
	}
	toc |= byte(b2i(channels == 2) << 2)
	return toc
}

// C: static void hp_cutoff(const opus_val16 *in, opus_int32 cutoff_Hz, opus_val16 *out, opus_val32 *hp_mem, int len, int channels, opus_int32 Fs)
func hp_cutoff(in []int16, cutoff_Hz int32, out []int16, hp_mem []int32, length int, channels int, Fs int32) {
	var B_Q28 [3]int32
	var A_Q28 [2]int32
	var Fc_Q19, r_Q28, r_Q22 int32

	silk.Assert(cutoff_Hz <= silk.Int32MAX()/silk.SILK_FIX_CONST(1.5*3.14159/1000, 19))
	Fc_Q19 = silk.DIV32_16(silk.SMULBB(silk.SILK_FIX_CONST(1.5*3.14159/1000, 19), cutoff_Hz), Fs/1000)
	silk.Assert(Fc_Q19 > 0 && Fc_Q19 < 32768)

	r_Q28 = silk.SILK_FIX_CONST(1.0, 28) - silk.MUL(silk.SILK_FIX_CONST(0.92, 9), Fc_Q19)

	// b = r * [ 1; -2; 1 ];
	// a = [ 1; -2 * r * ( 1 - 0.5 * Fc^2 ); r^2 ];
	B_Q28[0] = r_Q28
	B_Q28[1] = silk.LSHIFT(-r_Q28, 1)
	B_Q28[2] = r_Q28

	// -r * ( 2 - Fc * Fc );
	r_Q22 = silk.RSHIFT(r_Q28, 6)
	A_Q28[0] = silk.SMULWW(r_Q22, silk.SMULWW(Fc_Q19, Fc_Q19)-silk.SILK_FIX_CONST(2.0, 22))
	A_Q28[1] = silk.SMULWW(r_Q22, r_Q22)

	silk.BiquadAlt(in, B_Q28[:], A_Q28[:], hp_mem, out, int32(length), channels)
	if channels == 2 {
		silk.BiquadAlt(in[1:], B_Q28[:], A_Q28[:], hp_mem[2:], out[1:], int32(length), channels)
	}
}

// C: static void dc_reject(const opus_val16 *in, opus_int32 cutoff_Hz, opus_val16 *out, opus_val32 *hp_mem, int len, int channels, opus_int32 Fs)   (FIXED_POINT)
func dc_reject(in []int16, cutoff_Hz int32, out []int16, hp_mem []int32, length int, channels int, Fs int32) {
	var c, i int
	var shift int

	// Approximates -round(log2(4.*cutoff_Hz/Fs))
	shift = int(celt.Ilog2(Fs / (cutoff_Hz * 3)))
	for c = 0; c < channels; c++ {
		for i = 0; i < length; i++ {
			var x, tmp, y int32
			x = celt.SHL32(celt.EXTEND32(in[channels*i+c]), 15)
			// First stage
			tmp = x - hp_mem[2*c]
			hp_mem[2*c] = hp_mem[2*c] + celt.PSHR32(x-hp_mem[2*c], shift)
			// Second stage
			y = tmp - hp_mem[2*c+1]
			hp_mem[2*c+1] = hp_mem[2*c+1] + celt.PSHR32(tmp-hp_mem[2*c+1], shift)
			out[channels*i+c] = celt.EXTRACT16(celt.SATURATE(celt.PSHR32(y, 15), 32767))
		}
	}
}

// C: static void stereo_fade(const opus_val16 *in, opus_val16 *out, opus_val16 g1, opus_val16 g2,
//
//	int overlap48, int frame_size, int channels, const opus_val16 *window, opus_int32 Fs)
func stereo_fade(in []int16, out []int16, g1, g2 int16,
	overlap48 int, frame_size int, channels int, window []int16, Fs int32) {
	var i int
	var overlap int
	var inc int
	inc = int(48000 / Fs)
	overlap = overlap48 / inc
	g1 = celt.Q15ONE - g1
	g2 = celt.Q15ONE - g2
	for i = 0; i < overlap; i++ {
		var diff int32
		var g, w int16
		w = int16(celt.MULT16_16_Q15(window[i*inc], window[i*inc]))
		g = int16(celt.SHR32(celt.MAC16_16(celt.MULT16_16(w, g2),
			int32(celt.Q15ONE)-int32(w), g1), 15))
		diff = int32(celt.EXTRACT16(celt.HALF32(int32(in[i*channels]) - int32(in[i*channels+1]))))
		diff = celt.MULT16_16_Q15(g, diff)
		out[i*channels] = int16(int32(out[i*channels]) - diff)
		out[i*channels+1] = int16(int32(out[i*channels+1]) + diff)
	}
	for ; i < frame_size; i++ {
		var diff int32
		diff = int32(celt.EXTRACT16(celt.HALF32(int32(in[i*channels]) - int32(in[i*channels+1]))))
		diff = celt.MULT16_16_Q15(g2, diff)
		out[i*channels] = int16(int32(out[i*channels]) - diff)
		out[i*channels+1] = int16(int32(out[i*channels+1]) + diff)
	}
}

// C: static void gain_fade(const opus_val16 *in, opus_val16 *out, opus_val16 g1, opus_val16 g2,
//
//	int overlap48, int frame_size, int channels, const opus_val16 *window, opus_int32 Fs)
func gain_fade(in []int16, out []int16, g1, g2 int16,
	overlap48 int, frame_size int, channels int, window []int16, Fs int32) {
	var i int
	var inc int
	var overlap int
	var c int
	inc = int(48000 / Fs)
	overlap = overlap48 / inc
	if channels == 1 {
		for i = 0; i < overlap; i++ {
			var g, w int16
			w = int16(celt.MULT16_16_Q15(window[i*inc], window[i*inc]))
			g = int16(celt.SHR32(celt.MAC16_16(celt.MULT16_16(w, g2),
				int32(celt.Q15ONE)-int32(w), g1), 15))
			out[i] = int16(celt.MULT16_16_Q15(g, in[i]))
		}
	} else {
		for i = 0; i < overlap; i++ {
			var g, w int16
			w = int16(celt.MULT16_16_Q15(window[i*inc], window[i*inc]))
			g = int16(celt.SHR32(celt.MAC16_16(celt.MULT16_16(w, g2),
				int32(celt.Q15ONE)-int32(w), g1), 15))
			out[i*2] = int16(celt.MULT16_16_Q15(g, in[i*2]))
			out[i*2+1] = int16(celt.MULT16_16_Q15(g, in[i*2+1]))
		}
	}
	c = 0
	for {
		for i = overlap; i < frame_size; i++ {
			out[i*channels+c] = int16(celt.MULT16_16_Q15(g2, in[i*channels+c]))
		}
		c++
		if !(c < channels) {
			break
		}
	}
}

// C: OpusEncoder *opus_encoder_create(opus_int32 Fs, int channels, int application, int *error)
func EncoderCreate(Fs int32, channels int, application int, error *int) *OpusEncoder {
	var ret int
	if (Fs != 48000 && Fs != 24000 && Fs != 16000 && Fs != 12000 && Fs != 8000) || (channels != 1 && channels != 2) ||
		(application != OPUS_APPLICATION_VOIP && application != OPUS_APPLICATION_AUDIO &&
			application != OPUS_APPLICATION_RESTRICTED_LOWDELAY) {
		if error != nil {
			*error = OPUS_BAD_ARG
		}
		return nil
	}
	st := new(OpusEncoder)
	ret = st.EncoderInit(Fs, channels, application)
	if error != nil {
		*error = ret
	}
	if ret != OPUS_OK {
		st = nil
	}
	return st
}

// C: static opus_int32 user_bitrate_to_bitrate(OpusEncoder *st, int frame_size, int max_data_bytes)
func (st *OpusEncoder) user_bitrate_to_bitrate(frame_size int, max_data_bytes int) int32 {
	if frame_size == 0 {
		frame_size = int(st.Fs / 400)
	}
	if st.user_bitrate_bps == OPUS_AUTO {
		return 60*st.Fs/int32(frame_size) + st.Fs*int32(st.channels)
	} else if st.user_bitrate_bps == OPUS_BITRATE_MAX {
		return int32(max_data_bytes) * 8 * st.Fs / int32(frame_size)
	}
	return st.user_bitrate_bps
}

// Don't use more than 60 ms for the frame size analysis
const MAX_DYNAMIC_FRAMESIZE = 24

// Estimates how much the bitrate will be boosted based on the sub-frame energy
//
// C: static float transient_boost(const float *E, const float *E_1, int LM, int maxM)
func transient_boost(E []float32, E_1 []float32, LM int, maxM int) float32 {
	var i int
	var M int
	var sumE, sumE_1 float32 = 0, 0
	var metric float32

	M = IMIN(maxM, (1<<uint(LM))+1)
	for i = 0; i < M; i++ {
		sumE += E[i]
		sumE_1 += E_1[i]
	}
	metric = float32(sumE*sumE_1) / float32(M*M)
	/*if (LM==3)
	  printf("%f\n", metric);*/
	// return metric>10 ? 1 : 0;
	// return MAX16(0,1-exp(-.25*(metric-2.)));
	return minf(1, float32(math.Sqrt(float64(maxf(0, float32(.05*(metric-2)))))))
}

/* Viterbi decoding trying to find the best frame size combination using look-ahead

   State numbering:
    0: unused
    1:  2.5 ms
    2:  5 ms (#1)
    3:  5 ms (#2)
    4: 10 ms (#1)
    5: 10 ms (#2)
    6: 10 ms (#3)
    7: 10 ms (#4)
    8: 20 ms (#1)
    9: 20 ms (#2)
   10: 20 ms (#3)
   11: 20 ms (#4)
   12: 20 ms (#5)
   13: 20 ms (#6)
   14: 20 ms (#7)
   15: 20 ms (#8)
*/
// C: static int transient_viterbi(const float *E, const float *E_1, int N, int frame_cost, int rate)
func transient_viterbi(E []float32, E_1 []float32, N int, frame_cost int, rate int) int {
	var i int
	var cost [MAX_DYNAMIC_FRAMESIZE][16]float32
	var states [MAX_DYNAMIC_FRAMESIZE][16]int
	var best_cost float32
	var best_state int
	var factor float32
	// Take into account that we damp VBR in the 32 kb/s to 64 kb/s range.
	if rate < 80 {
		factor = 0
	} else if rate > 160 {
		factor = 1
	} else {
		factor = (float32(rate) - 80) / 80
	}
	// Makes variable framesize less aggressive at lower bitrates, but I can't
	// find any valid theoretical justification for this (other than it seems
	// to help)
	for i = 0; i < 16; i++ {
		// Impossible state
		states[0][i] = -1
		cost[0][i] = 1e10
	}
	for i = 0; i < 4; i++ {
		cost[0][1<<uint(i)] = float32(frame_cost+rate*(1<<uint(i))) * (1 + float32(factor*transient_boost(E, E_1, i, N+1)))
		states[0][1<<uint(i)] = i
	}
	for i = 1; i < N; i++ {
		var j int

		// Follow continuations
		for j = 2; j < 16; j++ {
			cost[i][j] = cost[i-1][j-1]
			states[i][j] = j - 1
		}

		// New frames
		for j = 0; j < 4; j++ {
			var k int
			var min_cost float32
			var curr_cost float32
			states[i][1<<uint(j)] = 1
			min_cost = cost[i-1][1]
			for k = 1; k < 4; k++ {
				var tmp float32 = cost[i-1][(1<<uint(k+1))-1]
				if tmp < min_cost {
					states[i][1<<uint(j)] = (1 << uint(k+1)) - 1
					min_cost = tmp
				}
			}
			curr_cost = float32(frame_cost+rate*(1<<uint(j))) * (1 + float32(factor*transient_boost(E[i:], E_1[i:], j, N-i+1)))
			cost[i][1<<uint(j)] = min_cost
			// If part of the frame is outside the analysis window, only count part of the cost
			if N-i < (1 << uint(j)) {
				cost[i][1<<uint(j)] += float32(curr_cost*float32(N-i)) / float32(int(1)<<uint(j))
			} else {
				cost[i][1<<uint(j)] += curr_cost
			}
		}
	}

	best_state = 1
	best_cost = cost[N-1][1]
	// Find best end state (doesn't force a frame to end at N-1)
	for i = 2; i < 16; i++ {
		if cost[N-1][i] < best_cost {
			best_cost = cost[N-1][i]
			best_state = i
		}
	}

	// Follow transitions back
	for i = N - 1; i >= 0; i-- {
		// printf("%d ", best_state);
		best_state = states[i][best_state]
	}
	// printf("%d\n", best_state);
	return best_state
}

// C: static int optimize_framesize(const void *x, int len, int C, opus_int32 Fs,
//
//	int bitrate, opus_val16 tonality, float *mem, int buffering,
//	downmix_func downmix)
func optimize_framesize(x any, length int, C int, Fs int32,
	bitrate int, tonality int16, mem []float32, buffering int,
	downmix downmix_func) int {
	var N int
	var i int
	var e [MAX_DYNAMIC_FRAMESIZE + 4]float32
	var e_1 [MAX_DYNAMIC_FRAMESIZE + 3]float32
	var memx int32
	var bestLM int = 0
	var subframe int
	var pos int
	var offset int

	subframe = int(Fs / 400)
	sub := make([]int32, subframe)
	e[0] = mem[0]
	e_1[0] = 1.0 / (celt.EPSILON + mem[0])
	if buffering != 0 {
		// Consider the CELT delay when not in restricted-lowdelay
		// We assume the buffering is between 2.5 and 5 ms
		offset = 2*subframe - buffering
		celt.Assert(offset >= 0 && offset <= subframe)
		length -= offset
		e[1] = mem[1]
		e_1[1] = 1.0 / (celt.EPSILON + mem[1])
		e[2] = mem[2]
		e_1[2] = 1.0 / (celt.EPSILON + mem[2])
		pos = 3
	} else {
		pos = 1
		offset = 0
	}
	N = IMIN(length/subframe, MAX_DYNAMIC_FRAMESIZE)
	// Just silencing a warning, it's really initialized later
	memx = 0
	for i = 0; i < N; i++ {
		var tmp float32
		var tmpx int32
		var j int
		tmp = celt.EPSILON

		downmix(x, sub, subframe, i*subframe+offset, 0, -2, C)
		if i == 0 {
			memx = sub[0]
		}
		for j = 0; j < subframe; j++ {
			tmpx = sub[j]
			tmp += float32(float32(tmpx-memx) * float32(tmpx-memx))
			memx = tmpx
		}
		e[i+pos] = tmp
		e_1[i+pos] = 1.0 / tmp
	}
	// Hack to get 20 ms working with APPLICATION_AUDIO
	// The real problem is that the corresponding memory needs to use 1.5 ms
	// from this frame and 1 ms from the next frame
	e[i+pos] = e[i+pos-1]
	if buffering != 0 {
		N = IMIN(MAX_DYNAMIC_FRAMESIZE, N+2)
	}
	bestLM = transient_viterbi(e[:], e_1[:], N, int((1+float32(.5*float32(tonality)))*float32(60*C+40)), bitrate/400)
	mem[0] = e[1<<uint(bestLM)]
	if buffering != 0 {
		mem[1] = e[(1<<uint(bestLM))+1]
		mem[2] = e[(1<<uint(bestLM))+2]
	}
	return bestLM
}

// C: typedef void (*downmix_func)(const void *, opus_val32 *, int, int, int, int, int);   (opus_private.h)
type downmix_func func(_x any, sub []int32, subframe int, offset int, c1 int, c2 int, C int)

// C: #define PCM2VAL(x) FLOAT2INT16(x)   (FIXED_POINT)
func PCM2VAL(x float32) int16 { return FLOAT2INT16(x) }

// C: void downmix_float(const void *_x, opus_val32 *sub, int subframe, int offset, int c1, int c2, int C)
func DownmixFloat(_x any, sub []int32, subframe int, offset int, c1 int, c2 int, C int) {
	var x []float32
	var scale int32
	var j int
	x = _x.([]float32)
	for j = 0; j < subframe; j++ {
		sub[j] = int32(PCM2VAL(x[(j+offset)*C+c1]))
	}
	if c2 > -1 {
		for j = 0; j < subframe; j++ {
			sub[j] += int32(PCM2VAL(x[(j+offset)*C+c2]))
		}
	} else if c2 == -2 {
		var c int
		for c = 1; c < C; c++ {
			for j = 0; j < subframe; j++ {
				sub[j] += int32(PCM2VAL(x[(j+offset)*C+c]))
			}
		}
	}
	scale = (1 << celt.SIG_SHIFT)
	if C == -2 {
		scale /= int32(C)
	} else {
		scale /= 2
	}
	for j = 0; j < subframe; j++ {
		sub[j] *= scale
	}
}

// C: void downmix_int(const void *_x, opus_val32 *sub, int subframe, int offset, int c1, int c2, int C)
func DownmixInt(_x any, sub []int32, subframe int, offset int, c1 int, c2 int, C int) {
	var x []int16
	var scale int32
	var j int
	x = _x.([]int16)
	for j = 0; j < subframe; j++ {
		sub[j] = int32(x[(j+offset)*C+c1])
	}
	if c2 > -1 {
		for j = 0; j < subframe; j++ {
			sub[j] += int32(x[(j+offset)*C+c2])
		}
	} else if c2 == -2 {
		var c int
		for c = 1; c < C; c++ {
			for j = 0; j < subframe; j++ {
				sub[j] += int32(x[(j+offset)*C+c])
			}
		}
	}
	scale = (1 << celt.SIG_SHIFT)
	if C == -2 {
		scale /= int32(C)
	} else {
		scale /= 2
	}
	for j = 0; j < subframe; j++ {
		sub[j] *= scale
	}
}

// C: opus_int32 frame_size_select(opus_int32 frame_size, int variable_duration, opus_int32 Fs)
func FrameSizeSelect(frame_size int32, variable_duration int, Fs int32) int32 {
	var new_size int32
	if frame_size < Fs/400 {
		return -1
	}
	if variable_duration == OPUS_FRAMESIZE_ARG {
		new_size = frame_size
	} else if variable_duration == OPUS_FRAMESIZE_VARIABLE {
		new_size = Fs / 50
	} else if variable_duration >= OPUS_FRAMESIZE_2_5_MS && variable_duration <= OPUS_FRAMESIZE_60_MS {
		new_size = IMIN(3*Fs/50, (Fs/400)<<uint(variable_duration-OPUS_FRAMESIZE_2_5_MS))
	} else {
		return -1
	}
	if new_size > frame_size {
		return -1
	}
	if 400*new_size != Fs && 200*new_size != Fs && 100*new_size != Fs &&
		50*new_size != Fs && 25*new_size != Fs && 50*new_size != 3*Fs {
		return -1
	}
	return new_size
}

// C: opus_int32 compute_frame_size(const void *analysis_pcm, int frame_size,
//
//	int variable_duration, int C, opus_int32 Fs, int bitrate_bps,
//	int delay_compensation, downmix_func downmix, float *subframe_mem)
func ComputeFrameSize(analysis_pcm any, frame_size int,
	variable_duration int, C int, Fs int32, bitrate_bps int,
	delay_compensation int, downmix downmix_func, subframe_mem []float32) int32 {
	if variable_duration == OPUS_FRAMESIZE_VARIABLE && frame_size >= int(Fs/200) {
		var LM int = 3
		LM = optimize_framesize(analysis_pcm, frame_size, C, Fs, bitrate_bps,
			0, subframe_mem, delay_compensation, downmix)
		for (int(Fs/400) << uint(LM)) > frame_size {
			LM--
		}
		frame_size = (int(Fs/400) << uint(LM))
	} else {
		frame_size = int(FrameSizeSelect(int32(frame_size), variable_duration, Fs))
	}
	if frame_size < 0 {
		return -1
	}
	return int32(frame_size)
}

// C: opus_val16 compute_stereo_width(const opus_val16 *pcm, int frame_size, opus_int32 Fs, StereoWidthState *mem)
func ComputeStereoWidth(pcm []int16, frame_size int, Fs int32, mem *StereoWidthState) int16 {
	var xx, xy, yy int32
	var sqrt_xx, sqrt_yy int16
	var qrrt_xx, qrrt_yy int16
	var frame_rate int
	var i int
	var short_alpha int16

	frame_rate = int(Fs) / frame_size
	short_alpha = int16(int32(celt.Q15ONE) - celt.MULT16_16(25, celt.Q15ONE)/int32(IMAX(50, frame_rate)))
	xx = 0
	xy = 0
	yy = 0
	// Unroll by 4. The frame size is always a multiple of 4 *except* for
	// 2.5 ms frames at 12 kHz. Since this setting is very rare (and very
	// stupid), we just discard the last two samples.
	for i = 0; i < frame_size-3; i += 4 {
		var pxx int32 = 0
		var pxy int32 = 0
		var pyy int32 = 0
		var x, y int16
		x = pcm[2*i]
		y = pcm[2*i+1]
		pxx = celt.SHR32(celt.MULT16_16(x, x), 2)
		pxy = celt.SHR32(celt.MULT16_16(x, y), 2)
		pyy = celt.SHR32(celt.MULT16_16(y, y), 2)
		x = pcm[2*i+2]
		y = pcm[2*i+3]
		pxx += celt.SHR32(celt.MULT16_16(x, x), 2)
		pxy += celt.SHR32(celt.MULT16_16(x, y), 2)
		pyy += celt.SHR32(celt.MULT16_16(y, y), 2)
		x = pcm[2*i+4]
		y = pcm[2*i+5]
		pxx += celt.SHR32(celt.MULT16_16(x, x), 2)
		pxy += celt.SHR32(celt.MULT16_16(x, y), 2)
		pyy += celt.SHR32(celt.MULT16_16(y, y), 2)
		x = pcm[2*i+6]
		y = pcm[2*i+7]
		pxx += celt.SHR32(celt.MULT16_16(x, x), 2)
		pxy += celt.SHR32(celt.MULT16_16(x, y), 2)
		pyy += celt.SHR32(celt.MULT16_16(y, y), 2)

		xx += celt.SHR32(pxx, 10)
		xy += celt.SHR32(pxy, 10)
		yy += celt.SHR32(pyy, 10)
	}
	mem.XX += celt.MULT16_32_Q15(short_alpha, xx-mem.XX)
	mem.XY += celt.MULT16_32_Q15(short_alpha, xy-mem.XY)
	mem.YY += celt.MULT16_32_Q15(short_alpha, yy-mem.YY)
	mem.XX = celt.MAX32(0, mem.XX)
	mem.XY = celt.MAX32(0, mem.XY)
	mem.YY = celt.MAX32(0, mem.YY)
	if celt.MAX32(mem.XX, mem.YY) > int32(celt.QCONST16(8e-4, 18)) {
		var corr int16
		var ldiff int16
		var width int16
		sqrt_xx = int16(celt.Sqrt(mem.XX))
		sqrt_yy = int16(celt.Sqrt(mem.YY))
		qrrt_xx = int16(celt.Sqrt(int32(sqrt_xx)))
		qrrt_yy = int16(celt.Sqrt(int32(sqrt_yy)))
		// Inter-channel correlation
		mem.XY = celt.MIN32(mem.XY, int32(sqrt_xx)*int32(sqrt_yy))
		corr = int16(celt.SHR32(celt.FracDiv32(mem.XY, celt.EPSILON+celt.MULT16_16(sqrt_xx, sqrt_yy)), 16))
		// Approximate loudness difference
		ldiff = int16(celt.MULT16_16(celt.Q15ONE, celt.ABS16(int32(qrrt_xx)-int32(qrrt_yy))) / int32(celt.EPSILON+int(qrrt_xx)+int(qrrt_yy)))
		width = int16(celt.MULT16_16_Q15(celt.Sqrt(celt.QCONST32(1.0, 30)-celt.MULT16_16(corr, corr)), ldiff))
		// Smoothing over one second
		mem.smoothed_width = int16(int(mem.smoothed_width) + (int(width)-int(mem.smoothed_width))/frame_rate)
		// Peak follower
		mem.max_follower = int16(celt.MAX16(int(mem.max_follower)-int(celt.QCONST16(.02, 15))/frame_rate, int(mem.smoothed_width)))
	}
	// printf("%f %f %f %f %f ", corr/(float)Q15ONE, ldiff/(float)Q15ONE, width/(float)Q15ONE, mem->smoothed_width/(float)Q15ONE, mem->max_follower/(float)Q15ONE);
	return celt.EXTRACT16(celt.MIN32(int32(celt.Q15ONE), celt.MULT16_16(20, mem.max_follower)))
}

// C: opus_int32 opus_encode_native(OpusEncoder *st, const opus_val16 *pcm, int frame_size,
//
//	unsigned char *data, opus_int32 out_data_bytes, int lsb_depth,
//	const void *analysis_pcm, opus_int32 analysis_size, int c1, int c2,
//	int analysis_channels, downmix_func downmix, int float_api)
func (st *OpusEncoder) EncodeNative(pcm []int16, frame_size int,
	data []byte, out_data_bytes int32, lsb_depth int,
	analysis_pcm any, analysis_size int32, c1 int, c2 int,
	analysis_channels int, downmix downmix_func, float_api int) int32 {
	var silk_enc *silk.Encoder
	var celt_enc *celt.CELTEncoder
	var i int
	var ret int = 0
	var nBytes int32
	var enc celt.EcEnc
	var bytes_target int
	var prefill int = 0
	var start_band int = 0
	var redundancy int = 0
	var redundancy_bytes int = 0 // Number of bytes to use for redundancy frame
	var celt_to_silk int = 0
	var nb_compr_bytes int
	var to_celt int = 0
	var redundant_rng uint32 = 0
	var cutoff_Hz, hp_freq_smth1 int32
	var voice_est int // Probability of voice in Q7
	var equiv_rate int32
	var delay_compensation int
	var frame_rate int
	var max_rate int32 // Max bitrate we're allowed to use
	var curr_bandwidth int
	var HB_gain int16
	var max_data_bytes int32 // Max number of bytes we're allowed to use
	var total_buffer int
	var stereo_width int16
	var celt_mode *celt.CELTMode
	var analysis_info celt.AnalysisInfo
	var analysis_read_pos_bak int = -1
	var analysis_read_subframe_bak int = -1

	max_data_bytes = IMIN(1276, out_data_bytes)

	st.rangeFinal = 0
	if (st.variable_duration == 0 && 400*int32(frame_size) != st.Fs && 200*int32(frame_size) != st.Fs && 100*int32(frame_size) != st.Fs &&
		50*int32(frame_size) != st.Fs && 25*int32(frame_size) != st.Fs && 50*int32(frame_size) != 3*st.Fs) ||
		(400*int32(frame_size) < st.Fs) ||
		max_data_bytes <= 0 {
		return OPUS_BAD_ARG
	}
	silk_enc = &st.silk_enc
	celt_enc = &st.celt_enc
	if st.application == OPUS_APPLICATION_RESTRICTED_LOWDELAY {
		delay_compensation = 0
	} else {
		delay_compensation = st.delay_compensation
	}

	lsb_depth = IMIN(lsb_depth, st.lsb_depth)

	celt_enc.Ctl(celt.CELT_GET_MODE_REQUEST, &celt_mode)
	analysis_info.Valid = 0
	if st.silk_mode.Complexity >= 10 && st.Fs == 48000 {
		analysis_read_pos_bak = st.analysis.read_pos
		analysis_read_subframe_bak = st.analysis.read_subframe
		st.analysis.RunAnalysis(celt_mode, analysis_pcm, int(analysis_size), frame_size,
			c1, c2, analysis_channels, st.Fs,
			lsb_depth, downmix, &analysis_info)
	}

	st.voice_ratio = -1

	st.detected_bandwidth = 0
	if analysis_info.Valid != 0 {
		var analysis_bandwidth int
		if st.signal_type == OPUS_AUTO {
			st.voice_ratio = int(math.Floor(.5 + float64(float32(100*(1-analysis_info.Music_prob)))))
		}

		analysis_bandwidth = analysis_info.Bandwidth
		if analysis_bandwidth <= 12 {
			st.detected_bandwidth = OPUS_BANDWIDTH_NARROWBAND
		} else if analysis_bandwidth <= 14 {
			st.detected_bandwidth = OPUS_BANDWIDTH_MEDIUMBAND
		} else if analysis_bandwidth <= 16 {
			st.detected_bandwidth = OPUS_BANDWIDTH_WIDEBAND
		} else if analysis_bandwidth <= 18 {
			st.detected_bandwidth = OPUS_BANDWIDTH_SUPERWIDEBAND
		} else {
			st.detected_bandwidth = OPUS_BANDWIDTH_FULLBAND
		}
	}

	if st.channels == 2 && st.force_channels != 1 {
		stereo_width = ComputeStereoWidth(pcm, frame_size, st.Fs, &st.width_mem)
	} else {
		stereo_width = 0
	}
	total_buffer = delay_compensation
	st.bitrate_bps = st.user_bitrate_to_bitrate(frame_size, int(max_data_bytes))

	frame_rate = int(st.Fs) / frame_size
	if st.use_vbr == 0 {
		var cbrBytes int
		// Multiply by 3 to make sure the division is exact.
		var frame_rate3 int = 3 * int(st.Fs) / frame_size
		// We need to make sure that "int" values always fit in 16 bits.
		cbrBytes = int(IMIN((3*st.bitrate_bps/8+int32(frame_rate3)/2)/int32(frame_rate3), max_data_bytes))
		st.bitrate_bps = int32(cbrBytes) * int32(frame_rate3) * 8 / 3
		max_data_bytes = int32(cbrBytes)
	}
	if max_data_bytes < 3 || st.bitrate_bps < int32(3*frame_rate*8) ||
		(frame_rate < 50 && (max_data_bytes*int32(frame_rate) < 300 || st.bitrate_bps < 2400)) {
		// If the space is too low to do something useful, emit 'PLC' frames.
		tocmode := st.mode
		bw := st.bandwidth
		if bw == 0 {
			bw = OPUS_BANDWIDTH_NARROWBAND
		}
		if tocmode == 0 {
			tocmode = MODE_SILK_ONLY
		}
		if frame_rate > 100 {
			tocmode = MODE_CELT_ONLY
		}
		if frame_rate < 50 {
			tocmode = MODE_SILK_ONLY
		}
		if tocmode == MODE_SILK_ONLY && bw > OPUS_BANDWIDTH_WIDEBAND {
			bw = OPUS_BANDWIDTH_WIDEBAND
		} else if tocmode == MODE_CELT_ONLY && bw == OPUS_BANDWIDTH_MEDIUMBAND {
			bw = OPUS_BANDWIDTH_NARROWBAND
		} else if tocmode == MODE_HYBRID && bw <= OPUS_BANDWIDTH_SUPERWIDEBAND {
			bw = OPUS_BANDWIDTH_SUPERWIDEBAND
		}
		data[0] = gen_toc(tocmode, frame_rate, bw, st.stream_channels)
		ret = 1
		if st.use_vbr == 0 {
			ret = PacketPad(data, int32(ret), max_data_bytes)
			if ret == OPUS_OK {
				ret = int(max_data_bytes)
			}
		}
		return int32(ret)
	}
	max_rate = int32(frame_rate) * max_data_bytes * 8

	// Equivalent 20-ms rate for mode/channel/bandwidth decisions
	equiv_rate = st.bitrate_bps - int32(40*st.channels+20)*(st.Fs/int32(frame_size)-50)

	if st.signal_type == OPUS_SIGNAL_VOICE {
		voice_est = 127
	} else if st.signal_type == OPUS_SIGNAL_MUSIC {
		voice_est = 0
	} else if st.voice_ratio >= 0 {
		voice_est = st.voice_ratio * 327 >> 8
		// For AUDIO, never be more than 90% confident of having speech
		if st.application == OPUS_APPLICATION_AUDIO {
			voice_est = IMIN(voice_est, 115)
		}
	} else if st.application == OPUS_APPLICATION_VOIP {
		voice_est = 115
	} else {
		voice_est = 48
	}

	if st.force_channels != OPUS_AUTO && st.channels == 2 {
		st.stream_channels = st.force_channels
	} else {
		// Rate-dependent mono-stereo decision
		if st.channels == 2 {
			var stereo_threshold int32
			stereo_threshold = stereo_music_threshold + int32((voice_est*voice_est*int(stereo_voice_threshold-stereo_music_threshold))>>14)
			if st.stream_channels == 2 {
				stereo_threshold -= 1000
			} else {
				stereo_threshold += 1000
			}
			if equiv_rate > stereo_threshold {
				st.stream_channels = 2
			} else {
				st.stream_channels = 1
			}
		} else {
			st.stream_channels = st.channels
		}
	}
	equiv_rate = st.bitrate_bps - int32(40*st.stream_channels+20)*(st.Fs/int32(frame_size)-50)

	// Mode selection depending on application and signal type
	if st.application == OPUS_APPLICATION_RESTRICTED_LOWDELAY {
		st.mode = MODE_CELT_ONLY
	} else if st.user_forced_mode == OPUS_AUTO {
		var mode_voice, mode_music int32
		var threshold int32

		// Interpolate based on stereo width
		mode_voice = int32(celt.MULT16_32_Q15(celt.Q15ONE-stereo_width, mode_thresholds[0][0]) +
			celt.MULT16_32_Q15(stereo_width, mode_thresholds[1][0]))
		mode_music = int32(celt.MULT16_32_Q15(celt.Q15ONE-stereo_width, mode_thresholds[1][1]) +
			celt.MULT16_32_Q15(stereo_width, mode_thresholds[1][1]))
		// Interpolate based on speech/music probability
		threshold = mode_music + int32((voice_est*voice_est*int(mode_voice-mode_music))>>14)
		// Bias towards SILK for VoIP because of some useful features
		if st.application == OPUS_APPLICATION_VOIP {
			threshold += 8000
		}

		// printf("%f %d\n", stereo_width/(float)Q15ONE, threshold);
		// Hysteresis
		if st.prev_mode == MODE_CELT_ONLY {
			threshold -= 4000
		} else if st.prev_mode > 0 {
			threshold += 4000
		}

		if equiv_rate >= threshold {
			st.mode = MODE_CELT_ONLY
		} else {
			st.mode = MODE_SILK_ONLY
		}

		// When FEC is enabled and there's enough packet loss, use SILK
		if st.silk_mode.UseInBandFEC != 0 && st.silk_mode.PacketLossPercentage > (128-voice_est)>>4 {
			st.mode = MODE_SILK_ONLY
		}
		// When encoding voice and DTX is enabled, set the encoder to SILK mode (at least for now)
		if st.silk_mode.UseDTX != 0 && voice_est > 100 {
			st.mode = MODE_SILK_ONLY
		}
	} else {
		st.mode = st.user_forced_mode
	}

	// Override the chosen mode to make sure we meet the requested frame size
	if st.mode != MODE_CELT_ONLY && frame_size < int(st.Fs/100) {
		st.mode = MODE_CELT_ONLY
	}
	if st.lfe != 0 {
		st.mode = MODE_CELT_ONLY
	}
	// If max_data_bytes represents less than 8 kb/s, switch to CELT-only mode
	{
		var r int32 = 8000
		if frame_rate > 50 {
			r = 12000
		}
		if max_data_bytes < r*int32(frame_size)/(st.Fs*8) {
			st.mode = MODE_CELT_ONLY
		}
	}

	if st.stream_channels == 1 && st.prev_channels == 2 && st.silk_mode.ToMono == 0 &&
		st.mode != MODE_CELT_ONLY && st.prev_mode != MODE_CELT_ONLY {
		// Delay stereo->mono transition by two frames so that SILK can do a smooth downmix
		st.silk_mode.ToMono = 1
		st.stream_channels = 2
	} else {
		st.silk_mode.ToMono = 0
	}

	if st.prev_mode > 0 &&
		((st.mode != MODE_CELT_ONLY && st.prev_mode == MODE_CELT_ONLY) ||
			(st.mode == MODE_CELT_ONLY && st.prev_mode != MODE_CELT_ONLY)) {
		redundancy = 1
		celt_to_silk = b2i(st.mode != MODE_CELT_ONLY)
		if celt_to_silk == 0 {
			// Switch to SILK/hybrid if frame size is 10 ms or more
			if frame_size >= int(st.Fs/100) {
				st.mode = st.prev_mode
				to_celt = 1
			} else {
				redundancy = 0
			}
		}
	}
	// For the first frame at a new SILK bandwidth
	if st.silk_bw_switch != 0 {
		redundancy = 1
		celt_to_silk = 1
		st.silk_bw_switch = 0
		prefill = 1
	}

	if redundancy != 0 {
		// Fair share of the max size allowed
		redundancy_bytes = int(IMIN(257, max_data_bytes*(st.Fs/200)/int32(frame_size+int(st.Fs/200))))
		// For VBR, target the actual bitrate (subject to the limit above)
		if st.use_vbr != 0 {
			redundancy_bytes = int(IMIN(int32(redundancy_bytes), st.bitrate_bps/1600))
		}
	}

	if st.mode != MODE_CELT_ONLY && st.prev_mode == MODE_CELT_ONLY {
		var dummy silk.EncControlStruct
		silk_enc.InitEncoder(st.arch, &dummy)
		prefill = 1
	}

	// Automatic (rate-dependent) bandwidth selection
	if st.mode == MODE_CELT_ONLY || st.first != 0 || st.silk_mode.AllowBandwidthSwitch != 0 {
		var voice_bandwidth_thresholds, music_bandwidth_thresholds *[8]int32
		var bandwidth_thresholds [8]int32
		var bandwidth int = OPUS_BANDWIDTH_FULLBAND
		var equiv_rate2 int32

		equiv_rate2 = equiv_rate
		if st.mode != MODE_CELT_ONLY {
			// Adjust the threshold +/- 10% depending on complexity
			equiv_rate2 = equiv_rate2 * int32(45+st.silk_mode.Complexity) / 50
			// CBR is less efficient by ~1 kb/s
			if st.use_vbr == 0 {
				equiv_rate2 -= 1000
			}
		}
		if st.channels == 2 && st.force_channels != 1 {
			voice_bandwidth_thresholds = &stereo_voice_bandwidth_thresholds
			music_bandwidth_thresholds = &stereo_music_bandwidth_thresholds
		} else {
			voice_bandwidth_thresholds = &mono_voice_bandwidth_thresholds
			music_bandwidth_thresholds = &mono_music_bandwidth_thresholds
		}
		// Interpolate bandwidth thresholds depending on voice estimation
		for i = 0; i < 8; i++ {
			bandwidth_thresholds[i] = music_bandwidth_thresholds[i] +
				int32((voice_est*voice_est*int(voice_bandwidth_thresholds[i]-music_bandwidth_thresholds[i]))>>14)
		}
		for {
			var threshold, hysteresis int
			threshold = int(bandwidth_thresholds[2*(bandwidth-OPUS_BANDWIDTH_MEDIUMBAND)])
			hysteresis = int(bandwidth_thresholds[2*(bandwidth-OPUS_BANDWIDTH_MEDIUMBAND)+1])
			if st.first == 0 {
				if st.bandwidth >= bandwidth {
					threshold -= hysteresis
				} else {
					threshold += hysteresis
				}
			}
			if int(equiv_rate2) >= threshold {
				break
			}
			bandwidth--
			if bandwidth <= OPUS_BANDWIDTH_NARROWBAND {
				break
			}
		}
		st.bandwidth = bandwidth
		// Prevents any transition to SWB/FB until the SILK layer has fully
		// switched to WB mode and turned the variable LP filter off
		if st.first == 0 && st.mode != MODE_CELT_ONLY && st.silk_mode.InWBmodeWithoutVariableLP == 0 && st.bandwidth > OPUS_BANDWIDTH_WIDEBAND {
			st.bandwidth = OPUS_BANDWIDTH_WIDEBAND
		}
	}

	if st.bandwidth > st.max_bandwidth {
		st.bandwidth = st.max_bandwidth
	}

	if st.user_bandwidth != OPUS_AUTO {
		st.bandwidth = st.user_bandwidth
	}

	// This prevents us from using hybrid at unsafe CBR/max rates
	if st.mode != MODE_CELT_ONLY && max_rate < 15000 {
		st.bandwidth = IMIN(st.bandwidth, OPUS_BANDWIDTH_WIDEBAND)
	}

	// Prevents Opus from wasting bits on frequencies that are above
	// the Nyquist rate of the input signal
	if st.Fs <= 24000 && st.bandwidth > OPUS_BANDWIDTH_SUPERWIDEBAND {
		st.bandwidth = OPUS_BANDWIDTH_SUPERWIDEBAND
	}
	if st.Fs <= 16000 && st.bandwidth > OPUS_BANDWIDTH_WIDEBAND {
		st.bandwidth = OPUS_BANDWIDTH_WIDEBAND
	}
	if st.Fs <= 12000 && st.bandwidth > OPUS_BANDWIDTH_MEDIUMBAND {
		st.bandwidth = OPUS_BANDWIDTH_MEDIUMBAND
	}
	if st.Fs <= 8000 && st.bandwidth > OPUS_BANDWIDTH_NARROWBAND {
		st.bandwidth = OPUS_BANDWIDTH_NARROWBAND
	}
	// Use detected bandwidth to reduce the encoded bandwidth.
	if st.detected_bandwidth != 0 && st.user_bandwidth == OPUS_AUTO {
		var min_detected_bandwidth int
		// Makes bandwidth detection more conservative just in case the detector
		// gets it wrong when we could have coded a high bandwidth transparently.
		// When operating in SILK/hybrid mode, we don't go below wideband to avoid
		// more complicated switches that require redundancy.
		if equiv_rate <= int32(18000*st.stream_channels) && st.mode == MODE_CELT_ONLY {
			min_detected_bandwidth = OPUS_BANDWIDTH_NARROWBAND
		} else if equiv_rate <= int32(24000*st.stream_channels) && st.mode == MODE_CELT_ONLY {
			min_detected_bandwidth = OPUS_BANDWIDTH_MEDIUMBAND
		} else if equiv_rate <= int32(30000*st.stream_channels) {
			min_detected_bandwidth = OPUS_BANDWIDTH_WIDEBAND
		} else if equiv_rate <= int32(44000*st.stream_channels) {
			min_detected_bandwidth = OPUS_BANDWIDTH_SUPERWIDEBAND
		} else {
			min_detected_bandwidth = OPUS_BANDWIDTH_FULLBAND
		}

		st.detected_bandwidth = IMAX(st.detected_bandwidth, min_detected_bandwidth)
		st.bandwidth = IMIN(st.bandwidth, st.detected_bandwidth)
	}
	celt_enc.Ctl(celt.OPUS_SET_LSB_DEPTH_REQUEST, int32(lsb_depth))

	// CELT mode doesn't support mediumband, use wideband instead
	if st.mode == MODE_CELT_ONLY && st.bandwidth == OPUS_BANDWIDTH_MEDIUMBAND {
		st.bandwidth = OPUS_BANDWIDTH_WIDEBAND
	}
	if st.lfe != 0 {
		st.bandwidth = OPUS_BANDWIDTH_NARROWBAND
	}

	// Can't support higher than wideband for >20 ms frames
	if frame_size > int(st.Fs/50) && (st.mode == MODE_CELT_ONLY || st.bandwidth > OPUS_BANDWIDTH_WIDEBAND) {
		var nb_frames int
		var bak_mode, bak_bandwidth, bak_channels, bak_to_mono int
		var rp OpusRepacketizer
		var bytes_per_frame int32
		var repacketize_len int32

		if analysis_read_pos_bak != -1 {
			st.analysis.read_pos = analysis_read_pos_bak
			st.analysis.read_subframe = analysis_read_subframe_bak
		}

		if frame_size > int(st.Fs/25) {
			nb_frames = 3
		} else {
			nb_frames = 2
		}
		bytes_per_frame = IMIN(1276, (out_data_bytes-3)/int32(nb_frames))

		tmp_data := make([]byte, nb_frames*int(bytes_per_frame))

		rp.RepacketizerInit()

		bak_mode = st.user_forced_mode
		bak_bandwidth = st.user_bandwidth
		bak_channels = st.force_channels

		st.user_forced_mode = st.mode
		st.user_bandwidth = st.bandwidth
		st.force_channels = st.stream_channels
		bak_to_mono = st.silk_mode.ToMono

		if bak_to_mono != 0 {
			st.force_channels = 1
		} else {
			st.prev_channels = st.stream_channels
		}
		for i = 0; i < nb_frames; i++ {
			var tmp_len int32
			st.silk_mode.ToMono = 0
			// When switching from SILK/Hybrid to CELT, only ask for a switch at the last frame
			if to_celt != 0 && i == nb_frames-1 {
				st.user_forced_mode = MODE_CELT_ONLY
			}
			tmp_len = st.EncodeNative(pcm[i*(st.channels*int(st.Fs)/50):], int(st.Fs/50),
				tmp_data[i*int(bytes_per_frame):], bytes_per_frame, lsb_depth,
				nil, 0, c1, c2, analysis_channels, downmix, float_api)
			if tmp_len < 0 {
				return OPUS_INTERNAL_ERROR
			}
			ret = rp.RepacketizerCat(tmp_data[i*int(bytes_per_frame):], tmp_len)
			if ret < 0 {
				return OPUS_INTERNAL_ERROR
			}
		}
		if st.use_vbr != 0 {
			repacketize_len = out_data_bytes
		} else {
			repacketize_len = IMIN(3*st.bitrate_bps/int32(3*8*50/nb_frames), out_data_bytes)
		}
		ret32 := rp.repacketizerOutRangeImpl(0, nb_frames, data, repacketize_len, 0, b2i(st.use_vbr == 0))
		if ret32 < 0 {
			return OPUS_INTERNAL_ERROR
		}
		st.user_forced_mode = bak_mode
		st.user_bandwidth = bak_bandwidth
		st.force_channels = bak_channels
		st.silk_mode.ToMono = bak_to_mono
		return ret32
	}
	curr_bandwidth = st.bandwidth

	// Chooses the appropriate mode for speech
	// *NEVER* switch to/from CELT-only mode here as this will invalidate some assumptions
	if st.mode == MODE_SILK_ONLY && curr_bandwidth > OPUS_BANDWIDTH_WIDEBAND {
		st.mode = MODE_HYBRID
	}
	if st.mode == MODE_HYBRID && curr_bandwidth <= OPUS_BANDWIDTH_WIDEBAND {
		st.mode = MODE_SILK_ONLY
	}

	// printf("%d %d %d %d\n", st->bitrate_bps, st->stream_channels, st->mode, curr_bandwidth);
	bytes_target = int(IMIN(max_data_bytes-int32(redundancy_bytes), st.bitrate_bps*int32(frame_size)/(st.Fs*8))) - 1

	// data += 1; the C code works on data+1 from here on and writes the ToC
	// to data[-1] at the end. In Go we keep `data` and use `data[1:]`.
	payload := data[1:]

	enc.EncInit(payload, uint32(max_data_bytes-1))

	pcm_buf := make([]int16, (total_buffer+frame_size)*st.channels)
	copy(pcm_buf, st.delay_buffer[(st.encoder_buffer-total_buffer)*st.channels:][:total_buffer*st.channels])

	if st.mode == MODE_CELT_ONLY {
		hp_freq_smth1 = silk.LSHIFT(silk.Lin2log(silk.VARIABLE_HP_MIN_CUTOFF_HZ), 8)
	} else {
		hp_freq_smth1 = silk_enc.VariableHPSmth1Q15()
	}

	st.variable_HP_smth2_Q15 = silk.SMLAWB(st.variable_HP_smth2_Q15,
		hp_freq_smth1-st.variable_HP_smth2_Q15, silk.SILK_FIX_CONST(silk.VARIABLE_HP_SMTH_COEF2, 16))

	// convert from log scale to Hertz
	cutoff_Hz = silk.Log2lin(silk.RSHIFT(st.variable_HP_smth2_Q15, 8))

	if st.application == OPUS_APPLICATION_VOIP {
		hp_cutoff(pcm, cutoff_Hz, pcm_buf[total_buffer*st.channels:], st.hp_mem[:], frame_size, st.channels, st.Fs)
	} else {
		dc_reject(pcm, 3, pcm_buf[total_buffer*st.channels:], st.hp_mem[:], frame_size, st.channels, st.Fs)
	}

	// SILK processing
	HB_gain = celt.Q15ONE
	if st.mode != MODE_CELT_ONLY {
		var total_bitRate, celt_rate int32
		var pcm_silk []int16

		// Distribute bits between SILK and CELT
		total_bitRate = int32(8 * bytes_target * frame_rate)
		if st.mode == MODE_HYBRID {
			var HB_gain_ref int
			// Base rate for SILK
			st.silk_mode.BitRate = int32(st.stream_channels) * (5000 + 1000*int32(b2i(st.Fs == 100*int32(frame_size))))
			if curr_bandwidth == OPUS_BANDWIDTH_SUPERWIDEBAND {
				// SILK gets 2/3 of the remaining bits
				st.silk_mode.BitRate += (total_bitRate - st.silk_mode.BitRate) * 2 / 3
			} else { // FULLBAND
				// SILK gets 3/5 of the remaining bits
				st.silk_mode.BitRate += (total_bitRate - st.silk_mode.BitRate) * 3 / 5
			}
			// Don't let SILK use more than 80%
			if st.silk_mode.BitRate > total_bitRate*4/5 {
				st.silk_mode.BitRate = total_bitRate * 4 / 5
			}
			if st.energy_masking == nil {
				// Increasingly attenuate high band when it gets allocated fewer bits
				celt_rate = total_bitRate - st.silk_mode.BitRate
				if curr_bandwidth == OPUS_BANDWIDTH_SUPERWIDEBAND {
					HB_gain_ref = 3000
				} else {
					HB_gain_ref = 3600
				}
				HB_gain = int16(celt.SHL32(celt_rate, 9) / celt.SHR32(celt_rate+int32(st.stream_channels*HB_gain_ref), 6))
				if int32(HB_gain) < int32(celt.Q15ONE)*6/7 {
					HB_gain = HB_gain + celt.Q15ONE/7
				} else {
					HB_gain = celt.Q15ONE
				}
			}
		} else {
			// SILK gets all bits
			st.silk_mode.BitRate = total_bitRate
		}

		// Surround masking for SILK
		if st.energy_masking != nil && st.use_vbr != 0 && st.lfe == 0 {
			var mask_sum int32 = 0
			var masking_depth int16
			var rate_offset int32
			var c int
			var end int = 17
			var srate int16 = 16000
			if st.bandwidth == OPUS_BANDWIDTH_NARROWBAND {
				end = 13
				srate = 8000
			} else if st.bandwidth == OPUS_BANDWIDTH_MEDIUMBAND {
				end = 15
				srate = 12000
			}
			for c = 0; c < st.channels; c++ {
				for i = 0; i < end; i++ {
					var mask int16
					mask = celt.MAX16(celt.MIN16(st.energy_masking[21*c+i],
						celt.QCONST16(.5, celt.DB_SHIFT)), -celt.QCONST16(2.0, celt.DB_SHIFT))
					if mask > 0 {
						mask = int16(celt.HALF16(mask))
					}
					mask_sum += int32(mask)
				}
			}
			// Conservative rate reduction, we cut the masking in half
			masking_depth = int16(mask_sum / int32(end) * int32(st.channels))
			masking_depth += celt.QCONST16(.2, celt.DB_SHIFT)
			rate_offset = celt.PSHR32(celt.MULT16_16(srate, masking_depth), celt.DB_SHIFT)
			rate_offset = celt.MAX32(rate_offset, -2*st.silk_mode.BitRate/3)
			// Split the rate change between the SILK and CELT part for hybrid.
			if st.bandwidth == OPUS_BANDWIDTH_SUPERWIDEBAND || st.bandwidth == OPUS_BANDWIDTH_FULLBAND {
				st.silk_mode.BitRate += 3 * rate_offset / 5
			} else {
				st.silk_mode.BitRate += rate_offset
			}
			bytes_target += int(rate_offset) * frame_size / (8 * int(st.Fs))
		}

		st.silk_mode.PayloadSize_ms = 1000 * frame_size / int(st.Fs)
		st.silk_mode.NChannelsAPI = int32(st.channels)
		st.silk_mode.NChannelsInternal = int32(st.stream_channels)
		if curr_bandwidth == OPUS_BANDWIDTH_NARROWBAND {
			st.silk_mode.DesiredInternalSampleRate = 8000
		} else if curr_bandwidth == OPUS_BANDWIDTH_MEDIUMBAND {
			st.silk_mode.DesiredInternalSampleRate = 12000
		} else {
			silk.Assert(st.mode == MODE_HYBRID || curr_bandwidth == OPUS_BANDWIDTH_WIDEBAND)
			st.silk_mode.DesiredInternalSampleRate = 16000
		}
		if st.mode == MODE_HYBRID {
			// Don't allow bandwidth reduction at lowest bitrates in hybrid mode
			st.silk_mode.MinInternalSampleRate = 16000
		} else {
			st.silk_mode.MinInternalSampleRate = 8000
		}

		if st.mode == MODE_SILK_ONLY {
			var effective_max_rate int32 = max_rate
			st.silk_mode.MaxInternalSampleRate = 16000
			if frame_rate > 50 {
				effective_max_rate = effective_max_rate * 2 / 3
			}
			if effective_max_rate < 13000 {
				st.silk_mode.MaxInternalSampleRate = 12000
				st.silk_mode.DesiredInternalSampleRate = IMIN(12000, st.silk_mode.DesiredInternalSampleRate)
			}
			if effective_max_rate < 9600 {
				st.silk_mode.MaxInternalSampleRate = 8000
				st.silk_mode.DesiredInternalSampleRate = IMIN(8000, st.silk_mode.DesiredInternalSampleRate)
			}
		} else {
			st.silk_mode.MaxInternalSampleRate = 16000
		}

		st.silk_mode.UseCBR = b2i(st.use_vbr == 0)

		// Call SILK encoder for the low band
		nBytes = IMIN(1275, max_data_bytes-1-int32(redundancy_bytes))

		st.silk_mode.MaxBits = int(nBytes) * 8
		// Only allow up to 90% of the bits for hybrid mode
		if st.mode == MODE_HYBRID {
			st.silk_mode.MaxBits = int(int32(st.silk_mode.MaxBits) * 9 / 10)
		}
		if st.silk_mode.UseCBR != 0 {
			st.silk_mode.MaxBits = int(st.silk_mode.BitRate*int32(frame_size)/(st.Fs*8)) * 8
			// Reduce the initial target to make it easier to reach the CBR rate
			st.silk_mode.BitRate = IMAX(1, st.silk_mode.BitRate-2000)
		}

		if prefill != 0 {
			var zero int32 = 0
			var prefill_offset int
			// Use a smooth onset for the SILK prefill to avoid the encoder trying to encode
			// a discontinuity. The exact location is what we need to avoid leaving any "gap"
			// in the audio when mixing with the redundant CELT frame. Here we can afford to
			// overwrite st->delay_buffer because the only thing that uses it before it gets
			// rewritten is tmp_prefill[] and even then only the part after the ramp really
			// gets used (rather than sent to the encoder and discarded)
			prefill_offset = st.channels * (st.encoder_buffer - st.delay_compensation - int(st.Fs/400))
			gain_fade(st.delay_buffer[prefill_offset:], st.delay_buffer[prefill_offset:],
				0, celt.Q15ONE, celt_mode.Overlap(), int(st.Fs/400), st.channels, celt_mode.Window(), st.Fs)
			clear(st.delay_buffer[:prefill_offset])
			pcm_silk = st.delay_buffer[:]
			silk_enc.Encode(&st.silk_mode, pcm_silk, st.encoder_buffer, nil, &zero, 1)
		}

		pcm_silk = pcm_buf[total_buffer*st.channels:]
		ret = silk_enc.Encode(&st.silk_mode, pcm_silk, frame_size, &enc, &nBytes, 0)
		if ret != 0 {
			// fprintf (stderr, "SILK encode error: %d\n", ret);
			// Handle error
			return OPUS_INTERNAL_ERROR
		}
		if nBytes == 0 {
			st.rangeFinal = 0
			data[0] = gen_toc(st.mode, int(st.Fs)/frame_size, curr_bandwidth, st.stream_channels)
			return 1
		}
		// Extract SILK internal bandwidth for signaling in first byte
		if st.mode == MODE_SILK_ONLY {
			if st.silk_mode.InternalSampleRate == 8000 {
				curr_bandwidth = OPUS_BANDWIDTH_NARROWBAND
			} else if st.silk_mode.InternalSampleRate == 12000 {
				curr_bandwidth = OPUS_BANDWIDTH_MEDIUMBAND
			} else if st.silk_mode.InternalSampleRate == 16000 {
				curr_bandwidth = OPUS_BANDWIDTH_WIDEBAND
			}
		} else {
			silk.Assert(st.silk_mode.InternalSampleRate == 16000)
		}

		st.silk_mode.OpusCanSwitch = st.silk_mode.SwitchReady
		// FIXME: How do we allocate the redundancy for CBR?
		if st.silk_mode.OpusCanSwitch != 0 {
			redundancy = 1
			celt_to_silk = 0
			st.silk_bw_switch = 1
		}
	}

	// CELT processing
	{
		endband := 21

		switch curr_bandwidth {
		case OPUS_BANDWIDTH_NARROWBAND:
			endband = 13
		case OPUS_BANDWIDTH_MEDIUMBAND, OPUS_BANDWIDTH_WIDEBAND:
			endband = 17
		case OPUS_BANDWIDTH_SUPERWIDEBAND:
			endband = 19
		case OPUS_BANDWIDTH_FULLBAND:
			endband = 21
		}
		celt_enc.Ctl(celt.CELT_SET_END_BAND_REQUEST, int32(endband))
		celt_enc.Ctl(celt.CELT_SET_CHANNELS_REQUEST, int32(st.stream_channels))
	}
	celt_enc.Ctl(celt.OPUS_SET_BITRATE_REQUEST, int32(OPUS_BITRATE_MAX))
	if st.mode != MODE_SILK_ONLY {
		var celt_pred int32 = 2
		celt_enc.Ctl(celt.OPUS_SET_VBR_REQUEST, int32(0))
		// We may still decide to disable prediction later
		if st.silk_mode.ReducedDependency != 0 {
			celt_pred = 0
		}
		celt_enc.Ctl(celt.CELT_SET_PREDICTION_REQUEST, celt_pred)

		if st.mode == MODE_HYBRID {
			var length int

			length = (enc.Tell() + 7) >> 3
			if redundancy != 0 {
				if st.mode == MODE_HYBRID {
					length += 3
				} else {
					length += 1
				}
			}
			if st.use_vbr != 0 {
				nb_compr_bytes = length + bytes_target - int((st.silk_mode.BitRate*int32(frame_size))/(8*st.Fs))
			} else {
				// check if SILK used up too much
				if length > bytes_target {
					nb_compr_bytes = length
				} else {
					nb_compr_bytes = bytes_target
				}
			}
		} else {
			if st.use_vbr != 0 {
				var bonus int32 = 0
				if st.variable_duration == OPUS_FRAMESIZE_VARIABLE && frame_size != int(st.Fs/50) {
					bonus = int32(60*st.stream_channels+40) * (st.Fs/int32(frame_size) - 50)
					if analysis_info.Valid != 0 {
						bonus = int32(float32(bonus) * (1 + float32(.5*analysis_info.Tonality)))
					}
				}
				celt_enc.Ctl(celt.OPUS_SET_VBR_REQUEST, int32(1))
				celt_enc.Ctl(celt.OPUS_SET_VBR_CONSTRAINT_REQUEST, int32(st.vbr_constraint))
				celt_enc.Ctl(celt.OPUS_SET_BITRATE_REQUEST, st.bitrate_bps+bonus)
				nb_compr_bytes = int(max_data_bytes) - 1 - redundancy_bytes
			} else {
				nb_compr_bytes = bytes_target
			}
		}

	} else {
		nb_compr_bytes = 0
	}

	tmp_prefill := make([]int16, st.channels*int(st.Fs)/400)
	if st.mode != MODE_SILK_ONLY && st.mode != st.prev_mode && st.prev_mode > 0 {
		copy(tmp_prefill, st.delay_buffer[(st.encoder_buffer-total_buffer-int(st.Fs)/400)*st.channels:][:st.channels*int(st.Fs)/400])
	}

	if st.channels*(st.encoder_buffer-(frame_size+total_buffer)) > 0 {
		copy(st.delay_buffer[:], st.delay_buffer[st.channels*frame_size:][:st.channels*(st.encoder_buffer-frame_size-total_buffer)])
		copy(st.delay_buffer[st.channels*(st.encoder_buffer-frame_size-total_buffer):],
			pcm_buf[:(frame_size+total_buffer)*st.channels])
	} else {
		copy(st.delay_buffer[:], pcm_buf[(frame_size+total_buffer-st.encoder_buffer)*st.channels:][:st.encoder_buffer*st.channels])
	}
	// gain_fade() and stereo_fade() need to be after the buffer copying
	// because we don't want any of this to affect the SILK part
	if st.prev_HB_gain < celt.Q15ONE || HB_gain < celt.Q15ONE {
		gain_fade(pcm_buf, pcm_buf,
			st.prev_HB_gain, HB_gain, celt_mode.Overlap(), frame_size, st.channels, celt_mode.Window(), st.Fs)
	}
	st.prev_HB_gain = HB_gain
	if st.mode != MODE_HYBRID || st.stream_channels == 1 {
		st.silk_mode.StereoWidth_Q14 = int(IMIN(int32(1<<14), 2*IMAX(0, equiv_rate-30000)))
	}
	if st.energy_masking == nil && st.channels == 2 {
		// Apply stereo width reduction (at low bitrates)
		if st.hybrid_stereo_width_Q14 < (1<<14) || st.silk_mode.StereoWidth_Q14 < (1<<14) {
			var g1, g2 int16
			g1 = st.hybrid_stereo_width_Q14
			g2 = int16(st.silk_mode.StereoWidth_Q14)
			if g1 == 16384 {
				g1 = celt.Q15ONE
			} else {
				g1 = celt.SHL16(g1, 1)
			}
			if g2 == 16384 {
				g2 = celt.Q15ONE
			} else {
				g2 = celt.SHL16(g2, 1)
			}
			stereo_fade(pcm_buf, pcm_buf, g1, g2, celt_mode.Overlap(),
				frame_size, st.channels, celt_mode.Window(), st.Fs)
			st.hybrid_stereo_width_Q14 = int16(st.silk_mode.StereoWidth_Q14)
		}
	}

	if st.mode != MODE_CELT_ONLY && enc.Tell()+17+20*b2i(st.mode == MODE_HYBRID) <= 8*int(max_data_bytes-1) {
		// For SILK mode, the redundancy is inferred from the length
		if st.mode == MODE_HYBRID && (redundancy != 0 || enc.Tell()+37 <= 8*nb_compr_bytes) {
			enc.EncBitLogp(redundancy, 12)
		}
		if redundancy != 0 {
			var max_redundancy int
			enc.EncBitLogp(celt_to_silk, 1)
			if st.mode == MODE_HYBRID {
				max_redundancy = int(max_data_bytes-1) - nb_compr_bytes
			} else {
				max_redundancy = int(max_data_bytes-1) - ((enc.Tell() + 7) >> 3)
			}
			// Target the same bit-rate for redundancy as for the rest,
			// up to a max of 257 bytes
			redundancy_bytes = int(IMIN(int32(max_redundancy), st.bitrate_bps/1600))
			redundancy_bytes = IMIN(257, IMAX(2, redundancy_bytes))
			if st.mode == MODE_HYBRID {
				enc.EncUint(uint32(redundancy_bytes-2), 256)
			}
		}
	} else {
		redundancy = 0
	}

	if redundancy == 0 {
		st.silk_bw_switch = 0
		redundancy_bytes = 0
	}
	if st.mode != MODE_CELT_ONLY {
		start_band = 17
	}

	if st.mode == MODE_SILK_ONLY {
		ret = (enc.Tell() + 7) >> 3
		enc.EncDone()
		nb_compr_bytes = ret
	} else {
		nb_compr_bytes = IMIN(int(max_data_bytes-1)-redundancy_bytes, nb_compr_bytes)
		enc.EncShrink(uint32(nb_compr_bytes))
	}

	if redundancy != 0 || st.mode != MODE_SILK_ONLY {
		celt_enc.Ctl(celt.CELT_SET_ANALYSIS_REQUEST, &analysis_info)
	}

	// 5 ms redundant frame for CELT->SILK
	if redundancy != 0 && celt_to_silk != 0 {
		var err int
		celt_enc.Ctl(celt.CELT_SET_START_BAND_REQUEST, int32(0))
		celt_enc.Ctl(celt.OPUS_SET_VBR_REQUEST, int32(0))
		err = celt_enc.EncodeWithEc(pcm_buf, int(st.Fs/200), payload[nb_compr_bytes:], redundancy_bytes, nil)
		if err < 0 {
			return OPUS_INTERNAL_ERROR
		}
		celt_enc.Ctl(celt.OPUS_GET_FINAL_RANGE_REQUEST, &redundant_rng)
		celt_enc.Ctl(celt.OPUS_RESET_STATE)
	}

	celt_enc.Ctl(celt.CELT_SET_START_BAND_REQUEST, int32(start_band))

	if st.mode != MODE_SILK_ONLY {
		if st.mode != st.prev_mode && st.prev_mode > 0 {
			var dummy [2]byte
			celt_enc.Ctl(celt.OPUS_RESET_STATE)

			// Prefilling
			celt_enc.EncodeWithEc(tmp_prefill, int(st.Fs/400), dummy[:], 2, nil)
			celt_enc.Ctl(celt.CELT_SET_PREDICTION_REQUEST, int32(0))
		}
		// If false, we already busted the budget and we'll end up with a "PLC packet"
		if enc.Tell() <= 8*nb_compr_bytes {
			ret = celt_enc.EncodeWithEc(pcm_buf, frame_size, nil, nb_compr_bytes, &enc)
			if ret < 0 {
				return OPUS_INTERNAL_ERROR
			}
		}
	}

	// 5 ms redundant frame for SILK->CELT
	if redundancy != 0 && celt_to_silk == 0 {
		var err int
		var dummy [2]byte
		var N2, N4 int
		N2 = int(st.Fs / 200)
		N4 = int(st.Fs / 400)

		celt_enc.Ctl(celt.OPUS_RESET_STATE)
		celt_enc.Ctl(celt.CELT_SET_START_BAND_REQUEST, int32(0))
		celt_enc.Ctl(celt.CELT_SET_PREDICTION_REQUEST, int32(0))

		// NOTE: We could speed this up slightly (at the expense of code size) by just adding a function that prefills the buffer
		celt_enc.EncodeWithEc(pcm_buf[st.channels*(frame_size-N2-N4):], N4, dummy[:], 2, nil)

		err = celt_enc.EncodeWithEc(pcm_buf[st.channels*(frame_size-N2):], N2, payload[nb_compr_bytes:], redundancy_bytes, nil)
		if err < 0 {
			return OPUS_INTERNAL_ERROR
		}
		celt_enc.Ctl(celt.OPUS_GET_FINAL_RANGE_REQUEST, &redundant_rng)
	}

	// Signalling the mode in the first byte
	// data--;
	data[0] = gen_toc(st.mode, int(st.Fs)/frame_size, curr_bandwidth, st.stream_channels)

	st.rangeFinal = enc.Rng() ^ redundant_rng

	if to_celt != 0 {
		st.prev_mode = MODE_CELT_ONLY
	} else {
		st.prev_mode = st.mode
	}
	st.prev_channels = st.stream_channels
	st.prev_framesize = frame_size

	st.first = 0

	// In the unlikely case that the SILK encoder busted its target, tell
	// the decoder to call the PLC
	if enc.Tell() > int(max_data_bytes-1)*8 {
		if max_data_bytes < 2 {
			return OPUS_BUFFER_TOO_SMALL
		}
		data[1] = 0
		ret = 1
		st.rangeFinal = 0
	} else if st.mode == MODE_SILK_ONLY && redundancy == 0 {
		// When in LPC only mode it's perfectly
		// reasonable to strip off trailing zero bytes as
		// the required range decoder behavior is to
		// fill these in. This can't be done when the MDCT
		// modes are used because the decoder needs to know
		// the actual length for allocation purposes.
		for ret > 2 && data[ret] == 0 {
			ret--
		}
	}
	// Count ToC and redundancy
	ret += 1 + redundancy_bytes
	if st.use_vbr == 0 {
		if PacketPad(data, int32(ret), max_data_bytes) != OPUS_OK {
			return OPUS_INTERNAL_ERROR
		}
		ret = int(max_data_bytes)
	}
	return int32(ret)
}

// C: static inline opus_int16 FLOAT2INT16(float x)   (float_cast.h)
func FLOAT2INT16(x float32) int16 {
	x = x * 32768.0
	if x < -32768 {
		x = -32768
	}
	if x > 32767 {
		x = 32767
	}
	return int16(celt.Float2int(x))
}

// EncodeFloat encodes float input (fixed-point build: converts to int16 first).
//
// C: opus_int32 opus_encode_float(OpusEncoder *st, const float *pcm, int analysis_frame_size, unsigned char *data, opus_int32 max_data_bytes)   (FIXED_POINT)
func (st *OpusEncoder) EncodeFloat(pcm []float32, analysis_frame_size int,
	data []byte, max_data_bytes int32) int32 {
	var i int
	var ret int32
	var frame_size int
	var delay_compensation int

	if st.application == OPUS_APPLICATION_RESTRICTED_LOWDELAY {
		delay_compensation = 0
	} else {
		delay_compensation = st.delay_compensation
	}
	frame_size = int(ComputeFrameSize(pcm, analysis_frame_size,
		st.variable_duration, st.channels, st.Fs, int(st.bitrate_bps),
		delay_compensation, DownmixFloat, st.analysis.subframe_mem[:]))

	// The C ALLOC would take a negative size here and opus_encode_native
	// then rejects the frame size; Go's make would panic, so reject early.
	if frame_size < 0 {
		return OPUS_BAD_ARG
	}
	in := make([]int16, frame_size*st.channels)

	for i = 0; i < frame_size*st.channels; i++ {
		in[i] = FLOAT2INT16(pcm[i])
	}
	ret = st.EncodeNative(in, frame_size, data, max_data_bytes, 16,
		pcm, int32(analysis_frame_size), 0, -2, st.channels, DownmixFloat, 1)
	return ret
}

// Encode encodes one frame of 16-bit PCM. Returns the packet length or a
// negative error code.
//
// C: opus_int32 opus_encode(OpusEncoder *st, const opus_int16 *pcm, int analysis_frame_size, unsigned char *data, opus_int32 out_data_bytes)   (FIXED_POINT)
func (st *OpusEncoder) Encode(pcm []int16, analysis_frame_size int,
	data []byte, out_data_bytes int32) int32 {
	var frame_size int
	var delay_compensation int
	if st.application == OPUS_APPLICATION_RESTRICTED_LOWDELAY {
		delay_compensation = 0
	} else {
		delay_compensation = st.delay_compensation
	}
	frame_size = int(ComputeFrameSize(pcm, analysis_frame_size,
		st.variable_duration, st.channels, st.Fs, int(st.bitrate_bps),
		delay_compensation, DownmixInt, st.analysis.subframe_mem[:]))
	return st.EncodeNative(pcm, frame_size, data, out_data_bytes, 16,
		pcm, int32(analysis_frame_size), 0, -2, st.channels, DownmixInt, 0)
}

// Ctl is the C opus_encoder_ctl. Value arguments are int32 (int accepted),
// output arguments are *int32 or *uint32.
//
// C: int opus_encoder_ctl(OpusEncoder *st, int request, ...)
func (st *OpusEncoder) Ctl(request int, args ...any) int {
	var ret int
	celt_enc := &st.celt_enc

	ret = OPUS_OK

	switch request {
	case OPUS_SET_APPLICATION_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || (value != OPUS_APPLICATION_VOIP && value != OPUS_APPLICATION_AUDIO &&
			value != OPUS_APPLICATION_RESTRICTED_LOWDELAY) ||
			(st.first == 0 && st.application != int(value)) {
			ret = OPUS_BAD_ARG
			break
		}
		st.application = int(value)
	case OPUS_GET_APPLICATION_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = int32(st.application)
	case OPUS_SET_BITRATE_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok {
			return OPUS_BAD_ARG
		}
		if value != OPUS_AUTO && value != OPUS_BITRATE_MAX {
			if value <= 0 {
				return OPUS_BAD_ARG
			} else if value <= 500 {
				value = 500
			} else if value > int32(300000)*int32(st.channels) {
				value = int32(300000) * int32(st.channels)
			}
		}
		st.user_bitrate_bps = value
	case OPUS_GET_BITRATE_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = st.user_bitrate_to_bitrate(st.prev_framesize, 1276)
	case OPUS_SET_FORCE_CHANNELS_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || ((value < 1 || int(value) > st.channels) && value != OPUS_AUTO) {
			return OPUS_BAD_ARG
		}
		st.force_channels = int(value)
	case OPUS_GET_FORCE_CHANNELS_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = int32(st.force_channels)
	case OPUS_SET_MAX_BANDWIDTH_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value < OPUS_BANDWIDTH_NARROWBAND || value > OPUS_BANDWIDTH_FULLBAND {
			return OPUS_BAD_ARG
		}
		st.max_bandwidth = int(value)
		if st.max_bandwidth == OPUS_BANDWIDTH_NARROWBAND {
			st.silk_mode.MaxInternalSampleRate = 8000
		} else if st.max_bandwidth == OPUS_BANDWIDTH_MEDIUMBAND {
			st.silk_mode.MaxInternalSampleRate = 12000
		} else {
			st.silk_mode.MaxInternalSampleRate = 16000
		}
	case OPUS_GET_MAX_BANDWIDTH_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = int32(st.max_bandwidth)
	case OPUS_SET_BANDWIDTH_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || ((value < OPUS_BANDWIDTH_NARROWBAND || value > OPUS_BANDWIDTH_FULLBAND) && value != OPUS_AUTO) {
			return OPUS_BAD_ARG
		}
		st.user_bandwidth = int(value)
		if st.user_bandwidth == OPUS_BANDWIDTH_NARROWBAND {
			st.silk_mode.MaxInternalSampleRate = 8000
		} else if st.user_bandwidth == OPUS_BANDWIDTH_MEDIUMBAND {
			st.silk_mode.MaxInternalSampleRate = 12000
		} else {
			st.silk_mode.MaxInternalSampleRate = 16000
		}
	case OPUS_GET_BANDWIDTH_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = int32(st.bandwidth)
	case OPUS_SET_DTX_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value < 0 || value > 1 {
			return OPUS_BAD_ARG
		}
		st.silk_mode.UseDTX = int(value)
	case OPUS_GET_DTX_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = int32(st.silk_mode.UseDTX)
	case OPUS_SET_COMPLEXITY_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value < 0 || value > 10 {
			return OPUS_BAD_ARG
		}
		st.silk_mode.Complexity = int(value)
		celt_enc.Ctl(celt.OPUS_SET_COMPLEXITY_REQUEST, value)
	case OPUS_GET_COMPLEXITY_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = int32(st.silk_mode.Complexity)
	case OPUS_SET_INBAND_FEC_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value < 0 || value > 1 {
			return OPUS_BAD_ARG
		}
		st.silk_mode.UseInBandFEC = int(value)
	case OPUS_GET_INBAND_FEC_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = int32(st.silk_mode.UseInBandFEC)
	case OPUS_SET_PACKET_LOSS_PERC_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value < 0 || value > 100 {
			return OPUS_BAD_ARG
		}
		st.silk_mode.PacketLossPercentage = int(value)
		celt_enc.Ctl(celt.OPUS_SET_PACKET_LOSS_PERC_REQUEST, value)
	case OPUS_GET_PACKET_LOSS_PERC_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = int32(st.silk_mode.PacketLossPercentage)
	case OPUS_SET_VBR_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value < 0 || value > 1 {
			return OPUS_BAD_ARG
		}
		st.use_vbr = int(value)
		st.silk_mode.UseCBR = 1 - int(value)
	case OPUS_GET_VBR_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = int32(st.use_vbr)
	case OPUS_SET_VOICE_RATIO_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value < -1 || value > 100 {
			return OPUS_BAD_ARG
		}
		st.voice_ratio = int(value)
	case OPUS_GET_VOICE_RATIO_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = int32(st.voice_ratio)
	case OPUS_SET_VBR_CONSTRAINT_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value < 0 || value > 1 {
			return OPUS_BAD_ARG
		}
		st.vbr_constraint = int(value)
	case OPUS_GET_VBR_CONSTRAINT_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = int32(st.vbr_constraint)
	case OPUS_SET_SIGNAL_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || (value != OPUS_AUTO && value != OPUS_SIGNAL_VOICE && value != OPUS_SIGNAL_MUSIC) {
			return OPUS_BAD_ARG
		}
		st.signal_type = int(value)
	case OPUS_GET_SIGNAL_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = int32(st.signal_type)
	case OPUS_GET_LOOKAHEAD_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = st.Fs / 400
		if st.application != OPUS_APPLICATION_RESTRICTED_LOWDELAY {
			*value += int32(st.delay_compensation)
		}
	case OPUS_GET_SAMPLE_RATE_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = st.Fs
	case OPUS_GET_FINAL_RANGE_REQUEST:
		value, ok := args[0].(*uint32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = st.rangeFinal
	case OPUS_SET_LSB_DEPTH_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value < 8 || value > 24 {
			return OPUS_BAD_ARG
		}
		st.lsb_depth = int(value)
	case OPUS_GET_LSB_DEPTH_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = int32(st.lsb_depth)
	case OPUS_SET_EXPERT_FRAME_DURATION_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || (value != OPUS_FRAMESIZE_ARG && value != OPUS_FRAMESIZE_2_5_MS &&
			value != OPUS_FRAMESIZE_5_MS && value != OPUS_FRAMESIZE_10_MS &&
			value != OPUS_FRAMESIZE_20_MS && value != OPUS_FRAMESIZE_40_MS &&
			value != OPUS_FRAMESIZE_60_MS && value != OPUS_FRAMESIZE_VARIABLE) {
			return OPUS_BAD_ARG
		}
		st.variable_duration = int(value)
		celt_enc.Ctl(celt.OPUS_SET_EXPERT_FRAME_DURATION_REQUEST, value)
	case OPUS_GET_EXPERT_FRAME_DURATION_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = int32(st.variable_duration)
	case OPUS_SET_PREDICTION_DISABLED_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value > 1 || value < 0 {
			return OPUS_BAD_ARG
		}
		st.silk_mode.ReducedDependency = value
	case OPUS_GET_PREDICTION_DISABLED_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = st.silk_mode.ReducedDependency
	case OPUS_RESET_STATE:
		var dummy silk.EncControlStruct
		st.analysis.TonalityAnalysisReset()

		// OPUS_CLEAR from OPUS_ENCODER_RESET_START (stream_channels) to the
		// end of the C struct (the embedded codec states are reset below).
		st.stream_channels = 0
		st.hybrid_stereo_width_Q14 = 0
		st.variable_HP_smth2_Q15 = 0
		st.prev_HB_gain = 0
		st.hp_mem = [4]int32{}
		st.mode = 0
		st.prev_mode = 0
		st.prev_channels = 0
		st.prev_framesize = 0
		st.bandwidth = 0
		st.silk_bw_switch = 0
		st.first = 0
		st.energy_masking = nil
		st.width_mem = StereoWidthState{}
		st.delay_buffer = [MAX_ENCODER_BUFFER * 2]int16{}
		st.detected_bandwidth = 0
		st.rangeFinal = 0

		celt_enc.Ctl(celt.OPUS_RESET_STATE)
		st.silk_enc.InitEncoder(st.arch, &dummy)
		st.stream_channels = st.channels
		st.hybrid_stereo_width_Q14 = 1 << 14
		st.prev_HB_gain = celt.Q15ONE
		st.first = 1
		st.mode = MODE_HYBRID
		st.bandwidth = OPUS_BANDWIDTH_FULLBAND
		st.variable_HP_smth2_Q15 = silk.LSHIFT(silk.Lin2log(silk.VARIABLE_HP_MIN_CUTOFF_HZ), 8)
	case OPUS_SET_FORCE_MODE_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || ((value < MODE_SILK_ONLY || value > MODE_CELT_ONLY) && value != OPUS_AUTO) {
			return OPUS_BAD_ARG
		}
		st.user_forced_mode = int(value)
	case celt.OPUS_SET_LFE_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok {
			return OPUS_BAD_ARG
		}
		st.lfe = int(value)
		ret = celt_enc.Ctl(celt.OPUS_SET_LFE_REQUEST, value)
	case celt.OPUS_SET_ENERGY_MASK_REQUEST:
		// opus_val16 *value; a nil argument is the C NULL
		var value []int16
		if len(args) < 1 {
			return OPUS_BAD_ARG
		}
		if args[0] != nil {
			var ok bool
			value, ok = args[0].([]int16)
			if !ok {
				return OPUS_BAD_ARG
			}
		}
		st.energy_masking = value
		ret = celt_enc.Ctl(celt.OPUS_SET_ENERGY_MASK_REQUEST, value)
	case celt.CELT_GET_MODE_REQUEST:
		value, ok := args[0].(**celt.CELTMode)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		ret = celt_enc.Ctl(celt.CELT_GET_MODE_REQUEST, value)
	default:
		// fprintf(stderr, "unknown opus_encoder_ctl() request: %d", request);
		ret = OPUS_UNIMPLEMENTED
	}
	return ret
}

// EncoderDestroy frees the state (no-op in Go; kept for API parity).
//
// C: void opus_encoder_destroy(OpusEncoder *st)
func (st *OpusEncoder) EncoderDestroy() {}
