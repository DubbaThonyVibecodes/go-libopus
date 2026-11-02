package celt

// C: celt/celt.h, celt/celt.c — the parts of CELT shared by the encoder
// (celt_encoder.go) and the decoder (celt_decoder.go).

// PACKAGE_VERSION mirrors the upstream version this port tracks.
const PACKAGE_VERSION = "1.3.1"

// AnalysisInfo is the C struct AnalysisInfo (celt.h). It is filled by the
// opus package's analysis code (analysis.c) and passed to the CELT encoder
// with CELT_SET_ANALYSIS.
// C: #define LEAK_BANDS 19
const LEAK_BANDS = 19

type AnalysisInfo struct {
	Valid                int
	Tonality             float32
	Tonality_slope       float32
	Noisiness            float32
	Activity             float32
	Music_prob           float32
	Music_prob_min       float32
	Music_prob_max       float32
	Bandwidth            int
	Activity_probability float32
	Max_pitch_ratio      float32
	// Store as Q6 char to save space.
	Leak_boost [LEAK_BANDS]uint8
}

// SILKInfo is the C struct SILKInfo (celt.h), passed to the CELT encoder
// with CELT_SET_SILK_INFO.
type SILKInfo struct {
	SignalType int
	Offset     int
}

var trim_icdf = [11]uint8{126, 124, 119, 109, 87, 41, 19, 9, 4, 2, 0}

// Probs: NONE: 21.875%, LIGHT: 6.25%, NORMAL: 65.625%, AGGRESSIVE: 6.25%
var spread_icdf = [4]uint8{25, 23, 2, 0}

var tapset_icdf = [3]uint8{2, 1, 0}

const (
	COMBFILTER_MAXPERIOD = 1024
	COMBFILTER_MINPERIOD = 15
)

// C: int resampling_factor(opus_int32 rate)
// ResamplingFactor is resampling_factor (non-static in celt.h since 1.1).
func ResamplingFactor(rate int32) int { return resampling_factor(rate) }

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

// comb_filter_const_c is the constant-filter part of comb_filter. y[yOff:]
// is the output and x[xOff:] the input; the input is read back to xOff-T-2.
//
// C: static void comb_filter_const_c(opus_val32 *y, opus_val32 *x, int T, int N,
//
//	opus_val16 g10, opus_val16 g11, opus_val16 g12)
//
// C (pitch.h since 1.5): #define comb_filter_const(y, x, T, N, g10, g11, g12, arch) ((void)(arch),comb_filter_const_c(y, x, T, N, g10, g11, g12))
func comb_filter_const(y []int32, yOff int, x []int32, xOff int, T, N int,
	g10, g11, g12 int16, arch int) {
	_ = arch
	var x0, x1, x2, x3, x4 int32
	var i int
	x4 = x[xOff-T-2]
	x3 = x[xOff-T-1]
	x2 = x[xOff-T]
	x1 = x[xOff-T+1]
	for i = 0; i < N; i++ {
		x0 = x[xOff+i-T+2]
		y[yOff+i] = x[xOff+i] +
			MULT16_32_Q15(g10, x2) +
			MULT16_32_Q15(g11, ADD32(x1, x3)) +
			MULT16_32_Q15(g12, ADD32(x0, x4))
		y[yOff+i] = SATURATE(y[yOff+i], SIG_SAT)
		x4 = x3
		x3 = x2
		x2 = x1
		x1 = x0
	}
}

var comb_gains = [3][3]int16{
	{QCONST16(0.3066406250, 15), QCONST16(0.2170410156, 15), QCONST16(0.1296386719, 15)},
	{QCONST16(0.4638671875, 15), QCONST16(0.2680664062, 15), QCONST16(0.0, 15)},
	{QCONST16(0.7998046875, 15), QCONST16(0.1000976562, 15), QCONST16(0.0, 15)},
}

// comb_filter applies the pitch pre/post-filter. y[yOff:] is the output and
// x[xOff:] the input; the input is read back to xOff-T1-2, so the C pointer
// arithmetic is preserved by passing base slices with offsets.
//
// C: void comb_filter(opus_val32 *y, opus_val32 *x, int T0, int T1, int N,
//
//	opus_val16 g0, opus_val16 g1, int tapset0, int tapset1,
//	const opus_val16 *window, int overlap, int arch)
func comb_filter(y []int32, yOff int, x []int32, xOff int, T0, T1, N int,
	g0, g1 int16, tapset0, tapset1 int,
	window []int16, overlap int, arch int) {
	var i int
	// printf ("%d %d %f %f\n", T0, T1, g0, g1);
	var g00, g01, g02, g10, g11, g12 int16
	var x0, x1, x2, x3, x4 int32
	gains := &comb_gains

	if g0 == 0 && g1 == 0 {
		// OPT: Happens to work without the OPUS_MOVE(), but only because the current encoder already copies x to y
		if &x[xOff] != &y[yOff] {
			copy(y[yOff:yOff+N], x[xOff:xOff+N])
		}
		return
	}
	// When the gain is zero, T0 and/or T1 is set to zero. We need
	// to have then be at least 2 to avoid processing garbage data.
	T0 = IMAX(T0, COMBFILTER_MINPERIOD)
	T1 = IMAX(T1, COMBFILTER_MINPERIOD)
	g00 = int16(MULT16_16_P15(g0, gains[tapset0][0]))
	g01 = int16(MULT16_16_P15(g0, gains[tapset0][1]))
	g02 = int16(MULT16_16_P15(g0, gains[tapset0][2]))
	g10 = int16(MULT16_16_P15(g1, gains[tapset1][0]))
	g11 = int16(MULT16_16_P15(g1, gains[tapset1][1]))
	g12 = int16(MULT16_16_P15(g1, gains[tapset1][2]))
	x1 = x[xOff-T1+1]
	x2 = x[xOff-T1]
	x3 = x[xOff-T1-1]
	x4 = x[xOff-T1-2]
	// If the filter didn't change, we don't need the overlap
	if g0 == g1 && T0 == T1 && tapset0 == tapset1 {
		overlap = 0
	}
	for i = 0; i < overlap; i++ {
		var f int16
		x0 = x[xOff+i-T1+2]
		f = int16(MULT16_16_Q15(window[i], window[i]))
		y[yOff+i] = x[xOff+i] +
			MULT16_32_Q15(MULT16_16_Q15(int32(Q15ONE)-int32(f), g00), x[xOff+i-T0]) +
			MULT16_32_Q15(MULT16_16_Q15(int32(Q15ONE)-int32(f), g01), ADD32(x[xOff+i-T0+1], x[xOff+i-T0-1])) +
			MULT16_32_Q15(MULT16_16_Q15(int32(Q15ONE)-int32(f), g02), ADD32(x[xOff+i-T0+2], x[xOff+i-T0-2])) +
			MULT16_32_Q15(MULT16_16_Q15(f, g10), x2) +
			MULT16_32_Q15(MULT16_16_Q15(f, g11), ADD32(x1, x3)) +
			MULT16_32_Q15(MULT16_16_Q15(f, g12), ADD32(x0, x4))
		y[yOff+i] = SATURATE(y[yOff+i], SIG_SAT)
		x4 = x3
		x3 = x2
		x2 = x1
		x1 = x0

	}
	if g1 == 0 {
		// OPT: Happens to work without the OPUS_MOVE(), but only because the current encoder already copies x to y
		if &x[xOff] != &y[yOff] {
			copy(y[yOff+overlap:yOff+N], x[xOff+overlap:xOff+N])
		}
		return
	}

	// Compute the part with the constant filter.
	comb_filter_const(y, yOff+i, x, xOff+i, T1, N-i, g10, g11, g12, arch)
}

// TF change table. Positive values mean better frequency resolution (longer
// effective window), whereas negative values mean better time resolution
// (shorter effective window). The second index is computed as:
// 4*isTransient + 2*tf_select + per_band_flag
var tf_select_table = [4][8]int8{
	/*isTransient=0     isTransient=1 */
	{0, -1, 0, -1, 0, -1, 0, -1}, /* 2.5 ms */
	{0, -1, 0, -2, 1, 0, 1, -1},  /* 5 ms */
	{0, -2, 0, -3, 2, 0, 1, -1},  /* 10 ms */
	{0, -2, 0, -3, 3, 0, 1, -1},  /* 20 ms */
}

// C: void init_caps(const CELTMode *m,int *cap,int LM,int C)
func init_caps(m *CELTMode, cap []int, LM, C int) {
	var i int
	for i = 0; i < m.nbEBands; i++ {
		var N int
		N = int(m.eBands[i+1]-m.eBands[i]) << uint(LM)
		cap[i] = (int(m.cache.caps[m.nbEBands*(2*LM+C-1)+i]) + 64) * C * N >> 2
	}
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
	// Applications may rely on the presence of this substring in the version
	// string to determine if they have a fixed-point or floating-point build
	// at runtime.
	return "libopus " + PACKAGE_VERSION + "-fixed"
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
