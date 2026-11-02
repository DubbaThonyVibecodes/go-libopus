package silk

// C: silk/fixed/pitch_analysis_core_FIX.c

/***********************************************************
* Pitch analyser function
********************************************************** */

const SCRATCH_SIZE = 22

/*************************************************************/
/*      FIXED POINT CORE PITCH ANALYSIS FUNCTION             */
/*************************************************************/

// C: opus_int silk_pitch_analysis_core(                  /* O    Voicing estimate: 0 voiced, 1 unvoiced                      */
//
//	const opus_int16            *frame,             /* I    Signal of length PE_FRAME_LENGTH_MS*Fs_kHz                  */
//	opus_int                    *pitch_out,         /* O    4 pitch lag values                                          */
//	opus_int16                  *lagIndex,          /* O    Lag Index                                                   */
//	opus_int8                   *contourIndex,      /* O    Pitch contour Index                                         */
//	opus_int                    *LTPCorr_Q15,       /* I/O  Normalized correlation; input: value from previous frame    */
//	opus_int                    prevLag,            /* I    Last lag of previous frame; set to zero is unvoiced         */
//	const opus_int32            search_thres1_Q16,  /* I    First stage threshold for lag candidates 0 - 1              */
//	const opus_int              search_thres2_Q15,  /* I    Final threshold for lag candidates 0 - 1                    */
//	const opus_int              Fs_kHz,             /* I    Sample frequency (kHz)                                      */
//	const opus_int              complexity,         /* I    Complexity setting, 0-2, where 2 is highest                 */
//	const opus_int              nb_subfr            /* I    number of 5 ms subframes                                    */
//
// )
//
// The C Lag_CB_ptr / Lag_range_ptr point at &table[ 0 ][ 0 ] and are indexed
// with matrix_ptr( ptr, row, col, N ) where N is always the row length of the
// table, so they are represented here as the rows of the Go 2-D arrays and
// matrix_ptr( ptr, r, c, N ) becomes ptr[ r ][ c ].
func PitchAnalysisCore(frame []int16, pitch_out []int, lagIndex *int16, contourIndex *int8, LTPCorr_Q15 *int, prevLag int, search_thres1_Q16 int32, search_thres2_Q15 int, Fs_kHz int, complexity int, nb_subfr int) int {
	var frame_8kHz [PE_MAX_FRAME_LENGTH_ST_2]int16
	var frame_4kHz [PE_MAX_FRAME_LENGTH_ST_1]int16
	var filt_state [6]int32
	/* C: opus_int32 scratch_mem[ 3 * PE_MAX_FRAME_LENGTH ], reused as an opus_int16 buffer for the scaled input frame */
	var scratch_mem [2 * 3 * PE_MAX_FRAME_LENGTH]int16
	var input_frame_ptr []int16
	var i, k, d, j int
	var C [PE_MAX_NB_SUBFR][(PE_MAX_LAG >> 1) + 5]int16
	/* target_ptr and basis_ptr are indices into frame_4kHz / frame_8kHz */
	var target_ptr, basis_ptr int
	var cross_corr, normalizer, energy, shift, energy_basis, energy_target int32
	var d_srch [PE_D_SRCH_LENGTH]int
	var Cmax, length_d_srch, length_d_comp int
	var d_comp [(PE_MAX_LAG >> 1) + 5]int16
	var sum, threshold, temp32, lag_counter int32
	var CBimax, CBimax_new, CBimax_old, lag, start_lag, end_lag, lag_new int
	var CC [PE_NB_CBKS_STAGE2_EXT]int32
	var CCmax, CCmax_b, CCmax_new_b, CCmax_new int32
	var energies_st3 [PE_MAX_NB_SUBFR][PE_NB_CBKS_STAGE3_MAX][PE_NB_STAGE3_LAGS]int32
	var crosscorr_st3 [PE_MAX_NB_SUBFR][PE_NB_CBKS_STAGE3_MAX][PE_NB_STAGE3_LAGS]int32
	var frame_length, frame_length_8kHz, frame_length_4kHz, max_sum_sq_length int
	var sf_length, sf_length_8kHz, sf_length_4kHz int
	var min_lag, min_lag_8kHz, min_lag_4kHz int
	var max_lag, max_lag_8kHz, max_lag_4kHz int
	var contour_bias_Q20, diff, lz, lshift int32
	var nb_cbk_search, cbk_size int
	var delta_lag_log2_sqr_Q7, lag_log2_Q7, prevLag_log2_Q7, prev_lag_bias_Q15, corr_thres_Q15 int32
	var Lag_CB_ptr [][]int8
	/* Check for valid sampling frequency */
	silk_assert(Fs_kHz == 8 || Fs_kHz == 12 || Fs_kHz == 16)

	/* Check for valid complexity setting */
	silk_assert(complexity >= SILK_PE_MIN_COMPLEX)
	silk_assert(complexity <= SILK_PE_MAX_COMPLEX)

	silk_assert(search_thres1_Q16 >= 0 && search_thres1_Q16 <= (1<<16))
	silk_assert(search_thres2_Q15 >= 0 && search_thres2_Q15 <= (1<<15))

	/* Set up frame lengths max / min lag for the sampling frequency */
	frame_length = (PE_LTP_MEM_LENGTH_MS + nb_subfr*PE_SUBFR_LENGTH_MS) * Fs_kHz
	frame_length_4kHz = (PE_LTP_MEM_LENGTH_MS + nb_subfr*PE_SUBFR_LENGTH_MS) * 4
	frame_length_8kHz = (PE_LTP_MEM_LENGTH_MS + nb_subfr*PE_SUBFR_LENGTH_MS) * 8
	sf_length = PE_SUBFR_LENGTH_MS * Fs_kHz
	sf_length_4kHz = PE_SUBFR_LENGTH_MS * 4
	sf_length_8kHz = PE_SUBFR_LENGTH_MS * 8
	min_lag = PE_MIN_LAG_MS * Fs_kHz
	min_lag_4kHz = PE_MIN_LAG_MS * 4
	min_lag_8kHz = PE_MIN_LAG_MS * 8
	max_lag = PE_MAX_LAG_MS*Fs_kHz - 1
	max_lag_4kHz = PE_MAX_LAG_MS * 4
	max_lag_8kHz = PE_MAX_LAG_MS*8 - 1

	clear(C[:nb_subfr])

	/* Resample from input sampled at Fs_kHz to 8 kHz */
	if Fs_kHz == 16 {
		clear(filt_state[:2])
		ResamplerDown2(filt_state[:], frame_8kHz[:], frame, int32(frame_length))
	} else if Fs_kHz == 12 {
		clear(filt_state[:6])
		ResamplerDown23(filt_state[:], frame_8kHz[:], frame, int32(frame_length))
	} else {
		silk_assert(Fs_kHz == 8)
		copy(frame_8kHz[:frame_length_8kHz], frame[:frame_length_8kHz])
	}

	/* Decimate again to 4 kHz */
	clear(filt_state[:2]) /* Set state to zero */
	ResamplerDown2(filt_state[:], frame_4kHz[:], frame_8kHz[:], int32(frame_length_8kHz))

	/* Low-pass filter */
	for i = frame_length_4kHz - 1; i > 0; i-- {
		frame_4kHz[i] = ADD_SAT16(frame_4kHz[i], frame_4kHz[i-1])
	}

	/*******************************************************************************
	** Scale 4 kHz signal down to prevent correlations measures from overflowing
	** find scaling as max scaling for each 8kHz(?) subframe
	*******************************************************************************/

	/* Inner product is calculated with different lengths, so scale for the worst case */
	max_sum_sq_length = int(max_32(int32(sf_length_8kHz), LSHIFT(sf_length_4kHz, 2)))
	shift = pAnaFindScaling(frame_4kHz[:], frame_length_4kHz, max_sum_sq_length)
	if shift > 0 {
		for i = 0; i < frame_length_4kHz; i++ {
			frame_4kHz[i] = RSHIFT(frame_4kHz[i], int(shift))
		}
	}

	/******************************************************************************
	* FIRST STAGE, operating in 4 khz
	******************************************************************************/
	target_ptr = int(LSHIFT(sf_length_4kHz, 2))
	for k = 0; k < nb_subfr>>1; k++ {
		/* Check that we are within range of the array */
		silk_assert(target_ptr >= 0)
		silk_assert(target_ptr+sf_length_8kHz <= frame_length_4kHz)

		basis_ptr = target_ptr - min_lag_4kHz

		/* Check that we are within range of the array */
		silk_assert(basis_ptr >= 0)
		silk_assert(basis_ptr+sf_length_8kHz <= frame_length_4kHz)

		/* Calculate first vector products before loop */
		cross_corr = InnerProdAligned(frame_4kHz[target_ptr:], frame_4kHz[basis_ptr:], sf_length_8kHz)
		normalizer = InnerProdAligned(frame_4kHz[basis_ptr:], frame_4kHz[basis_ptr:], sf_length_8kHz)
		normalizer = ADD_SAT32(normalizer, SMULBB(sf_length_8kHz, 4000))

		temp32 = DIV32(cross_corr, SQRT_APPROX(normalizer)+1)
		C[k][min_lag_4kHz] = int16(SAT16(temp32)) /* Q0 */

		/* From now on normalizer is computed recursively */
		for d = min_lag_4kHz + 1; d <= max_lag_4kHz; d++ {
			basis_ptr--

			/* Check that we are within range of the array */
			silk_assert(basis_ptr >= 0)
			silk_assert(basis_ptr+sf_length_8kHz <= frame_length_4kHz)

			cross_corr = InnerProdAligned(frame_4kHz[target_ptr:], frame_4kHz[basis_ptr:], sf_length_8kHz)

			/* Add contribution of new sample and remove contribution from oldest sample */
			normalizer +=
				SMULBB(frame_4kHz[basis_ptr], frame_4kHz[basis_ptr]) -
					SMULBB(frame_4kHz[basis_ptr+sf_length_8kHz], frame_4kHz[basis_ptr+sf_length_8kHz])

			temp32 = DIV32(cross_corr, SQRT_APPROX(normalizer)+1)
			C[k][d] = int16(SAT16(temp32)) /* Q0 */
		}
		/* Update target pointer */
		target_ptr += sf_length_8kHz
	}

	/* Combine two subframes into single correlation measure and apply short-lag bias */
	if nb_subfr == PE_MAX_NB_SUBFR {
		for i = max_lag_4kHz; i >= min_lag_4kHz; i-- {
			sum = int32(C[0][i]) + int32(C[1][i]) /* Q0 */
			silk_assert(RSHIFT(sum, 1) == SAT16(RSHIFT(sum, 1)))
			sum = RSHIFT(sum, 1) /* Q-1 */
			silk_assert(LSHIFT(int32(-i), 4) == SAT16(LSHIFT(int32(-i), 4)))
			sum = SMLAWB(sum, sum, LSHIFT(-i, 4)) /* Q-1 */
			silk_assert(sum == SAT16(sum))
			C[0][i] = int16(sum) /* Q-1 */
		}
	} else {
		/* Only short-lag bias */
		for i = max_lag_4kHz; i >= min_lag_4kHz; i-- {
			sum = int32(C[0][i])
			sum = SMLAWB(sum, sum, LSHIFT(-i, 4)) /* Q-1 */
			C[0][i] = int16(sum)                  /* Q-1 */
		}
	}

	/* Sort */
	length_d_srch = int(ADD_LSHIFT32(4, complexity, 1))
	silk_assert(3*length_d_srch <= PE_D_SRCH_LENGTH)
	InsertionSortDecreasingInt16(C[0][min_lag_4kHz:], d_srch[:], max_lag_4kHz-min_lag_4kHz+1, length_d_srch)

	/* Escape if correlation is very low already here */
	target_ptr = int(SMULBB(sf_length_4kHz, nb_subfr))
	energy = InnerProdAligned(frame_4kHz[target_ptr:], frame_4kHz[target_ptr:], int(LSHIFT(sf_length_4kHz, 2)))
	energy = ADD_SAT32(energy, 1000) /* Q0 */
	Cmax = int(C[0][min_lag_4kHz])   /* Q-1 */
	threshold = SMULBB(Cmax, Cmax)   /* Q-2 */

	/* Compare in Q-2 domain */
	if RSHIFT(energy, 4+2) > threshold {
		clear(pitch_out[:nb_subfr])
		*LTPCorr_Q15 = 0
		*lagIndex = 0
		*contourIndex = 0
		return 1
	}

	threshold = SMULWB(search_thres1_Q16, Cmax)
	for i = 0; i < length_d_srch; i++ {
		/* Convert to 8 kHz indices for the sorted correlation that exceeds the threshold */
		if int32(C[0][min_lag_4kHz+i]) > threshold {
			d_srch[i] = int(LSHIFT(d_srch[i]+min_lag_4kHz, 1))
		} else {
			length_d_srch = i
			break
		}
	}
	silk_assert(length_d_srch > 0)

	for i = min_lag_8kHz - 5; i < max_lag_8kHz+5; i++ {
		d_comp[i] = 0
	}
	for i = 0; i < length_d_srch; i++ {
		d_comp[d_srch[i]] = 1
	}

	/* Convolution */
	for i = max_lag_8kHz + 3; i >= min_lag_8kHz; i-- {
		d_comp[i] += d_comp[i-1] + d_comp[i-2]
	}

	length_d_srch = 0
	for i = min_lag_8kHz; i < max_lag_8kHz+1; i++ {
		if d_comp[i+1] > 0 {
			d_srch[length_d_srch] = i
			length_d_srch++
		}
	}

	/* Convolution */
	for i = max_lag_8kHz + 3; i >= min_lag_8kHz; i-- {
		d_comp[i] += d_comp[i-1] + d_comp[i-2] + d_comp[i-3]
	}

	length_d_comp = 0
	for i = min_lag_8kHz; i < max_lag_8kHz+4; i++ {
		if d_comp[i] > 0 {
			d_comp[length_d_comp] = int16(i - 2)
			length_d_comp++
		}
	}

	/**********************************************************************************
	** SECOND STAGE, operating at 8 kHz, on lag sections with high correlation
	*************************************************************************************/

	/******************************************************************************
	** Scale signal down to avoid correlations measures from overflowing
	*******************************************************************************/
	/* find scaling as max scaling for each subframe */
	shift = pAnaFindScaling(frame_8kHz[:], frame_length_8kHz, sf_length_8kHz)
	if shift > 0 {
		for i = 0; i < frame_length_8kHz; i++ {
			frame_8kHz[i] = RSHIFT(frame_8kHz[i], int(shift))
		}
	}

	/*********************************************************************************
	* Find energy of each subframe projected onto its history, for a range of delays
	*********************************************************************************/
	clear(C[:])

	target_ptr = PE_LTP_MEM_LENGTH_MS * 8
	for k = 0; k < nb_subfr; k++ {

		/* Check that we are within range of the array */
		silk_assert(target_ptr >= 0)
		silk_assert(target_ptr+sf_length_8kHz <= frame_length_8kHz)

		energy_target = InnerProdAligned(frame_8kHz[target_ptr:], frame_8kHz[target_ptr:], sf_length_8kHz)
		for j = 0; j < length_d_comp; j++ {
			d = int(d_comp[j])
			basis_ptr = target_ptr - d

			/* Check that we are within range of the array */
			silk_assert(basis_ptr >= 0)
			silk_assert(basis_ptr+sf_length_8kHz <= frame_length_8kHz)

			cross_corr = InnerProdAligned(frame_8kHz[target_ptr:], frame_8kHz[basis_ptr:], sf_length_8kHz)
			energy_basis = InnerProdAligned(frame_8kHz[basis_ptr:], frame_8kHz[basis_ptr:], sf_length_8kHz)
			if cross_corr > 0 {
				energy = max(energy_target, energy_basis) /* Find max to make sure first division < 1.0 */
				lz = CLZ32(cross_corr)
				lshift = LIMIT_32(lz-1, 0, 15)
				temp32 = DIV32(LSHIFT(cross_corr, int(lshift)), RSHIFT(energy, int(15-lshift))+1) /* Q15 */
				silk_assert(temp32 == SAT16(temp32))
				temp32 = SMULWB(cross_corr, temp32) /* Q(-1), cc * ( cc / max(b, t) ) */
				temp32 = ADD_SAT32(temp32, temp32)  /* Q(0) */
				lz = CLZ32(temp32)
				lshift = LIMIT_32(lz-1, 0, 15)
				energy = min(energy_target, energy_basis)
				C[k][d] = int16(DIV32(LSHIFT(temp32, int(lshift)), RSHIFT(energy, int(15-lshift))+1)) /* Q15*/
			} else {
				C[k][d] = 0
			}
		}
		target_ptr += sf_length_8kHz
	}

	/* search over lag range and lags codebook */
	/* scale factor for lag codebook, as a function of center lag */

	CCmax = int32_MIN
	CCmax_b = int32_MIN

	CBimax = 0 /* To avoid returning undefined lag values */
	lag = -1   /* To check if lag with strong enough correlation has been found */

	if prevLag > 0 {
		if Fs_kHz == 12 {
			prevLag = int(DIV32_16(LSHIFT(prevLag, 1), 3))
		} else if Fs_kHz == 16 {
			prevLag = RSHIFT(prevLag, 1)
		}
		prevLag_log2_Q7 = Lin2log(int32(prevLag))
	} else {
		prevLag_log2_Q7 = 0
	}
	silk_assert(search_thres2_Q15 == SAT16(search_thres2_Q15))
	/* Set up stage 2 codebook based on number of subframes */
	if nb_subfr == PE_MAX_NB_SUBFR {
		cbk_size = PE_NB_CBKS_STAGE2_EXT
		Lag_CB_ptr = [][]int8{CB_lags_stage2[0][:], CB_lags_stage2[1][:], CB_lags_stage2[2][:], CB_lags_stage2[3][:]}
		if Fs_kHz == 8 && complexity > SILK_PE_MIN_COMPLEX {
			/* If input is 8 khz use a larger codebook here because it is last stage */
			nb_cbk_search = PE_NB_CBKS_STAGE2_EXT
		} else {
			nb_cbk_search = PE_NB_CBKS_STAGE2
		}
		corr_thres_Q15 = RSHIFT(SMULBB(search_thres2_Q15, search_thres2_Q15), 13)
	} else {
		cbk_size = PE_NB_CBKS_STAGE2_10MS
		Lag_CB_ptr = [][]int8{CB_lags_stage2_10_ms[0][:], CB_lags_stage2_10_ms[1][:]}
		nb_cbk_search = PE_NB_CBKS_STAGE2_10MS
		corr_thres_Q15 = RSHIFT(SMULBB(search_thres2_Q15, search_thres2_Q15), 14)
	}
	_ = cbk_size /* only used by matrix_ptr() in C, which is Lag_CB_ptr[ row ][ col ] here */

	for k = 0; k < length_d_srch; k++ {
		d = d_srch[k]
		for j = 0; j < nb_cbk_search; j++ {
			CC[j] = 0
			for i = 0; i < nb_subfr; i++ {
				/* Try all codebooks */
				CC[j] = CC[j] + int32(C[i][d+int(Lag_CB_ptr[i][j])])
			}
		}
		/* Find best codebook */
		CCmax_new = int32_MIN
		CBimax_new = 0
		for i = 0; i < nb_cbk_search; i++ {
			if CC[i] > CCmax_new {
				CCmax_new = CC[i]
				CBimax_new = i
			}
		}

		/* Bias towards shorter lags */
		lag_log2_Q7 = Lin2log(int32(d)) /* Q7 */
		silk_assert(lag_log2_Q7 == SAT16(lag_log2_Q7))
		silk_assert(int32(nb_subfr)*SILK_FIX_CONST(PE_SHORTLAG_BIAS, 15) == SAT16(int32(nb_subfr)*SILK_FIX_CONST(PE_SHORTLAG_BIAS, 15)))
		CCmax_new_b = CCmax_new - RSHIFT(SMULBB(int32(nb_subfr)*SILK_FIX_CONST(PE_SHORTLAG_BIAS, 15), lag_log2_Q7), 7) /* Q15 */

		/* Bias towards previous lag */
		silk_assert(int32(nb_subfr)*SILK_FIX_CONST(PE_PREVLAG_BIAS, 15) == SAT16(int32(nb_subfr)*SILK_FIX_CONST(PE_PREVLAG_BIAS, 15)))
		if prevLag > 0 {
			delta_lag_log2_sqr_Q7 = lag_log2_Q7 - prevLag_log2_Q7
			silk_assert(delta_lag_log2_sqr_Q7 == SAT16(delta_lag_log2_sqr_Q7))
			delta_lag_log2_sqr_Q7 = RSHIFT(SMULBB(delta_lag_log2_sqr_Q7, delta_lag_log2_sqr_Q7), 7)
			prev_lag_bias_Q15 = RSHIFT(SMULBB(int32(nb_subfr)*SILK_FIX_CONST(PE_PREVLAG_BIAS, 15), *LTPCorr_Q15), 15) /* Q15 */
			prev_lag_bias_Q15 = DIV32(MUL(prev_lag_bias_Q15, delta_lag_log2_sqr_Q7), delta_lag_log2_sqr_Q7+(1<<6))
			CCmax_new_b -= prev_lag_bias_Q15 /* Q15 */
		}

		if CCmax_new_b > CCmax_b && /* Find maximum biased correlation                  */
			CCmax_new > corr_thres_Q15 && /* Correlation needs to be high enough to be voiced */
			int(CB_lags_stage2[0][CBimax_new]) <= min_lag_8kHz /* Lag must be in range                             */ {
			CCmax_b = CCmax_new_b
			CCmax = CCmax_new
			lag = d
			CBimax = CBimax_new
		}
	}

	if lag == -1 {
		/* No suitable candidate found */
		clear(pitch_out[:nb_subfr])
		*LTPCorr_Q15 = 0
		*lagIndex = 0
		*contourIndex = 0
		return 1
	}

	if Fs_kHz > 8 {
		/***************************************************************************/
		/* Scale input signal down to avoid correlations measures from overflowing */
		/***************************************************************************/
		/* find scaling as max scaling for each subframe */
		shift = pAnaFindScaling(frame, frame_length, sf_length)
		if shift > 0 {
			/* Move signal to scratch mem because the input signal should be unchanged */
			/* Reuse the 32 bit scratch mem vector, use a 16 bit pointer from now */
			input_frame_ptr = scratch_mem[:]
			for i = 0; i < frame_length; i++ {
				input_frame_ptr[i] = RSHIFT(frame[i], int(shift))
			}
		} else {
			input_frame_ptr = frame
		}

		/* Search in original signal */

		CBimax_old = CBimax
		/* Compensate for decimation */
		silk_assert(lag == int(SAT16(lag)))
		if Fs_kHz == 12 {
			lag = int(RSHIFT(SMULBB(lag, 3), 1))
		} else if Fs_kHz == 16 {
			lag = int(LSHIFT(lag, 1))
		} else {
			lag = int(SMULBB(lag, 3))
		}

		lag = LIMIT_int(lag, min_lag, max_lag)
		start_lag = max_int(lag-2, min_lag)
		end_lag = min_int(lag+2, max_lag)
		lag_new = lag /* to avoid undefined lag */
		CBimax = 0    /* to avoid undefined lag */
		silk_assert(LSHIFT(CCmax, 13) >= 0)
		*LTPCorr_Q15 = int(SQRT_APPROX(LSHIFT(CCmax, 13))) /* Output normalized correlation */

		CCmax = int32_MIN
		/* pitch lags according to second stage */
		for k = 0; k < nb_subfr; k++ {
			pitch_out[k] = lag + 2*int(CB_lags_stage2[k][CBimax_old])
		}
		/* Calculate the correlations and energies needed in stage 3 */
		pAnaCalcCorrSt3(&crosscorr_st3, input_frame_ptr, start_lag, sf_length, nb_subfr, complexity)
		pAnaCalcEnergySt3(&energies_st3, input_frame_ptr, start_lag, sf_length, nb_subfr, complexity)

		lag_counter = 0
		silk_assert(lag == int(SAT16(lag)))
		contour_bias_Q20 = DIV32_16(SILK_FIX_CONST(PE_FLATCONTOUR_BIAS, 20), lag)

		/* Set up codebook parameters according to complexity setting and frame length */
		if nb_subfr == PE_MAX_NB_SUBFR {
			nb_cbk_search = int(nb_cbk_searchs_stage3[complexity])
			cbk_size = PE_NB_CBKS_STAGE3_MAX
			Lag_CB_ptr = [][]int8{CB_lags_stage3[0][:], CB_lags_stage3[1][:], CB_lags_stage3[2][:], CB_lags_stage3[3][:]}
		} else {
			nb_cbk_search = PE_NB_CBKS_STAGE3_10MS
			cbk_size = PE_NB_CBKS_STAGE3_10MS
			Lag_CB_ptr = [][]int8{CB_lags_stage3_10_ms[0][:], CB_lags_stage3_10_ms[1][:]}
		}
		for d = start_lag; d <= end_lag; d++ {
			for j = 0; j < nb_cbk_search; j++ {
				cross_corr = 0
				energy = 0
				for k = 0; k < nb_subfr; k++ {
					silk_assert(PE_MAX_NB_SUBFR == 4)
					energy += RSHIFT(energies_st3[k][j][lag_counter], 2) /* use mean, to avoid overflow */
					silk_assert(energy >= 0)
					cross_corr += RSHIFT(crosscorr_st3[k][j][lag_counter], 2) /* use mean, to avoid overflow */
				}
				if cross_corr > 0 {
					/* Divide cross_corr / energy and get result in Q15 */
					lz = CLZ32(cross_corr)
					/* Divide with result in Q13, cross_corr could be larger than energy */
					lshift = LIMIT_32(lz-1, 0, 13)
					CCmax_new = DIV32(LSHIFT(cross_corr, int(lshift)), RSHIFT(energy, int(13-lshift))+1)
					CCmax_new = SAT16(CCmax_new)
					CCmax_new = SMULWB(cross_corr, CCmax_new)
					/* Saturate */
					if CCmax_new > RSHIFT(int32_MAX, 3) {
						CCmax_new = int32_MAX
					} else {
						CCmax_new = LSHIFT(CCmax_new, 3)
					}
					/* Reduce depending on flatness of contour */
					diff = int32(int16_MAX) - RSHIFT(MUL(contour_bias_Q20, j), 5) /* Q20 -> Q15 */
					silk_assert(diff == SAT16(diff))
					CCmax_new = LSHIFT(SMULWB(CCmax_new, diff), 1)
				} else {
					CCmax_new = 0
				}

				if CCmax_new > CCmax &&
					(d+int(CB_lags_stage3[0][j])) <= max_lag {
					CCmax = CCmax_new
					lag_new = d
					CBimax = j
				}
			}
			lag_counter++
		}

		for k = 0; k < nb_subfr; k++ {
			pitch_out[k] = lag_new + int(Lag_CB_ptr[k][CBimax])
			pitch_out[k] = LIMIT(pitch_out[k], min_lag, PE_MAX_LAG_MS*Fs_kHz)
		}
		*lagIndex = int16(lag_new - min_lag)
		*contourIndex = int8(CBimax)
	} else { /* Fs_kHz == 8 */
		/* Save Lags and correlation */
		CCmax = max(CCmax, 0)
		*LTPCorr_Q15 = int(SQRT_APPROX(LSHIFT(CCmax, 13))) /* Output normalized correlation */
		for k = 0; k < nb_subfr; k++ {
			pitch_out[k] = lag + int(Lag_CB_ptr[k][CBimax])
			pitch_out[k] = LIMIT(pitch_out[k], min_lag_8kHz, PE_MAX_LAG_MS*Fs_kHz)
		}
		*lagIndex = int16(lag - min_lag_8kHz)
		*contourIndex = int8(CBimax)
	}
	silk_assert(*lagIndex >= 0)
	/* return as voiced */
	return 0
}

/*************************************************************************/
/* Calculates the correlations used in stage 3 search. In order to cover */
/* the whole lag codebook for all the searched offset lags (lag +- 2),   */
/*************************************************************************/

// C: void silk_P_Ana_calc_corr_st3(
//
//	opus_int32        cross_corr_st3[ PE_MAX_NB_SUBFR ][ PE_NB_CBKS_STAGE3_MAX ][ PE_NB_STAGE3_LAGS ],/* (O) 3 DIM correlation array */
//	const opus_int16  frame[],                         /* I vector to correlate         */
//	opus_int          start_lag,                       /* I lag offset to search around */
//	opus_int          sf_length,                       /* I length of a 5 ms subframe   */
//	opus_int          nb_subfr,                        /* I number of subframes         */
//	opus_int          complexity                       /* I Complexity setting          */
//
// )
func pAnaCalcCorrSt3(cross_corr_st3 *[PE_MAX_NB_SUBFR][PE_NB_CBKS_STAGE3_MAX][PE_NB_STAGE3_LAGS]int32, frame []int16, start_lag int, sf_length int, nb_subfr int, complexity int) {
	/* target_ptr and basis_ptr are indices into frame */
	var target_ptr, basis_ptr int
	var cross_corr int32
	var i, j, k, lag_counter, lag_low, lag_high int
	var nb_cbk_search, delta, idx, cbk_size int
	var scratch_mem [SCRATCH_SIZE]int32
	var Lag_range_ptr, Lag_CB_ptr [][]int8

	silk_assert(complexity >= SILK_PE_MIN_COMPLEX)
	silk_assert(complexity <= SILK_PE_MAX_COMPLEX)

	if nb_subfr == PE_MAX_NB_SUBFR {
		Lag_range_ptr = [][]int8{Lag_range_stage3[complexity][0][:], Lag_range_stage3[complexity][1][:], Lag_range_stage3[complexity][2][:], Lag_range_stage3[complexity][3][:]}
		Lag_CB_ptr = [][]int8{CB_lags_stage3[0][:], CB_lags_stage3[1][:], CB_lags_stage3[2][:], CB_lags_stage3[3][:]}
		nb_cbk_search = int(nb_cbk_searchs_stage3[complexity])
		cbk_size = PE_NB_CBKS_STAGE3_MAX
	} else {
		silk_assert(nb_subfr == PE_MAX_NB_SUBFR>>1)
		Lag_range_ptr = [][]int8{Lag_range_stage3_10_ms[0][:], Lag_range_stage3_10_ms[1][:]}
		Lag_CB_ptr = [][]int8{CB_lags_stage3_10_ms[0][:], CB_lags_stage3_10_ms[1][:]}
		nb_cbk_search = PE_NB_CBKS_STAGE3_10MS
		cbk_size = PE_NB_CBKS_STAGE3_10MS
	}
	_ = cbk_size /* only used by matrix_ptr() in C, which is tab[ row ][ col ] here */

	target_ptr = int(LSHIFT(sf_length, 2)) /* Pointer to middle of frame */
	for k = 0; k < nb_subfr; k++ {
		lag_counter = 0

		/* Calculate the correlations for each subframe */
		lag_low = int(Lag_range_ptr[k][0])
		lag_high = int(Lag_range_ptr[k][1])
		for j = lag_low; j <= lag_high; j++ {
			basis_ptr = target_ptr - (start_lag + j)
			cross_corr = InnerProdAligned(frame[target_ptr:], frame[basis_ptr:], sf_length)
			silk_assert(lag_counter < SCRATCH_SIZE)
			scratch_mem[lag_counter] = cross_corr
			lag_counter++
		}

		delta = int(Lag_range_ptr[k][0])
		for i = 0; i < nb_cbk_search; i++ {
			/* Fill out the 3 dim array that stores the correlations for */
			/* each code_book vector for each start lag */
			idx = int(Lag_CB_ptr[k][i]) - delta
			for j = 0; j < PE_NB_STAGE3_LAGS; j++ {
				silk_assert(idx+j < SCRATCH_SIZE)
				silk_assert(idx+j < lag_counter)
				cross_corr_st3[k][i][j] = scratch_mem[idx+j]
			}
		}
		target_ptr += sf_length
	}
}

/********************************************************************/
/* Calculate the energies for first two subframes. The energies are */
/* calculated recursively.                                          */
/********************************************************************/

// C: void silk_P_Ana_calc_energy_st3(
//
//	opus_int32        energies_st3[ PE_MAX_NB_SUBFR ][ PE_NB_CBKS_STAGE3_MAX ][ PE_NB_STAGE3_LAGS ],/* (O) 3 DIM energy array */
//	const opus_int16  frame[],                         /* I vector to calc energy in    */
//	opus_int          start_lag,                       /* I lag offset to search around */
//	opus_int          sf_length,                       /* I length of one 5 ms subframe */
//	opus_int          nb_subfr,                     /* I number of subframes         */
//	opus_int          complexity                       /* I Complexity setting          */
//
// )
func pAnaCalcEnergySt3(energies_st3 *[PE_MAX_NB_SUBFR][PE_NB_CBKS_STAGE3_MAX][PE_NB_STAGE3_LAGS]int32, frame []int16, start_lag int, sf_length int, nb_subfr int, complexity int) {
	/* target_ptr and basis_ptr are indices into frame */
	var target_ptr, basis_ptr int
	var energy int32
	var k, i, j, lag_counter int
	var nb_cbk_search, delta, idx, cbk_size, lag_diff int
	var scratch_mem [SCRATCH_SIZE]int32
	var Lag_range_ptr, Lag_CB_ptr [][]int8

	silk_assert(complexity >= SILK_PE_MIN_COMPLEX)
	silk_assert(complexity <= SILK_PE_MAX_COMPLEX)

	if nb_subfr == PE_MAX_NB_SUBFR {
		Lag_range_ptr = [][]int8{Lag_range_stage3[complexity][0][:], Lag_range_stage3[complexity][1][:], Lag_range_stage3[complexity][2][:], Lag_range_stage3[complexity][3][:]}
		Lag_CB_ptr = [][]int8{CB_lags_stage3[0][:], CB_lags_stage3[1][:], CB_lags_stage3[2][:], CB_lags_stage3[3][:]}
		nb_cbk_search = int(nb_cbk_searchs_stage3[complexity])
		cbk_size = PE_NB_CBKS_STAGE3_MAX
	} else {
		silk_assert(nb_subfr == PE_MAX_NB_SUBFR>>1)
		Lag_range_ptr = [][]int8{Lag_range_stage3_10_ms[0][:], Lag_range_stage3_10_ms[1][:]}
		Lag_CB_ptr = [][]int8{CB_lags_stage3_10_ms[0][:], CB_lags_stage3_10_ms[1][:]}
		nb_cbk_search = PE_NB_CBKS_STAGE3_10MS
		cbk_size = PE_NB_CBKS_STAGE3_10MS
	}
	_ = cbk_size /* only used by matrix_ptr() in C, which is tab[ row ][ col ] here */

	target_ptr = int(LSHIFT(sf_length, 2))
	for k = 0; k < nb_subfr; k++ {
		lag_counter = 0

		/* Calculate the energy for first lag */
		basis_ptr = target_ptr - (start_lag + int(Lag_range_ptr[k][0]))
		energy = InnerProdAligned(frame[basis_ptr:], frame[basis_ptr:], sf_length)
		silk_assert(energy >= 0)
		scratch_mem[lag_counter] = energy
		lag_counter++

		lag_diff = (int(Lag_range_ptr[k][1]) - int(Lag_range_ptr[k][0]) + 1)
		for i = 1; i < lag_diff; i++ {
			/* remove part outside new window */
			energy -= SMULBB(frame[basis_ptr+sf_length-i], frame[basis_ptr+sf_length-i])
			silk_assert(energy >= 0)

			/* add part that comes into window */
			energy = ADD_SAT32(energy, SMULBB(frame[basis_ptr-i], frame[basis_ptr-i]))
			silk_assert(energy >= 0)
			silk_assert(lag_counter < SCRATCH_SIZE)
			scratch_mem[lag_counter] = energy
			lag_counter++
		}

		delta = int(Lag_range_ptr[k][0])
		for i = 0; i < nb_cbk_search; i++ {
			/* Fill out the 3 dim array that stores the correlations for    */
			/* each code_book vector for each start lag                     */
			idx = int(Lag_CB_ptr[k][i]) - delta
			for j = 0; j < PE_NB_STAGE3_LAGS; j++ {
				silk_assert(idx+j < SCRATCH_SIZE)
				silk_assert(idx+j < lag_counter)
				energies_st3[k][i][j] = scratch_mem[idx+j]
				silk_assert(energies_st3[k][i][j] >= 0)
			}
		}
		target_ptr += sf_length
	}
}

// C: opus_int32 silk_P_Ana_find_scaling(
//
//	const opus_int16  *frame,
//	const opus_int    frame_length,
//	const opus_int    sum_sqr_len
//
// )
func pAnaFindScaling(frame []int16, frame_length int, sum_sqr_len int) int32 {
	var nbits, x_max int32

	x_max = int32(Int16ArrayMaxabs(frame, int32(frame_length)))

	if x_max < int32(int16_MAX) {
		/* Number of bits needed for the sum of the squares */
		nbits = 32 - CLZ32(SMULBB(x_max, x_max))
	} else {
		/* Here we don't know if x_max should have been silk_int16_MAX + 1, so we expect the worst case */
		nbits = 30
	}
	nbits += 17 - CLZ16(int16(sum_sqr_len))

	/* Without a guarantee of saturation, we need to keep the 31st bit free */
	if nbits < 31 {
		return 0
	} else {
		return (nbits - 30)
	}
}
