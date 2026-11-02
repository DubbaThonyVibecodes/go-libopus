package silk

// C: silk/fixed/find_LPC_FIX.c

/* Finds LPC vector from correlations, and converts to NLSF */

// C: void silk_find_LPC_FIX(
//
//	silk_encoder_state              *psEncC,                                /* I/O  Encoder state                                                               */
//	opus_int16                      NLSF_Q15[],                             /* O    NLSFs                                                                       */
//	const opus_int16                x[],                                    /* I    Input signal                                                                */
//	const opus_int32                minInvGain_Q30                          /* I    Inverse of max prediction gain                                              */
//
// )
func (psEncC *EncoderState) FindLPCFIX(NLSF_Q15 []int16, x []int16, minInvGain_Q30 int32) {
	var k, subfr_length int
	var a_Q16 [MAX_LPC_ORDER]int32
	var isInterpLower, shift int
	var res_nrg0, res_nrg1 int32
	var rshift0, rshift1 int

	/* Used only for LSF interpolation */
	var a_tmp_Q16 [MAX_LPC_ORDER]int32
	var res_nrg_interp, res_nrg, res_tmp_nrg int32
	var res_nrg_interp_Q, res_nrg_Q, res_tmp_nrg_Q int
	var a_tmp_Q12 [MAX_LPC_ORDER]int16
	var NLSF0_Q15 [MAX_LPC_ORDER]int16

	subfr_length = psEncC.subfr_length + psEncC.predictLPCOrder

	/* Default: no interpolation */
	psEncC.indices.NLSFInterpCoef_Q2 = 4

	/* Burg AR analysis for the full frame */
	BurgModified(&res_nrg, &res_nrg_Q, a_Q16[:], x, minInvGain_Q30, subfr_length, psEncC.nb_subfr, psEncC.predictLPCOrder, psEncC.arch)

	if psEncC.useInterpolatedNLSFs != 0 && psEncC.first_frame_after_reset == 0 && psEncC.nb_subfr == MAX_NB_SUBFR {
		var LPC_res []int16

		/* Optimal solution for last 10 ms */
		BurgModified(&res_tmp_nrg, &res_tmp_nrg_Q, a_tmp_Q16[:], x[2*subfr_length:], minInvGain_Q30, subfr_length, 2, psEncC.predictLPCOrder, psEncC.arch)

		/* subtract residual energy here, as that's easier than adding it to the    */
		/* residual energy of the first 10 ms in each iteration of the search below */
		shift = res_tmp_nrg_Q - res_nrg_Q
		if shift >= 0 {
			if shift < 32 {
				res_nrg = res_nrg - RSHIFT(res_tmp_nrg, shift)
			}
		} else {
			silk_assert(shift > -32)
			res_nrg = RSHIFT(res_nrg, -shift) - res_tmp_nrg
			res_nrg_Q = res_tmp_nrg_Q
		}

		/* Convert to NLSFs */
		A2NLSF(NLSF_Q15, a_tmp_Q16[:], psEncC.predictLPCOrder)

		LPC_res = make([]int16, 2*subfr_length)

		/* Search over interpolation indices to find the one with lowest residual energy */
		for k = 3; k >= 0; k-- {
			/* Interpolate NLSFs for first half */
			Interpolate(NLSF0_Q15[:], psEncC.prev_NLSFq_Q15[:], NLSF_Q15, k, psEncC.predictLPCOrder)

			/* Convert to LPC for residual energy evaluation */
			NLSF2A(a_tmp_Q12[:], NLSF0_Q15[:], psEncC.predictLPCOrder, psEncC.arch)

			/* Calculate residual energy with NLSF interpolation */
			LPCAnalysisFilter(LPC_res, x, a_tmp_Q12[:], int32(2*subfr_length), int32(psEncC.predictLPCOrder), psEncC.arch)

			SumSqrShift(&res_nrg0, &rshift0, LPC_res[psEncC.predictLPCOrder:], subfr_length-psEncC.predictLPCOrder)
			SumSqrShift(&res_nrg1, &rshift1, LPC_res[psEncC.predictLPCOrder+subfr_length:], subfr_length-psEncC.predictLPCOrder)

			/* Add subframe energies from first half frame */
			shift = rshift0 - rshift1
			if shift >= 0 {
				res_nrg1 = RSHIFT(res_nrg1, shift)
				res_nrg_interp_Q = -rshift0
			} else {
				res_nrg0 = RSHIFT(res_nrg0, -shift)
				res_nrg_interp_Q = -rshift1
			}
			res_nrg_interp = ADD32(res_nrg0, res_nrg1)

			/* Compare with first half energy without NLSF interpolation, or best interpolated value so far */
			shift = res_nrg_interp_Q - res_nrg_Q
			if shift >= 0 {
				if RSHIFT(res_nrg_interp, shift) < res_nrg {
					isInterpLower = TRUE
				} else {
					isInterpLower = FALSE
				}
			} else {
				if -shift < 32 {
					if res_nrg_interp < RSHIFT(res_nrg, -shift) {
						isInterpLower = TRUE
					} else {
						isInterpLower = FALSE
					}
				} else {
					isInterpLower = FALSE
				}
			}

			/* Determine whether current interpolated NLSFs are best so far */
			if isInterpLower == TRUE {
				/* Interpolation has lower residual energy */
				res_nrg = res_nrg_interp
				res_nrg_Q = res_nrg_interp_Q
				psEncC.indices.NLSFInterpCoef_Q2 = int8(k)
			}
		}
	}

	if psEncC.indices.NLSFInterpCoef_Q2 == 4 {
		/* NLSF interpolation is currently inactive, calculate NLSFs from full frame AR coefficients */
		A2NLSF(NLSF_Q15, a_Q16[:], psEncC.predictLPCOrder)
	}

	silk_assert(psEncC.indices.NLSFInterpCoef_Q2 == 4 || (psEncC.useInterpolatedNLSFs != 0 && psEncC.first_frame_after_reset == 0 && psEncC.nb_subfr == MAX_NB_SUBFR))
}
