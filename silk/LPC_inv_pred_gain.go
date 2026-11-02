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
// C: static opus_int32 LPC_inverse_pred_gain_QA_c(opus_int32 A_QA[ SILK_MAX_ORDER_LPC ], const opus_int order)
func lpcInversePredGainQA( // O   Returns inverse prediction gain in energy domain, Q30
	A_QA []int32, // I   Prediction coefficients
	order int, // I   Prediction order
) int32 {
	const QA = 24
	var k, n, mult2Q int
	var invGain_Q30, rc_Q31, rc_mult1_Q30, rc_mult2, tmp1, tmp2 int32

	invGain_Q30 = SILK_FIX_CONST(1.0, 30)
	for k = order - 1; k > 0; k-- {
		// Check for stability
		if (A_QA[k] > A_LIMIT) || (A_QA[k] < -A_LIMIT) {
			return 0
		}

		// Set RC equal to negated AR coef
		rc_Q31 = -LSHIFT(A_QA[k], 31-QA)

		// rc_mult1_Q30 range: [ 1 : 2^30 ]
		rc_mult1_Q30 = SUB32(SILK_FIX_CONST(1.0, 30), SMMUL(rc_Q31, rc_Q31))
		silk_assert(rc_mult1_Q30 > (1 << 15)) // reduce A_LIMIT if fails
		silk_assert(rc_mult1_Q30 <= (1 << 30))

		// Update inverse gain
		// invGain_Q30 range: [ 0 : 2^30 ]
		invGain_Q30 = LSHIFT(SMMUL(invGain_Q30, rc_mult1_Q30), 2)
		silk_assert(invGain_Q30 >= 0)
		silk_assert(invGain_Q30 <= (1 << 30))
		if invGain_Q30 < SILK_FIX_CONST(float32(1.0)/MAX_PREDICTION_POWER_GAIN, 30) {
			return 0
		}

		// rc_mult2 range: [ 2^30 : silk_int32_MAX ]
		mult2Q = int(32 - CLZ32(abs(rc_mult1_Q30)))
		rc_mult2 = INVERSE32_varQ(rc_mult1_Q30, mult2Q+30)

		// Update AR coefficient
		for n = 0; n < (k+1)>>1; n++ {
			var tmp64 int64
			tmp1 = A_QA[n]
			tmp2 = A_QA[k-n-1]
			tmp64 = RSHIFT_ROUND64(SMULL(SUB_SAT32(tmp1,
				MUL32_FRAC_Q(tmp2, rc_Q31, 31)), rc_mult2), mult2Q)
			if tmp64 > int64(int32_MAX) || tmp64 < int64(int32_MIN) {
				return 0
			}
			A_QA[n] = int32(tmp64)
			tmp64 = RSHIFT_ROUND64(SMULL(SUB_SAT32(tmp2,
				MUL32_FRAC_Q(tmp1, rc_Q31, 31)), rc_mult2), mult2Q)
			if tmp64 > int64(int32_MAX) || tmp64 < int64(int32_MIN) {
				return 0
			}
			A_QA[k-n-1] = int32(tmp64)
		}
	}

	// Check for stability
	if (A_QA[k] > A_LIMIT) || (A_QA[k] < -A_LIMIT) {
		return 0
	}

	// Set RC equal to negated AR coef
	rc_Q31 = -LSHIFT(A_QA[0], 31-QA)

	// Range: [ 1 : 2^30 ]
	rc_mult1_Q30 = SUB32(SILK_FIX_CONST(1.0, 30), SMMUL(rc_Q31, rc_Q31))

	// Update inverse gain
	// Range: [ 0 : 2^30 ]
	invGain_Q30 = LSHIFT(SMMUL(invGain_Q30, rc_mult1_Q30), 2)
	silk_assert(invGain_Q30 >= 0)
	silk_assert(invGain_Q30 <= (1 << 30))
	if invGain_Q30 < SILK_FIX_CONST(float32(1.0)/MAX_PREDICTION_POWER_GAIN, 30) {
		return 0
	}

	return invGain_Q30
}

// For input in Q12 domain
//
// C: opus_int32 silk_LPC_inverse_pred_gain_c(const opus_int16 *A_Q12, const opus_int order)
// C: #define silk_LPC_inverse_pred_gain(A_Q12, order, arch) ((void)(arch), silk_LPC_inverse_pred_gain_c(A_Q12, order))
func LPCInversePredGain( // O   Returns inverse prediction gain in energy domain, Q30
	A_Q12 []int16, // I   Prediction coefficients, Q12 [order]
	order int, // I   Prediction order
	arch int, // I   Run-time architecture
) int32 {
	const QA = 24
	var k int
	var Atmp_QA [SILK_MAX_ORDER_LPC]int32
	var DC_resp int32 = 0

	// Increase Q domain of the AR coefficients
	for k = 0; k < order; k++ {
		DC_resp += int32(A_Q12[k])
		Atmp_QA[k] = LSHIFT32(int32(A_Q12[k]), QA-12)
	}
	// If the DC is unstable, we don't even need to do the full calculations
	if DC_resp >= 4096 {
		return 0
	}
	return lpcInversePredGainQA(Atmp_QA[:], order)
}
