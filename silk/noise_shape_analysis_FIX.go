package silk

// C: silk/fixed/noise_shape_analysis_FIX.c

/* Compute gain to make warped filter coefficients have a zero mean log frequency response on a   */
/* non-warped frequency scale. (So that it can be implemented with a minimum-phase monic filter.) */
/* Note: A monic filter is one with the first coefficient equal to 1.0. In Silk we omit the first */
/* coefficient in an array of coefficients, for monic filters.                                    */

// C: static inline opus_int32 warped_gain( /* gain in Q16*/
//
//	const opus_int32     *coefs_Q24,
//	opus_int             lambda_Q16,
//	opus_int             order
//
// )
func warpedGain(coefs_Q24 []int32, lambda_Q16 int, order int) int32 {
	var i int
	var gain_Q24 int32

	lambda_Q16 = -lambda_Q16
	gain_Q24 = coefs_Q24[order-1]
	for i = order - 2; i >= 0; i-- {
		gain_Q24 = SMLAWB(coefs_Q24[i], gain_Q24, lambda_Q16)
	}
	gain_Q24 = SMLAWB(SILK_FIX_CONST(1.0, 24), gain_Q24, -lambda_Q16)
	return INVERSE32_varQ(gain_Q24, 40)
}

/* Convert warped filter coefficients to monic pseudo-warped coefficients and limit maximum     */
/* amplitude of monic warped coefficients by using bandwidth expansion on the true coefficients */

// C: static inline void limit_warped_coefs(
//
//	opus_int32           *coefs_syn_Q24,
//	opus_int32           *coefs_ana_Q24,
//	opus_int             lambda_Q16,
//	opus_int32           limit_Q24,
//	opus_int             order
//
// )
func limitWarpedCoefs(coefs_syn_Q24 []int32, coefs_ana_Q24 []int32, lambda_Q16 int, limit_Q24 int32, order int) {
	var i, iter int
	ind := 0
	var tmp, maxabs_Q24, chirp_Q16, gain_syn_Q16, gain_ana_Q16 int32
	var nom_Q16, den_Q24 int32

	/* Convert to monic coefficients */
	lambda_Q16 = -lambda_Q16
	for i = order - 1; i > 0; i-- {
		coefs_syn_Q24[i-1] = SMLAWB(coefs_syn_Q24[i-1], coefs_syn_Q24[i], lambda_Q16)
		coefs_ana_Q24[i-1] = SMLAWB(coefs_ana_Q24[i-1], coefs_ana_Q24[i], lambda_Q16)
	}
	lambda_Q16 = -lambda_Q16
	nom_Q16 = SMLAWB(SILK_FIX_CONST(1.0, 16), -int32(lambda_Q16), lambda_Q16)
	den_Q24 = SMLAWB(SILK_FIX_CONST(1.0, 24), coefs_syn_Q24[0], lambda_Q16)
	gain_syn_Q16 = DIV32_varQ(nom_Q16, den_Q24, 24)
	den_Q24 = SMLAWB(SILK_FIX_CONST(1.0, 24), coefs_ana_Q24[0], lambda_Q16)
	gain_ana_Q16 = DIV32_varQ(nom_Q16, den_Q24, 24)
	for i = 0; i < order; i++ {
		coefs_syn_Q24[i] = SMULWW(gain_syn_Q16, coefs_syn_Q24[i])
		coefs_ana_Q24[i] = SMULWW(gain_ana_Q16, coefs_ana_Q24[i])
	}

	for iter = 0; iter < 10; iter++ {
		/* Find maximum absolute value */
		maxabs_Q24 = -1
		for i = 0; i < order; i++ {
			tmp = max(abs_int32(coefs_syn_Q24[i]), abs_int32(coefs_ana_Q24[i]))
			if tmp > maxabs_Q24 {
				maxabs_Q24 = tmp
				ind = i
			}
		}
		if maxabs_Q24 <= limit_Q24 {
			/* Coefficients are within range - done */
			return
		}

		/* Convert back to true warped coefficients */
		for i = 1; i < order; i++ {
			coefs_syn_Q24[i-1] = SMLAWB(coefs_syn_Q24[i-1], coefs_syn_Q24[i], lambda_Q16)
			coefs_ana_Q24[i-1] = SMLAWB(coefs_ana_Q24[i-1], coefs_ana_Q24[i], lambda_Q16)
		}
		gain_syn_Q16 = INVERSE32_varQ(gain_syn_Q16, 32)
		gain_ana_Q16 = INVERSE32_varQ(gain_ana_Q16, 32)
		for i = 0; i < order; i++ {
			coefs_syn_Q24[i] = SMULWW(gain_syn_Q16, coefs_syn_Q24[i])
			coefs_ana_Q24[i] = SMULWW(gain_ana_Q16, coefs_ana_Q24[i])
		}

		/* Apply bandwidth expansion */
		chirp_Q16 = SILK_FIX_CONST(0.99, 16) - DIV32_varQ(
			SMULWB(maxabs_Q24-limit_Q24, SMLABB(SILK_FIX_CONST(0.8, 10), SILK_FIX_CONST(0.1, 10), iter)),
			MUL(maxabs_Q24, ind+1), 22)
		Bwexpander32(coefs_syn_Q24, order, chirp_Q16)
		Bwexpander32(coefs_ana_Q24, order, chirp_Q16)

		/* Convert to monic warped coefficients */
		lambda_Q16 = -lambda_Q16
		for i = order - 1; i > 0; i-- {
			coefs_syn_Q24[i-1] = SMLAWB(coefs_syn_Q24[i-1], coefs_syn_Q24[i], lambda_Q16)
			coefs_ana_Q24[i-1] = SMLAWB(coefs_ana_Q24[i-1], coefs_ana_Q24[i], lambda_Q16)
		}
		lambda_Q16 = -lambda_Q16
		nom_Q16 = SMLAWB(SILK_FIX_CONST(1.0, 16), -int32(lambda_Q16), lambda_Q16)
		den_Q24 = SMLAWB(SILK_FIX_CONST(1.0, 24), coefs_syn_Q24[0], lambda_Q16)
		gain_syn_Q16 = DIV32_varQ(nom_Q16, den_Q24, 24)
		den_Q24 = SMLAWB(SILK_FIX_CONST(1.0, 24), coefs_ana_Q24[0], lambda_Q16)
		gain_ana_Q16 = DIV32_varQ(nom_Q16, den_Q24, 24)
		for i = 0; i < order; i++ {
			coefs_syn_Q24[i] = SMULWW(gain_syn_Q16, coefs_syn_Q24[i])
			coefs_ana_Q24[i] = SMULWW(gain_ana_Q16, coefs_ana_Q24[i])
		}
	}
	silk_assert(false)
}

/**************************************************************/
/* Compute noise shaping coefficients and initial gain values */
/**************************************************************/

// C: void silk_noise_shape_analysis_FIX(
//
//	silk_encoder_state_FIX          *psEnc,                                 /* I/O  Encoder state FIX                                                           */
//	silk_encoder_control_FIX        *psEncCtrl,                             /* I/O  Encoder control FIX                                                         */
//	const opus_int16                *pitch_res,                             /* I    LPC residual from pitch analysis                                            */
//	const opus_int16                *x                                      /* I    Input signal [ frame_length + la_shape ]                                    */
//
// )
func (psEnc *EncoderStateFIX) NoiseShapeAnalysisFIX(psEncCtrl *EncoderControlFIX, pitch_res []int16, x []int16) {
	psShapeSt := &psEnc.sShape
	var k, i, nSamples, Qnrg, b_Q14, warping_Q16 int
	scale := 0
	var SNR_adj_dB_Q7, HarmBoost_Q16, HarmShapeGain_Q16, Tilt_Q16, tmp32 int32
	var nrg, pre_nrg_Q30, log_energy_Q7, log_energy_prev_Q7, energy_variation_Q7 int32
	var delta_Q16, BWExp1_Q16, BWExp2_Q16, gain_mult_Q16, gain_add_Q16, strength_Q16, b_Q8 int32
	var auto_corr [MAX_SHAPE_LPC_ORDER + 1]int32
	var refl_coef_Q16 [MAX_SHAPE_LPC_ORDER]int32
	var AR1_Q24 [MAX_SHAPE_LPC_ORDER]int32
	var AR2_Q24 [MAX_SHAPE_LPC_ORDER]int32
	var x_windowed [SHAPE_LPC_WIN_MAX]int16
	var x_ptr, pitch_res_ptr []int16

	/* Point to start of first LPC analysis block */
	/* C: x_ptr = x - psEnc->sCmn.la_shape; this reads before x. The caller (encode_frame_FIX.c)  */
	/* passes x = &psEnc->x_buf[ psEnc->sCmn.ltp_mem_length ], so index psEnc.x_buf directly.     */
	silk_assert(&x[0] == &psEnc.x_buf[psEnc.sCmn.ltp_mem_length])
	x_ptr = psEnc.x_buf[psEnc.sCmn.ltp_mem_length-psEnc.sCmn.la_shape:]

	/****************/
	/* GAIN CONTROL */
	/****************/
	SNR_adj_dB_Q7 = int32(psEnc.sCmn.SNR_dB_Q7)

	/* Input quality is the average of the quality in the lowest two VAD bands */
	psEncCtrl.input_quality_Q14 = int(RSHIFT(int32(psEnc.sCmn.input_quality_bands_Q15[0])+
		int32(psEnc.sCmn.input_quality_bands_Q15[1]), 2))

	/* Coding quality level, between 0.0_Q0 and 1.0_Q0, but in Q14 */
	psEncCtrl.coding_quality_Q14 = RSHIFT(SigmQ15(int(RSHIFT_ROUND(SNR_adj_dB_Q7-
		SILK_FIX_CONST(20.0, 7), 4))), 1)

	/* Reduce coding SNR during low speech activity */
	if psEnc.sCmn.useCBR == 0 {
		b_Q8 = SILK_FIX_CONST(1.0, 8) - int32(psEnc.sCmn.speech_activity_Q8)
		b_Q8 = SMULWB(LSHIFT(b_Q8, 8), b_Q8)
		SNR_adj_dB_Q7 = SMLAWB(SNR_adj_dB_Q7,
			SMULBB(SILK_FIX_CONST(-BG_SNR_DECR_dB, 7)>>(4+1), b_Q8),                                          /* Q11*/
			SMULWB(SILK_FIX_CONST(1.0, 14)+int32(psEncCtrl.input_quality_Q14), psEncCtrl.coding_quality_Q14)) /* Q12*/
	}

	if psEnc.sCmn.indices.signalType == TYPE_VOICED {
		/* Reduce gains for periodic signals */
		SNR_adj_dB_Q7 = SMLAWB(SNR_adj_dB_Q7, SILK_FIX_CONST(HARM_SNR_INCR_dB, 8), psEnc.LTPCorr_Q15)
	} else {
		/* For unvoiced signals and low-quality input, adjust the quality slower than SNR_dB setting */
		SNR_adj_dB_Q7 = SMLAWB(SNR_adj_dB_Q7,
			SMLAWB(SILK_FIX_CONST(6.0, 9), -SILK_FIX_CONST(0.4, 18), psEnc.sCmn.SNR_dB_Q7),
			SILK_FIX_CONST(1.0, 14)-int32(psEncCtrl.input_quality_Q14))
	}

	/*************************/
	/* SPARSENESS PROCESSING */
	/*************************/
	/* Set quantizer offset */
	if psEnc.sCmn.indices.signalType == TYPE_VOICED {
		/* Initially set to 0; may be overruled in process_gains(..) */
		psEnc.sCmn.indices.quantOffsetType = 0
		psEncCtrl.sparseness_Q8 = 0
	} else {
		/* Sparseness measure, based on relative fluctuations of energy per 2 milliseconds */
		nSamples = int(LSHIFT(psEnc.sCmn.fs_kHz, 1))
		energy_variation_Q7 = 0
		log_energy_prev_Q7 = 0
		pitch_res_ptr = pitch_res
		for k = 0; k < int(SMULBB(SUB_FRAME_LENGTH_MS, psEnc.sCmn.nb_subfr))/2; k++ {
			SumSqrShift(&nrg, &scale, pitch_res_ptr, nSamples)
			nrg += int32(RSHIFT(nSamples, scale)) /* Q(-scale)*/

			log_energy_Q7 = Lin2log(nrg)
			if k > 0 {
				energy_variation_Q7 += abs(log_energy_Q7 - log_energy_prev_Q7)
			}
			log_energy_prev_Q7 = log_energy_Q7
			pitch_res_ptr = pitch_res_ptr[nSamples:]
		}

		psEncCtrl.sparseness_Q8 = RSHIFT(SigmQ15(int(SMULWB(energy_variation_Q7-
			SILK_FIX_CONST(5.0, 7), SILK_FIX_CONST(0.1, 16)))), 7)

		/* Set quantization offset depending on sparseness measure */
		if psEncCtrl.sparseness_Q8 > int(SILK_FIX_CONST(SPARSENESS_THRESHOLD_QNT_OFFSET, 8)) {
			psEnc.sCmn.indices.quantOffsetType = 0
		} else {
			psEnc.sCmn.indices.quantOffsetType = 1
		}

		/* Increase coding SNR for sparse signals */
		SNR_adj_dB_Q7 = SMLAWB(SNR_adj_dB_Q7, SILK_FIX_CONST(SPARSE_SNR_INCR_dB, 15), int32(psEncCtrl.sparseness_Q8)-SILK_FIX_CONST(0.5, 8))
	}

	/*******************************/
	/* Control bandwidth expansion */
	/*******************************/
	/* More BWE for signals with high prediction gain */
	strength_Q16 = SMULWB(psEncCtrl.predGain_Q16, SILK_FIX_CONST(FIND_PITCH_WHITE_NOISE_FRACTION, 16))
	BWExp2_Q16 = DIV32_varQ(SILK_FIX_CONST(BANDWIDTH_EXPANSION, 16),
		SMLAWW(SILK_FIX_CONST(1.0, 16), strength_Q16, strength_Q16), 16)
	BWExp1_Q16 = BWExp2_Q16
	delta_Q16 = SMULWB(SILK_FIX_CONST(1.0, 16)-SMULBB(3, psEncCtrl.coding_quality_Q14),
		SILK_FIX_CONST(LOW_RATE_BANDWIDTH_EXPANSION_DELTA, 16))
	BWExp1_Q16 = SUB32(BWExp1_Q16, delta_Q16)
	BWExp2_Q16 = ADD32(BWExp2_Q16, delta_Q16)
	/* BWExp1 will be applied after BWExp2, so make it relative */
	BWExp1_Q16 = DIV32_16(LSHIFT(BWExp1_Q16, 14), RSHIFT(BWExp2_Q16, 2))

	if psEnc.sCmn.warping_Q16 > 0 {
		/* Slightly more warping in analysis will move quantization noise up in frequency, where it's better masked */
		warping_Q16 = int(SMLAWB(psEnc.sCmn.warping_Q16, int32(psEncCtrl.coding_quality_Q14), SILK_FIX_CONST(0.01, 18)))
	} else {
		warping_Q16 = 0
	}

	/********************************************/
	/* Compute noise shaping AR coefs and gains */
	/********************************************/
	for k = 0; k < psEnc.sCmn.nb_subfr; k++ {
		/* Apply window: sine slope followed by flat part followed by cosine slope */
		var shift, slope_part, flat_part int
		flat_part = psEnc.sCmn.fs_kHz * 3
		slope_part = RSHIFT(psEnc.sCmn.shapeWinLength-flat_part, 1)

		ApplySineWindow(x_windowed[:], x_ptr, 1, slope_part)
		shift = slope_part
		copy(x_windowed[shift:shift+flat_part], x_ptr[shift:shift+flat_part])
		shift += flat_part
		ApplySineWindow(x_windowed[shift:], x_ptr[shift:], 2, slope_part)

		/* Update pointer: next LPC analysis block */
		x_ptr = x_ptr[psEnc.sCmn.subfr_length:]

		if psEnc.sCmn.warping_Q16 > 0 {
			/* Calculate warped auto correlation */
			WarpedAutocorrelationFIX(auto_corr[:], &scale, x_windowed[:], warping_Q16, psEnc.sCmn.shapeWinLength, psEnc.sCmn.shapingLPCOrder)
		} else {
			/* Calculate regular auto correlation */
			Autocorr(auto_corr[:], &scale, x_windowed[:], psEnc.sCmn.shapeWinLength, psEnc.sCmn.shapingLPCOrder+1)
		}

		/* Add white noise, as a fraction of energy */
		auto_corr[0] = ADD32(auto_corr[0], max_32(SMULWB(RSHIFT(auto_corr[0], 4),
			SILK_FIX_CONST(SHAPE_WHITE_NOISE_FRACTION, 20)), 1))

		/* Calculate the reflection coefficients using schur */
		nrg = Schur64(refl_coef_Q16[:], auto_corr[:], int32(psEnc.sCmn.shapingLPCOrder))
		silk_assert(nrg >= 0)

		/* Convert reflection coefficients to prediction coefficients */
		K2aQ16(AR2_Q24[:], refl_coef_Q16[:], int32(psEnc.sCmn.shapingLPCOrder))

		Qnrg = -scale /* range: -12...30*/
		silk_assert(Qnrg >= -12)
		silk_assert(Qnrg <= 30)

		/* Make sure that Qnrg is an even number */
		if Qnrg&1 != 0 {
			Qnrg -= 1
			nrg >>= 1
		}

		tmp32 = SQRT_APPROX(nrg)
		Qnrg >>= 1 /* range: -6...15*/

		psEncCtrl.Gains_Q16[k] = LSHIFT_SAT32(tmp32, 16-Qnrg)

		if psEnc.sCmn.warping_Q16 > 0 {
			/* Adjust gain for warping */
			gain_mult_Q16 = warpedGain(AR2_Q24[:], warping_Q16, psEnc.sCmn.shapingLPCOrder)
			silk_assert(psEncCtrl.Gains_Q16[k] >= 0)
			if SMULWW(RSHIFT_ROUND(psEncCtrl.Gains_Q16[k], 1), gain_mult_Q16) >= (int32_MAX >> 1) {
				psEncCtrl.Gains_Q16[k] = int32_MAX
			} else {
				psEncCtrl.Gains_Q16[k] = SMULWW(psEncCtrl.Gains_Q16[k], gain_mult_Q16)
			}
		}

		/* Bandwidth expansion for synthesis filter shaping */
		Bwexpander32(AR2_Q24[:], psEnc.sCmn.shapingLPCOrder, BWExp2_Q16)

		/* Compute noise shaping filter coefficients */
		copy(AR1_Q24[:psEnc.sCmn.shapingLPCOrder], AR2_Q24[:psEnc.sCmn.shapingLPCOrder])

		/* Bandwidth expansion for analysis filter shaping */
		silk_assert(BWExp1_Q16 <= SILK_FIX_CONST(1.0, 16))
		Bwexpander32(AR1_Q24[:], psEnc.sCmn.shapingLPCOrder, BWExp1_Q16)

		/* Ratio of prediction gains, in energy domain */
		pre_nrg_Q30 = LPCInversePredGainQ24(AR2_Q24[:], psEnc.sCmn.shapingLPCOrder)
		nrg = LPCInversePredGainQ24(AR1_Q24[:], psEnc.sCmn.shapingLPCOrder)

		/*psEncCtrl->GainsPre[ k ] = 1.0f - 0.7f * ( 1.0f - pre_nrg / nrg ) = 0.3f + 0.7f * pre_nrg / nrg;*/
		pre_nrg_Q30 = LSHIFT32(SMULWB(pre_nrg_Q30, SILK_FIX_CONST(0.7, 15)), 1)
		psEncCtrl.GainsPre_Q14[k] = int(SILK_FIX_CONST(0.3, 14)) + int(DIV32_varQ(pre_nrg_Q30, nrg, 14))

		/* Convert to monic warped prediction coefficients and limit absolute values */
		limitWarpedCoefs(AR2_Q24[:], AR1_Q24[:], warping_Q16, SILK_FIX_CONST(3.999, 24), psEnc.sCmn.shapingLPCOrder)

		/* Convert from Q24 to Q13 and store in int16 */
		for i = 0; i < psEnc.sCmn.shapingLPCOrder; i++ {
			psEncCtrl.AR1_Q13[k*MAX_SHAPE_LPC_ORDER+i] = int16(SAT16(RSHIFT_ROUND(AR1_Q24[i], 11)))
			psEncCtrl.AR2_Q13[k*MAX_SHAPE_LPC_ORDER+i] = int16(SAT16(RSHIFT_ROUND(AR2_Q24[i], 11)))
		}
	}

	/*****************/
	/* Gain tweaking */
	/*****************/
	/* Increase gains during low speech activity and put lower limit on gains */
	gain_mult_Q16 = Log2lin(-SMLAWB(-SILK_FIX_CONST(16.0, 7), SNR_adj_dB_Q7, SILK_FIX_CONST(0.16, 16)))
	gain_add_Q16 = Log2lin(SMLAWB(SILK_FIX_CONST(16.0, 7), SILK_FIX_CONST(float64(MIN_QGAIN_DB), 7), SILK_FIX_CONST(0.16, 16)))
	silk_assert(gain_mult_Q16 > 0)
	for k = 0; k < psEnc.sCmn.nb_subfr; k++ {
		psEncCtrl.Gains_Q16[k] = SMULWW(psEncCtrl.Gains_Q16[k], gain_mult_Q16)
		silk_assert(psEncCtrl.Gains_Q16[k] >= 0)
		psEncCtrl.Gains_Q16[k] = ADD_POS_SAT32(psEncCtrl.Gains_Q16[k], gain_add_Q16)
	}

	gain_mult_Q16 = SILK_FIX_CONST(1.0, 16) + RSHIFT_ROUND(MLA(SILK_FIX_CONST(INPUT_TILT, 26),
		psEncCtrl.coding_quality_Q14, SILK_FIX_CONST(HIGH_RATE_INPUT_TILT, 12)), 10)
	for k = 0; k < psEnc.sCmn.nb_subfr; k++ {
		psEncCtrl.GainsPre_Q14[k] = int(SMULWB(gain_mult_Q16, psEncCtrl.GainsPre_Q14[k]))
	}

	/************************************************/
	/* Control low-frequency shaping and noise tilt */
	/************************************************/
	/* Less low frequency shaping for noisy inputs */
	strength_Q16 = MUL(SILK_FIX_CONST(LOW_FREQ_SHAPING, 4), SMLAWB(SILK_FIX_CONST(1.0, 12),
		SILK_FIX_CONST(LOW_QUALITY_LOW_FREQ_SHAPING_DECR, 13), int32(psEnc.sCmn.input_quality_bands_Q15[0])-SILK_FIX_CONST(1.0, 15)))
	strength_Q16 = RSHIFT(MUL(strength_Q16, psEnc.sCmn.speech_activity_Q8), 8)
	if psEnc.sCmn.indices.signalType == TYPE_VOICED {
		/* Reduce low frequencies quantization noise for periodic signals, depending on pitch lag */
		/*f = 400; freqz([1, -0.98 + 2e-4 * f], [1, -0.97 + 7e-4 * f], 2^12, Fs); axis([0, 1000, -10, 1])*/
		fs_kHz_inv := int(DIV32_16(SILK_FIX_CONST(0.2, 14), psEnc.sCmn.fs_kHz))
		for k = 0; k < psEnc.sCmn.nb_subfr; k++ {
			b_Q14 = fs_kHz_inv + int(DIV32_16(SILK_FIX_CONST(3.0, 14), psEncCtrl.pitchL[k]))
			/* Pack two coefficients in one int32 */
			psEncCtrl.LF_shp_Q14[k] = LSHIFT(SILK_FIX_CONST(1.0, 14)-int32(b_Q14)-SMULWB(strength_Q16, b_Q14), 16)
			psEncCtrl.LF_shp_Q14[k] |= int32(uint16(b_Q14 - int(SILK_FIX_CONST(1.0, 14))))
		}
		silk_assert(SILK_FIX_CONST(HARM_HP_NOISE_COEF, 24) < SILK_FIX_CONST(0.5, 24)) /* Guarantees that second argument to SMULWB() is within range of an opus_int16*/
		Tilt_Q16 = -SILK_FIX_CONST(HP_NOISE_COEF, 16) -
			SMULWB(SILK_FIX_CONST(1.0, 16)-SILK_FIX_CONST(HP_NOISE_COEF, 16),
				SMULWB(SILK_FIX_CONST(HARM_HP_NOISE_COEF, 24), psEnc.sCmn.speech_activity_Q8))
	} else {
		b_Q14 = int(DIV32_16(21299, psEnc.sCmn.fs_kHz)) /* 1.3_Q0 = 21299_Q14*/
		/* Pack two coefficients in one int32 */
		psEncCtrl.LF_shp_Q14[0] = LSHIFT(SILK_FIX_CONST(1.0, 14)-int32(b_Q14)-
			SMULWB(strength_Q16, SMULWB(SILK_FIX_CONST(0.6, 16), b_Q14)), 16)
		psEncCtrl.LF_shp_Q14[0] |= int32(uint16(b_Q14 - int(SILK_FIX_CONST(1.0, 14))))
		for k = 1; k < psEnc.sCmn.nb_subfr; k++ {
			psEncCtrl.LF_shp_Q14[k] = psEncCtrl.LF_shp_Q14[0]
		}
		Tilt_Q16 = -SILK_FIX_CONST(HP_NOISE_COEF, 16)
	}

	/****************************/
	/* HARMONIC SHAPING CONTROL */
	/****************************/
	/* Control boosting of harmonic frequencies */
	HarmBoost_Q16 = SMULWB(SMULWB(SILK_FIX_CONST(1.0, 17)-LSHIFT(psEncCtrl.coding_quality_Q14, 3),
		psEnc.LTPCorr_Q15), SILK_FIX_CONST(LOW_RATE_HARMONIC_BOOST, 16))

	/* More harmonic boost for noisy input signals */
	HarmBoost_Q16 = SMLAWB(HarmBoost_Q16,
		SILK_FIX_CONST(1.0, 16)-LSHIFT(psEncCtrl.input_quality_Q14, 2), SILK_FIX_CONST(LOW_INPUT_QUALITY_HARMONIC_BOOST, 16))

	if USE_HARM_SHAPING != 0 && psEnc.sCmn.indices.signalType == TYPE_VOICED {
		/* More harmonic noise shaping for high bitrates or noisy input */
		HarmShapeGain_Q16 = SMLAWB(SILK_FIX_CONST(HARMONIC_SHAPING, 16),
			SILK_FIX_CONST(1.0, 16)-SMULWB(SILK_FIX_CONST(1.0, 18)-LSHIFT(psEncCtrl.coding_quality_Q14, 4),
				psEncCtrl.input_quality_Q14), SILK_FIX_CONST(HIGH_RATE_OR_LOW_QUALITY_HARMONIC_SHAPING, 16))

		/* Less harmonic noise shaping for less periodic signals */
		HarmShapeGain_Q16 = SMULWB(LSHIFT(HarmShapeGain_Q16, 1),
			SQRT_APPROX(LSHIFT(psEnc.LTPCorr_Q15, 15)))
	} else {
		HarmShapeGain_Q16 = 0
	}

	/*************************/
	/* Smooth over subframes */
	/*************************/
	for k = 0; k < MAX_NB_SUBFR; k++ {
		psShapeSt.HarmBoost_smth_Q16 =
			SMLAWB(psShapeSt.HarmBoost_smth_Q16, HarmBoost_Q16-psShapeSt.HarmBoost_smth_Q16, SILK_FIX_CONST(SUBFR_SMTH_COEF, 16))
		psShapeSt.HarmShapeGain_smth_Q16 =
			SMLAWB(psShapeSt.HarmShapeGain_smth_Q16, HarmShapeGain_Q16-psShapeSt.HarmShapeGain_smth_Q16, SILK_FIX_CONST(SUBFR_SMTH_COEF, 16))
		psShapeSt.Tilt_smth_Q16 =
			SMLAWB(psShapeSt.Tilt_smth_Q16, Tilt_Q16-psShapeSt.Tilt_smth_Q16, SILK_FIX_CONST(SUBFR_SMTH_COEF, 16))

		psEncCtrl.HarmBoost_Q14[k] = int(RSHIFT_ROUND(psShapeSt.HarmBoost_smth_Q16, 2))
		psEncCtrl.HarmShapeGain_Q14[k] = int(RSHIFT_ROUND(psShapeSt.HarmShapeGain_smth_Q16, 2))
		psEncCtrl.Tilt_Q14[k] = int(RSHIFT_ROUND(psShapeSt.Tilt_smth_Q16, 2))
	}
}
