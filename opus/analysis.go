package opus

import (
	"math"

	"github.com/DubbaThonyVibecodes/go-libopus/celt"
)

// C: src/analysis.c, src/analysis.h

const (
	NB_FRAMES         = 8
	NB_TBANDS         = 18
	ANALYSIS_BUF_SIZE = 720 // 30 ms at 24 kHz

	// At that point we can stop counting frames because it no longer matters.
	ANALYSIS_COUNT_MAX = 10000

	DETECT_SIZE = 100
)

// Uncomment this to print the MLP features on stdout.
// #define MLP_TRAINING

// TonalityAnalysisState is the C struct TonalityAnalysisState.
type TonalityAnalysisState struct {
	arch        int
	application int
	Fs          int32
	// TONALITY_ANALYSIS_RESET_START angle
	angle              [240]float32
	d_angle            [240]float32
	d2_angle           [240]float32
	inmem              [ANALYSIS_BUF_SIZE]int32
	mem_fill           int // number of usable samples in the buffer
	prev_band_tonality [NB_TBANDS]float32
	prev_tonality      float32
	prev_bandwidth     int
	E                  [NB_FRAMES][NB_TBANDS]float32
	logE               [NB_FRAMES][NB_TBANDS]float32
	lowE               [NB_TBANDS]float32
	highE              [NB_TBANDS]float32
	meanE              [NB_TBANDS + 1]float32
	mem                [32]float32
	cmean              [8]float32
	std                [9]float32
	Etracker           float32
	lowECount          float32
	E_count            int
	count              int
	analysis_offset    int
	write_pos          int
	read_pos           int
	read_subframe      int
	hp_ener_accum      float32
	initialized        int
	rnn_state          [MAX_NEURONS]float32
	downmix_state      [3]int32
	info               [DETECT_SIZE]celt.AnalysisInfo
}

// MIN16/MAX16/MIN32/MAX32/ABS16 applied to C floats (arch.h defines them as
// plain conditionals, so they work on any type).
func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func absf(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

const TRANSITION_PENALTY = 10

var dct_table = [128]float32{
	0.250000, 0.250000, 0.250000, 0.250000, 0.250000, 0.250000, 0.250000, 0.250000,
	0.250000, 0.250000, 0.250000, 0.250000, 0.250000, 0.250000, 0.250000, 0.250000,
	0.351851, 0.338330, 0.311806, 0.273300, 0.224292, 0.166664, 0.102631, 0.034654,
	-0.034654, -0.102631, -0.166664, -0.224292, -0.273300, -0.311806, -0.338330, -0.351851,
	0.346760, 0.293969, 0.196424, 0.068975, -0.068975, -0.196424, -0.293969, -0.346760,
	-0.346760, -0.293969, -0.196424, -0.068975, 0.068975, 0.196424, 0.293969, 0.346760,
	0.338330, 0.224292, 0.034654, -0.166664, -0.311806, -0.351851, -0.273300, -0.102631,
	0.102631, 0.273300, 0.351851, 0.311806, 0.166664, -0.034654, -0.224292, -0.338330,
	0.326641, 0.135299, -0.135299, -0.326641, -0.326641, -0.135299, 0.135299, 0.326641,
	0.326641, 0.135299, -0.135299, -0.326641, -0.326641, -0.135299, 0.135299, 0.326641,
	0.311806, 0.034654, -0.273300, -0.338330, -0.102631, 0.224292, 0.351851, 0.166664,
	-0.166664, -0.351851, -0.224292, 0.102631, 0.338330, 0.273300, -0.034654, -0.311806,
	0.293969, -0.068975, -0.346760, -0.196424, 0.196424, 0.346760, 0.068975, -0.293969,
	-0.293969, 0.068975, 0.346760, 0.196424, -0.196424, -0.346760, -0.068975, 0.293969,
	0.273300, -0.166664, -0.338330, 0.034654, 0.351851, 0.102631, -0.311806, -0.224292,
	0.224292, 0.311806, -0.102631, -0.351851, -0.034654, 0.338330, 0.166664, -0.273300,
}

var analysis_window = [240]float32{
	0.000043, 0.000171, 0.000385, 0.000685, 0.001071, 0.001541, 0.002098, 0.002739,
	0.003466, 0.004278, 0.005174, 0.006156, 0.007222, 0.008373, 0.009607, 0.010926,
	0.012329, 0.013815, 0.015385, 0.017037, 0.018772, 0.020590, 0.022490, 0.024472,
	0.026535, 0.028679, 0.030904, 0.033210, 0.035595, 0.038060, 0.040604, 0.043227,
	0.045928, 0.048707, 0.051564, 0.054497, 0.057506, 0.060591, 0.063752, 0.066987,
	0.070297, 0.073680, 0.077136, 0.080665, 0.084265, 0.087937, 0.091679, 0.095492,
	0.099373, 0.103323, 0.107342, 0.111427, 0.115579, 0.119797, 0.124080, 0.128428,
	0.132839, 0.137313, 0.141849, 0.146447, 0.151105, 0.155823, 0.160600, 0.165435,
	0.170327, 0.175276, 0.180280, 0.185340, 0.190453, 0.195619, 0.200838, 0.206107,
	0.211427, 0.216797, 0.222215, 0.227680, 0.233193, 0.238751, 0.244353, 0.250000,
	0.255689, 0.261421, 0.267193, 0.273005, 0.278856, 0.284744, 0.290670, 0.296632,
	0.302628, 0.308658, 0.314721, 0.320816, 0.326941, 0.333097, 0.339280, 0.345492,
	0.351729, 0.357992, 0.364280, 0.370590, 0.376923, 0.383277, 0.389651, 0.396044,
	0.402455, 0.408882, 0.415325, 0.421783, 0.428254, 0.434737, 0.441231, 0.447736,
	0.454249, 0.460770, 0.467298, 0.473832, 0.480370, 0.486912, 0.493455, 0.500000,
	0.506545, 0.513088, 0.519630, 0.526168, 0.532702, 0.539230, 0.545751, 0.552264,
	0.558769, 0.565263, 0.571746, 0.578217, 0.584675, 0.591118, 0.597545, 0.603956,
	0.610349, 0.616723, 0.623077, 0.629410, 0.635720, 0.642008, 0.648271, 0.654508,
	0.660720, 0.666903, 0.673059, 0.679184, 0.685279, 0.691342, 0.697372, 0.703368,
	0.709330, 0.715256, 0.721144, 0.726995, 0.732807, 0.738579, 0.744311, 0.750000,
	0.755647, 0.761249, 0.766807, 0.772320, 0.777785, 0.783203, 0.788573, 0.793893,
	0.799162, 0.804381, 0.809547, 0.814660, 0.819720, 0.824724, 0.829673, 0.834565,
	0.839400, 0.844177, 0.848895, 0.853553, 0.858151, 0.862687, 0.867161, 0.871572,
	0.875920, 0.880203, 0.884421, 0.888573, 0.892658, 0.896677, 0.900627, 0.904508,
	0.908321, 0.912063, 0.915735, 0.919335, 0.922864, 0.926320, 0.929703, 0.933013,
	0.936248, 0.939409, 0.942494, 0.945503, 0.948436, 0.951293, 0.954072, 0.956773,
	0.959396, 0.961940, 0.964405, 0.966790, 0.969096, 0.971321, 0.973465, 0.975528,
	0.977510, 0.979410, 0.981228, 0.982963, 0.984615, 0.986185, 0.987671, 0.989074,
	0.990393, 0.991627, 0.992778, 0.993844, 0.994826, 0.995722, 0.996534, 0.997261,
	0.997902, 0.998459, 0.998929, 0.999315, 0.999615, 0.999829, 0.999957, 1.000000,
}

var tbands = [NB_TBANDS + 1]int{
	4, 8, 12, 16, 20, 24, 28, 32, 40, 48, 56, 64, 80, 96, 112, 136, 160, 192, 240,
}

const NB_TONAL_SKIP_BANDS = 9

// C: static opus_val32 silk_resampler_down2_hp(opus_val32 *S, opus_val32 *out, const opus_val32 *in, int inLen)
func silk_resampler_down2_hp(
	S []int32, // I/O  State vector [ 2 ]
	out []int32, // O    Output signal [ floor(len/2) ]
	in []int32, // I    Input signal [ len ]
	inLen int, // I    Number of input samples
) int32 {
	var k int
	var len2 int = inLen / 2
	var in32, out32, out32_hp, Y, X int32
	var hp_ener int64 = 0
	// Internal variables and state are in Q10 format
	for k = 0; k < len2; k++ {
		// Convert to Q10
		in32 = in[2*k]

		// All-pass section for even input sample
		Y = celt.SUB32(in32, S[0])
		X = celt.MULT16_32_Q15(celt.QCONST16(0.6074371, 15), Y)
		out32 = celt.ADD32(S[0], X)
		S[0] = celt.ADD32(in32, X)
		out32_hp = out32
		// Convert to Q10
		in32 = in[2*k+1]

		// All-pass section for odd input sample, and add to output of previous section
		Y = celt.SUB32(in32, S[1])
		X = celt.MULT16_32_Q15(celt.QCONST16(0.15063, 15), Y)
		out32 = celt.ADD32(out32, S[1])
		out32 = celt.ADD32(out32, X)
		S[1] = celt.ADD32(in32, X)

		Y = celt.SUB32(-in32, S[2])
		X = celt.MULT16_32_Q15(celt.QCONST16(0.15063, 15), Y)
		out32_hp = celt.ADD32(out32_hp, S[2])
		out32_hp = celt.ADD32(out32_hp, X)
		S[2] = celt.ADD32(-in32, X)

		hp_ener += int64(out32_hp) * int64(out32_hp)
		// Add, convert back to int16 and store to output
		out[k] = celt.HALF32(out32)
	}
	// len2 can be up to 480, so we shift by 8 more to make it fit.   (FIXED_POINT)
	hp_ener = hp_ener >> (2*celt.SIG_SHIFT + 8)
	return int32(hp_ener)
}

// C: static opus_val32 downmix_and_resample(downmix_func downmix, const void *_x, opus_val32 *y, opus_val32 S[3], int subframe, int offset, int c1, int c2, int C, int Fs)
func downmix_and_resample(downmix downmix_func, _x any, y []int32, S []int32, subframe int, offset int, c1 int, c2 int, C int, Fs int) int32 {
	var scale int32
	var j int
	var ret int32 = 0

	if subframe == 0 {
		return 0
	}
	if Fs == 48000 {
		subframe *= 2
		offset *= 2
	} else if Fs == 16000 {
		subframe = subframe * 2 / 3
		offset = offset * 2 / 3
	}
	tmp := make([]int32, subframe)

	downmix(_x, tmp, subframe, offset, c1, c2, C)
	scale = (1 << celt.SIG_SHIFT) // FIXED_POINT
	if c2 == -2 {
		scale /= int32(C)
	} else if c2 > -1 {
		scale /= 2
	}
	for j = 0; j < subframe; j++ {
		tmp[j] *= scale
	}
	if Fs == 48000 {
		ret = silk_resampler_down2_hp(S, y, tmp, subframe)
	} else if Fs == 24000 {
		copy(y[:subframe], tmp[:subframe])
	} else if Fs == 16000 {
		tmp3x := make([]int32, 3*subframe)
		// Don't do this at home! This resampler is horrible and it's only (barely)
		// usable for the purpose of the analysis because we don't care about all
		// the aliasing between 8 kHz and 12 kHz.
		for j = 0; j < subframe; j++ {
			tmp3x[3*j] = tmp[j]
			tmp3x[3*j+1] = tmp[j]
			tmp3x[3*j+2] = tmp[j]
		}
		silk_resampler_down2_hp(S, y, tmp3x, 3*subframe)
	}
	return ret
}

// C: void tonality_analysis_init(TonalityAnalysisState *tonal, opus_int32 Fs)
func (tonal *TonalityAnalysisState) TonalityAnalysisInit(Fs int32) {
	// Initialize reusable fields.
	tonal.arch = celt.SelectArch()
	tonal.Fs = Fs
	// Clear remaining fields.
	tonal.TonalityAnalysisReset()
}

// C: void tonality_analysis_reset(TonalityAnalysisState *tonal)
func (tonal *TonalityAnalysisState) TonalityAnalysisReset() {
	// Clear non-reusable fields (everything from TONALITY_ANALYSIS_RESET_START on).
	arch := tonal.arch
	application := tonal.application
	Fs := tonal.Fs
	*tonal = TonalityAnalysisState{}
	tonal.arch = arch
	tonal.application = application
	tonal.Fs = Fs
}

// C: void tonality_get_info(TonalityAnalysisState *tonal, AnalysisInfo *info_out, int len)
func (tonal *TonalityAnalysisState) TonalityGetInfo(info_out *celt.AnalysisInfo, length int) {
	var pos int
	var curr_lookahead int
	var tonality_max float32
	var tonality_avg float32
	var tonality_count int
	var i int
	var pos0 int
	var prob_avg float32
	var prob_count float32
	var prob_min, prob_max float32
	var vad_prob float32
	var mpos, vpos int
	var bandwidth_span int

	pos = tonal.read_pos
	curr_lookahead = tonal.write_pos - tonal.read_pos
	if curr_lookahead < 0 {
		curr_lookahead += DETECT_SIZE
	}

	tonal.read_subframe += length / int(tonal.Fs/400)
	for tonal.read_subframe >= 8 {
		tonal.read_subframe -= 8
		tonal.read_pos++
	}
	if tonal.read_pos >= DETECT_SIZE {
		tonal.read_pos -= DETECT_SIZE
	}

	// On long frames, look at the second analysis window rather than the first.
	if length > int(tonal.Fs/50) && pos != tonal.write_pos {
		pos++
		if pos == DETECT_SIZE {
			pos = 0
		}
	}
	if pos == tonal.write_pos {
		pos--
	}
	if pos < 0 {
		pos = DETECT_SIZE - 1
	}
	pos0 = pos
	*info_out = tonal.info[pos]
	if info_out.Valid == 0 {
		return
	}
	tonality_max = info_out.Tonality
	tonality_avg = info_out.Tonality
	tonality_count = 1
	// Look at the neighbouring frames and pick largest bandwidth found (to be safe).
	bandwidth_span = 6
	// If possible, look ahead for a tone to compensate for the delay in the tone detector.
	for i = 0; i < 3; i++ {
		pos++
		if pos == DETECT_SIZE {
			pos = 0
		}
		if pos == tonal.write_pos {
			break
		}
		tonality_max = maxf(tonality_max, tonal.info[pos].Tonality)
		tonality_avg += tonal.info[pos].Tonality
		tonality_count++
		info_out.Bandwidth = IMAX(info_out.Bandwidth, tonal.info[pos].Bandwidth)
		bandwidth_span--
	}
	pos = pos0
	// Look back in time to see if any has a wider bandwidth than the current frame.
	for i = 0; i < bandwidth_span; i++ {
		pos--
		if pos < 0 {
			pos = DETECT_SIZE - 1
		}
		if pos == tonal.write_pos {
			break
		}
		info_out.Bandwidth = IMAX(info_out.Bandwidth, tonal.info[pos].Bandwidth)
	}
	info_out.Tonality = maxf(tonality_avg/float32(tonality_count), tonality_max-.2)

	mpos = pos0
	vpos = pos0
	// If we have enough look-ahead, compensate for the ~5-frame delay in the music prob and
	// ~1 frame delay in the VAD prob.
	if curr_lookahead > 15 {
		mpos += 5
		if mpos >= DETECT_SIZE {
			mpos -= DETECT_SIZE
		}
		vpos += 1
		if vpos >= DETECT_SIZE {
			vpos -= DETECT_SIZE
		}
	}

	// The following calculations attempt to minimize a "badness function"
	// for the transition. When switching from speech to music, the badness
	// of switching at frame k is
	// b_k = S*v_k + \sum_{i=0}^{k-1} v_i*(p_i - T)
	// where
	// v_i is the activity probability (VAD) at frame i,
	// p_i is the music probability at frame i
	// T is the probability threshold for switching
	// S is the penalty for switching during active audio rather than silence
	// the current frame has index i=0
	//
	// Rather than apply badness to directly decide when to switch, what we compute
	// instead is the threshold for which the optimal switching point is now. When
	// considering whether to switch now (frame 0) or at frame k, we have:
	// S*v_0 = S*v_k + \sum_{i=0}^{k-1} v_i*(p_i - T)
	// which gives us:
	// T = ( \sum_{i=0}^{k-1} v_i*p_i + S*(v_k-v_0) ) / ( \sum_{i=0}^{k-1} v_i )
	// We take the min threshold across all positive values of k (up to the maximum
	// amount of lookahead we have) to give us the threshold for which the current
	// frame is the optimal switch point.
	//
	// The last step is that we need to consider whether we want to switch at all.
	// For that we use the average of the music probability over the entire window.
	// If the threshold is higher than that average we're not going to
	// switch, so we compute a min with the average as well. The result of all these
	// min operations is music_prob_min, which gives the threshold for switching to music
	// if we're currently encoding for speech.
	//
	// We do the exact opposite to compute music_prob_max which is used for switching
	// from music to speech.
	prob_min = 1
	prob_max = 0
	vad_prob = tonal.info[vpos].Activity_probability
	prob_count = maxf(.1, vad_prob)
	prob_avg = maxf(.1, vad_prob) * tonal.info[mpos].Music_prob
	for {
		var pos_vad float32
		mpos++
		if mpos == DETECT_SIZE {
			mpos = 0
		}
		if mpos == tonal.write_pos {
			break
		}
		vpos++
		if vpos == DETECT_SIZE {
			vpos = 0
		}
		if vpos == tonal.write_pos {
			break
		}
		pos_vad = tonal.info[vpos].Activity_probability
		prob_min = minf((prob_avg-float32(TRANSITION_PENALTY*(vad_prob-pos_vad)))/prob_count, prob_min)
		prob_max = maxf((prob_avg+float32(TRANSITION_PENALTY*(vad_prob-pos_vad)))/prob_count, prob_max)
		prob_count += maxf(.1, pos_vad)
		prob_avg += float32(maxf(.1, pos_vad) * tonal.info[mpos].Music_prob)
	}
	info_out.Music_prob = prob_avg / prob_count
	prob_min = minf(prob_avg/prob_count, prob_min)
	prob_max = maxf(prob_avg/prob_count, prob_max)
	prob_min = maxf(prob_min, 0)
	prob_max = minf(prob_max, 1)

	// If we don't have enough look-ahead, do our best to make a decent decision.
	if curr_lookahead < 10 {
		var pmin, pmax float32
		pmin = prob_min
		pmax = prob_max
		pos = pos0
		// Look for min/max in the past.
		for i = 0; i < IMIN(tonal.count-1, 15); i++ {
			pos--
			if pos < 0 {
				pos = DETECT_SIZE - 1
			}
			pmin = minf(pmin, tonal.info[pos].Music_prob)
			pmax = maxf(pmax, tonal.info[pos].Music_prob)
		}
		// Bias against switching on active audio.
		pmin = maxf(0, pmin-float32(.1*vad_prob))
		pmax = minf(1, pmax+float32(.1*vad_prob))
		prob_min += float32((1 - float32(.1*float32(curr_lookahead))) * (pmin - prob_min))
		prob_max += float32((1 - float32(.1*float32(curr_lookahead))) * (pmax - prob_max))
	}
	info_out.Music_prob_min = prob_min
	info_out.Music_prob_max = prob_max

	// printf("%f %f %f %f %f\n", prob_min, prob_max, prob_avg/prob_count, vad_prob, info_out->music_prob);
}

var std_feature_bias = [9]float32{
	5.684947, 3.475288, 1.770634, 1.599784, 3.773215,
	2.163313, 1.260756, 1.116868, 1.918795,
}

const LEAKAGE_OFFSET float32 = 2.5
const LEAKAGE_SLOPE float32 = 2.0

// For fixed-point, the input is +/-2^15 shifted up by SIG_SHIFT, so we need to
// compensate for that in the energy.
//
// C: #define SCALE_COMPENS (1.f/((opus_int32)1<<(15+SIG_SHIFT)))
const SCALE_COMPENS float32 = 1.0 / float32(int32(1)<<(15+celt.SIG_SHIFT))

// C: #define SCALE_ENER(e) ((SCALE_COMPENS*SCALE_COMPENS)*(e))   (FIXED_POINT)
func SCALE_ENER(e float32) float32 { return float32(SCALE_COMPENS*SCALE_COMPENS) * e }

// C: static int is_digital_silence32(const opus_val32* pcm, int frame_size, int channels, int lsb_depth)   (FIXED_POINT)
func is_digital_silence32(pcm []int32, frame_size int, channels int, lsb_depth int) int {
	var silence int = 0
	var sample_max int32 = 0
	sample_max = celt.Maxabs32(pcm, frame_size*channels)

	silence = b2i(sample_max == 0)
	_ = lsb_depth
	return silence
}

// C: static void tonality_analysis(TonalityAnalysisState *tonal, const CELTMode *celt_mode, const void *x, int len, int offset, int c1, int c2, int C, int lsb_depth, downmix_func downmix)
func (tonal *TonalityAnalysisState) tonality_analysis(celt_mode *celt.CELTMode, x any, length int, offset int, c1 int, c2 int, C int, lsb_depth int, downmix downmix_func) {
	var i, b int
	var N int = 480
	var N2 int = 240
	A := tonal.angle[:]
	dA := tonal.d_angle[:]
	d2A := tonal.d2_angle[:]
	var band_tonality [NB_TBANDS]float32
	var logE [NB_TBANDS]float32
	var BFCC [8]float32
	var features [25]float32
	var frame_tonality float32
	var max_frame_tonality float32
	// float tw_sum=0;
	var frame_noisiness float32
	const pi4 float32 = float32(math.Pi * math.Pi * math.Pi * math.Pi)
	var slope float32 = 0
	var frame_stationarity float32
	var relativeE float32
	var frame_probs [2]float32
	var alpha, alphaE, alphaE2 float32
	var frame_loudness float32
	var bandwidth_mask float32
	var is_masked [NB_TBANDS + 1]int
	var bandwidth int = 0
	var maxE float32 = 0
	var noise_floor float32
	var remaining int
	var info *celt.AnalysisInfo
	var hp_ener float32
	var tonality2 [240]float32
	var midE [8]float32
	var spec_variability float32 = 0
	var band_log2 [NB_TBANDS + 1]float32
	var leakage_from [NB_TBANDS + 1]float32
	var leakage_to [NB_TBANDS + 1]float32
	var layer_out [MAX_NEURONS]float32
	var below_max_pitch float32
	var above_max_pitch float32
	var is_silence int

	if tonal.initialized == 0 {
		tonal.mem_fill = 240
		tonal.initialized = 1
	}
	alpha = 1.0 / float32(IMIN(10, 1+tonal.count))
	alphaE = 1.0 / float32(IMIN(25, 1+tonal.count))
	// Noise floor related decay for bandwidth detection: -2.2 dB/second
	alphaE2 = 1.0 / float32(IMIN(100, 1+tonal.count))
	if tonal.count <= 1 {
		alphaE2 = 1
	}

	if tonal.Fs == 48000 {
		// len and offset are now at 24 kHz.
		length /= 2
		offset /= 2
	} else if tonal.Fs == 16000 {
		length = 3 * length / 2
		offset = 3 * offset / 2
	}

	kfft := celt_mode.Kfft(0)
	tonal.hp_ener_accum += float32(downmix_and_resample(downmix, x,
		tonal.inmem[tonal.mem_fill:], tonal.downmix_state[:],
		IMIN(length, ANALYSIS_BUF_SIZE-tonal.mem_fill), offset, c1, c2, C, int(tonal.Fs)))
	if tonal.mem_fill+length < ANALYSIS_BUF_SIZE {
		tonal.mem_fill += length
		// Don't have enough to update the analysis
		return
	}
	hp_ener = tonal.hp_ener_accum
	info = &tonal.info[tonal.write_pos]
	tonal.write_pos++
	if tonal.write_pos >= DETECT_SIZE {
		tonal.write_pos -= DETECT_SIZE
	}

	is_silence = is_digital_silence32(tonal.inmem[:], ANALYSIS_BUF_SIZE, 1, lsb_depth)

	in := make([]celt.KissFftCpx, 480)
	out := make([]celt.KissFftCpx, 480)
	tonality := make([]float32, 240)
	noisiness := make([]float32, 240)
	for i = 0; i < N2; i++ {
		var w float32 = analysis_window[i]
		in[i].R = int32(w * float32(tonal.inmem[i]))
		in[i].I = int32(w * float32(tonal.inmem[N2+i]))
		in[N-i-1].R = int32(w * float32(tonal.inmem[N-i-1]))
		in[N-i-1].I = int32(w * float32(tonal.inmem[N+N2-i-1]))
	}
	copy(tonal.inmem[:240], tonal.inmem[ANALYSIS_BUF_SIZE-240:])
	remaining = length - (ANALYSIS_BUF_SIZE - tonal.mem_fill)
	tonal.hp_ener_accum = float32(downmix_and_resample(downmix, x,
		tonal.inmem[240:], tonal.downmix_state[:], remaining,
		offset+ANALYSIS_BUF_SIZE-tonal.mem_fill, c1, c2, C, int(tonal.Fs)))
	tonal.mem_fill = 240 + remaining
	if is_silence != 0 {
		// On silence, copy the previous analysis.
		var prev_pos int = tonal.write_pos - 2
		if prev_pos < 0 {
			prev_pos += DETECT_SIZE
		}
		*info = tonal.info[prev_pos]
		return
	}
	celt.Fft(kfft, in, out, tonal.arch)

	for i = 1; i < N2; i++ {
		var X1r, X2r, X1i, X2i float32
		var angle, d_angle, d2_angle float32
		var angle2, d_angle2, d2_angle2 float32
		var mod1, mod2, avg_mod float32
		X1r = float32(out[i].R) + float32(out[N-i].R)
		X1i = float32(out[i].I) - float32(out[N-i].I)
		X2r = float32(out[i].I) + float32(out[N-i].I)
		X2i = float32(out[N-i].R) - float32(out[i].R)

		angle = float32(.5/math.Pi) * celt.FastAtan2f(X1i, X1r)
		d_angle = angle - A[i]
		d2_angle = d_angle - dA[i]

		angle2 = float32(.5/math.Pi) * celt.FastAtan2f(X2i, X2r)
		d_angle2 = angle2 - angle
		d2_angle2 = d_angle2 - d_angle

		mod1 = d2_angle - float32(celt.Float2int(d2_angle))
		noisiness[i] = absf(mod1)
		mod1 *= mod1
		mod1 *= mod1

		mod2 = d2_angle2 - float32(celt.Float2int(d2_angle2))
		noisiness[i] += absf(mod2)
		mod2 *= mod2
		mod2 *= mod2

		avg_mod = .25 * (d2A[i] + mod1 + float32(2*mod2))
		// This introduces an extra delay of 2 frames in the detection.
		tonality[i] = 1/(1+float32(float32(40*16*pi4)*avg_mod)) - .015
		// No delay on this detection, but it's less reliable.
		tonality2[i] = 1/(1+float32(float32(40*16*pi4)*mod2)) - .015

		A[i] = angle2
		dA[i] = d_angle2
		d2A[i] = mod2
	}
	for i = 2; i < N2-1; i++ {
		var tt float32 = minf(tonality2[i], maxf(tonality2[i-1], tonality2[i+1]))
		tonality[i] = .9 * maxf(tonality[i], tt-.1)
	}
	frame_tonality = 0
	max_frame_tonality = 0
	// tw_sum = 0;
	info.Activity = 0
	frame_noisiness = 0
	frame_stationarity = 0
	if tonal.count == 0 {
		for b = 0; b < NB_TBANDS; b++ {
			tonal.lowE[b] = 1e10
			tonal.highE[b] = -1e10
		}
	}
	relativeE = 0
	frame_loudness = 0
	// The energy of the very first band is special because of DC.
	{
		var E float32 = 0
		var X1r, X2r float32
		X1r = 2 * float32(out[0].R)
		X2r = 2 * float32(out[0].I)
		E = float32(X1r*X1r) + float32(X2r*X2r)
		for i = 1; i < 4; i++ {
			var binE float32 = float32(float32(out[i].R)*float32(out[i].R)) + float32(float32(out[N-i].R)*float32(out[N-i].R)) +
				float32(float32(out[i].I)*float32(out[i].I)) + float32(float32(out[N-i].I)*float32(out[N-i].I))
			E += binE
		}
		E = SCALE_ENER(E)
		band_log2[0] = float32(float32(.5)*float32(1.442695)) * float32(math.Log(float64(E+1e-10)))
	}
	for b = 0; b < NB_TBANDS; b++ {
		var E, tE, nE float32 = 0, 0, 0
		var L1, L2 float32
		var stationarity float32
		for i = tbands[b]; i < tbands[b+1]; i++ {
			var binE float32 = float32(float32(out[i].R)*float32(out[i].R)) + float32(float32(out[N-i].R)*float32(out[N-i].R)) +
				float32(float32(out[i].I)*float32(out[i].I)) + float32(float32(out[N-i].I)*float32(out[N-i].I))
			binE = SCALE_ENER(binE)
			E += binE
			tE += float32(binE * maxf(0, tonality[i]))
			nE += float32(float32(binE*2) * (.5 - noisiness[i]))
		}

		tonal.E[tonal.E_count][b] = E
		frame_noisiness += nE / (1e-15 + E)

		frame_loudness += float32(math.Sqrt(float64(E + 1e-10)))
		logE[b] = float32(math.Log(float64(E + 1e-10)))
		band_log2[b+1] = float32(float32(.5)*float32(1.442695)) * float32(math.Log(float64(E+1e-10)))
		tonal.logE[tonal.E_count][b] = logE[b]
		if tonal.count == 0 {
			tonal.lowE[b] = logE[b]
			tonal.highE[b] = tonal.lowE[b]
		}
		if float64(tonal.highE[b]) > float64(tonal.lowE[b])+7.5 {
			if tonal.highE[b]-logE[b] > logE[b]-tonal.lowE[b] {
				tonal.highE[b] -= .01
			} else {
				tonal.lowE[b] += .01
			}
		}
		if logE[b] > tonal.highE[b] {
			tonal.highE[b] = logE[b]
			tonal.lowE[b] = maxf(tonal.highE[b]-15, tonal.lowE[b])
		} else if logE[b] < tonal.lowE[b] {
			tonal.lowE[b] = logE[b]
			tonal.highE[b] = minf(tonal.lowE[b]+15, tonal.highE[b])
		}
		relativeE += (logE[b] - tonal.lowE[b]) / (1e-5 + (tonal.highE[b] - tonal.lowE[b]))

		L1 = 0
		L2 = 0
		for i = 0; i < NB_FRAMES; i++ {
			L1 += float32(math.Sqrt(float64(tonal.E[i][b])))
			L2 += tonal.E[i][b]
		}

		stationarity = minf(0.99, L1/float32(math.Sqrt(1e-15+float64(float32(NB_FRAMES*L2)))))
		stationarity *= stationarity
		stationarity *= stationarity
		frame_stationarity += stationarity
		// band_tonality[b] = tE/(1e-15+E)
		band_tonality[b] = maxf(tE/(1e-15+E), float32(stationarity*tonal.prev_band_tonality[b]))
		frame_tonality += band_tonality[b]
		if b >= NB_TBANDS-NB_TONAL_SKIP_BANDS {
			frame_tonality -= band_tonality[b-NB_TBANDS+NB_TONAL_SKIP_BANDS]
		}
		max_frame_tonality = maxf(max_frame_tonality, float32((1+float32(.03*float32(b-NB_TBANDS)))*frame_tonality))
		slope += float32(band_tonality[b] * float32(b-8))
		// printf("%f %f ", band_tonality[b], stationarity);
		tonal.prev_band_tonality[b] = band_tonality[b]
	}

	leakage_from[0] = band_log2[0]
	leakage_to[0] = band_log2[0] - LEAKAGE_OFFSET
	for b = 1; b < NB_TBANDS+1; b++ {
		var leak_slope float32 = float32(LEAKAGE_SLOPE*float32(tbands[b]-tbands[b-1])) / 4
		leakage_from[b] = minf(leakage_from[b-1]+leak_slope, band_log2[b])
		leakage_to[b] = maxf(leakage_to[b-1]-leak_slope, band_log2[b]-LEAKAGE_OFFSET)
	}
	for b = NB_TBANDS - 2; b >= 0; b-- {
		var leak_slope float32 = float32(LEAKAGE_SLOPE*float32(tbands[b+1]-tbands[b])) / 4
		leakage_from[b] = minf(leakage_from[b+1]+leak_slope, leakage_from[b])
		leakage_to[b] = maxf(leakage_to[b+1]-leak_slope, leakage_to[b])
	}
	celt.Assert(NB_TBANDS+1 <= celt.LEAK_BANDS)
	for b = 0; b < NB_TBANDS+1; b++ {
		// leak_boost[] is made up of two terms. The first, based on leakage_to[],
		// represents the boost needed to overcome the amount of analysis leakage
		// cause in a weaker band b by louder neighbouring bands.
		// The second, based on leakage_from[], applies to a loud band b for
		// which the quantization noise causes synthesis leakage to the weaker
		// neighbouring bands.
		var boost float32 = maxf(0, leakage_to[b]-band_log2[b]) +
			maxf(0, band_log2[b]-(leakage_from[b]+LEAKAGE_OFFSET))
		info.Leak_boost[b] = uint8(IMIN(255, int(math.Floor(.5+float64(float32(64*boost))))))
	}
	for ; b < celt.LEAK_BANDS; b++ {
		info.Leak_boost[b] = 0
	}

	for i = 0; i < NB_FRAMES; i++ {
		var j int
		var mindist float32 = 1e15
		for j = 0; j < NB_FRAMES; j++ {
			var k int
			var dist float32 = 0
			for k = 0; k < NB_TBANDS; k++ {
				var tmp float32
				tmp = tonal.logE[i][k] - tonal.logE[j][k]
				dist += float32(tmp * tmp)
			}
			if j != i {
				mindist = minf(mindist, dist)
			}
		}
		spec_variability += mindist
	}
	spec_variability = float32(math.Sqrt(float64(spec_variability / NB_FRAMES / NB_TBANDS)))
	bandwidth_mask = 0
	bandwidth = 0
	maxE = 0
	noise_floor = 5.7e-4 / float32(int(1)<<uint(IMAX(0, lsb_depth-8)))
	noise_floor *= noise_floor
	below_max_pitch = 0
	above_max_pitch = 0
	for b = 0; b < NB_TBANDS; b++ {
		var E float32 = 0
		var Em float32
		var band_start, band_end int
		// Keep a margin of 300 Hz for aliasing
		band_start = tbands[b]
		band_end = tbands[b+1]
		for i = band_start; i < band_end; i++ {
			var binE float32 = float32(float32(out[i].R)*float32(out[i].R)) + float32(float32(out[N-i].R)*float32(out[N-i].R)) +
				float32(float32(out[i].I)*float32(out[i].I)) + float32(float32(out[N-i].I)*float32(out[N-i].I))
			E += binE
		}
		E = SCALE_ENER(E)
		maxE = maxf(maxE, E)
		if band_start < 64 {
			below_max_pitch += E
		} else {
			above_max_pitch += E
		}
		tonal.meanE[b] = maxf(float32((1-alphaE2)*tonal.meanE[b]), E)
		Em = maxf(E, tonal.meanE[b])
		// Consider the band "active" only if all these conditions are met:
		// 1) less than 90 dB below the peak band (maximal masking possible considering
		//    both the ATH and the loudness-dependent slope of the spreading function)
		// 2) above the PCM quantization noise floor
		// We use b+1 because the first CELT band isn't included in tbands[]
		if float32(E*1e9) > maxE && (Em > float32(float32(3*noise_floor)*float32(band_end-band_start)) || E > float32(noise_floor*float32(band_end-band_start))) {
			bandwidth = b + 1
		}
		// Check if the band is masked (see below).
		{
			var r float32 = .05
			if tonal.prev_bandwidth >= b+1 {
				r = .01
			}
			is_masked[b] = b2i(E < float32(r*bandwidth_mask))
		}
		// Use a simple follower with 13 dB/Bark slope for spreading function.
		bandwidth_mask = maxf(float32(.05*bandwidth_mask), E)
	}
	// Special case for the last two bands, for which we don't have spectrum but only
	// the energy above 12 kHz. The difficulty here is that the high-pass we use
	// leaks some LF energy, so we need to increase the threshold without accidentally cutting
	// off the band.
	if tonal.Fs == 48000 {
		var noise_ratio float32
		var Em float32
		var E float32 = hp_ener * float32(1.0/(60*60))
		if tonal.prev_bandwidth == 20 {
			noise_ratio = 10
		} else {
			noise_ratio = 30
		}

		// silk_resampler_down2_hp() shifted right by an extra 8 bits.   (FIXED_POINT)
		E *= float32(float32(256.0*float32(1.0/float32(celt.Q15ONE))) * float32(1.0/float32(celt.Q15ONE)))
		above_max_pitch += E
		tonal.meanE[b] = maxf(float32((1-alphaE2)*tonal.meanE[b]), E)
		Em = maxf(E, tonal.meanE[b])
		if Em > float32(float32(float32(3*noise_ratio)*noise_floor)*160) || E > float32(float32(noise_ratio*noise_floor)*160) {
			bandwidth = 20
		}
		// Check if the band is masked (see below).
		{
			var r float32 = .05
			if tonal.prev_bandwidth == 20 {
				r = .01
			}
			is_masked[b] = b2i(E < float32(r*bandwidth_mask))
		}
	}
	if above_max_pitch > below_max_pitch {
		info.Max_pitch_ratio = below_max_pitch / above_max_pitch
	} else {
		info.Max_pitch_ratio = 1
	}
	// In some cases, resampling aliasing can create a small amount of energy in the first band
	// being cut. So if the last band is masked, we don't include it.
	if bandwidth == 20 && is_masked[NB_TBANDS] != 0 {
		bandwidth -= 2
	} else if bandwidth > 0 && bandwidth <= NB_TBANDS && is_masked[bandwidth-1] != 0 {
		bandwidth--
	}
	if tonal.count <= 2 {
		bandwidth = 20
	}
	frame_loudness = 20 * float32(math.Log10(float64(frame_loudness)))
	tonal.Etracker = maxf(tonal.Etracker-.003, frame_loudness)
	tonal.lowECount *= (1 - alphaE)
	if frame_loudness < tonal.Etracker-30 {
		tonal.lowECount += alphaE
	}

	for i = 0; i < 8; i++ {
		var sum float32 = 0
		for b = 0; b < 16; b++ {
			sum += float32(dct_table[i*16+b] * logE[b])
		}
		BFCC[i] = sum
	}
	for i = 0; i < 8; i++ {
		var sum float32 = 0
		for b = 0; b < 16; b++ {
			sum += float32(float32(dct_table[i*16+b]*.5) * (tonal.highE[b] + tonal.lowE[b]))
		}
		midE[i] = sum
	}

	frame_stationarity /= NB_TBANDS
	relativeE /= NB_TBANDS
	if tonal.count < 10 {
		relativeE = .5
	}
	frame_noisiness /= NB_TBANDS
	info.Activity = frame_noisiness + float32((1-frame_noisiness)*relativeE)
	frame_tonality = (max_frame_tonality / (NB_TBANDS - NB_TONAL_SKIP_BANDS))
	frame_tonality = maxf(frame_tonality, float32(tonal.prev_tonality*.8))
	tonal.prev_tonality = frame_tonality

	slope /= 8 * 8
	info.Tonality_slope = slope

	tonal.E_count = (tonal.E_count + 1) % NB_FRAMES
	tonal.count = IMIN(tonal.count+1, ANALYSIS_COUNT_MAX)
	info.Tonality = frame_tonality

	for i = 0; i < 4; i++ {
		features[i] = float32(-0.12299*(BFCC[i]+tonal.mem[i+24])) + float32(0.49195*(tonal.mem[i]+tonal.mem[i+16])) + float32(0.69693*tonal.mem[i+8]) - float32(1.4349*tonal.cmean[i])
	}

	for i = 0; i < 4; i++ {
		tonal.cmean[i] = float32((1-alpha)*tonal.cmean[i]) + float32(alpha*BFCC[i])
	}

	for i = 0; i < 4; i++ {
		features[4+i] = float32(0.63246*(BFCC[i]-tonal.mem[i+24])) + float32(0.31623*(tonal.mem[i]-tonal.mem[i+16]))
	}
	for i = 0; i < 3; i++ {
		features[8+i] = float32(0.53452*(BFCC[i]+tonal.mem[i+24])) - float32(0.26726*(tonal.mem[i]+tonal.mem[i+16])) - float32(0.53452*tonal.mem[i+8])
	}

	if tonal.count > 5 {
		for i = 0; i < 9; i++ {
			tonal.std[i] = float32((1-alpha)*tonal.std[i]) + float32(float32(alpha*features[i])*features[i])
		}
	}
	for i = 0; i < 4; i++ {
		features[i] = BFCC[i] - midE[i]
	}

	for i = 0; i < 8; i++ {
		tonal.mem[i+24] = tonal.mem[i+16]
		tonal.mem[i+16] = tonal.mem[i+8]
		tonal.mem[i+8] = tonal.mem[i]
		tonal.mem[i] = BFCC[i]
	}
	for i = 0; i < 9; i++ {
		features[11+i] = float32(math.Sqrt(float64(tonal.std[i]))) - std_feature_bias[i]
	}
	features[18] = spec_variability - 0.78
	features[20] = info.Tonality - 0.154723
	features[21] = info.Activity - 0.724643
	features[22] = frame_stationarity - 0.743717
	features[23] = info.Tonality_slope + 0.069216
	features[24] = tonal.lowECount - 0.067930

	compute_dense(&layer0, layer_out[:], features[:])
	compute_gru(&layer1, tonal.rnn_state[:], layer_out[:])
	compute_dense(&layer2, frame_probs[:], tonal.rnn_state[:])

	// Probability of speech or music vs noise
	info.Activity_probability = frame_probs[1]
	info.Music_prob = frame_probs[0]

	// printf("%f %f %f\n", frame_probs[0], frame_probs[1], info->music_prob);
	/*#ifdef MLP_TRAINING
	for (i=0;i<25;i++)
	   printf("%f ", features[i]);
	printf("\n");
	#endif*/

	info.Bandwidth = bandwidth
	tonal.prev_bandwidth = bandwidth
	// printf("%d %d\n", info->bandwidth, info->opus_bandwidth);
	info.Noisiness = frame_noisiness
	info.Valid = 1
}

// C: void run_analysis(TonalityAnalysisState *analysis, const CELTMode *celt_mode, const void *analysis_pcm,
//
//	int analysis_frame_size, int frame_size, int c1, int c2, int C, opus_int32 Fs,
//	int lsb_depth, downmix_func downmix, AnalysisInfo *analysis_info)
func (analysis *TonalityAnalysisState) RunAnalysis(celt_mode *celt.CELTMode, analysis_pcm any,
	analysis_frame_size int, frame_size int, c1 int, c2 int, C int, Fs int32,
	lsb_depth int, downmix downmix_func, analysis_info *celt.AnalysisInfo) {
	var offset int
	var pcm_len int

	analysis_frame_size -= analysis_frame_size & 1
	if analysis_pcm != nil {
		// Avoid overflow/wrap-around of the analysis buffer
		analysis_frame_size = IMIN((DETECT_SIZE-5)*int(Fs)/50, analysis_frame_size)

		pcm_len = analysis_frame_size - analysis.analysis_offset
		offset = analysis.analysis_offset
		for pcm_len > 0 {
			analysis.tonality_analysis(celt_mode, analysis_pcm, IMIN(int(Fs/50), pcm_len), offset, c1, c2, C, lsb_depth, downmix)
			offset += int(Fs / 50)
			pcm_len -= int(Fs / 50)
		}
		analysis.analysis_offset = analysis_frame_size

		analysis.analysis_offset -= frame_size
	}

	analysis.TonalityGetInfo(analysis_info, frame_size)
}
