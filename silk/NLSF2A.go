package silk

// C: silk/NLSF2A.c

// conversion between prediction filter coefficients and LSFs
// order should be even
// a piecewise linear approximation maps LSF <-> cos(LSF)
// therefore the result is not accurate LSFs, but the two
// functions are accurate inverses of each other

// helper function for NLSF2A(..)
//
// C: static inline void silk_NLSF2A_find_poly(opus_int32 *out, const opus_int32 *cLSF, opus_int dd)
func nlsf2AFindPoly(
	out []int32, // O    intermediate polynomial, QA [dd+1]
	cLSF []int32, // I    vector of interleaved 2*cos(LSFs), QA [d]
	dd int, // I    polynomial order (= 1/2 * filter order)
) {
	const QA = 16
	var k, n int
	var ftmp int32

	out[0] = LSHIFT(1, QA)
	out[1] = -cLSF[0]
	for k = 1; k < dd; k++ {
		ftmp = cLSF[2*k] // QA
		out[k+1] = LSHIFT(out[k-1], 1) - int32(RSHIFT_ROUND64(SMULL(ftmp, out[k]), QA))
		for n = k; n > 1; n-- {
			out[n] += out[n-2] - int32(RSHIFT_ROUND64(SMULL(ftmp, out[n-1]), QA))
		}
		out[1] -= ftmp
	}
}

// This ordering was found to maximize quality. It improves numerical accuracy of
// silk_NLSF2A_find_poly() compared to "standard" ordering.
var ordering16 = [16]uint8{
	0, 15, 8, 7, 4, 11, 12, 3, 2, 13, 10, 5, 6, 9, 14, 1,
}
var ordering10 = [10]uint8{
	0, 9, 6, 3, 4, 5, 8, 1, 2, 7,
}

// compute whitening filter coefficients from normalized line spectral frequencies
//
// C: void silk_NLSF2A(opus_int16 *a_Q12, const opus_int16 *NLSF, const opus_int d, int arch)
func NLSF2A(
	a_Q12 []int16, // O    monic whitening filter coefficients in Q12,  [ d ]
	NLSF []int16, // I    normalized line spectral frequencies in Q15, [ d ]
	d int, // I    filter order (should be even)
	arch int, // I    Run-time architecture
) {
	const QA = 16
	var ordering []uint8
	var k, i, dd int
	var cos_LSF_QA [SILK_MAX_ORDER_LPC]int32
	var P, Q [SILK_MAX_ORDER_LPC/2 + 1]int32
	var Ptmp, Qtmp, f_int, f_frac, cos_val, delta int32
	var a32_QA1 [SILK_MAX_ORDER_LPC]int32

	silk_assert(LSF_COS_TAB_SZ_FIX == 128)
	silk_assert(d == 10 || d == 16)

	// convert LSFs to 2*cos(LSF), using piecewise linear curve from table
	if d == 16 {
		ordering = ordering16[:]
	} else {
		ordering = ordering10[:]
	}
	for k = 0; k < d; k++ {
		silk_assert(NLSF[k] >= 0)

		// f_int on a scale 0-127 (rounded down)
		f_int = RSHIFT(int32(NLSF[k]), 15-7)

		// f_frac, range: 0..255
		f_frac = int32(NLSF[k]) - LSHIFT(f_int, 15-7)

		silk_assert(f_int >= 0)
		silk_assert(f_int < LSF_COS_TAB_SZ_FIX)

		// Read start and end value from table
		cos_val = int32(LSFCosTab_FIX_Q12[f_int])           // Q12
		delta = int32(LSFCosTab_FIX_Q12[f_int+1]) - cos_val // Q12, with a range of 0..200

		// Linear interpolation
		cos_LSF_QA[ordering[k]] = RSHIFT_ROUND(LSHIFT(cos_val, 8)+MUL(delta, f_frac), 20-QA) // QA
	}

	dd = RSHIFT(d, 1)

	// generate even and odd polynomials using convolution
	nlsf2AFindPoly(P[:], cos_LSF_QA[0:], dd)
	nlsf2AFindPoly(Q[:], cos_LSF_QA[1:], dd)

	// convert even and odd polynomials to opus_int32 Q12 filter coefs
	for k = 0; k < dd; k++ {
		Ptmp = P[k+1] + P[k]
		Qtmp = Q[k+1] - Q[k]

		// the Ptmp and Qtmp values at this stage need to fit in int32
		a32_QA1[k] = -Qtmp - Ptmp    // QA+1
		a32_QA1[d-k-1] = Qtmp - Ptmp // QA+1
	}

	// Convert int32 coefficients to Q12 int16 coefs
	LPCFit(a_Q12, a32_QA1[:], 12, QA+1, d)

	for i = 0; LPCInversePredGain(a_Q12, d, arch) == 0 && i < MAX_LPC_STABILIZE_ITERATIONS; i++ {
		// Prediction coefficients are (too close to) unstable; apply bandwidth expansion
		// on the unscaled coefficients, convert to Q12 and measure again
		Bwexpander32(a32_QA1[:], d, 65536-LSHIFT(2, i))
		for k = 0; k < d; k++ {
			a_Q12[k] = int16(RSHIFT_ROUND(a32_QA1[k], QA+1-12)) // QA+1 -> Q12
		}
	}
}
