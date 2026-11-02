package silk

// C: silk/fixed/burg_modified_FIX.c

// Compute reflection coefficients from input signal
//
// C: void silk_burg_modified(opus_int32 *res_nrg, opus_int *res_nrg_Q, opus_int32 A_Q16[], const opus_int16 x[], const opus_int32 minInvGain_Q30, const opus_int subfr_length, const opus_int nb_subfr, const opus_int D)
func BurgModified(
	res_nrg *int32, // O    Residual energy
	res_nrg_Q *int, // O    Residual energy Q value
	A_Q16 []int32, // O    Prediction coefficients (length order)
	x []int16, // I    Input signal, length: nb_subfr * ( D + subfr_length )
	minInvGain_Q30 int32, // I    Inverse of max prediction gain
	subfr_length int, // I    Input signal subframe length (incl. D preceding samples)
	nb_subfr int, // I    Number of subframes stacked in x
	D int, // I    Order
) {
	const MAX_FRAME_SIZE = 384 // subfr_length * nb_subfr = ( 0.005 * 16000 + 16 ) * 4 = 384

	const QA = 25
	const N_BITS_HEAD_ROOM = 2
	const MIN_RSHIFTS = -16
	const MAX_RSHIFTS = (32 - QA)

	var k, n, s, lz, rshifts, rshifts_extra, reached_max_gain int
	var C0, num, nrg, rc_Q31, invGain_Q30, Atmp_QA, Atmp1, tmp1, tmp2, x1, x2 int32
	var x_ptr []int16
	var C_first_row [SILK_MAX_ORDER_LPC]int32
	var C_last_row [SILK_MAX_ORDER_LPC]int32
	var Af_QA [SILK_MAX_ORDER_LPC]int32
	var CAf [SILK_MAX_ORDER_LPC + 1]int32
	var CAb [SILK_MAX_ORDER_LPC + 1]int32

	silk_assert(subfr_length*nb_subfr <= MAX_FRAME_SIZE)

	// Compute autocorrelations, added over subframes
	SumSqrShift(&C0, &rshifts, x, nb_subfr*subfr_length)
	if rshifts > MAX_RSHIFTS {
		C0 = LSHIFT32(C0, rshifts-MAX_RSHIFTS)
		silk_assert(C0 > 0)
		rshifts = MAX_RSHIFTS
	} else {
		lz = int(CLZ32(C0)) - 1
		rshifts_extra = N_BITS_HEAD_ROOM - lz
		if rshifts_extra > 0 {
			rshifts_extra = min(rshifts_extra, MAX_RSHIFTS-rshifts)
			C0 = RSHIFT32(C0, rshifts_extra)
		} else {
			rshifts_extra = max(rshifts_extra, MIN_RSHIFTS-rshifts)
			C0 = LSHIFT32(C0, -rshifts_extra)
		}
		rshifts += rshifts_extra
	}
	CAf[0] = C0 + SMMUL(SILK_FIX_CONST(FIND_LPC_COND_FAC, 32), C0) + 1 // Q(-rshifts)
	CAb[0] = CAf[0]
	clear(C_first_row[:SILK_MAX_ORDER_LPC])
	if rshifts > 0 {
		for s = 0; s < nb_subfr; s++ {
			x_ptr = x[s*subfr_length:]
			for n = 1; n < D+1; n++ {
				C_first_row[n-1] += int32(RSHIFT64(
					InnerProd16Aligned64(x_ptr, x_ptr[n:], subfr_length-n), rshifts))
			}
		}
	} else {
		for s = 0; s < nb_subfr; s++ {
			x_ptr = x[s*subfr_length:]
			for n = 1; n < D+1; n++ {
				C_first_row[n-1] += LSHIFT32(
					InnerProdAligned(x_ptr, x_ptr[n:], subfr_length-n), -rshifts)
			}
		}
	}
	copy(C_last_row[:SILK_MAX_ORDER_LPC], C_first_row[:SILK_MAX_ORDER_LPC])

	// Initialize
	CAf[0] = C0 + SMMUL(SILK_FIX_CONST(FIND_LPC_COND_FAC, 32), C0) + 1 // Q(-rshifts)
	CAb[0] = CAf[0]

	invGain_Q30 = int32(1) << 30
	reached_max_gain = 0
	for n = 0; n < D; n++ {
		// Update first row of correlation matrix (without first element)
		// Update last row of correlation matrix (without last element, stored in reversed order)
		// Update C * Af
		// Update C * flipud(Af) (stored in reversed order)
		if rshifts > -2 {
			for s = 0; s < nb_subfr; s++ {
				x_ptr = x[s*subfr_length:]
				x1 = -LSHIFT32(int32(x_ptr[n]), 16-rshifts)                // Q(16-rshifts)
				x2 = -LSHIFT32(int32(x_ptr[subfr_length-n-1]), 16-rshifts) // Q(16-rshifts)
				tmp1 = LSHIFT32(int32(x_ptr[n]), QA-16)                    // Q(QA-16)
				tmp2 = LSHIFT32(int32(x_ptr[subfr_length-n-1]), QA-16)     // Q(QA-16)
				for k = 0; k < n; k++ {
					C_first_row[k] = SMLAWB(C_first_row[k], x1, x_ptr[n-k-1])          // Q( -rshifts )
					C_last_row[k] = SMLAWB(C_last_row[k], x2, x_ptr[subfr_length-n+k]) // Q( -rshifts )
					Atmp_QA = Af_QA[k]
					tmp1 = SMLAWB(tmp1, Atmp_QA, x_ptr[n-k-1])            // Q(QA-16)
					tmp2 = SMLAWB(tmp2, Atmp_QA, x_ptr[subfr_length-n+k]) // Q(QA-16)
				}
				tmp1 = LSHIFT32(-tmp1, 32-QA-rshifts) // Q(16-rshifts)
				tmp2 = LSHIFT32(-tmp2, 32-QA-rshifts) // Q(16-rshifts)
				for k = 0; k <= n; k++ {
					CAf[k] = SMLAWB(CAf[k], tmp1, x_ptr[n-k])                // Q( -rshift )
					CAb[k] = SMLAWB(CAb[k], tmp2, x_ptr[subfr_length-n+k-1]) // Q( -rshift )
				}
			}
		} else {
			for s = 0; s < nb_subfr; s++ {
				x_ptr = x[s*subfr_length:]
				x1 = -LSHIFT32(int32(x_ptr[n]), -rshifts)                // Q( -rshifts )
				x2 = -LSHIFT32(int32(x_ptr[subfr_length-n-1]), -rshifts) // Q( -rshifts )
				tmp1 = LSHIFT32(int32(x_ptr[n]), 17)                     // Q17
				tmp2 = LSHIFT32(int32(x_ptr[subfr_length-n-1]), 17)      // Q17
				for k = 0; k < n; k++ {
					C_first_row[k] = MLA(C_first_row[k], x1, x_ptr[n-k-1])          // Q( -rshifts )
					C_last_row[k] = MLA(C_last_row[k], x2, x_ptr[subfr_length-n+k]) // Q( -rshifts )
					Atmp1 = RSHIFT_ROUND(Af_QA[k], QA-17)                           // Q17
					tmp1 = MLA(tmp1, x_ptr[n-k-1], Atmp1)                           // Q17
					tmp2 = MLA(tmp2, x_ptr[subfr_length-n+k], Atmp1)                // Q17
				}
				tmp1 = -tmp1 // Q17
				tmp2 = -tmp2 // Q17
				for k = 0; k <= n; k++ {
					CAf[k] = SMLAWW(CAf[k], tmp1,
						LSHIFT32(int32(x_ptr[n-k]), -rshifts-1)) // Q( -rshift )
					CAb[k] = SMLAWW(CAb[k], tmp2,
						LSHIFT32(int32(x_ptr[subfr_length-n+k-1]), -rshifts-1)) // Q( -rshift )
				}
			}
		}

		// Calculate nominator and denominator for the next order reflection (parcor) coefficient
		tmp1 = C_first_row[n]       // Q( -rshifts )
		tmp2 = C_last_row[n]        // Q( -rshifts )
		num = 0                     // Q( -rshifts )
		nrg = ADD32(CAb[0], CAf[0]) // Q( 1-rshifts )
		for k = 0; k < n; k++ {
			Atmp_QA = Af_QA[k]
			lz = int(CLZ32(abs(Atmp_QA))) - 1
			lz = min(32-QA, lz)
			Atmp1 = LSHIFT32(Atmp_QA, lz) // Q( QA + lz )

			tmp1 = ADD_LSHIFT32(tmp1, SMMUL(C_last_row[n-k-1], Atmp1), 32-QA-lz)  // Q( -rshifts )
			tmp2 = ADD_LSHIFT32(tmp2, SMMUL(C_first_row[n-k-1], Atmp1), 32-QA-lz) // Q( -rshifts )
			num = ADD_LSHIFT32(num, SMMUL(CAb[n-k], Atmp1), 32-QA-lz)             // Q( -rshifts )
			nrg = ADD_LSHIFT32(nrg, SMMUL(ADD32(CAb[k+1], CAf[k+1]),
				Atmp1), 32-QA-lz) // Q( 1-rshifts )
		}
		CAf[n+1] = tmp1         // Q( -rshifts )
		CAb[n+1] = tmp2         // Q( -rshifts )
		num = ADD32(num, tmp2)  // Q( -rshifts )
		num = LSHIFT32(-num, 1) // Q( 1-rshifts )

		// Calculate the next order reflection (parcor) coefficient
		if abs(num) < nrg {
			rc_Q31 = DIV32_varQ(num, nrg, 31)
		} else {
			if num > 0 {
				rc_Q31 = int32_MAX
			} else {
				rc_Q31 = int32_MIN
			}
		}

		// Update inverse prediction gain
		tmp1 = (int32(1) << 30) - SMMUL(rc_Q31, rc_Q31)
		tmp1 = LSHIFT(SMMUL(invGain_Q30, tmp1), 2)
		if tmp1 <= minInvGain_Q30 {
			// Max prediction gain exceeded; set reflection coefficient such that max prediction gain is exactly hit
			tmp2 = (int32(1) << 30) - DIV32_varQ(minInvGain_Q30, invGain_Q30, 30) // Q30
			rc_Q31 = SQRT_APPROX(tmp2)                                            // Q15
			// Newton-Raphson iteration
			rc_Q31 = RSHIFT32(rc_Q31+DIV32(tmp2, rc_Q31), 1) // Q15
			rc_Q31 = LSHIFT32(rc_Q31, 16)                    // Q31
			if num < 0 {
				// Ensure adjusted reflection coefficients has the original sign
				rc_Q31 = -rc_Q31
			}
			invGain_Q30 = minInvGain_Q30
			reached_max_gain = 1
		} else {
			invGain_Q30 = tmp1
		}

		// Update the AR coefficients
		for k = 0; k < (n+1)>>1; k++ {
			tmp1 = Af_QA[k]                                           // QA
			tmp2 = Af_QA[n-k-1]                                       // QA
			Af_QA[k] = ADD_LSHIFT32(tmp1, SMMUL(tmp2, rc_Q31), 1)     // QA
			Af_QA[n-k-1] = ADD_LSHIFT32(tmp2, SMMUL(tmp1, rc_Q31), 1) // QA
		}
		Af_QA[n] = RSHIFT32(rc_Q31, 31-QA) // QA

		if reached_max_gain != 0 {
			// Reached max prediction gain; set remaining coefficients to zero and exit loop
			for k = n + 1; k < D; k++ {
				Af_QA[k] = 0
			}
			break
		}

		// Update C * Af and C * Ab
		for k = 0; k <= n+1; k++ {
			tmp1 = CAf[k]                                           // Q( -rshifts )
			tmp2 = CAb[n-k+1]                                       // Q( -rshifts )
			CAf[k] = ADD_LSHIFT32(tmp1, SMMUL(tmp2, rc_Q31), 1)     // Q( -rshifts )
			CAb[n-k+1] = ADD_LSHIFT32(tmp2, SMMUL(tmp1, rc_Q31), 1) // Q( -rshifts )
		}
	}

	if reached_max_gain != 0 {
		for k = 0; k < D; k++ {
			// Scale coefficients
			A_Q16[k] = -RSHIFT_ROUND(Af_QA[k], QA-16)
		}
		// Subtract energy of preceding samples from C0
		if rshifts > 0 {
			for s = 0; s < nb_subfr; s++ {
				x_ptr = x[s*subfr_length:]
				C0 -= int32(RSHIFT64(InnerProd16Aligned64(x_ptr, x_ptr, D), rshifts))
			}
		} else {
			for s = 0; s < nb_subfr; s++ {
				x_ptr = x[s*subfr_length:]
				C0 -= LSHIFT32(InnerProdAligned(x_ptr, x_ptr, D), -rshifts)
			}
		}
		// Approximate residual energy
		*res_nrg = LSHIFT(SMMUL(invGain_Q30, C0), 2)
		*res_nrg_Q = -rshifts
	} else {
		// Return residual energy
		nrg = CAf[0]          // Q( -rshifts )
		tmp1 = int32(1) << 16 // Q16
		for k = 0; k < D; k++ {
			Atmp1 = RSHIFT_ROUND(Af_QA[k], QA-16) // Q16
			nrg = SMLAWW(nrg, CAf[k+1], Atmp1)    // Q( -rshifts )
			tmp1 = SMLAWW(tmp1, Atmp1, Atmp1)     // Q16
			A_Q16[k] = -Atmp1
		}
		*res_nrg = SMLAWW(nrg, SMMUL(SILK_FIX_CONST(FIND_LPC_COND_FAC, 32), C0), -tmp1) // Q( -rshifts )
		*res_nrg_Q = -rshifts
	}
}
