package silk

// C: silk/fixed/find_pred_coefs_FIX.c

// C: void silk_find_pred_coefs_FIX(
//
//	silk_encoder_state_FIX          *psEnc,                                 /* I/O  encoder state                                                               */
//	silk_encoder_control_FIX        *psEncCtrl,                             /* I/O  encoder control                                                             */
//	const opus_int16                res_pitch[],                            /* I    Residual from pitch analysis                                                */
//	const opus_int16                x[],                                    /* I    Speech signal                                                               */
//	opus_int                        condCoding                              /* I    The type of conditional coding to use                                       */
//
// )
//
// The C res_pitch pointer (res_pitch_frame in the caller) is read behind by
// silk_find_LTP_FIX, so it is passed as the base slice plus the index
// res_pitch_idx of the C pointer within it.
func (psEnc *EncoderStateFIX) FindPredCoefsFIX(psEncCtrl *EncoderControlFIX, res_pitch []int16, res_pitch_idx int, x []int16, condCoding int) {
	var i int
	var invGains_Q16, local_gains [MAX_NB_SUBFR]int32
	/* Set to NLSF_Q15 to zero so we don't copy junk to the state. */
	var NLSF_Q15 [MAX_LPC_ORDER]int16 // C: ={0}; a Go array is zero-initialised
	var x_ptr []int16
	var x_pre_ptr []int16
	var LPC_in_pre []int16
	var min_gain_Q16, minInvGain_Q30 int32

	/* C reads x - psEnc->sCmn.predictLPCOrder (and, inside silk_LTP_analysis_filter_FIX, further */
	/* behind by the pitch lag). The caller (encode_frame_FIX.c) passes                            */
	/* x = &psEnc->x_buf[ psEnc->sCmn.ltp_mem_length ], so psEnc.x_buf is indexed directly:         */
	/* x_minus_LPC is the index of the C pointer x - predictLPCOrder within psEnc.x_buf.             */
	silk_assert(&x[0] == &psEnc.x_buf[psEnc.sCmn.ltp_mem_length])
	x_minus_LPC := psEnc.sCmn.ltp_mem_length - psEnc.sCmn.predictLPCOrder

	/* weighting for weighted least squares */
	min_gain_Q16 = int32_MAX >> 6
	for i = 0; i < psEnc.sCmn.nb_subfr; i++ {
		min_gain_Q16 = min(min_gain_Q16, psEncCtrl.Gains_Q16[i])
	}
	for i = 0; i < psEnc.sCmn.nb_subfr; i++ {
		/* Divide to Q16 */
		silk_assert(psEncCtrl.Gains_Q16[i] > 0)
		/* Invert and normalize gains, and ensure that maximum invGains_Q16 is within range of a 16 bit int */
		invGains_Q16[i] = DIV32_varQ(min_gain_Q16, psEncCtrl.Gains_Q16[i], 16-2)

		/* Limit inverse */
		invGains_Q16[i] = max(invGains_Q16[i], 100)

		/* Square the inverted gains */
		silk_assert(invGains_Q16[i] == SAT16(invGains_Q16[i]))

		/* Invert the inverted and normalized gains */
		local_gains[i] = DIV32((int32(1) << 16), invGains_Q16[i])
	}

	LPC_in_pre = make([]int16, psEnc.sCmn.nb_subfr*psEnc.sCmn.predictLPCOrder+
		psEnc.sCmn.frame_length)
	if psEnc.sCmn.indices.signalType == TYPE_VOICED {
		var xXLTP_Q17 []int32
		var XXLTP_Q17 []int32

		/**********/
		/* VOICED */
		/**********/
		silk_assert(psEnc.sCmn.ltp_mem_length-psEnc.sCmn.predictLPCOrder >= psEncCtrl.pitchL[0]+LTP_ORDER/2)

		xXLTP_Q17 = make([]int32, psEnc.sCmn.nb_subfr*LTP_ORDER)
		XXLTP_Q17 = make([]int32, psEnc.sCmn.nb_subfr*LTP_ORDER*LTP_ORDER)

		/* LTP analysis */
		FindLTPFIX(XXLTP_Q17, xXLTP_Q17, res_pitch, res_pitch_idx,
			psEncCtrl.pitchL[:], psEnc.sCmn.subfr_length, psEnc.sCmn.nb_subfr, psEnc.sCmn.arch)

		/* Quantize LTP gain parameters */
		QuantLTPGains(psEncCtrl.LTPCoef_Q14[:], psEnc.sCmn.indices.LTPIndex[:], &psEnc.sCmn.indices.PERIndex,
			&psEnc.sCmn.sum_log_gain_Q7, &psEncCtrl.LTPredCodGain_Q7, XXLTP_Q17, xXLTP_Q17, psEnc.sCmn.subfr_length, psEnc.sCmn.nb_subfr, psEnc.sCmn.arch)

		/* Control LTP scaling */
		psEnc.LTPScaleCtrlFIX(psEncCtrl, condCoding)

		/* Create LTP residual */
		LTPAnalysisFilterFIX(LPC_in_pre, psEnc.x_buf[:], x_minus_LPC, psEncCtrl.LTPCoef_Q14[:],
			psEncCtrl.pitchL[:], invGains_Q16[:], psEnc.sCmn.subfr_length, psEnc.sCmn.nb_subfr, psEnc.sCmn.predictLPCOrder)

	} else {
		/************/
		/* UNVOICED */
		/************/
		/* Create signal with prepended subframes, scaled by inverse gains */
		x_ptr = psEnc.x_buf[x_minus_LPC:]
		x_pre_ptr = LPC_in_pre
		for i = 0; i < psEnc.sCmn.nb_subfr; i++ {
			ScaleCopyVector16(x_pre_ptr, x_ptr, invGains_Q16[i],
				psEnc.sCmn.subfr_length+psEnc.sCmn.predictLPCOrder)
			x_pre_ptr = x_pre_ptr[psEnc.sCmn.subfr_length+psEnc.sCmn.predictLPCOrder:]
			x_ptr = x_ptr[psEnc.sCmn.subfr_length:]
		}

		clear(psEncCtrl.LTPCoef_Q14[:psEnc.sCmn.nb_subfr*LTP_ORDER])
		psEncCtrl.LTPredCodGain_Q7 = 0
		psEnc.sCmn.sum_log_gain_Q7 = 0
		psEncCtrl.LTP_scale_Q14 = 0
	}

	/* Limit on total predictive coding gain */
	if psEnc.sCmn.first_frame_after_reset != 0 {
		minInvGain_Q30 = SILK_FIX_CONST(1.0/MAX_PREDICTION_POWER_GAIN_AFTER_RESET, 30)
	} else {
		minInvGain_Q30 = Log2lin(SMLAWB(16<<7, int32(psEncCtrl.LTPredCodGain_Q7), SILK_FIX_CONST(1.0/3, 16))) /* Q16 */
		minInvGain_Q30 = DIV32_varQ(minInvGain_Q30,
			SMULWW(SILK_FIX_CONST(MAX_PREDICTION_POWER_GAIN, 0),
				SMLAWB(SILK_FIX_CONST(0.25, 18), SILK_FIX_CONST(0.75, 18), psEncCtrl.coding_quality_Q14)), 14)
	}

	/* LPC_in_pre contains the LTP-filtered input for voiced, and the unfiltered input for unvoiced */
	psEnc.sCmn.FindLPCFIX(NLSF_Q15[:], LPC_in_pre, minInvGain_Q30)

	/* Quantize LSFs */
	psEnc.sCmn.ProcessNLSFs(&psEncCtrl.PredCoef_Q12, NLSF_Q15[:], psEnc.sCmn.prev_NLSFq_Q15[:])

	/* Calculate residual energy using quantized LPC coefficients */
	ResidualEnergyFIX(psEncCtrl.ResNrg[:], psEncCtrl.ResNrgQ[:], LPC_in_pre, &psEncCtrl.PredCoef_Q12, local_gains[:],
		psEnc.sCmn.subfr_length, psEnc.sCmn.nb_subfr, psEnc.sCmn.predictLPCOrder, psEnc.sCmn.arch)

	/* Copy to prediction struct for use in next frame for interpolation */
	copy(psEnc.sCmn.prev_NLSFq_Q15[:], NLSF_Q15[:])
}
