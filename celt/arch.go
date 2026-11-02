// Package celt is a mechanical port of libopus' celt/ directory (fixed-point
// build). See PORTING.md at the module root for the conventions.
package celt

// C: celt/arch.h

const CELT_SIG_SCALE = 32768.0

// enableAssertions mirrors ENABLE_ASSERTIONS; celt_assert compiles to
// nothing when it is false.
const enableAssertions = false

// C: celt_assert(cond)
func celt_assert(cond bool) {
	if enableAssertions && !cond {
		panic("celt: assertion failed")
	}
}

// C: celt_assert2(cond, message)
func celt_assert2(cond bool, message string) {
	if enableAssertions && !cond {
		panic("celt: assertion failed: " + message)
	}
}

// C: celt_sig_assert(cond)
// Only active with ENABLE_ASSERTIONS (not with ENABLE_HARDENING), so it is a
// no-op in the configuration we port.
func celt_sig_assert(cond bool) {
	if enableAssertions && !cond {
		panic("celt: signal assertion failed")
	}
}

// C: #define celt_isnan(x) 0  (fixed-point build)
func celt_isnan[T integer](x T) bool { return false }

type integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

// C: IMUL32(a,b) ((a)*(b))
func IMUL32[T integer](a, b T) T { return a * b }

// C: MIN16(a,b)
func MIN16[T integer](a, b T) T {
	if a < b {
		return a
	}
	return b
}

// C: MAX16(a,b)
func MAX16[T integer](a, b T) T {
	if a > b {
		return a
	}
	return b
}

// C: MIN32(a,b)
func MIN32[T integer](a, b T) T { return MIN16(a, b) }

// C: MAX32(a,b)
func MAX32[T integer](a, b T) T { return MAX16(a, b) }

// C: IMIN(a,b)
func IMIN[T integer](a, b T) T { return MIN16(a, b) }

// C: IMAX(a,b)
func IMAX[T integer](a, b T) T { return MAX16(a, b) }

// C: FMIN(a,b) ((a) < (b) ? (a) : (b))   /**< Minimum float value.   */
func FMIN[T ~float32 | ~float64](a, b T) T {
	if a < b {
		return a
	}
	return b
}

// C: FMAX(a,b) ((a) > (b) ? (a) : (b))   /**< Maximum float value.   */
func FMAX[T ~float32 | ~float64](a, b T) T {
	if a > b {
		return a
	}
	return b
}

// C: UADD32(a,b)
func UADD32(a, b uint32) uint32 { return a + b }

// C: USUB32(a,b)
func USUB32(a, b uint32) uint32 { return a - b }

// C: MAXG(a,b) MAX32(a, b)
func MAXG[T integer](a, b T) T { return MAX32(a, b) }

// C: MING(a,b) MIN32(a, b)
func MING[T integer](a, b T) T { return MIN32(a, b) }

// Throughout the code, we use the following scaling for signals:
//   FLOAT: used for float API, normalized to +/-1.
//   INT16: used for 16-bit API, normalized to +/- 32768
//   RES: internal Opus resolution, defined as +/-1. in float builds, or either 16-bit or 24-bit int for fixed-point builds
//   SIG: internal CELT resolution: defined as +/- 32768. in float builds, or Q27 in fixed-point builds (int16 shifted by 12)

// Set this if opus_int64 is a native type of the CPU (64-bit host).
const OPUS_FAST_INT64 = 1

// C: typedef opus_int64 opus_val64;   (FIXED_POINT)
type opus_val64 = int64

// C: GLOBAL_STACK_SIZE 120000   (unused: the Go port allocates with make)
const GLOBAL_STACK_SIZE = 120000

// Fixed-point build types (arch.h, FIXED_POINT branch):
//
//	opus_val16 → int16, opus_val32 → int32, opus_val64 → int64,
//	celt_sig → int32, celt_norm → int32, celt_ener → int32, celt_glog → int32,
//	opus_res → int32 (ENABLE_RES24), celt_coef → int16 (no ENABLE_QEXT).
const (
	Q15ONE    = int16(32767)
	Q31ONE    = int32(2147483647)
	SIG_SHIFT = 12
	// Safe saturation value for 32-bit signals. We need to make sure that we can
	// add two sig values and that the first stages of the MDCT don't cause an overflow.
	// The most constraining is the ARM_ASM comb filter where we shift left by one
	// and then add two values. Because of that, we use 2^29-1. SIG_SAT must be large
	// enough to fit a full-scale high-freq tone through the prefilter and comb filter,
	// meaning 1.85*1.75*2^(15+SIG_SHIFT) =  434529895.
	// so the limit should be about 2^31*sqrt(.5).
	SIG_SAT      = 536870911
	NORM_SHIFT   = 24
	NORM_SCALING = 1 << NORM_SHIFT
	DB_SHIFT     = 24
	EPSILON      = 1
	VERY_SMALL   = 0
	VERY_LARGE16 = int16(32767)
	Q15_ONE      = int16(32767)

	// ENABLE_RES24: opus_res is opus_val32.
	RES_SHIFT          = 8
	MAX_ENCODING_DEPTH = 24

	// No ENABLE_QEXT: celt_coef is opus_val16.
	COEF_ONE = Q15ONE
)

// C: SIG2RES(a) PSHR32(a, SIG_SHIFT-RES_SHIFT)   (ENABLE_RES24)
func SIG2RES(a int32) int32 { return PSHR32(a, SIG_SHIFT-RES_SHIFT) }

// C: RES2INT16(a) SAT16(PSHR32(a, RES_SHIFT))   (ENABLE_RES24)
func RES2INT16(a int32) int16 { return SAT16(PSHR32(a, RES_SHIFT)) }

// C: RES2INT24(a) (a)   (ENABLE_RES24)
func RES2INT24(a int32) int32 { return a }

// C: RES2FLOAT(a) ((1.f/32768.f/256.f)*(a))   (ENABLE_RES24)
func RES2FLOAT(a int32) float32 { return (1.0 / 32768.0 / 256.0) * float32(a) }

// C: INT16TORES(a) SHL32(EXTEND32(a), RES_SHIFT)   (ENABLE_RES24)
func INT16TORES[T integer](a T) int32 { return SHL32(EXTEND32(a), RES_SHIFT) }

// C: INT24TORES(a) (a)   (ENABLE_RES24)
func INT24TORES(a int32) int32 { return a }

// C: ADD_RES(a, b) ADD32(a, b)   (ENABLE_RES24)
func ADD_RES[A, B integer](a A, b B) int32 { return ADD32(a, b) }

// C: FLOAT2RES(a) FLOAT2INT24(a)   (ENABLE_RES24)
func FLOAT2RES(a float32) int32 { return FLOAT2INT24(a) }

// C: RES2SIG(a) SHL32((a), SIG_SHIFT-RES_SHIFT)   (ENABLE_RES24)
func RES2SIG[T integer](a T) int32 { return SHL32(a, SIG_SHIFT-RES_SHIFT) }

// C: MULT16_RES_Q15(a,b) MULT16_32_Q15(a,b)   (ENABLE_RES24)
func MULT16_RES_Q15[A, B integer](a A, b B) int32 { return MULT16_32_Q15(a, b) }

// C: RES2VAL16(a) RES2INT16(a)
func RES2VAL16(a int32) int16 { return RES2INT16(a) }

// C: INT16TOSIG(a) SHL32(EXTEND32(a), SIG_SHIFT)
func INT16TOSIG[T integer](a T) int32 { return SHL32(EXTEND32(a), SIG_SHIFT) }

// C: INT24TOSIG(a) SHL32(a, SIG_SHIFT-8)
func INT24TOSIG[T integer](a T) int32 { return SHL32(a, SIG_SHIFT-8) }

// C: MULT_COEF_32(a, b) MULT16_32_Q15(a,b)   (no ENABLE_QEXT)
func MULT_COEF_32[A, B integer](a A, b B) int32 { return MULT16_32_Q15(a, b) }

// C: MAC_COEF_32_ARM(a, b, c) MAC16_32_Q16(a,b,c)   (no ENABLE_QEXT)
func MAC_COEF_32_ARM[A, B, C integer](a A, b B, c C) int32 { return MAC16_32_Q16(a, b, c) }

// C: MULT_COEF(a, b) MULT16_16_Q15(a,b)   (no ENABLE_QEXT)
func MULT_COEF[A, B integer](a A, b B) int32 { return MULT16_16_Q15(a, b) }

// C: MULT_COEF_TAPS(a, b) MULT16_16_P15(a,b)   (no ENABLE_QEXT)
func MULT_COEF_TAPS[A, B integer](a A, b B) int32 { return MULT16_16_P15(a, b) }

// C: COEF2VAL16(x) (x)   (no ENABLE_QEXT)
func COEF2VAL16(x int16) int16 { return x }

// C: ABS16(x) ((x) < 0 ? (-(x)) : (x))
func ABS16[T integer](x T) T {
	if x < 0 {
		return -x
	}
	return x
}

// C: ABS32(x) ((x) < 0 ? (-(x)) : (x))
func ABS32[T integer](x T) T {
	if x < 0 {
		return -x
	}
	return x
}

// ABS is kept for the generic integer uses (the C code uses ABS16/ABS32 or
// plain conditionals on ints).
func ABS[T integer](x T) T {
	if x < 0 {
		return -x
	}
	return x
}

// C: static OPUS_INLINE opus_int16 SAT16(opus_int32 x)
func SAT16(x int32) int16 {
	if x > 32767 {
		return 32767
	}
	if x < -32768 {
		return -32768
	}
	return int16(x)
}

// Assert is celt_assert for other packages.
func Assert(cond bool) { celt_assert(cond) }

// MustSucceed is MUST_SUCCEED(call) for other packages: with ENABLE_HARDENING
// it is celt_assert((call) == OPUS_OK).
// C: #define MUST_SUCCEED(call) celt_assert((call) == OPUS_OK)
func MustSucceed(ret int) { celt_assert(ret == OPUS_OK) }
