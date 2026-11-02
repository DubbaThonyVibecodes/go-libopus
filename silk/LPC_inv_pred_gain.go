package silk

// C: silk/LPC_inv_pred_gain.c

// C: #define QA 24 — declared as a function-level const below (the name is
// reused with other values in NLSF2A.c and burg_modified_FIX.c).

// C: #define A_LIMIT SILK_FIX_CONST( 0.99975, QA )
var A_LIMIT = SILK_FIX_CONST(0.99975, 24)

// C: #define MUL32_FRAC_Q(a32, b32, Q) ((opus_int32)(silk_RSHIFT_ROUND64(silk_SMULL(a32, b32), Q)))
func MUL32_FRAC_Q(a32, b32 int32, Q int) int32 {
	return int32(RSHIFT_ROUND64(SMULL(a32, b32), Q))
}

// Compute inverse of LPC prediction gain, and
// test if LPC coefficients are stable (all poles within unit circle)
//
// C: static opus_int32 LPC_inverse_pred_gain_QA(opus_int32 A_QA[ 2 ][ SILK_MAX_ORDER_LPC ], const opus_int order)
func lpcInversePredGainQA( // O   Returns inverse prediction gain in energy domain, Q30
	A_QA *[2][SILK_MAX_ORDER_LPC]int32, // I   Prediction coefficients
	order int, // I   Prediction order
) int32 {
	const QA = 24
	var k, n, mult2Q int
	var invGain_Q30, rc_Q31, rc_mult1_Q30, rc_mult2, tmp_QA int32
	var Aold_QA, Anew_QA []int32

	Anew_QA = A_QA[order&1][:]

	invGain_Q30 = int32(1) << 30
	for k = order - 1; k > 0; k-- {
		// Check for stability
		if (Anew_QA[k] > A_LIMIT) || (Anew_QA[k] < -A_LIMIT) {
			return 0
		}

		// Set RC equal to negated AR coef
		rc_Q31 = -LSHIFT(Anew_QA[k], 31-QA)

		// rc_mult1_Q30 range: [ 1 : 2^30 ]
		rc_mult1_Q30 = (int32(1) << 30) - SMMUL(rc_Q31, rc_Q31)
		silk_assert(rc_mult1_Q30 > (1 << 15)) // reduce A_LIMIT if fails
		silk_assert(rc_mult1_Q30 <= (1 << 30))

		// rc_mult2 range: [ 2^30 : silk_int32_MAX ]
		mult2Q = int(32 - CLZ32(abs(rc_mult1_Q30)))
		rc_mult2 = INVERSE32_varQ(rc_mult1_Q30, mult2Q+30)

		// Update inverse gain
		// invGain_Q30 range: [ 0 : 2^30 ]
		invGain_Q30 = LSHIFT(SMMUL(invGain_Q30, rc_mult1_Q30), 2)
		silk_assert(invGain_Q30 >= 0)
		silk_assert(invGain_Q30 <= (1 << 30))

		// Swap pointers
		Aold_QA = Anew_QA
		Anew_QA = A_QA[k&1][:]

		// Update AR coefficient
		for n = 0; n < k; n++ {
			tmp_QA = Aold_QA[n] - MUL32_FRAC_Q(Aold_QA[k-n-1], rc_Q31, 31)
			Anew_QA[n] = MUL32_FRAC_Q(tmp_QA, rc_mult2, mult2Q)
		}
	}

	// Check for stability
	if (Anew_QA[0] > A_LIMIT) || (Anew_QA[0] < -A_LIMIT) {
		return 0
	}

	// Set RC equal to negated AR coef
	rc_Q31 = -LSHIFT(Anew_QA[0], 31-QA)

	// Range: [ 1 : 2^30 ]
	rc_mult1_Q30 = (int32(1) << 30) - SMMUL(rc_Q31, rc_Q31)

	// Update inverse gain
	// Range: [ 0 : 2^30 ]
	invGain_Q30 = LSHIFT(SMMUL(invGain_Q30, rc_mult1_Q30), 2)
	silk_assert(invGain_Q30 >= 0)
	silk_assert(invGain_Q30 <= 1<<30)

	return invGain_Q30
}

// For input in Q12 domain
//
// C: opus_int32 silk_LPC_inverse_pred_gain(const opus_int16 *A_Q12, const opus_int order)
func LPCInversePredGain( // O   Returns inverse prediction gain in energy domain, Q30
	A_Q12 []int16, // I   Prediction coefficients, Q12 [order]
	order int, // I   Prediction order
) int32 {
	const QA = 24
	var k int
	var Atmp_QA [2][SILK_MAX_ORDER_LPC]int32
	var Anew_QA []int32
	var DC_resp int32 = 0

	Anew_QA = Atmp_QA[order&1][:]

	// Increase Q domain of the AR coefficients
	for k = 0; k < order; k++ {
		DC_resp += int32(A_Q12[k])
		Anew_QA[k] = LSHIFT32(int32(A_Q12[k]), QA-12)
	}
	// If the DC is unstable, we don't even need to do the full calculations
	if DC_resp >= 4096 {
		return 0
	}
	return lpcInversePredGainQA(&Atmp_QA, order)
}

// For input in Q24 domain
//
// C: opus_int32 silk_LPC_inverse_pred_gain_Q24(const opus_int32 *A_Q24, const opus_int order)
func LPCInversePredGainQ24( // O    Returns inverse prediction gain in energy domain, Q30
	A_Q24 []int32, // I    Prediction coefficients [order]
	order int, // I    Prediction order
) int32 {
	const QA = 24
	var k int
	var Atmp_QA [2][SILK_MAX_ORDER_LPC]int32
	var Anew_QA []int32

	Anew_QA = Atmp_QA[order&1][:]

	// Increase Q domain of the AR coefficients
	for k = 0; k < order; k++ {
		Anew_QA[k] = RSHIFT32(A_Q24[k], 24-QA)
	}

	return lpcInversePredGainQA(&Atmp_QA, order)
}
