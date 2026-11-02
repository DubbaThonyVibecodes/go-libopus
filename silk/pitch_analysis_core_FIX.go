package silk

// C: silk/fixed/pitch_analysis_core_FIX.c

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

/***********************************************************
* Pitch analyser function
********************************************************** */

const SCRATCH_SIZE = 22
const SF_LENGTH_4KHZ = (PE_SUBFR_LENGTH_MS * 4)
const SF_LENGTH_8KHZ = (PE_SUBFR_LENGTH_MS * 8)
const MIN_LAG_4KHZ = (PE_MIN_LAG_MS * 4)
const MIN_LAG_8KHZ = (PE_MIN_LAG_MS * 8)
const MAX_LAG_4KHZ = (PE_MAX_LAG_MS * 4)
const MAX_LAG_8KHZ = (PE_MAX_LAG_MS*8 - 1)
const CSTRIDE_4KHZ = (MAX_LAG_4KHZ + 1 - MIN_LAG_4KHZ)
const CSTRIDE_8KHZ = (MAX_LAG_8KHZ + 3 - (MIN_LAG_8KHZ - 2))
const D_COMP_MIN = (MIN_LAG_8KHZ - 3)
const D_COMP_MAX = (MAX_LAG_8KHZ + 4)
const D_COMP_STRIDE = (D_COMP_MAX - D_COMP_MIN)

// C: typedef opus_int32 silk_pe_stage3_vals[ PE_NB_STAGE3_LAGS ];
type peStage3Vals [PE_NB_STAGE3_LAGS]int32

/*************************************************************/
/*      FIXED POINT CORE PITCH ANALYSIS FUNCTION             */
/*************************************************************/

// C: opus_int silk_pitch_analysis_core(                  /* O    Voicing estimate: 0 voiced, 1 unvoiced                      */
//
//	const opus_int16            *frame_unscaled,    /* I    Signal of length PE_FRAME_LENGTH_MS*Fs_kHz                  */
//	opus_int                    *pitch_out,         /* O    4 pitch lag values                                          */
//	opus_int16                  *lagIndex,          /* O    Lag Index                                                   */
//	opus_int8                   *contourIndex,      /* O    Pitch contour Index                                         */
//	opus_int                    *LTPCorr_Q15,       /* I/O  Normalized correlation; input: value from previous frame    */
//	opus_int                    prevLag,            /* I    Last lag of previous frame; set to zero is unvoiced         */
//	const opus_int32            search_thres1_Q16,  /* I    First stage threshold for lag candidates 0 - 1              */
//	const opus_int              search_thres2_Q13,  /* I    Final threshold for lag candidates 0 - 1                    */
//	const opus_int              Fs_kHz,             /* I    Sample frequency (kHz)                                      */
//	const opus_int              complexity,         /* I    Complexity setting, 0-2, where 2 is highest                 */
//	const opus_int              nb_subfr,           /* I    number of 5 ms subframes                                    */
//	int                         arch                /* I    Run-time architecture                                       */
//
// )
//
// The C Lag_CB_ptr / Lag_range_ptr point at &table[ 0 ][ 0 ] and are indexed
// with matrix_ptr( ptr, row, col, N ) where N is always the row length of the
// table, so they are represented here as the rows of the Go 2-D arrays and
// matrix_ptr( ptr, r, c, N ) becomes ptr[ r ][ c ]. The flat C matrix is a
// slice indexed as C[ row*stride + col ].
func PitchAnalysisCore(frame_unscaled []int16, pitch_out []int, lagIndex *int16, contourIndex *int8, LTPCorr_Q15 *int, prevLag int, search_thres1_Q16 int32, search_thres2_Q13 int, Fs_kHz int, complexity int, nb_subfr int, arch int) int {
	var frame_8kHz_buf []int16
	var frame_4kHz []int16
	var frame_scaled []int16
	var filt_state [6]int32
	var frame, frame_8kHz []int16
	var i, k, d, j int
	var C []int16
	var xcorr32 []int32
	/* target_ptr and basis_ptr are indices into frame_4kHz / frame_8kHz / frame */
	var target_ptr, basis_ptr int
	var cross_corr, normalizer, energy, energy_basis, energy_target int32
	var shift int
	var d_srch [PE_D_SRCH_LENGTH]int
	var Cmax, length_d_srch, length_d_comp int
	var d_comp []int16
	var sum, threshold, lag_counter int32
	var CBimax, CBimax_new, CBimax_old, lag, start_lag, end_lag, lag_new int
	var CC [PE_NB_CBKS_STAGE2_EXT]int32
	var CCmax, CCmax_b, CCmax_new_b, CCmax_new int32
	var energies_st3 []peStage3Vals
	var cross_corr_st3 []peStage3Vals
	var frame_length, frame_length_8kHz, frame_length_4kHz int
	var sf_length int
	var min_lag int
	var max_lag int
	var contour_bias_Q15, diff int32
	var nb_cbk_search, cbk_size int
	var delta_lag_log2_sqr_Q7, lag_log2_Q7, prevLag_log2_Q7, prev_lag_bias_Q13 int32
	var Lag_CB_ptr [][]int8
	/* Check for valid sampling frequency */
	silk_assert(Fs_kHz == 8 || Fs_kHz == 12 || Fs_kHz == 16)

	/* Check for valid complexity setting */
	silk_assert(complexity >= SILK_PE_MIN_COMPLEX)
	silk_assert(complexity <= SILK_PE_MAX_COMPLEX)

	silk_assert(search_thres1_Q16 >= 0 && search_thres1_Q16 <= (1<<16))
	silk_assert(search_thres2_Q13 >= 0 && search_thres2_Q13 <= (1<<13))

	/* Set up frame lengths max / min lag for the sampling frequency */
	frame_length = (PE_LTP_MEM_LENGTH_MS + nb_subfr*PE_SUBFR_LENGTH_MS) * Fs_kHz
	frame_length_4kHz = (PE_LTP_MEM_LENGTH_MS + nb_subfr*PE_SUBFR_LENGTH_MS) * 4
	frame_length_8kHz = (PE_LTP_MEM_LENGTH_MS + nb_subfr*PE_SUBFR_LENGTH_MS) * 8
	sf_length = PE_SUBFR_LENGTH_MS * Fs_kHz
	min_lag = PE_MIN_LAG_MS * Fs_kHz
	max_lag = PE_MAX_LAG_MS*Fs_kHz - 1

	/* Downscale input if necessary */
	SumSqrShift(&energy, &shift, frame_unscaled, frame_length)
	shift += 3 - int(CLZ32(energy)) /* at least two bits headroom */
	frame_scaled = make([]int16, frame_length)
	if shift > 0 {
		shift = RSHIFT(shift+1, 1)
		for i = 0; i < frame_length; i++ {
			frame_scaled[i] = RSHIFT(frame_unscaled[i], shift)
		}
		frame = frame_scaled
	} else {
		frame = frame_unscaled
	}

	if Fs_kHz == 8 {
		frame_8kHz_buf = make([]int16, 1)
	} else {
		frame_8kHz_buf = make([]int16, frame_length_8kHz)
	}
	/* Resample from input sampled at Fs_kHz to 8 kHz */
	if Fs_kHz == 16 {
		clear(filt_state[:2])
		ResamplerDown2(filt_state[:], frame_8kHz_buf, frame, int32(frame_length))
		frame_8kHz = frame_8kHz_buf
	} else if Fs_kHz == 12 {
		clear(filt_state[:6])
		ResamplerDown23(filt_state[:], frame_8kHz_buf, frame, int32(frame_length))
		frame_8kHz = frame_8kHz_buf
	} else {
		silk_assert(Fs_kHz == 8)
		frame_8kHz = frame
	}

	/* Decimate again to 4 kHz */
	clear(filt_state[:2]) /* Set state to zero */
	frame_4kHz = make([]int16, frame_length_4kHz)
	ResamplerDown2(filt_state[:], frame_4kHz, frame_8kHz, int32(frame_length_8kHz))

	/* Low-pass filter */
	for i = frame_length_4kHz - 1; i > 0; i-- {
		frame_4kHz[i] = ADD_SAT16(frame_4kHz[i], frame_4kHz[i-1])
	}

	/******************************************************************************
	* FIRST STAGE, operating in 4 khz
	******************************************************************************/
	C = make([]int16, nb_subfr*CSTRIDE_8KHZ)
	xcorr32 = make([]int32, MAX_LAG_4KHZ-MIN_LAG_4KHZ+1)
	clear(C[:(nb_subfr>>1)*CSTRIDE_4KHZ])
	target_ptr = int(LSHIFT(SF_LENGTH_4KHZ, 2))
	for k = 0; k < nb_subfr>>1; k++ {
		/* Check that we are within range of the array */
		silk_assert(target_ptr >= 0)
		silk_assert(target_ptr+SF_LENGTH_8KHZ <= frame_length_4kHz)

		basis_ptr = target_ptr - MIN_LAG_4KHZ

		/* Check that we are within range of the array */
		silk_assert(basis_ptr >= 0)
		silk_assert(basis_ptr+SF_LENGTH_8KHZ <= frame_length_4kHz)

		celt.PitchXcorr(frame_4kHz[target_ptr:], frame_4kHz[target_ptr-MAX_LAG_4KHZ:], xcorr32, SF_LENGTH_8KHZ, MAX_LAG_4KHZ-MIN_LAG_4KHZ+1, arch)

		/* Calculate first vector products before loop */
		cross_corr = xcorr32[MAX_LAG_4KHZ-MIN_LAG_4KHZ]
		normalizer = InnerProdAligned(frame_4kHz[target_ptr:], frame_4kHz[target_ptr:], SF_LENGTH_8KHZ, arch)
		normalizer = ADD32(normalizer, InnerProdAligned(frame_4kHz[basis_ptr:], frame_4kHz[basis_ptr:], SF_LENGTH_8KHZ, arch))
		normalizer = ADD32(normalizer, SMULBB(SF_LENGTH_8KHZ, 4000))

		C[k*CSTRIDE_4KHZ+0] =
			int16(DIV32_varQ(cross_corr, normalizer, 13+1)) /* Q13 */

		/* From now on normalizer is computed recursively */
		for d = MIN_LAG_4KHZ + 1; d <= MAX_LAG_4KHZ; d++ {
			basis_ptr--

			/* Check that we are within range of the array */
			silk_assert(basis_ptr >= 0)
			silk_assert(basis_ptr+SF_LENGTH_8KHZ <= frame_length_4kHz)

			cross_corr = xcorr32[MAX_LAG_4KHZ-d]

			/* Add contribution of new sample and remove contribution from oldest sample */
			normalizer = ADD32(normalizer,
				SMULBB(frame_4kHz[basis_ptr], frame_4kHz[basis_ptr])-
					SMULBB(frame_4kHz[basis_ptr+SF_LENGTH_8KHZ], frame_4kHz[basis_ptr+SF_LENGTH_8KHZ]))

			C[k*CSTRIDE_4KHZ+d-MIN_LAG_4KHZ] =
				int16(DIV32_varQ(cross_corr, normalizer, 13+1)) /* Q13 */
		}
		/* Update target pointer */
		target_ptr += SF_LENGTH_8KHZ
	}

	/* Combine two subframes into single correlation measure and apply short-lag bias */
	if nb_subfr == PE_MAX_NB_SUBFR {
		for i = MAX_LAG_4KHZ; i >= MIN_LAG_4KHZ; i-- {
			sum = int32(C[0*CSTRIDE_4KHZ+i-MIN_LAG_4KHZ]) +
				int32(C[1*CSTRIDE_4KHZ+i-MIN_LAG_4KHZ]) /* Q14 */
			sum = SMLAWB(sum, sum, LSHIFT(-i, 4)) /* Q14 */
			C[i-MIN_LAG_4KHZ] = int16(sum)        /* Q14 */
		}
	} else {
		/* Only short-lag bias */
		for i = MAX_LAG_4KHZ; i >= MIN_LAG_4KHZ; i-- {
			sum = LSHIFT(int32(C[i-MIN_LAG_4KHZ]), 1) /* Q14 */
			sum = SMLAWB(sum, sum, LSHIFT(-i, 4))     /* Q14 */
			C[i-MIN_LAG_4KHZ] = int16(sum)            /* Q14 */
		}
	}

	/* Sort */
	length_d_srch = int(ADD_LSHIFT32(4, complexity, 1))
	silk_assert(3*length_d_srch <= PE_D_SRCH_LENGTH)
	InsertionSortDecreasingInt16(C, d_srch[:], CSTRIDE_4KHZ,
		length_d_srch)

	/* Escape if correlation is very low already here */
	Cmax = int(C[0]) /* Q14 */
	if int32(Cmax) < SILK_FIX_CONST(0.2, 14) {
		clear(pitch_out[:nb_subfr])
		*LTPCorr_Q15 = 0
		*lagIndex = 0
		*contourIndex = 0
		return 1
	}

	threshold = SMULWB(search_thres1_Q16, Cmax)
	for i = 0; i < length_d_srch; i++ {
		/* Convert to 8 kHz indices for the sorted correlation that exceeds the threshold */
		if int32(C[i]) > threshold {
			d_srch[i] = int(LSHIFT(d_srch[i]+MIN_LAG_4KHZ, 1))
		} else {
			length_d_srch = i
			break
		}
	}
	silk_assert(length_d_srch > 0)

	d_comp = make([]int16, D_COMP_STRIDE)
	for i = D_COMP_MIN; i < D_COMP_MAX; i++ {
		d_comp[i-D_COMP_MIN] = 0
	}
	for i = 0; i < length_d_srch; i++ {
		d_comp[d_srch[i]-D_COMP_MIN] = 1
	}

	/* Convolution */
	for i = D_COMP_MAX - 1; i >= MIN_LAG_8KHZ; i-- {
		d_comp[i-D_COMP_MIN] +=
			d_comp[i-1-D_COMP_MIN] + d_comp[i-2-D_COMP_MIN]
	}

	length_d_srch = 0
	for i = MIN_LAG_8KHZ; i < MAX_LAG_8KHZ+1; i++ {
		if d_comp[i+1-D_COMP_MIN] > 0 {
			d_srch[length_d_srch] = i
			length_d_srch++
		}
	}

	/* Convolution */
	for i = D_COMP_MAX - 1; i >= MIN_LAG_8KHZ; i-- {
		d_comp[i-D_COMP_MIN] += d_comp[i-1-D_COMP_MIN] +
			d_comp[i-2-D_COMP_MIN] + d_comp[i-3-D_COMP_MIN]
	}

	length_d_comp = 0
	for i = MIN_LAG_8KHZ; i < D_COMP_MAX; i++ {
		if d_comp[i-D_COMP_MIN] > 0 {
			d_comp[length_d_comp] = int16(i - 2)
			length_d_comp++
		}
	}

	/**********************************************************************************
	** SECOND STAGE, operating at 8 kHz, on lag sections with high correlation
	*************************************************************************************/

	/*********************************************************************************
	* Find energy of each subframe projected onto its history, for a range of delays
	*********************************************************************************/
	clear(C[:nb_subfr*CSTRIDE_8KHZ])

	target_ptr = PE_LTP_MEM_LENGTH_MS * 8
	for k = 0; k < nb_subfr; k++ {

		/* Check that we are within range of the array */
		silk_assert(target_ptr >= 0)
		silk_assert(target_ptr+SF_LENGTH_8KHZ <= frame_length_8kHz)

		energy_target = ADD32(InnerProdAligned(frame_8kHz[target_ptr:], frame_8kHz[target_ptr:], SF_LENGTH_8KHZ, arch), 1)
		for j = 0; j < length_d_comp; j++ {
			d = int(d_comp[j])
			basis_ptr = target_ptr - d

			/* Check that we are within range of the array */
			silk_assert(basis_ptr >= 0)
			silk_assert(basis_ptr+SF_LENGTH_8KHZ <= frame_length_8kHz)

			cross_corr = InnerProdAligned(frame_8kHz[target_ptr:], frame_8kHz[basis_ptr:], SF_LENGTH_8KHZ, arch)
			if cross_corr > 0 {
				energy_basis = InnerProdAligned(frame_8kHz[basis_ptr:], frame_8kHz[basis_ptr:], SF_LENGTH_8KHZ, arch)
				C[k*CSTRIDE_8KHZ+d-(MIN_LAG_8KHZ-2)] =
					int16(DIV32_varQ(cross_corr,
						ADD32(energy_target,
							energy_basis),
						13+1)) /* Q13 */
			} else {
				C[k*CSTRIDE_8KHZ+d-(MIN_LAG_8KHZ-2)] = 0
			}
		}
		target_ptr += SF_LENGTH_8KHZ
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
	silk_assert(search_thres2_Q13 == int(SAT16(search_thres2_Q13)))
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
	} else {
		cbk_size = PE_NB_CBKS_STAGE2_10MS
		Lag_CB_ptr = [][]int8{CB_lags_stage2_10_ms[0][:], CB_lags_stage2_10_ms[1][:]}
		nb_cbk_search = PE_NB_CBKS_STAGE2_10MS
	}
	_ = cbk_size /* only used by matrix_ptr() in C, which is Lag_CB_ptr[ row ][ col ] here */

	for k = 0; k < length_d_srch; k++ {
		d = d_srch[k]
		for j = 0; j < nb_cbk_search; j++ {
			CC[j] = 0
			for i = 0; i < nb_subfr; i++ {
				var d_subfr int
				/* Try all codebooks */
				d_subfr = d + int(Lag_CB_ptr[i][j])
				CC[j] = CC[j] +
					int32(C[i*CSTRIDE_8KHZ+
						d_subfr-(MIN_LAG_8KHZ-2)])
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
		silk_assert(int32(nb_subfr)*SILK_FIX_CONST(PE_SHORTLAG_BIAS, 13) == SAT16(int32(nb_subfr)*SILK_FIX_CONST(PE_SHORTLAG_BIAS, 13)))
		CCmax_new_b = CCmax_new - RSHIFT(SMULBB(int32(nb_subfr)*SILK_FIX_CONST(PE_SHORTLAG_BIAS, 13), lag_log2_Q7), 7) /* Q13 */

		/* Bias towards previous lag */
		silk_assert(int32(nb_subfr)*SILK_FIX_CONST(PE_PREVLAG_BIAS, 13) == SAT16(int32(nb_subfr)*SILK_FIX_CONST(PE_PREVLAG_BIAS, 13)))
		if prevLag > 0 {
			delta_lag_log2_sqr_Q7 = lag_log2_Q7 - prevLag_log2_Q7
			silk_assert(delta_lag_log2_sqr_Q7 == SAT16(delta_lag_log2_sqr_Q7))
			delta_lag_log2_sqr_Q7 = RSHIFT(SMULBB(delta_lag_log2_sqr_Q7, delta_lag_log2_sqr_Q7), 7)
			prev_lag_bias_Q13 = RSHIFT(SMULBB(int32(nb_subfr)*SILK_FIX_CONST(PE_PREVLAG_BIAS, 13), *LTPCorr_Q15), 15) /* Q13 */
			prev_lag_bias_Q13 = DIV32(MUL(prev_lag_bias_Q13, delta_lag_log2_sqr_Q7), delta_lag_log2_sqr_Q7+SILK_FIX_CONST(0.5, 7))
			CCmax_new_b -= prev_lag_bias_Q13 /* Q13 */
		}

		if CCmax_new_b > CCmax_b && /* Find maximum biased correlation                  */
			CCmax_new > SMULBB(nb_subfr, search_thres2_Q13) && /* Correlation needs to be high enough to be voiced */
			int(CB_lags_stage2[0][CBimax_new]) <= MIN_LAG_8KHZ /* Lag must be in range                             */ {
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

	/* Output normalized correlation */
	*LTPCorr_Q15 = int(LSHIFT(DIV32_16(CCmax, nb_subfr), 2))
	silk_assert(*LTPCorr_Q15 >= 0)

	if Fs_kHz > 8 {
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

		CCmax = int32_MIN
		/* pitch lags according to second stage */
		for k = 0; k < nb_subfr; k++ {
			pitch_out[k] = lag + 2*int(CB_lags_stage2[k][CBimax_old])
		}

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

		/* Calculate the correlations and energies needed in stage 3 */
		energies_st3 = make([]peStage3Vals, nb_subfr*nb_cbk_search)
		cross_corr_st3 = make([]peStage3Vals, nb_subfr*nb_cbk_search)
		pAnaCalcCorrSt3(cross_corr_st3, frame, start_lag, sf_length, nb_subfr, complexity, arch)
		pAnaCalcEnergySt3(energies_st3, frame, start_lag, sf_length, nb_subfr, complexity, arch)

		lag_counter = 0
		silk_assert(lag == int(SAT16(lag)))
		contour_bias_Q15 = DIV32_16(SILK_FIX_CONST(PE_FLATCONTOUR_BIAS, 15), lag)

		target_ptr = PE_LTP_MEM_LENGTH_MS * Fs_kHz
		energy_target = ADD32(InnerProdAligned(frame[target_ptr:], frame[target_ptr:], nb_subfr*sf_length, arch), 1)
		for d = start_lag; d <= end_lag; d++ {
			for j = 0; j < nb_cbk_search; j++ {
				cross_corr = 0
				energy = energy_target
				for k = 0; k < nb_subfr; k++ {
					cross_corr = ADD32(cross_corr,
						cross_corr_st3[k*nb_cbk_search+j][lag_counter])
					energy = ADD32(energy,
						energies_st3[k*nb_cbk_search+j][lag_counter])
					silk_assert(energy >= 0)
				}
				if cross_corr > 0 {
					CCmax_new = DIV32_varQ(cross_corr, energy, 13+1) /* Q13 */
					/* Reduce depending on flatness of contour */
					diff = int32(int16_MAX) - MUL(contour_bias_Q15, j) /* Q15 */
					silk_assert(diff == SAT16(diff))
					CCmax_new = SMULWB(CCmax_new, diff) /* Q14 */
				} else {
					CCmax_new = 0
				}

				if CCmax_new > CCmax && (d+int(CB_lags_stage3[0][j])) <= max_lag {
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
		/* Save Lags */
		for k = 0; k < nb_subfr; k++ {
			pitch_out[k] = lag + int(Lag_CB_ptr[k][CBimax])
			pitch_out[k] = LIMIT(pitch_out[k], MIN_LAG_8KHZ, PE_MAX_LAG_MS*8)
		}
		*lagIndex = int16(lag - MIN_LAG_8KHZ)
		*contourIndex = int8(CBimax)
	}
	silk_assert(*lagIndex >= 0)
	/* return as voiced */
	return 0
}

/***********************************************************************
 * Calculates the correlations used in stage 3 search. In order to cover
 * the whole lag codebook for all the searched offset lags (lag +- 2),
 * the following correlations are needed in each sub frame:
 *
 * sf1: lag range [-8,...,7] total 16 correlations
 * sf2: lag range [-4,...,4] total 9 correlations
 * sf3: lag range [-3,....4] total 8 correltions
 * sf4: lag range [-6,....8] total 15 correlations
 *
 * In total 48 correlations. The direct implementation computed in worst
 * case 4*12*5 = 240 correlations, but more likely around 120.
 ***********************************************************************/

// C: static void silk_P_Ana_calc_corr_st3(
//
//	silk_pe_stage3_vals cross_corr_st3[],              /* O 3 DIM correlation array */
//	const opus_int16  frame[],                         /* I vector to correlate         */
//	opus_int          start_lag,                       /* I lag offset to search around */
//	opus_int          sf_length,                       /* I length of a 5 ms subframe   */
//	opus_int          nb_subfr,                        /* I number of subframes         */
//	opus_int          complexity,                      /* I Complexity setting          */
//	int               arch                             /* I Run-time architecture       */
//
// )
func pAnaCalcCorrSt3(cross_corr_st3 []peStage3Vals, frame []int16, start_lag int, sf_length int, nb_subfr int, complexity int, arch int) {
	/* target_ptr is an index into frame */
	var target_ptr int
	var i, j, k, lag_counter, lag_low, lag_high int
	var nb_cbk_search, delta, idx, cbk_size int
	var scratch_mem []int32
	var xcorr32 []int32
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
	scratch_mem = make([]int32, SCRATCH_SIZE)
	xcorr32 = make([]int32, SCRATCH_SIZE)

	target_ptr = int(LSHIFT(sf_length, 2)) /* Pointer to middle of frame */
	for k = 0; k < nb_subfr; k++ {
		lag_counter = 0

		/* Calculate the correlations for each subframe */
		lag_low = int(Lag_range_ptr[k][0])
		lag_high = int(Lag_range_ptr[k][1])
		silk_assert(lag_high-lag_low+1 <= SCRATCH_SIZE)
		celt.PitchXcorr(frame[target_ptr:], frame[target_ptr-start_lag-lag_high:], xcorr32, sf_length, lag_high-lag_low+1, arch)
		for j = lag_low; j <= lag_high; j++ {
			silk_assert(lag_counter < SCRATCH_SIZE)
			scratch_mem[lag_counter] = xcorr32[lag_high-j]
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
				cross_corr_st3[k*nb_cbk_search+i][j] =
					scratch_mem[idx+j]
			}
		}
		target_ptr += sf_length
	}
}

/********************************************************************/
/* Calculate the energies for first two subframes. The energies are */
/* calculated recursively.                                          */
/********************************************************************/

// C: static void silk_P_Ana_calc_energy_st3(
//
//	silk_pe_stage3_vals energies_st3[],                 /* O 3 DIM energy array */
//	const opus_int16  frame[],                          /* I vector to calc energy in    */
//	opus_int          start_lag,                        /* I lag offset to search around */
//	opus_int          sf_length,                        /* I length of one 5 ms subframe */
//	opus_int          nb_subfr,                         /* I number of subframes         */
//	opus_int          complexity,                       /* I Complexity setting          */
//	int               arch                              /* I Run-time architecture       */
//
// )
func pAnaCalcEnergySt3(energies_st3 []peStage3Vals, frame []int16, start_lag int, sf_length int, nb_subfr int, complexity int, arch int) {
	/* target_ptr and basis_ptr are indices into frame */
	var target_ptr, basis_ptr int
	var energy int32
	var k, i, j, lag_counter int
	var nb_cbk_search, delta, idx, cbk_size, lag_diff int
	var scratch_mem []int32
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
	scratch_mem = make([]int32, SCRATCH_SIZE)

	target_ptr = int(LSHIFT(sf_length, 2))
	for k = 0; k < nb_subfr; k++ {
		lag_counter = 0

		/* Calculate the energy for first lag */
		basis_ptr = target_ptr - (start_lag + int(Lag_range_ptr[k][0]))
		energy = InnerProdAligned(frame[basis_ptr:], frame[basis_ptr:], sf_length, arch)
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
				energies_st3[k*nb_cbk_search+i][j] =
					scratch_mem[idx+j]
				silk_assert(
					energies_st3[k*nb_cbk_search+i][j] >= 0)
			}
		}
		target_ptr += sf_length
	}
}
