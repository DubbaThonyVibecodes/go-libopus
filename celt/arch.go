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

// C: UADD32(a,b)
func UADD32(a, b uint32) uint32 { return a + b }

// C: USUB32(a,b)
func USUB32(a, b uint32) uint32 { return a - b }

// Set this if opus_int64 is a native type of the CPU (64-bit host).
const OPUS_FAST_INT64 = 1

// C: typedef opus_int64 opus_val64;   (FIXED_POINT)
type opus_val64 = int64

// C: GLOBAL_STACK_SIZE 120000   (unused: the Go port allocates with make)
const GLOBAL_STACK_SIZE = 120000

// Fixed-point build constants (arch.h, FIXED_POINT branch).
const (
	Q15ONE    = int16(32767)
	SIG_SHIFT = 12
	// Safe saturation value for 32-bit signals. Should be less than
	// 2^31*(1-0.85) to avoid blowing up on DC at deemphasis.
	SIG_SAT      = 300000000
	NORM_SCALING = 16384
	DB_SHIFT     = 10
	EPSILON      = 1
	VERY_SMALL   = 0
	VERY_LARGE16 = int16(32767)
	Q15_ONE      = int16(32767)
)

// C: SCALEIN(a) / SCALEOUT(a) are identities in the fixed-point build.
func SCALEIN[T integer](a T) T  { return a }
func SCALEOUT[T integer](a T) T { return a }

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
