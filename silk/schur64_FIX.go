package silk

// C: silk/fixed/schur64_FIX.c

// Slower than schur(), but more accurate.
// Uses SMULL(), available on armv4
//
// C: opus_int32 silk_schur64(opus_int32 rc_Q16[], const opus_int32 c[], opus_int32 order)
func Schur64( // O    returns residual energy
	rc_Q16 []int32, // O    Reflection coefficients [order] Q16
	c []int32, // I    Correlations [order+1]
	order int32, // I    Prediction order
) int32 {
	var k, n int
	var C [SILK_MAX_ORDER_LPC + 1][2]int32
	var Ctmp1_Q30, Ctmp2_Q30, rc_tmp_Q31 int32

	silk_assert(order >= 0 && order <= SILK_MAX_ORDER_LPC)

	// Check for invalid input
	if c[0] <= 0 {
		clear(rc_Q16[:order])
		return 0
	}

	k = 0
	for { // C: do { ... } while( ++k <= order );
		C[k][1] = c[k]
		C[k][0] = C[k][1]
		k++
		if !(k <= int(order)) {
			break
		}
	}

	for k = 0; k < int(order); k++ {
		// Check that we won't be getting an unstable rc, otherwise stop here.
		if abs_int32(C[k+1][0]) >= C[0][1] {
			if C[k+1][0] > 0 {
				rc_Q16[k] = -SILK_FIX_CONST(float32(.99), 16)
			} else {
				rc_Q16[k] = SILK_FIX_CONST(float32(.99), 16)
			}
			k++
			break
		}

		// Get reflection coefficient: divide two Q30 values and get result in Q31
		rc_tmp_Q31 = DIV32_varQ(-C[k+1][0], C[0][1], 31)

		// Save the output
		rc_Q16[k] = RSHIFT_ROUND(rc_tmp_Q31, 15)

		// Update correlations
		for n = 0; n < int(order)-k; n++ {
			Ctmp1_Q30 = C[n+k+1][0]
			Ctmp2_Q30 = C[n][1]

			// Multiply and add the highest int32
			C[n+k+1][0] = Ctmp1_Q30 + SMMUL(LSHIFT(Ctmp2_Q30, 1), rc_tmp_Q31)
			C[n][1] = Ctmp2_Q30 + SMMUL(LSHIFT(Ctmp1_Q30, 1), rc_tmp_Q31)
		}
	}

	for ; k < int(order); k++ {
		rc_Q16[k] = 0
	}

	return max_32(1, C[0][1])
}
