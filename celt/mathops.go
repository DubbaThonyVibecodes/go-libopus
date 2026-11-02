package celt

import "math"

// C: celt/mathops.h, celt/mathops.c — various math functions (fixed-point build).

// C: #define PI 3.1415926535897931   (a double literal since 1.6)
const PI = 3.1415926535897931

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

// C: static OPUS_INLINE opus_res celt_maxabs_res(const opus_res *x, int len)   (ENABLE_RES24 && FIXED_POINT)
func MaxabsRes(x []int32, n int) int32 {
	var i int
	var maxval int32 = 0
	var minval int32 = 0
	for i = 0; i < n; i++ {
		maxval = MAX32(maxval, x[i])
		minval = MIN32(minval, x[i])
	}
	// opus_res should never reach such amplitude, so we should be safe.
	celt_sig_assert(minval != -2147483648)
	return MAX32(maxval, -minval)
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
	// -0.41509302963303146, 0.9609890551383969, -0.31836011537636605,
	//  0.15530808010959576, -0.08556153059057618
	var C = [5]int16{-6801 + (1 << (13 - 10)), 15746, -5217, 2545, -1401}
	if x == 0 {
		return -32767
	}
	i = int(Ilog2(x))
	n = int16(VSHR32(x, i-15) - 32768 - 16384)
	frac = ADD16(C[0], MULT16_16_Q15(n, ADD16(C[1], MULT16_16_Q15(n, ADD16(C[2], MULT16_16_Q15(n, ADD16(C[3], MULT16_16_Q15(n, C[4]))))))))
	return int16(SHL32(i-13, 10) + SHR32(frac, 14-10))
}

// K0 = 1, K1 = log(2), K2 = 3-4*log(2), K3 = 3*log(2) - 2
const (
	// (C #undefs D0..D3 right after celt_exp2_frac; they are package constants here.)
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

// Calculates the base-2 logarithm of a Q14 input value. The result is returned
// in Q(DB_SHIFT).
//
// C: #define celt_log2_db(x) SHL32(EXTEND32(celt_log2(x)), DB_SHIFT-10)   (no ENABLE_QEXT)
func Log2Db(x int32) int32 { return SHL32(EXTEND32(Log2(x)), DB_SHIFT-10) }

// C: #define celt_exp2_db_frac(x) SHL32(celt_exp2_frac(PSHR32(x, DB_SHIFT-10)), 14)   (no ENABLE_QEXT)
func Exp2DbFrac(x int32) int32 { return SHL32(Exp2Frac(int16(PSHR32(x, DB_SHIFT-10))), 14) }

// C: #define celt_exp2_db(x) celt_exp2(PSHR32(x, DB_SHIFT-10))   (no ENABLE_QEXT)
func Exp2Db(x int32) int32 { return Exp2(int16(PSHR32(x, DB_SHIFT-10))) }

// C: #define celt_div(a,b) MULT32_32_Q31((opus_val32)(a),celt_rcp(b))
func Div(a, b int32) int32 { return MULT32_32_Q31(a, Rcp(b)) }

// C: opus_val32 frac_div32_q29(opus_val32 a, opus_val32 b)
func FracDiv32Q29(a, b int32) int32 {
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
	return result
}

// C: opus_val32 frac_div32(opus_val32 a, opus_val32 b)
func FracDiv32(a, b int32) int32 {
	var result int32 = FracDiv32Q29(a, b)
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

// Reciprocal sqrt approximation in the range [0.25,1) (Q31 in, Q29 out)
//
// C: opus_val32 celt_rsqrt_norm32(opus_val32 x)
func RsqrtNorm32(x int32) int32 {
	var tmp int32
	// Use the first-order Newton-Raphson method to refine the root estimate.
	// r = r * (1.5 - 0.5*x*r*r)
	var r_q29 int32 = SHL32(RsqrtNorm(SHR32(x, 31-16)), 15)
	// Split evaluation in steps to avoid exploding macro expansion.
	tmp = MULT32_32_Q31(r_q29, r_q29)
	tmp = MULT32_32_Q31(int32(1073741824) /* Q31 */, tmp)
	tmp = MULT32_32_Q31(x, tmp)
	return SHL32(MULT32_32_Q31(r_q29, SUB32(int32(201326592) /* Q27 */, tmp)), 4)
}

// Sqrt approximation (QX input, QX/2 output)
//
// C: opus_val32 celt_sqrt(opus_val32 x)
func Sqrt(x int32) int32 {
	var k int
	var n int16
	var rt int32
	// These coeffs are optimized in fixed-point to minimize both RMS and max error
	// of sqrt(x) over .25<x<1 without exceeding 32767.
	// The RMS error is 3.4e-5 and the max is 8.2e-5.
	var C = [6]int16{23171, 11574, -2901, 1592, -1002, 336}
	if x == 0 {
		return 0
	} else if x >= 1073741824 {
		return 32767
	}
	k = (int(Ilog2(x)) >> 1) - 7
	x = VSHR32(x, 2*k)
	n = int16(x - 32768)
	rt = ADD32(C[0], MULT16_16_Q15(n, ADD16(C[1], MULT16_16_Q15(n, ADD16(C[2], MULT16_16_Q15(n, ADD16(C[3], MULT16_16_Q15(n, ADD16(C[4], MULT16_16_Q15(n, C[5]))))))))))
	rt = VSHR32(rt, 7-k)
	return rt
}

// Perform fixed-point arithmetic to approximate the square root. When the input
// is in Qx format, the output will be in Q(x/2 + 16) format.
//
// C: opus_val32 celt_sqrt32(opus_val32 x)
func Sqrt32(x int32) int32 {
	var k int
	var x_frac int32
	if x == 0 {
		return 0
	} else if x >= 1073741824 {
		return 2147483647 // 2^31 -1
	}
	k = int(Ilog2(x)) >> 1
	x_frac = VSHR32(x, 2*(k-14)-1)
	x_frac = MULT32_32_Q31(RsqrtNorm32(x_frac), x_frac)
	if k < 12 {
		return PSHR32(x_frac, 12-k)
	}
	return SHL32(x_frac, k-12)
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

// Calculates the cosine of (PI*0.5*x) where the input x ranges from -1 to 1 and
// is in Q30 format. The output will also be in Q31 format.
//
// C: opus_val32 celt_cos_norm32(opus_val32 x)
func CosNorm32(x int32) int32 {
	const COS_NORM_COEFF_A0 int32 = 134217720  // Q27
	const COS_NORM_COEFF_A1 int32 = -662336704 // Q29
	const COS_NORM_COEFF_A2 int32 = 544710848  // Q31
	const COS_NORM_COEFF_A3 int32 = -178761936 // Q33
	const COS_NORM_COEFF_A4 int32 = 29487206   // Q35
	var x_sq_q29, tmp int32
	// The expected x is in the range of [-1.0f, 1.0f]
	celt_sig_assert((x >= -1073741824) && (x <= 1073741824))
	// Make cos(+/- pi/2) exactly zero.
	if ABS32(x) == 1<<30 {
		return 0
	}
	x_sq_q29 = MULT32_32_Q31(x, x)
	// Split evaluation in steps to avoid exploding macro expansion.
	tmp = ADD32(COS_NORM_COEFF_A3, MULT32_32_Q31(x_sq_q29, COS_NORM_COEFF_A4))
	tmp = ADD32(COS_NORM_COEFF_A2, MULT32_32_Q31(x_sq_q29, tmp))
	tmp = ADD32(COS_NORM_COEFF_A1, MULT32_32_Q31(x_sq_q29, tmp))
	return SHL32(ADD32(COS_NORM_COEFF_A0, MULT32_32_Q31(x_sq_q29, tmp)), 4)
}

// Computes a 16 bit approximate reciprocal (1/x) for a normalized Q15 input,
// resulting in a Q15 output.
//
// C: opus_val16 celt_rcp_norm16(opus_val16 x)
func RcpNorm16(x int16) int16 {
	var r int16
	// Start with a linear approximation:
	// r = 1.8823529411764706-0.9411764705882353*n.
	// The coefficients and the result are Q14 in the range [15420,30840].
	r = ADD16(30840, MULT16_16_Q15(-15420, x))
	// Perform two Newton iterations:
	// r -= r*((r*n)+(r-1.Q15))
	//    = r*((r*n)+(r-1.Q15)).
	r = int16(SUB16(r, MULT16_16_Q15(r, ADD16(MULT16_16_Q15(r, x), ADD16(r, -32768)))))
	// We subtract an extra 1 in the second iteration to avoid overflow; it also
	// neatly compensates for truncation error in the rest of the process.
	return int16(SUB16(r, ADD16(1, MULT16_16_Q15(r, ADD16(MULT16_16_Q15(r, x), ADD16(r, -32768))))))
}

// Computes a 32 bit approximated reciprocal (1/x) for a normalized Q31 input,
// resulting in a Q30 output. The expected input range is [0.5f, 1.0f) in Q31
// and the expected output range is [1.0f, 2.0f) in Q30.
//
// C: opus_val32 celt_rcp_norm32(opus_val32 x)
func RcpNorm32(x int32) int32 {
	var r_q30 int32
	celt_sig_assert(x >= 1073741824)
	r_q30 = SHL32(EXTEND32(RcpNorm16(int16(SHR32(x, 15)-32768))), 16)
	// Solving f(y) = a - 1/y using the Newton Method
	// Note: f(y)' = 1/y^2
	// r = r - f(r)/f(r)' = r - (x * r*r - r)
	//   = r - r*(r*x - 1)
	// where
	//   - r means 1/y's approximation.
	//   - x means a, the input of function.
	// Please note that:
	//   - It adds 1 to avoid overflow
	//   - -1.0f in Q30 is -1073741824.
	return SUB32(r_q30, ADD32(SHL32(
		MULT32_32_Q31(ADD32(MULT32_32_Q31(r_q30, x), -1073741824),
			r_q30), 1), 1))
}

// Reciprocal approximation (Q15 input, Q16 output).
//
// C: opus_val32 celt_rcp(opus_val32 x)
func Rcp(x int32) int32 {
	var i int
	var r int16
	celt_sig_assert(x > 0)
	i = int(Ilog2(x))

	// Compute the reciprocal of a Q15 number in the range [0, 1).
	r = RcpNorm16(int16(VSHR32(x, i-15) - 32768))

	// r is now the Q15 solution to 2/(n+1), with a maximum relative error
	// of 7.05346E-5, a (relative) RMSE of 2.14418E-5, and a peak absolute
	// error of 1.24665/32768.
	return VSHR32(EXTEND32(r), i-16)
}

// Computes atan(x) multiplied by 2/PI. The input value (x) should be within the
// range of -1 to 1 and represented in Q30 format. The function will return the
// result in Q30 format.
//
// C: static OPUS_INLINE opus_val32 celt_atan_norm(opus_val32 x)
func AtanNorm(x int32) int32 {
	// Approximation constants.
	const ATAN_2_OVER_PI int32 = 1367130551  // Q31
	const ATAN_COEFF_A03 int32 = -715791936  // Q31
	const ATAN_COEFF_A05 int32 = 857391616   // Q32
	const ATAN_COEFF_A07 int32 = -1200579328 // Q33
	const ATAN_COEFF_A09 int32 = 1682636672  // Q34
	const ATAN_COEFF_A11 int32 = -1985085440 // Q35
	const ATAN_COEFF_A13 int32 = 1583306112  // Q36
	const ATAN_COEFF_A15 int32 = -598602432  // Q37
	var x_sq_q30 int32
	var x_q31 int32
	var tmp int32
	// The expected x is in the range of [-1.0f, 1.0f]
	celt_sig_assert((x <= 1073741824) && (x >= -1073741824))

	// If x = 1.0f, returns 0.5f
	if x == 1073741824 {
		return 536870912 // 0.5f (Q30)
	}
	// If x = 1.0f, returns 0.5f
	if x == -1073741824 {
		return -536870912 // -0.5f (Q30)
	}
	x_q31 = SHL32(x, 1)
	x_sq_q30 = MULT32_32_Q31(x_q31, x)
	// Split evaluation in steps to avoid exploding macro expansion.
	tmp = MULT32_32_Q31(x_sq_q30, ATAN_COEFF_A15)
	tmp = MULT32_32_Q31(x_sq_q30, ADD32(ATAN_COEFF_A13, tmp))
	tmp = MULT32_32_Q31(x_sq_q30, ADD32(ATAN_COEFF_A11, tmp))
	tmp = MULT32_32_Q31(x_sq_q30, ADD32(ATAN_COEFF_A09, tmp))
	tmp = MULT32_32_Q31(x_sq_q30, ADD32(ATAN_COEFF_A07, tmp))
	tmp = MULT32_32_Q31(x_sq_q30, ADD32(ATAN_COEFF_A05, tmp))
	tmp = MULT32_32_Q31(x_sq_q30, ADD32(ATAN_COEFF_A03, tmp))
	tmp = ADD32(x, MULT32_32_Q31(x_q31, tmp))
	return MULT32_32_Q31(ATAN_2_OVER_PI, tmp)
}

// Calculates the arctangent of y/x, multiplies the result by 2/pi, and returns
// the value in Q30 format. Both input values (x and y) must be within the range
// of 0 to 1 and represented in Q30 format. Inputs must be zero or greater, and
// at least one input must be non-zero.
//
// C: static OPUS_INLINE opus_val32 celt_atan2p_norm(opus_val32 y, opus_val32 x)
func Atan2pNorm(y, x int32) int32 {
	celt_sig_assert(x >= 0 && y >= 0)
	if y == 0 && x == 0 {
		return 0
	} else if y < x {
		return AtanNorm(SHR32(FracDiv32(y, x), 1))
	} else {
		celt_sig_assert(y > 0)
		return 1073741824 /* 1.0f Q30 */ -
			AtanNorm(SHR32(FracDiv32(x, y), 1))
	}
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
	if x == 0 && y == 0 {
		return 0
	} else if y < x {
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
	const cE float32 = float32(PI) / 2
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

// C: static OPUS_INLINE opus_int16 FLOAT2INT16(float x)   (float_cast.h)
func FLOAT2INT16(x float32) int16 {
	x = x * CELT_SIG_SCALE
	x = FMAX(x, -32768)
	x = FMIN(x, 32767)
	return int16(Float2int(x))
}

// C: static OPUS_INLINE opus_int32 FLOAT2INT24(float x)   (float_cast.h)
func FLOAT2INT24(x float32) int32 {
	x = x * (CELT_SIG_SCALE * 256.0)
	x = FMAX(x, -16777216)
	x = FMIN(x, 16777216)
	return Float2int(x)
}

// C: static OPUS_INLINE opus_int32 FLOAT2SIG(float x)   (float_cast.h, FIXED_POINT)
func FLOAT2SIG(x float32) int32 {
	x = x * float32(int32(32768)<<SIG_SHIFT)
	x = FMAX(x, -float32(65536<<SIG_SHIFT))
	x = FMIN(x, float32(65536<<SIG_SHIFT))
	return Float2int(x)
}

// C: void celt_float2int16_c(const float * OPUS_RESTRICT in, short * OPUS_RESTRICT out, int cnt)
// C: #define celt_float2int16(in, out, cnt, arch) ((void)(arch), celt_float2int16_c(in, out, cnt))
func Float2int16(in []float32, out []int16, cnt int, arch int) {
	var i int
	for i = 0; i < cnt; i++ {
		out[i] = FLOAT2INT16(in[i])
	}
}

// C: int opus_limit2_checkwithin1_c(float * samples, int cnt)
// C: #define opus_limit2_checkwithin1(samples, cnt, arch) ((void)(arch), opus_limit2_checkwithin1_c(samples, cnt))
func Limit2Checkwithin1(samples []float32, cnt int, arch int) int {
	var i int
	if cnt <= 0 {
		return 1
	}

	for i = 0; i < cnt; i++ {
		var clippedVal float32 = samples[i]
		clippedVal = FMAX(-2.0, clippedVal)
		clippedVal = FMIN(2.0, clippedVal)
		samples[i] = clippedVal
	}

	// C implementation can't provide quick hint. Assume it might exceed -1/+1.
	return 0
}
