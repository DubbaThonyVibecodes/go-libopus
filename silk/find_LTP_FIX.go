package silk

// C: silk/fixed/find_LTP_FIX.c

/* Head room for correlations */
const LTP_CORRS_HEAD_ROOM = 2

// C: void silk_find_LTP_FIX(
//
//	opus_int16                      b_Q14[ MAX_NB_SUBFR * LTP_ORDER ],      /* O    LTP coefs                                                                   */
//	opus_int32                      WLTP[ MAX_NB_SUBFR * LTP_ORDER * LTP_ORDER ], /* O    Weight for LTP quantization                                           */
//	opus_int                        *LTPredCodGain_Q7,                      /* O    LTP coding gain                                                             */
//	const opus_int16                r_lpc[],                                /* I    residual signal after LPC signal + state for first 10 ms                    */
//	const opus_int                  lag[ MAX_NB_SUBFR ],                    /* I    LTP lags                                                                    */
//	const opus_int32                Wght_Q15[ MAX_NB_SUBFR ],               /* I    weights                                                                     */
//	const opus_int                  subfr_length,                           /* I    subframe length                                                             */
//	const opus_int                  nb_subfr,                               /* I    number of subframes                                                         */
//	const opus_int                  mem_offset,                             /* I    number of samples in LTP memory                                             */
//	opus_int                        corr_rshifts[ MAX_NB_SUBFR ]            /* O    right shifts applied to correlations                                        */
//
// )
func FindLTPFIX(b_Q14 []int16, WLTP []int32, LTPredCodGain_Q7 *int, r_lpc []int16, lag []int, Wght_Q15 []int32, subfr_length int, nb_subfr int, mem_offset int, corr_rshifts []int) {
	var i, k, lshift int
	/* C: r_ptr = &r_lpc[ mem_offset ] is read behind ( lag_ptr = r_ptr - ... ), so it is an index into r_lpc */
	var r_ptr int
	var lag_ptr []int16
	var b_Q14_ptr []int16

	var regu int32
	var WLTP_ptr []int32
	var b_Q16, delta_b_Q14 [LTP_ORDER]int32
	var d_Q14, nrg [MAX_NB_SUBFR]int32
	var g_Q26 int32
	var w [MAX_NB_SUBFR]int32
	var WLTP_max, max_abs_d_Q14, max_w_bits int32

	var temp32, denom32 int32
	var extra_shifts int
	var rr_shifts, maxRshifts, maxRshifts_wxtra, LZs int
	var LPC_res_nrg, LPC_LTP_res_nrg, div_Q16 int32
	var Rr [LTP_ORDER]int32
	var rr [MAX_NB_SUBFR]int32
	var wd, m_Q12 int32

	b_Q14_ptr = b_Q14
	WLTP_ptr = WLTP
	r_ptr = mem_offset
	for k = 0; k < nb_subfr; k++ {
		lag_ptr = r_lpc[r_ptr-(lag[k]+LTP_ORDER/2):]

		SumSqrShift(&rr[k], &rr_shifts, r_lpc[r_ptr:], subfr_length) /* rr[ k ] in Q( -rr_shifts ) */

		/* Assure headroom */
		LZs = int(CLZ32(rr[k]))
		if LZs < LTP_CORRS_HEAD_ROOM {
			rr[k] = RSHIFT_ROUND(rr[k], LTP_CORRS_HEAD_ROOM-LZs)
			rr_shifts += (LTP_CORRS_HEAD_ROOM - LZs)
		}
		corr_rshifts[k] = rr_shifts
		CorrMatrixFIX(lag_ptr, subfr_length, LTP_ORDER, LTP_CORRS_HEAD_ROOM, WLTP_ptr, &corr_rshifts[k]) /* WLTP_fix_ptr in Q( -corr_rshifts[ k ] ) */

		/* The correlation vector always has lower max abs value than rr and/or RR so head room is assured */
		CorrVectorFIX(lag_ptr, r_lpc[r_ptr:], subfr_length, LTP_ORDER, Rr[:], corr_rshifts[k]) /* Rr_fix_ptr   in Q( -corr_rshifts[ k ] ) */
		if corr_rshifts[k] > rr_shifts {
			rr[k] = RSHIFT(rr[k], corr_rshifts[k]-rr_shifts) /* rr[ k ] in Q( -corr_rshifts[ k ] ) */
		}
		silk_assert(rr[k] >= 0)

		regu = 1
		regu = SMLAWB(regu, rr[k], SILK_FIX_CONST(LTP_DAMPING/3, 16))
		regu = SMLAWB(regu, *matrix_ptr(WLTP_ptr, 0, 0, LTP_ORDER), SILK_FIX_CONST(LTP_DAMPING/3, 16))
		regu = SMLAWB(regu, *matrix_ptr(WLTP_ptr, LTP_ORDER-1, LTP_ORDER-1, LTP_ORDER), SILK_FIX_CONST(LTP_DAMPING/3, 16))
		RegularizeCorrelationsFIX(WLTP_ptr, rr[k:], regu, LTP_ORDER)

		SolveLDLFIX(WLTP_ptr, LTP_ORDER, Rr[:], b_Q16[:]) /* WLTP_fix_ptr and Rr_fix_ptr both in Q(-corr_rshifts[k]) */

		/* Limit and store in Q14 */
		fitLTP(b_Q16[:], b_Q14_ptr)

		/* Calculate residual energy */
		nrg[k] = ResidualEnergy16CovarFIX(b_Q14_ptr, WLTP_ptr, Rr[:], rr[k], LTP_ORDER, 14) /* nrg_fix in Q( -corr_rshifts[ k ] ) */

		/* temp = Wght[ k ] / ( nrg[ k ] * Wght[ k ] + 0.01f * subfr_length ); */
		extra_shifts = min_int(corr_rshifts[k], LTP_CORRS_HEAD_ROOM)
		denom32 = LSHIFT_SAT32(SMULWB(nrg[k], Wght_Q15[k]), 1+extra_shifts) + /* Q( -corr_rshifts[ k ] + extra_shifts ) */
			RSHIFT(SMULWB(int32(subfr_length), 655), corr_rshifts[k]-extra_shifts) /* Q( -corr_rshifts[ k ] + extra_shifts ) */
		denom32 = max(denom32, 1)
		silk_assert((int64(Wght_Q15[k]) << 16) < int64(int32_MAX))  /* Wght always < 0.5 in Q0 */
		temp32 = DIV32(LSHIFT(int32(Wght_Q15[k]), 16), denom32)     /* Q( 15 + 16 + corr_rshifts[k] - extra_shifts ) */
		temp32 = RSHIFT(temp32, 31+corr_rshifts[k]-extra_shifts-26) /* Q26 */

		/* Limit temp such that the below scaling never wraps around */
		WLTP_max = 0
		for i = 0; i < LTP_ORDER*LTP_ORDER; i++ {
			WLTP_max = max(WLTP_ptr[i], WLTP_max)
		}
		lshift = int(CLZ32(WLTP_max)) - 1 - 3 /* keep 3 bits free for vq_nearest_neighbor_fix */
		silk_assert(26-18+lshift >= 0)
		if 26-18+lshift < 31 {
			temp32 = min_32(temp32, LSHIFT(int32(1), 26-18+lshift))
		}

		ScaleVector32Q26Lshift18(WLTP_ptr, temp32, LTP_ORDER*LTP_ORDER) /* WLTP_ptr in Q( 18 - corr_rshifts[ k ] ) */

		w[k] = *matrix_ptr(WLTP_ptr, LTP_ORDER/2, LTP_ORDER/2, LTP_ORDER) /* w in Q( 18 - corr_rshifts[ k ] ) */
		silk_assert(w[k] >= 0)

		r_ptr += subfr_length
		b_Q14_ptr = b_Q14_ptr[LTP_ORDER:]
		WLTP_ptr = WLTP_ptr[LTP_ORDER*LTP_ORDER:]
	}

	maxRshifts = 0
	for k = 0; k < nb_subfr; k++ {
		maxRshifts = max_int(corr_rshifts[k], maxRshifts)
	}

	/* Compute LTP coding gain */
	if LTPredCodGain_Q7 != nil {
		LPC_LTP_res_nrg = 0
		LPC_res_nrg = 0
		silk_assert(LTP_CORRS_HEAD_ROOM >= 2) /* Check that no overflow will happen when adding */
		for k = 0; k < nb_subfr; k++ {
			LPC_res_nrg = ADD32(LPC_res_nrg, RSHIFT(ADD32(SMULWB(rr[k], Wght_Q15[k]), 1), 1+(maxRshifts-corr_rshifts[k])))          /* Q( -maxRshifts ) */
			LPC_LTP_res_nrg = ADD32(LPC_LTP_res_nrg, RSHIFT(ADD32(SMULWB(nrg[k], Wght_Q15[k]), 1), 1+(maxRshifts-corr_rshifts[k]))) /* Q( -maxRshifts ) */
		}
		LPC_LTP_res_nrg = max(LPC_LTP_res_nrg, 1) /* avoid division by zero */

		div_Q16 = DIV32_varQ(LPC_res_nrg, LPC_LTP_res_nrg, 16)
		*LTPredCodGain_Q7 = int(SMULBB(3, Lin2log(div_Q16)-(16<<7)))

		silk_assert(*LTPredCodGain_Q7 == int(SAT16(MUL(3, Lin2log(div_Q16)-(16<<7)))))
	}

	/* smoothing */
	/* d = sum( B, 1 ); */
	b_Q14_ptr = b_Q14
	for k = 0; k < nb_subfr; k++ {
		d_Q14[k] = 0
		for i = 0; i < LTP_ORDER; i++ {
			d_Q14[k] += int32(b_Q14_ptr[i])
		}
		b_Q14_ptr = b_Q14_ptr[LTP_ORDER:]
	}

	/* m = ( w * d' ) / ( sum( w ) + 1e-3 ); */

	/* Find maximum absolute value of d_Q14 and the bits used by w in Q0 */
	max_abs_d_Q14 = 0
	max_w_bits = 0
	for k = 0; k < nb_subfr; k++ {
		max_abs_d_Q14 = max_32(max_abs_d_Q14, abs(d_Q14[k]))
		/* w[ k ] is in Q( 18 - corr_rshifts[ k ] ) */
		/* Find bits needed in Q( 18 - maxRshifts ) */
		max_w_bits = max_32(max_w_bits, 32-CLZ32(w[k])+int32(corr_rshifts[k])-int32(maxRshifts))
	}

	/* max_abs_d_Q14 = (5 << 15); worst case, i.e. LTP_ORDER * -silk_int16_MIN */
	silk_assert(max_abs_d_Q14 <= (5 << 15))

	/* How many bits is needed for w*d' in Q( 18 - maxRshifts ) in the worst case, of all d_Q14's being equal to max_abs_d_Q14 */
	extra_shifts = int(max_w_bits + 32 - CLZ32(max_abs_d_Q14) - 14)

	/* Subtract what we got available; bits in output var plus maxRshifts */
	extra_shifts -= (32 - 1 - 2 + maxRshifts) /* Keep sign bit free as well as 2 bits for accumulation */
	extra_shifts = max_int(extra_shifts, 0)

	maxRshifts_wxtra = maxRshifts + extra_shifts

	temp32 = RSHIFT(int32(262), maxRshifts+extra_shifts) + 1 /* 1e-3f in Q( 18 - (maxRshifts + extra_shifts) ) */
	wd = 0
	for k = 0; k < nb_subfr; k++ {
		/* w has at least 2 bits of headroom so no overflow should happen */
		temp32 = ADD32(temp32, RSHIFT(w[k], maxRshifts_wxtra-corr_rshifts[k]))                      /* Q( 18 - maxRshifts_wxtra ) */
		wd = ADD32(wd, LSHIFT(SMULWW(RSHIFT(w[k], maxRshifts_wxtra-corr_rshifts[k]), d_Q14[k]), 2)) /* Q( 18 - maxRshifts_wxtra ) */
	}
	m_Q12 = DIV32_varQ(wd, temp32, 12)

	b_Q14_ptr = b_Q14
	for k = 0; k < nb_subfr; k++ {
		/* w_fix[ k ] from Q( 18 - corr_rshifts[ k ] ) to Q( 16 ) */
		if 2-corr_rshifts[k] > 0 {
			temp32 = RSHIFT(w[k], 2-corr_rshifts[k])
		} else {
			temp32 = LSHIFT_SAT32(w[k], corr_rshifts[k]-2)
		}

		g_Q26 = MUL(
			DIV32(
				SILK_FIX_CONST(LTP_SMOOTHING, 26),
				RSHIFT(SILK_FIX_CONST(LTP_SMOOTHING, 26), 10)+temp32), /* Q10 */
			LSHIFT_SAT32(SUB_SAT32(m_Q12, RSHIFT(d_Q14[k], 2)), 4)) /* Q16 */

		temp32 = 0
		for i = 0; i < LTP_ORDER; i++ {
			delta_b_Q14[i] = int32(max_16(b_Q14_ptr[i], 1638)) /* 1638_Q14 = 0.1_Q0 */
			temp32 += delta_b_Q14[i]                           /* Q14 */
		}
		temp32 = DIV32(g_Q26, temp32) /* Q14 -> Q12 */
		for i = 0; i < LTP_ORDER; i++ {
			b_Q14_ptr[i] = int16(LIMIT_32(int32(b_Q14_ptr[i])+SMULWB(LSHIFT_SAT32(temp32, 4), delta_b_Q14[i]), -16000, 28000))
		}
		b_Q14_ptr = b_Q14_ptr[LTP_ORDER:]
	}
}

// C: void silk_fit_LTP(
//
//	opus_int32 LTP_coefs_Q16[ LTP_ORDER ],
//	opus_int16 LTP_coefs_Q14[ LTP_ORDER ]
//
// )
func fitLTP(LTP_coefs_Q16 []int32, LTP_coefs_Q14 []int16) {
	var i int

	for i = 0; i < LTP_ORDER; i++ {
		LTP_coefs_Q14[i] = int16(SAT16(RSHIFT_ROUND(LTP_coefs_Q16[i], 2)))
	}
}
