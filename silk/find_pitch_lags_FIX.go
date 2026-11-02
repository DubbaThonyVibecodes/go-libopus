package silk

// C: silk/fixed/find_pitch_lags_FIX.c

/* Find pitch lags */

// C: void silk_find_pitch_lags_FIX(
//
//	silk_encoder_state_FIX          *psEnc,                                 /* I/O  encoder state                                                               */
//	silk_encoder_control_FIX        *psEncCtrl,                             /* I/O  encoder control                                                             */
//	opus_int16                      res[],                                  /* O    residual                                                                    */
//	const opus_int16                x[],                                    /* I    Speech signal                                                               */
//	int                             arch                                    /* I    Run-time architecture                                                       */
//
// )
func (psEnc *EncoderStateFIX) FindPitchLagsFIX(psEncCtrl *EncoderControlFIX, res []int16, x []int16, arch int) {
	var buf_len, i, scale int
	var thrhld_Q13, res_nrg int32
	var x_buf, x_buf_ptr []int16
	var Wsig []int16
	var Wsig_ptr []int16
	var auto_corr [MAX_FIND_PITCH_LPC_ORDER + 1]int32
	var rc_Q15 [MAX_FIND_PITCH_LPC_ORDER]int16
	var A_Q24 [MAX_FIND_PITCH_LPC_ORDER]int32
	var A_Q12 [MAX_FIND_PITCH_LPC_ORDER]int16

	/******************************************/
	/* Set up buffer lengths etc based on Fs  */
	/******************************************/
	buf_len = psEnc.sCmn.la_pitch + psEnc.sCmn.frame_length + psEnc.sCmn.ltp_mem_length

	/* Safety check */
	silk_assert(buf_len >= psEnc.sCmn.pitch_LPC_win_length)

	/* C: x_buf = x - psEnc->sCmn.ltp_mem_length; this reads before x. The caller (encode_frame_FIX.c) */
	/* passes x = &psEnc->x_buf[ psEnc->sCmn.ltp_mem_length ], so x_buf is the start of psEnc.x_buf.    */
	silk_assert(&x[0] == &psEnc.x_buf[psEnc.sCmn.ltp_mem_length])
	x_buf = psEnc.x_buf[:]

	/*************************************/
	/* Estimate LPC AR coefficients      */
	/*************************************/

	/* Calculate windowed signal */

	Wsig = make([]int16, psEnc.sCmn.pitch_LPC_win_length)

	/* First LA_LTP samples */
	x_buf_ptr = x_buf[buf_len-psEnc.sCmn.pitch_LPC_win_length:]
	Wsig_ptr = Wsig
	ApplySineWindow(Wsig_ptr, x_buf_ptr, 1, psEnc.sCmn.la_pitch)

	/* Middle un - windowed samples */
	Wsig_ptr = Wsig_ptr[psEnc.sCmn.la_pitch:]
	x_buf_ptr = x_buf_ptr[psEnc.sCmn.la_pitch:]
	copy(Wsig_ptr[:psEnc.sCmn.pitch_LPC_win_length-int(LSHIFT(psEnc.sCmn.la_pitch, 1))],
		x_buf_ptr[:psEnc.sCmn.pitch_LPC_win_length-int(LSHIFT(psEnc.sCmn.la_pitch, 1))])

	/* Last LA_LTP samples */
	Wsig_ptr = Wsig_ptr[psEnc.sCmn.pitch_LPC_win_length-int(LSHIFT(psEnc.sCmn.la_pitch, 1)):]
	x_buf_ptr = x_buf_ptr[psEnc.sCmn.pitch_LPC_win_length-int(LSHIFT(psEnc.sCmn.la_pitch, 1)):]
	ApplySineWindow(Wsig_ptr, x_buf_ptr, 2, psEnc.sCmn.la_pitch)

	/* Calculate autocorrelation sequence */
	Autocorr(auto_corr[:], &scale, Wsig, psEnc.sCmn.pitch_LPC_win_length, psEnc.sCmn.pitchEstimationLPCOrder+1, arch)

	/* Add white noise, as fraction of energy */
	auto_corr[0] = SMLAWB(auto_corr[0], auto_corr[0], SILK_FIX_CONST(FIND_PITCH_WHITE_NOISE_FRACTION, 16)) + 1

	/* Calculate the reflection coefficients using schur */
	res_nrg = Schur(rc_Q15[:], auto_corr[:], int32(psEnc.sCmn.pitchEstimationLPCOrder))

	/* Prediction gain */
	psEncCtrl.predGain_Q16 = DIV32_varQ(auto_corr[0], int32(max_int(int(res_nrg), 1)), 16)

	/* Convert reflection coefficients to prediction coefficients */
	K2a(A_Q24[:], rc_Q15[:], int32(psEnc.sCmn.pitchEstimationLPCOrder))

	/* Convert From 32 bit Q24 to 16 bit Q12 coefs */
	for i = 0; i < psEnc.sCmn.pitchEstimationLPCOrder; i++ {
		A_Q12[i] = int16(SAT16(RSHIFT(A_Q24[i], 12)))
	}

	/* Do BWE */
	Bwexpander(A_Q12[:], psEnc.sCmn.pitchEstimationLPCOrder, SILK_FIX_CONST(FIND_PITCH_BANDWIDTH_EXPANSION, 16))

	/*****************************************/
	/* LPC analysis filtering                */
	/*****************************************/
	LPCAnalysisFilter(res, x_buf, A_Q12[:], int32(buf_len), int32(psEnc.sCmn.pitchEstimationLPCOrder), psEnc.sCmn.arch)

	if psEnc.sCmn.indices.signalType != TYPE_NO_VOICE_ACTIVITY && psEnc.sCmn.first_frame_after_reset == 0 {
		/* Threshold for pitch estimator */
		thrhld_Q13 = SILK_FIX_CONST(0.6, 13)
		thrhld_Q13 = SMLABB(thrhld_Q13, SILK_FIX_CONST(-0.004, 13), psEnc.sCmn.pitchEstimationLPCOrder)
		thrhld_Q13 = SMLAWB(thrhld_Q13, SILK_FIX_CONST(-0.1, 21), psEnc.sCmn.speech_activity_Q8)
		thrhld_Q13 = SMLABB(thrhld_Q13, SILK_FIX_CONST(-0.15, 13), RSHIFT(psEnc.sCmn.prevSignalType, 1))
		thrhld_Q13 = SMLAWB(thrhld_Q13, SILK_FIX_CONST(-0.1, 14), psEnc.sCmn.input_tilt_Q15)
		thrhld_Q13 = SAT16(thrhld_Q13)

		/*****************************************/
		/* Call pitch estimator                  */
		/*****************************************/
		if PitchAnalysisCore(res, psEncCtrl.pitchL[:], &psEnc.sCmn.indices.lagIndex, &psEnc.sCmn.indices.contourIndex,
			&psEnc.LTPCorr_Q15, psEnc.sCmn.prevLag, psEnc.sCmn.pitchEstimationThreshold_Q16,
			int(thrhld_Q13), psEnc.sCmn.fs_kHz, psEnc.sCmn.pitchEstimationComplexity, psEnc.sCmn.nb_subfr,
			psEnc.sCmn.arch) == 0 {
			psEnc.sCmn.indices.signalType = TYPE_VOICED
		} else {
			psEnc.sCmn.indices.signalType = TYPE_UNVOICED
		}
	} else {
		clear(psEncCtrl.pitchL[:])
		psEnc.sCmn.indices.lagIndex = 0
		psEnc.sCmn.indices.contourIndex = 0
		psEnc.LTPCorr_Q15 = 0
	}
}
