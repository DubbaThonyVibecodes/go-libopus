package silk

// C: silk/fixed/find_LTP_FIX.c

// C: void silk_find_LTP_FIX(
//
//	opus_int32                      XXLTP_Q17[ MAX_NB_SUBFR * LTP_ORDER * LTP_ORDER ], /* O    Correlation matrix                                               */
//	opus_int32                      xXLTP_Q17[ MAX_NB_SUBFR * LTP_ORDER ],  /* O    Correlation vector                                                          */
//	const opus_int16                r_ptr[],                                /* I    Residual signal after LPC                                                   */
//	const opus_int                  lag[ MAX_NB_SUBFR ],                    /* I    LTP lags                                                                    */
//	const opus_int                  subfr_length,                           /* I    Subframe length                                                             */
//	const opus_int                  nb_subfr,                               /* I    Number of subframes                                                         */
//	int                             arch                                    /* I    Run-time architecture                                                       */
//
// )
//
// The C reads up to lag[ k ] + LTP_ORDER / 2 samples behind r_ptr, so r_ptr is
// passed as the base slice plus the index r_idx of the C pointer within it.
func FindLTPFIX(XXLTP_Q17 []int32, xXLTP_Q17 []int32, r_ptr []int16, r_idx int, lag []int, subfr_length int, nb_subfr int, arch int) {
	var i, k, extra_shifts int
	var xx_shifts, xX_shifts, XX_shifts int
	var lag_ptr []int16
	var XXLTP_Q17_ptr, xXLTP_Q17_ptr []int32
	var xx, nrg, temp int32

	xXLTP_Q17_ptr = xXLTP_Q17
	XXLTP_Q17_ptr = XXLTP_Q17
	for k = 0; k < nb_subfr; k++ {
		lag_ptr = r_ptr[r_idx-(lag[k]+LTP_ORDER/2):]

		SumSqrShift(&xx, &xx_shifts, r_ptr[r_idx:], subfr_length+LTP_ORDER)                    /* xx in Q( -xx_shifts ) */
		CorrMatrixFIX(lag_ptr, subfr_length, LTP_ORDER, XXLTP_Q17_ptr, &nrg, &XX_shifts, arch) /* XXLTP_Q17_ptr and nrg in Q( -XX_shifts ) */
		extra_shifts = xx_shifts - XX_shifts
		if extra_shifts > 0 {
			/* Shift XX */
			xX_shifts = xx_shifts
			for i = 0; i < LTP_ORDER*LTP_ORDER; i++ {
				XXLTP_Q17_ptr[i] = RSHIFT32(XXLTP_Q17_ptr[i], extra_shifts) /* Q( -xX_shifts ) */
			}
			nrg = RSHIFT32(nrg, extra_shifts) /* Q( -xX_shifts ) */
		} else if extra_shifts < 0 {
			/* Shift xx */
			xX_shifts = XX_shifts
			xx = RSHIFT32(xx, -extra_shifts) /* Q( -xX_shifts ) */
		} else {
			xX_shifts = xx_shifts
		}
		CorrVectorFIX(lag_ptr, r_ptr[r_idx:], subfr_length, LTP_ORDER, xXLTP_Q17_ptr, xX_shifts, arch) /* xXLTP_Q17_ptr in Q( -xX_shifts ) */

		/* At this point all correlations are in Q(-xX_shifts) */
		temp = SMLAWB(1, nrg, SILK_FIX_CONST(LTP_CORR_INV_MAX, 16))
		temp = max(temp, xx)
		/* C: #if 0 ... #else */
		for i = 0; i < LTP_ORDER*LTP_ORDER; i++ {
			XXLTP_Q17_ptr[i] = int32(LSHIFT64(int64(XXLTP_Q17_ptr[i]), 17) / int64(temp))
		}
		for i = 0; i < LTP_ORDER; i++ {
			xXLTP_Q17_ptr[i] = int32(LSHIFT64(int64(xXLTP_Q17_ptr[i]), 17) / int64(temp))
		}
		/* C: #endif */
		r_idx += subfr_length
		XXLTP_Q17_ptr = XXLTP_Q17_ptr[LTP_ORDER*LTP_ORDER:]
		xXLTP_Q17_ptr = xXLTP_Q17_ptr[LTP_ORDER:]
	}
}
