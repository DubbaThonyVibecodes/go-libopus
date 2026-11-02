package silk

// C: silk/fixed/schur_FIX.c

// Faster than schur64(), but much less accurate.
// uses SMLAWB(), requiring armv5E and higher.
//
// C: opus_int32 silk_schur(opus_int16 *rc_Q15, const opus_int32 *c, const opus_int32 order)
func Schur( // O    Returns residual energy
	rc_Q15 []int16, // O    reflection coefficients [order] Q15
	c []int32, // I    correlations [order+1]
	order int32, // I    prediction order
) int32 {
	var k, n, lz int
	var C [SILK_MAX_ORDER_LPC + 1][2]int32
	var Ctmp1, Ctmp2, rc_tmp_Q15 int32

	silk_assert(order == 6 || order == 8 || order == 10 || order == 12 || order == 14 || order == 16)

	// Get number of leading zeros
	lz = int(CLZ32(c[0]))

	// Copy correlations and adjust level to Q30
	if lz < 2 {
		// lz must be 1, so shift one to the right
		for k = 0; k < int(order)+1; k++ {
			C[k][1] = RSHIFT(c[k], 1)
			C[k][0] = C[k][1]
		}
	} else if lz > 2 {
		// Shift to the left
		lz -= 2
		for k = 0; k < int(order)+1; k++ {
			C[k][1] = LSHIFT(c[k], lz)
			C[k][0] = C[k][1]
		}
	} else {
		// No need to shift
		for k = 0; k < int(order)+1; k++ {
			C[k][1] = c[k]
			C[k][0] = C[k][1]
		}
	}

	for k = 0; k < int(order); k++ {
		// Check that we won't be getting an unstable rc, otherwise stop here.
		if abs_int32(C[k+1][0]) >= C[0][1] {
			if C[k+1][0] > 0 {
				rc_Q15[k] = int16(-SILK_FIX_CONST(float32(.99), 15))
			} else {
				rc_Q15[k] = int16(SILK_FIX_CONST(float32(.99), 15))
			}
			k++
			break
		}

		// Get reflection coefficient
		rc_tmp_Q15 = -DIV32_16(C[k+1][0], max_32(RSHIFT(C[0][1], 15), 1))

		// Clip (shouldn't happen for properly conditioned inputs)
		rc_tmp_Q15 = SAT16(rc_tmp_Q15)

		// Store
		rc_Q15[k] = int16(rc_tmp_Q15)

		// Update correlations
		for n = 0; n < int(order)-k; n++ {
			Ctmp1 = C[n+k+1][0]
			Ctmp2 = C[n][1]
			C[n+k+1][0] = SMLAWB(Ctmp1, LSHIFT(Ctmp2, 1), rc_tmp_Q15)
			C[n][1] = SMLAWB(Ctmp2, LSHIFT(Ctmp1, 1), rc_tmp_Q15)
		}
	}

	for ; k < int(order); k++ {
		rc_Q15[k] = 0
	}

	// return residual energy
	return max_32(1, C[0][1])
}
