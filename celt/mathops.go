package celt

import "math"

// C: celt/mathops.h, celt/mathops.c — various math functions (fixed-point build).

// C: #define PI 3.141592653f
const PI float32 = 3.141592653

// C: FRAC_MUL16(a,b) ((16384+((opus_int32)(opus_int16)(a)*(opus_int16)(b)))>>15)
func FRAC_MUL16[A, B integer](a A, b B) int32 {
	return (16384 + int32(int16(a))*int32(int16(b))) >> 15
}

// Isqrt32 computes the integer square root; tested on all possible 32-bit inputs.
//
// C: unsigned isqrt32(opus_uint32 _val)
func Isqrt32(val uint32) uint32 {
	var b, g uint32
	var bshift int
	// Uses the second method from
	// http://www.azillionmonkeys.com/qed/sqroot.html
	// The main idea is to search for the largest binary digit b such that
	// (g+b)*(g+b) <= _val, and add it to the solution g.
	g = 0
	bshift = (EC_ILOG(val) - 1) >> 1
	b = uint32(1) << uint(bshift)
	for {
		var t uint32
		t = ((g << 1) + b) << uint(bshift)
		if t <= val {
			g += b
			val -= t
		}
		b >>= 1
		bshift--
		if bshift < 0 {
			break
		}
	}
	return g
}

// C: static OPUS_INLINE opus_int16 celt_ilog2(opus_int32 x)
func Ilog2(x int32) int16 {
	celt_sig_assert(x > 0)
	return int16(EC_ILOG(uint32(x)) - 1)
}

// C: static OPUS_INLINE opus_val32 celt_maxabs16(const opus_val16 *x, int len)
func Maxabs16(x []int16, n int) int32 {
	var i int
	var maxval int16 = 0
	var minval int16 = 0
	for i = 0; i < n; i++ {
		maxval = MAX16(maxval, x[i])
		minval = MIN16(minval, x[i])
	}
	return MAX32(EXTEND32(maxval), -EXTEND32(minval))
}

// C: static OPUS_INLINE opus_val32 celt_maxabs32(const opus_val32 *x, int len)
func Maxabs32(x []int32, n int) int32 {
	var i int
	var maxval int32 = 0
	var minval int32 = 0
	for i = 0; i < n; i++ {
		maxval = MAX32(maxval, x[i])
		minval = MIN32(minval, x[i])
	}
	return MAX32(maxval, -minval)
}

// Integer log in base2. Defined for zero, but not for negative numbers.
//
// C: static OPUS_INLINE opus_int16 celt_zlog2(opus_val32 x)
func Zlog2(x int32) int16 {
	if x <= 0 {
		return 0
	}
	return Ilog2(x)
}

// Base-2 log approximation (log2(x)). Output is in Q10 (DB_SHIFT).
//
// C: static OPUS_INLINE opus_val16 celt_log2(opus_val32 x)
func Log2(x int32) int16 {
	var i int
	var n, frac int16
	// -0.4300142410, 0.7625620574, -0.4016729063, 0.0834883101, -0.0208236129
	// in Q13 after bringing the first coefficient back to Q13 precision.
	var C = [5]int16{-6801 + (1 << (13 - DB_SHIFT)), 15746, -5217, 2545, -1401}
	if x == 0 {
		return -32767
	}
	i = int(Ilog2(x))
	n = int16(VSHR32(x, i-15) - 32768 - 16384)
	frac = ADD16(C[0], MULT16_16_Q15(n, ADD16(C[1], MULT16_16_Q15(n, ADD16(C[2], MULT16_16_Q15(n, ADD16(C[3], MULT16_16_Q15(n, C[4]))))))))
	return int16(int32(SHL16(i-13, DB_SHIFT)) + SHR16(frac, 14-DB_SHIFT))
}

// K0 = 1, K1 = log(2), K2 = 3-4*log(2), K3 = 3*log(2) - 2
const (
	D0 = 16383
	D1 = 22804
	D2 = 14819
	D3 = 10204
)

// C: static OPUS_INLINE opus_val32 celt_exp2_frac(opus_val16 x)
func Exp2Frac(x int16) int32 {
	var frac int16
	frac = SHL16(x, 4)
	return int32(ADD16(D0, MULT16_16_Q15(frac, ADD16(D1, MULT16_16_Q15(frac, ADD16(D2, MULT16_16_Q15(D3, frac)))))))
}

// Base-2 exponential approximation (2^x). (Q10 input, Q16 output)
//
// C: static OPUS_INLINE opus_val32 celt_exp2(opus_val16 x)
func Exp2(x int16) int32 {
	var integer int
	var frac int16
	integer = int(SHR16(x, 10))
	if integer > 14 {
		return 0x7f000000
	} else if integer < -15 {
		return 0
	}
	frac = int16(Exp2Frac(int16(int32(x) - int32(SHL16(integer, 10)))))
	return VSHR32(EXTEND32(frac), -integer-2)
}

// C: #define celt_div(a,b) MULT32_32_Q31((opus_val32)(a),celt_rcp(b))
func Div(a, b int32) int32 { return MULT32_32_Q31(a, Rcp(b)) }

// C: opus_val32 frac_div32(opus_val32 a, opus_val32 b)
func FracDiv32(a, b int32) int32 {
	var rcp int16
	var result, rem int32
	shift := int(Ilog2(b)) - 29
	a = VSHR32(a, shift)
	b = VSHR32(b, shift)
	// 16-bit reciprocal
	rcp = ROUND16(Rcp(int32(ROUND16(b, 16))), 3)
	result = MULT16_32_Q15(rcp, a)
	rem = PSHR32(a, 2) - MULT32_32_Q31(result, b)
	result = ADD32(result, SHL32(MULT16_32_Q15(rcp, rem), 2))
	if result >= 536870912 { //  2^29
		return 2147483647 //  2^31 - 1
	} else if result <= -536870912 { // -2^29
		return -2147483647 // -2^31
	}
	return SHL32(result, 2)
}

// Reciprocal sqrt approximation in the range [0.25,1) (Q16 in, Q14 out).
//
// C: opus_val16 celt_rsqrt_norm(opus_val32 x)
func RsqrtNorm(x int32) int16 {
	var n, r, r2, y int16
	// Range of n is [-16384,32767] ([-0.5,1) in Q15).
	n = int16(x - 32768)
	// Get a rough initial guess for the root.
	// The optimal minimax quadratic approximation (using relative error) is
	// r = 1.437799046117536+n*(-0.823394375837328+n*0.4096419668459485).
	// Coefficients here, and the final result r, are Q14.
	r = ADD16(23557, MULT16_16_Q15(n, ADD16(-13490, MULT16_16_Q15(n, 6713))))
	// We want y = x*r*r-1 in Q15, but x is 32-bit and we need to
	// evaluate it to at least Q15 precision. We can compute the result from
	// n and r using Q15 multiplies with some adjustment, carefully done to
	// avoid overflow. Range of y is [-1564,1594].
	r2 = int16(MULT16_16_Q15(r, r))
	y = SHL16(int32(SUB16(ADD16(MULT16_16_Q15(r2, n), r2), 16384)), 1)
	// Apply a 2nd-order Householder iteration: r += r*y*(y*0.375-0.5).
	// This yields the Q14 reciprocal square root of the Q16 x, with a maximum
	// relative error of 1.04956E-4, a (relative) RMSE of 2.80979E-5, and a
	// peak absolute error of 2.26591/16384.
	return ADD16(r, MULT16_16_Q15(r, MULT16_16_Q15(y, SUB16(MULT16_16_Q15(y, 12288), 16384))))
}

// Sqrt approximation (Q16 in, Q16 out... or whatever the shifts give).
//
// C: opus_val32 celt_sqrt(opus_val32 x)
func Sqrt(x int32) int32 {
	var k int
	var n int16
	var rt int32
	var C = [5]int16{23175, 11561, -3011, 1699, -664}
	if x == 0 {
		return 0
	} else if x >= 1073741824 {
		return 32767
	}
	k = (int(Ilog2(x)) >> 1) - 7
	x = VSHR32(x, 2*k)
	n = int16(x - 32768)
	rt = int32(ADD16(C[0], MULT16_16_Q15(n, ADD16(C[1], MULT16_16_Q15(n, ADD16(C[2], MULT16_16_Q15(n, ADD16(C[3], MULT16_16_Q15(n, C[4])))))))))
	rt = VSHR32(rt, 7-k)
	return rt
}

const (
	l1 = 32767
	l2 = -7651
	l3 = 8277
	l4 = -626
)

// C: static OPUS_INLINE opus_val16 _celt_cos_pi_2(opus_val16 x)
func cosPi2(x int16) int16 {
	var x2 int16
	x2 = int16(MULT16_16_P15(x, x))
	return ADD16(1, MIN16(32766, ADD32(SUB16(l1, x2), MULT16_16_P15(x2, ADD32(l2, MULT16_16_P15(x2, ADD32(l3, MULT16_16_P15(l4, x2))))))))
}

// C: opus_val16 celt_cos_norm(opus_val32 x)
func CosNorm(x int32) int16 {
	x = x & 0x0001ffff
	if x > SHL32(EXTEND32(1), 16) {
		x = SUB32(SHL32(EXTEND32(1), 17), x)
	}
	if x&0x00007fff != 0 {
		if x < SHL32(EXTEND32(1), 15) {
			return cosPi2(EXTRACT16(x))
		}
		return NEG16(cosPi2(EXTRACT16(65536 - x)))
	}
	if x&0x0000ffff != 0 {
		return 0
	} else if x&0x0001ffff != 0 {
		return -32767
	}
	return 32767
}

// Reciprocal approximation (Q15 input, Q16 output).
//
// C: opus_val32 celt_rcp(opus_val32 x)
func Rcp(x int32) int32 {
	var i int
	var n, r int16
	celt_sig_assert(x > 0)
	i = int(Ilog2(x))
	// n is Q15 with range [0,1).
	n = int16(VSHR32(x, i-15) - 32768)
	// Start with a linear approximation:
	// r = 1.8823529411764706-0.9411764705882353*n.
	// The coefficients and the result are Q14 in the range [15420,30840].
	r = ADD16(30840, MULT16_16_Q15(-15420, n))
	// Perform two Newton iterations:
	// r -= r*((r*n)-1.Q15)
	//    = r*((r*n)+(r-1.Q15)).
	r = int16(SUB16(r, MULT16_16_Q15(r, ADD16(MULT16_16_Q15(r, n), ADD16(r, -32768)))))
	// We subtract an extra 1 in the second iteration to avoid overflow; it also
	// neatly compensates for truncation error in the rest of the process.
	r = int16(SUB16(r, ADD16(1, MULT16_16_Q15(r, ADD16(MULT16_16_Q15(r, n), ADD16(r, -32768))))))
	// r is now the Q15 solution to 2/(n+1), with a maximum relative error
	// of 7.05346E-5, a (relative) RMSE of 2.14418E-5, and a peak absolute
	// error of 1.24665/32768.
	return VSHR32(EXTEND32(r), i-16)
}

const (
	m1 = 32767
	m2 = -21
	m3 = -11943
	m4 = 4936
)

// Atan approximation, for inputs in [0,1] normalized by pi/4. Output is in Q15 format.
//
// C: static OPUS_INLINE opus_val16 celt_atan01(opus_val16 x)
func atan01(x int16) int16 {
	return int16(MULT16_16_P15(x, ADD32(m1, MULT16_16_P15(x, ADD32(m2, MULT16_16_P15(x, ADD32(m3, MULT16_16_P15(m4, x))))))))
}

// atan2() approximation valid for positive input values.
//
// C: static OPUS_INLINE opus_val16 celt_atan2p(opus_val16 y, opus_val16 x)
func Atan2p(y, x int16) int16 {
	if y < x {
		var arg int32
		arg = Div(SHL32(EXTEND32(y), 15), int32(x))
		if arg >= 32767 {
			arg = 32767
		}
		return int16(SHR16(atan01(EXTRACT16(arg)), 1))
	}
	var arg int32
	arg = Div(SHL32(EXTEND32(x), 15), int32(y))
	if arg >= 32767 {
		arg = 32767
	}
	return int16(25736 - SHR16(atan01(EXTRACT16(arg)), 1))
}

// CELT doesn't need it for fixed-point, by analysis.c does.
//
// C: static OPUS_INLINE float fast_atan2f(float y, float x)   (#if !defined(FIXED_POINT) || defined(ANALYSIS_C))
func FastAtan2f(y, x float32) float32 {
	const cA float32 = 0.43157974
	const cB float32 = 0.67848403
	const cC float32 = 0.08595542
	const cE float32 = PI / 2
	var x2, y2 float32
	x2 = x * x
	y2 = y * y
	// For very small values, we don't care about the answer, so
	// we can just return 0.
	if x2+y2 < 1e-18 {
		return 0
	}
	if x2 < y2 {
		var den float32 = (y2 + float32(cB*x2)) * (y2 + float32(cC*x2))
		var e float32
		if y < 0 {
			e = -cE
		} else {
			e = cE
		}
		return float32(-x*y*(y2+float32(cA*x2))/den) + e
	} else {
		var den float32 = (x2 + float32(cB*y2)) * (x2 + float32(cC*y2))
		var e, f float32
		if y < 0 {
			e = -cE
		} else {
			e = cE
		}
		if x*y < 0 {
			f = -cE
		} else {
			f = cE
		}
		return float32(float32(x*y*(x2+float32(cA*y2))/den)+e) - f
	}
}

// Float2int is the C float2int (float_cast.h): lrintf, i.e. round to
// nearest with ties to even.
func Float2int(x float32) int32 {
	return int32(math.RoundToEven(float64(x)))
}
