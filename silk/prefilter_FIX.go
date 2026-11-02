package silk

// C: silk/fixed/prefilter_FIX.c

// C: void silk_warped_LPC_analysis_filter_FIX_c(
//
//	      opus_int32            state[],                    /* I/O  State [order + 1]                   */
//	      opus_int32            res_Q2[],                   /* O    Residual signal [length]            */
//	const opus_int16            coef_Q13[],                 /* I    Coefficients [order]                */
//	const opus_int16            input[],                    /* I    Input signal [length]               */
//	const opus_int16            lambda_Q16,                 /* I    Warping factor                      */
//	const opus_int              length,                     /* I    Length of input signal              */
//	const opus_int              order                       /* I    Filter order (even)                 */
//
// )
// C: #define silk_warped_LPC_analysis_filter_FIX(state, res_Q2, coef_Q13, input, lambda_Q16, length, order, arch) ((void)(arch),silk_warped_LPC_analysis_filter_FIX_c(state, res_Q2, coef_Q13, input, lambda_Q16, length, order))
func WarpedLPCAnalysisFilterFIX(state []int32, res_Q2 []int32, coef_Q13 []int16, input []int16, lambda_Q16 int16, length int, order int, arch int) {
	var n, i int
	var acc_Q11, tmp1, tmp2 int32

	/* Order must be even */
	silk_assert((order & 1) == 0)

	for n = 0; n < length; n++ {
		/* Output of lowpass section */
		tmp2 = SMLAWB(state[0], state[1], lambda_Q16)
		state[0] = LSHIFT(input[n], 14)
		/* Output of allpass section */
		tmp1 = SMLAWB(state[1], state[2]-tmp2, lambda_Q16)
		state[1] = tmp2
		acc_Q11 = int32(RSHIFT(order, 1))
		acc_Q11 = SMLAWB(acc_Q11, tmp2, coef_Q13[0])
		/* Loop over allpass sections */
		for i = 2; i < order; i += 2 {
			/* Output of allpass section */
			tmp2 = SMLAWB(state[i], state[i+1]-tmp1, lambda_Q16)
			state[i] = tmp1
			acc_Q11 = SMLAWB(acc_Q11, tmp1, coef_Q13[i-1])
			/* Output of allpass section */
			tmp1 = SMLAWB(state[i+1], state[i+2]-tmp2, lambda_Q16)
			state[i+1] = tmp2
			acc_Q11 = SMLAWB(acc_Q11, tmp2, coef_Q13[i])
		}
		state[order] = tmp1
		acc_Q11 = SMLAWB(acc_Q11, tmp1, coef_Q13[order-1])
		res_Q2[n] = LSHIFT(int32(input[n]), 2) - RSHIFT_ROUND(acc_Q11, 9)
	}
}

// C: void silk_prefilter_FIX(
//
//	silk_encoder_state_FIX          *psEnc,                                 /* I/O  Encoder state                                                               */
//	const silk_encoder_control_FIX  *psEncCtrl,                             /* I    Encoder control                                                             */
//	opus_int32                      xw_Q3[],                                /* O    Weighted signal                                                             */
//	const opus_int16                x[]                                     /* I    Speech signal                                                               */
//
// )
//
// The stub names the output xw_Q10; it is the C xw_Q3.
func (psEnc *EncoderStateFIX) PrefilterFIX(psEncCtrl *EncoderControlFIX, xw_Q10 []int32, x []int16) {
	P := &psEnc.sPrefilt
	var j, k, lag int
	var tmp_32 int32
	var AR1_shp_Q13 []int16
	var px []int16
	var pxw_Q3 []int32
	var HarmShapeGain_Q12, Tilt_Q14 int
	var HarmShapeFIRPacked_Q12, LF_shp_Q14 int32
	var x_filt_Q12 []int32
	var st_res_Q2 []int32
	var B_Q10 [2]int16

	/* Set up pointers */
	px = x
	pxw_Q3 = xw_Q10
	lag = P.lagPrev
	x_filt_Q12 = make([]int32, psEnc.sCmn.subfr_length)
	st_res_Q2 = make([]int32, psEnc.sCmn.subfr_length)
	for k = 0; k < psEnc.sCmn.nb_subfr; k++ {
		/* Update Variables that change per sub frame */
		if psEnc.sCmn.indices.signalType == TYPE_VOICED {
			lag = psEncCtrl.pitchL[k]
		}

		/* Noise shape parameters */
		HarmShapeGain_Q12 = int(SMULWB(int32(psEncCtrl.HarmShapeGain_Q14[k]), 16384-psEncCtrl.HarmBoost_Q14[k]))
		silk_assert(HarmShapeGain_Q12 >= 0)
		HarmShapeFIRPacked_Q12 = int32(RSHIFT(HarmShapeGain_Q12, 2))
		HarmShapeFIRPacked_Q12 |= LSHIFT(int32(RSHIFT(HarmShapeGain_Q12, 1)), 16)
		Tilt_Q14 = psEncCtrl.Tilt_Q14[k]
		LF_shp_Q14 = psEncCtrl.LF_shp_Q14[k]
		AR1_shp_Q13 = psEncCtrl.AR1_Q13[k*MAX_SHAPE_LPC_ORDER:]

		/* Short term FIR filtering*/
		WarpedLPCAnalysisFilterFIX(P.sAR_shp[:], st_res_Q2, AR1_shp_Q13, px,
			int16(psEnc.sCmn.warping_Q16), psEnc.sCmn.subfr_length, psEnc.sCmn.shapingLPCOrder, psEnc.sCmn.arch)

		/* Reduce (mainly) low frequencies during harmonic emphasis */
		B_Q10[0] = int16(RSHIFT_ROUND(psEncCtrl.GainsPre_Q14[k], 4))
		tmp_32 = SMLABB(SILK_FIX_CONST(INPUT_TILT, 26), psEncCtrl.HarmBoost_Q14[k], HarmShapeGain_Q12)  /* Q26 */
		tmp_32 = SMLABB(tmp_32, psEncCtrl.coding_quality_Q14, SILK_FIX_CONST(HIGH_RATE_INPUT_TILT, 12)) /* Q26 */
		tmp_32 = SMULWB(tmp_32, -psEncCtrl.GainsPre_Q14[k])                                             /* Q24 */
		tmp_32 = RSHIFT_ROUND(tmp_32, 14)                                                               /* Q10 */
		B_Q10[1] = int16(SAT16(tmp_32))
		x_filt_Q12[0] = MLA(MUL(st_res_Q2[0], B_Q10[0]), P.sHarmHP_Q2, B_Q10[1])
		for j = 1; j < psEnc.sCmn.subfr_length; j++ {
			x_filt_Q12[j] = MLA(MUL(st_res_Q2[j], B_Q10[0]), st_res_Q2[j-1], B_Q10[1])
		}
		P.sHarmHP_Q2 = st_res_Q2[psEnc.sCmn.subfr_length-1]

		prefiltFIX(P, x_filt_Q12, pxw_Q3, HarmShapeFIRPacked_Q12, Tilt_Q14, LF_shp_Q14, lag, psEnc.sCmn.subfr_length)

		px = px[psEnc.sCmn.subfr_length:]
		pxw_Q3 = pxw_Q3[psEnc.sCmn.subfr_length:]
	}

	P.lagPrev = psEncCtrl.pitchL[psEnc.sCmn.nb_subfr-1]
}

/* Prefilter for finding Quantizer input signal */

// C: static OPUS_INLINE void silk_prefilt_FIX(
//
//	silk_prefilter_state_FIX    *P,                         /* I/O  state                               */
//	opus_int32                  st_res_Q12[],               /* I    short term residual signal          */
//	opus_int32                  xw_Q3[],                    /* O    prefiltered signal                  */
//	opus_int32                  HarmShapeFIRPacked_Q12,     /* I    Harmonic shaping coeficients        */
//	opus_int                    Tilt_Q14,                   /* I    Tilt shaping coeficient             */
//	opus_int32                  LF_shp_Q14,                 /* I    Low-frequancy shaping coeficients   */
//	opus_int                    lag,                        /* I    Lag for harmonic shaping            */
//	opus_int                    length                      /* I    Length of signals                   */
//
// )
func prefiltFIX(P *PrefilterStateFIX, st_res_Q12 []int32, xw_Q3 []int32, HarmShapeFIRPacked_Q12 int32, Tilt_Q14 int, LF_shp_Q14 int32, lag int, length int) {
	var i, idx, LTP_shp_buf_idx int
	var n_LTP_Q12, n_Tilt_Q10, n_LF_Q10 int32
	var sLF_MA_shp_Q12, sLF_AR_shp_Q12 int32
	var LTP_shp_buf []int16

	/* To speed up use temp variables instead of using the struct */
	LTP_shp_buf = P.sLTP_shp[:]
	LTP_shp_buf_idx = P.sLTP_shp_buf_idx
	sLF_AR_shp_Q12 = P.sLF_AR_shp_Q12
	sLF_MA_shp_Q12 = P.sLF_MA_shp_Q12

	for i = 0; i < length; i++ {
		if lag > 0 {
			/* unrolled loop */
			silk_assert(HARM_SHAPE_FIR_TAPS == 3)
			idx = lag + LTP_shp_buf_idx
			n_LTP_Q12 = SMULBB(LTP_shp_buf[(idx-HARM_SHAPE_FIR_TAPS/2-1)&LTP_MASK], HarmShapeFIRPacked_Q12)
			n_LTP_Q12 = SMLABT(n_LTP_Q12, LTP_shp_buf[(idx-HARM_SHAPE_FIR_TAPS/2)&LTP_MASK], HarmShapeFIRPacked_Q12)
			n_LTP_Q12 = SMLABB(n_LTP_Q12, LTP_shp_buf[(idx-HARM_SHAPE_FIR_TAPS/2+1)&LTP_MASK], HarmShapeFIRPacked_Q12)
		} else {
			n_LTP_Q12 = 0
		}

		n_Tilt_Q10 = SMULWB(sLF_AR_shp_Q12, Tilt_Q14)
		n_LF_Q10 = SMLAWB(SMULWT(sLF_AR_shp_Q12, LF_shp_Q14), sLF_MA_shp_Q12, LF_shp_Q14)

		sLF_AR_shp_Q12 = SUB32(st_res_Q12[i], LSHIFT(n_Tilt_Q10, 2))
		sLF_MA_shp_Q12 = SUB32(sLF_AR_shp_Q12, LSHIFT(n_LF_Q10, 2))

		LTP_shp_buf_idx = (LTP_shp_buf_idx - 1) & LTP_MASK
		LTP_shp_buf[LTP_shp_buf_idx] = int16(SAT16(RSHIFT_ROUND(sLF_MA_shp_Q12, 12)))

		xw_Q3[i] = RSHIFT_ROUND(SUB32(sLF_MA_shp_Q12, n_LTP_Q12), 9)
	}

	/* Copy temp variable back to state */
	P.sLF_AR_shp_Q12 = sLF_AR_shp_Q12
	P.sLF_MA_shp_Q12 = sLF_MA_shp_Q12
	P.sLTP_shp_buf_idx = LTP_shp_buf_idx
}
