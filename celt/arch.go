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

type integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

// C: IMUL32(a,b) ((a)*(b))
func IMUL32[T integer](a, b T) T { return a * b }

// C: ABS(x)
func ABS[T integer](x T) T {
	if x < 0 {
		return -x
	}
	return x
}

// C: ABS16(x)
func ABS16[T integer](x T) T { return ABS(x) }

// C: ABS32(x)
func ABS32[T integer](x T) T { return ABS(x) }

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

// Fixed-point build constants (arch.h, FIXED_POINT branch).
const (
	Q15ONE       = int16(32767)
	SIG_SHIFT    = 12
	NORM_SCALING = 16384
	DB_SHIFT     = 10
	EPSILON      = 1
	VERY_LARGE16 = int16(32767)
	Q15_ONE      = int16(32767)
)

// C: SCALEIN(a) / SCALEOUT(a) are identities in the fixed-point build.
func SCALEIN[T integer](a T) T  { return a }
func SCALEOUT[T integer](a T) T { return a }

// Assert is celt_assert for other packages.
func Assert(cond bool) { celt_assert(cond) }
