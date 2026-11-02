package silk

// C: silk/NSQ.c

// C: void silk_NSQ_c(
//
//	const silk_encoder_state    *psEncC,                                    /* I    Encoder State                   */
//	silk_nsq_state              *NSQ,                                       /* I/O  NSQ state                       */
//	SideInfoIndices             *psIndices,                                 /* I/O  Quantization Indices            */
//	const opus_int16            x16[],                                      /* I    Input                           */
//	opus_int8                   pulses[],                                   /* O    Quantized pulse signal          */
//	const opus_int16            PredCoef_Q12[ 2 * MAX_LPC_ORDER ],          /* I    Short term prediction coefs     */
//	const opus_int16            LTPCoef_Q14[ LTP_ORDER * MAX_NB_SUBFR ],    /* I    Long term prediction coefs      */
//	const opus_int16            AR_Q13[ MAX_NB_SUBFR * MAX_SHAPE_LPC_ORDER ], /* I  Noise shaping coefs             */
//	const opus_int              HarmShapeGain_Q14[ MAX_NB_SUBFR ],          /* I    Long term shaping coefs         */
//	const opus_int              Tilt_Q14[ MAX_NB_SUBFR ],                   /* I    Spectral tilt                   */
//	const opus_int32            LF_shp_Q14[ MAX_NB_SUBFR ],                 /* I    Low frequency shaping coefs     */
//	const opus_int32            Gains_Q16[ MAX_NB_SUBFR ],                  /* I    Quantization step sizes         */
//	const opus_int              pitchL[ MAX_NB_SUBFR ],                     /* I    Pitch lags                      */
//	const opus_int              Lambda_Q10,                                 /* I    Rate/distortion tradeoff        */
//	const opus_int              LTP_scale_Q14                               /* I    LTP state scaling               */
//
// )
// C: #define silk_NSQ(psEncC, NSQ, psIndices, x16, pulses, PredCoef_Q12, LTPCoef_Q14, AR_Q13, HarmShapeGain_Q14, Tilt_Q14, LF_shp_Q14, Gains_Q16, pitchL, Lambda_Q10, LTP_scale_Q14, arch) ((void)(arch),silk_NSQ_c(...))
func (psEncC *EncoderState) NSQ(NSQ *NsqState, psIndices *SideInfoIndices, x16 []int16, pulses []int8, PredCoef_Q12 []int16, LTPCoef_Q14 []int16, AR_Q13 []int16, HarmShapeGain_Q14 []int, Tilt_Q14 []int, LF_shp_Q14 []int32, Gains_Q16 []int32, pitchL []int, Lambda_Q10 int, LTP_scale_Q14 int, arch int) {
	var k, lag, start_idx, LSF_interpolation_flag int
	var A_Q12, B_Q14, AR_shp_Q13 []int16
	var pxq int /* index into NSQ.xq (C: opus_int16 *pxq) */
	var sLTP_Q15 []int32
	var sLTP []int16
	var HarmShapeFIRPacked_Q14 int32
	var offset_Q10 int
	var x_sc_Q10 []int32

	NSQ.rand_seed = int32(psIndices.Seed)

	/* Set unvoiced lag to the previous one, overwrite later for voiced */
	lag = NSQ.lagPrev

	silk_assert(NSQ.prev_gain_Q16 != 0)

	offset_Q10 = int(Quantization_Offsets_Q10[psIndices.signalType>>1][psIndices.quantOffsetType])

	if psIndices.NLSFInterpCoef_Q2 == 4 {
		LSF_interpolation_flag = 0
	} else {
		LSF_interpolation_flag = 1
	}

	sLTP_Q15 = make([]int32, psEncC.ltp_mem_length+psEncC.frame_length)
	sLTP = make([]int16, psEncC.ltp_mem_length+psEncC.frame_length)
	x_sc_Q10 = make([]int32, psEncC.subfr_length)
	/* Set up pointers to start of sub frame */
	NSQ.sLTP_shp_buf_idx = psEncC.ltp_mem_length
	NSQ.sLTP_buf_idx = psEncC.ltp_mem_length
	pxq = psEncC.ltp_mem_length
	for k = 0; k < psEncC.nb_subfr; k++ {
		A_Q12 = PredCoef_Q12[((k>>1)|(1-LSF_interpolation_flag))*MAX_LPC_ORDER:]
		B_Q14 = LTPCoef_Q14[k*LTP_ORDER:]
		AR_shp_Q13 = AR_Q13[k*MAX_SHAPE_LPC_ORDER:]

		/* Noise shape parameters */
		silk_assert(HarmShapeGain_Q14[k] >= 0)
		HarmShapeFIRPacked_Q14 = int32(RSHIFT(HarmShapeGain_Q14[k], 2))
		HarmShapeFIRPacked_Q14 |= LSHIFT(int32(RSHIFT(HarmShapeGain_Q14[k], 1)), 16)

		NSQ.rewhite_flag = 0
		if psIndices.signalType == TYPE_VOICED {
			/* Voiced */
			lag = pitchL[k]

			/* Re-whitening */
			if (k & (3 - int(LSHIFT(LSF_interpolation_flag, 1)))) == 0 {
				/* Rewhiten with new A coefs */
				start_idx = psEncC.ltp_mem_length - lag - psEncC.predictLPCOrder - LTP_ORDER/2
				silk_assert(start_idx > 0)

				LPCAnalysisFilter(sLTP[start_idx:], NSQ.xq[start_idx+k*psEncC.subfr_length:],
					A_Q12, int32(psEncC.ltp_mem_length-start_idx), int32(psEncC.predictLPCOrder), psEncC.arch)

				NSQ.rewhite_flag = 1
				NSQ.sLTP_buf_idx = psEncC.ltp_mem_length
			}
		}

		nsqScaleStates(psEncC, NSQ, x16, x_sc_Q10, sLTP, sLTP_Q15, k, LTP_scale_Q14, Gains_Q16, pitchL, int(psIndices.signalType))

		noiseShapeQuantizer(NSQ, int(psIndices.signalType), x_sc_Q10, pulses, NSQ.xq[pxq:], sLTP_Q15, A_Q12, B_Q14,
			AR_shp_Q13, lag, HarmShapeFIRPacked_Q14, Tilt_Q14[k], LF_shp_Q14[k], Gains_Q16[k], Lambda_Q10,
			offset_Q10, psEncC.subfr_length, psEncC.shapingLPCOrder, psEncC.predictLPCOrder, psEncC.arch)

		x16 = x16[psEncC.subfr_length:]
		pulses = pulses[psEncC.subfr_length:]
		pxq += psEncC.subfr_length
	}

	/* Update lagPrev for next frame */
	NSQ.lagPrev = pitchL[psEncC.nb_subfr-1]

	/* Save quantized speech and noise shaping signals */
	copy(NSQ.xq[:psEncC.ltp_mem_length], NSQ.xq[psEncC.frame_length:psEncC.frame_length+psEncC.ltp_mem_length])
	copy(NSQ.sLTP_shp_Q14[:psEncC.ltp_mem_length], NSQ.sLTP_shp_Q14[psEncC.frame_length:psEncC.frame_length+psEncC.ltp_mem_length])
}

/***********************************/
/* silk_noise_shape_quantizer  */
/***********************************/

// C: static OPUS_INLINE void silk_noise_shape_quantizer(
//
//	silk_nsq_state      *NSQ,                   /* I/O  NSQ state                       */
//	opus_int            signalType,             /* I    Signal type                     */
//	const opus_int32    x_sc_Q10[],             /* I                                    */
//	opus_int8           pulses[],               /* O                                    */
//	opus_int16          xq[],                   /* O                                    */
//	opus_int32          sLTP_Q15[],             /* I/O  LTP state                       */
//	const opus_int16    a_Q12[],                /* I    Short term prediction coefs     */
//	const opus_int16    b_Q14[],                /* I    Long term prediction coefs      */
//	const opus_int16    AR_shp_Q13[],           /* I    Noise shaping AR coefs          */
//	opus_int            lag,                    /* I    Pitch lag                       */
//	opus_int32          HarmShapeFIRPacked_Q14, /* I                                    */
//	opus_int            Tilt_Q14,               /* I    Spectral tilt                   */
//	opus_int32          LF_shp_Q14,             /* I                                    */
//	opus_int32          Gain_Q16,               /* I                                    */
//	opus_int            Lambda_Q10,             /* I                                    */
//	opus_int            offset_Q10,             /* I                                    */
//	opus_int            length,                 /* I    Input length                    */
//	opus_int            shapingLPCOrder,        /* I    Noise shaping AR filter order   */
//	opus_int            predictLPCOrder,        /* I    Prediction filter order         */
//	int                 arch                    /* I    Architecture                    */
//
// )
func noiseShapeQuantizer(NSQ *NsqState, signalType int, x_sc_Q10 []int32, pulses []int8, xq []int16, sLTP_Q15 []int32, a_Q12 []int16, b_Q14 []int16, AR_shp_Q13 []int16, lag int, HarmShapeFIRPacked_Q14 int32, Tilt_Q14 int, LF_shp_Q14 int32, Gain_Q16 int32, Lambda_Q10 int, offset_Q10 int, length int, shapingLPCOrder int, predictLPCOrder int, arch int) {
	var i int
	var LTP_pred_Q13, LPC_pred_Q10, n_AR_Q12, n_LTP_Q13 int32
	var n_LF_Q12, r_Q10, rr_Q10, q1_Q0, q1_Q10, q2_Q10, rd1_Q20, rd2_Q20 int32
	var exc_Q14, LPC_exc_Q14, xq_Q14, Gain_Q10 int32
	var tmp1, tmp2, sLF_AR_shp_Q14 int32
	/* The C pointers psLPC_Q14, shp_lag_ptr and pred_lag_ptr are read at negative offsets, */
	/* so they are kept as indices into NSQ.sLPC_Q14, NSQ.sLTP_shp_Q14 and sLTP_Q15.        */
	var psLPC_Q14, shp_lag_ptr, pred_lag_ptr int

	shp_lag_ptr = NSQ.sLTP_shp_buf_idx - lag + HARM_SHAPE_FIR_TAPS/2
	pred_lag_ptr = NSQ.sLTP_buf_idx - lag + LTP_ORDER/2
	Gain_Q10 = RSHIFT(Gain_Q16, 6)

	/* Set up short term AR state */
	psLPC_Q14 = NSQ_LPC_BUF_LENGTH - 1

	for i = 0; i < length; i++ {
		/* Generate dither */
		NSQ.rand_seed = RAND(NSQ.rand_seed)

		/* Short-term prediction */
		LPC_pred_Q10 = noiseShapeQuantizerShortPrediction(NSQ.sLPC_Q14[:], psLPC_Q14, a_Q12, nil, predictLPCOrder, arch)

		/* Long-term prediction */
		if signalType == TYPE_VOICED {
			/* Unrolled loop */
			/* Avoids introducing a bias because silk_SMLAWB() always rounds to -inf */
			LTP_pred_Q13 = 2
			LTP_pred_Q13 = SMLAWB(LTP_pred_Q13, sLTP_Q15[pred_lag_ptr-0], b_Q14[0])
			LTP_pred_Q13 = SMLAWB(LTP_pred_Q13, sLTP_Q15[pred_lag_ptr-1], b_Q14[1])
			LTP_pred_Q13 = SMLAWB(LTP_pred_Q13, sLTP_Q15[pred_lag_ptr-2], b_Q14[2])
			LTP_pred_Q13 = SMLAWB(LTP_pred_Q13, sLTP_Q15[pred_lag_ptr-3], b_Q14[3])
			LTP_pred_Q13 = SMLAWB(LTP_pred_Q13, sLTP_Q15[pred_lag_ptr-4], b_Q14[4])
			pred_lag_ptr++
		} else {
			LTP_pred_Q13 = 0
		}

		/* Noise shape feedback */
		silk_assert((shapingLPCOrder & 1) == 0) /* check that order is even */
		n_AR_Q12 = nsqNoiseShapeFeedbackLoop(&NSQ.sDiff_shp_Q14, NSQ.sAR2_Q14[:], AR_shp_Q13, shapingLPCOrder, arch)

		n_AR_Q12 = SMLAWB(n_AR_Q12, NSQ.sLF_AR_shp_Q14, Tilt_Q14)

		n_LF_Q12 = SMULWB(NSQ.sLTP_shp_Q14[NSQ.sLTP_shp_buf_idx-1], LF_shp_Q14)
		n_LF_Q12 = SMLAWT(n_LF_Q12, NSQ.sLF_AR_shp_Q14, LF_shp_Q14)

		silk_assert(lag > 0 || signalType != TYPE_VOICED)

		/* Combine prediction and noise shaping signals */
		tmp1 = SUB32(LSHIFT32(LPC_pred_Q10, 2), n_AR_Q12) /* Q12 */
		tmp1 = SUB32(tmp1, n_LF_Q12)                      /* Q12 */
		if lag > 0 {
			/* Symmetric, packed FIR coefficients */
			n_LTP_Q13 = SMULWB(ADD32(NSQ.sLTP_shp_Q14[shp_lag_ptr+0], NSQ.sLTP_shp_Q14[shp_lag_ptr-2]), HarmShapeFIRPacked_Q14)
			n_LTP_Q13 = SMLAWT(n_LTP_Q13, NSQ.sLTP_shp_Q14[shp_lag_ptr-1], HarmShapeFIRPacked_Q14)
			n_LTP_Q13 = LSHIFT(n_LTP_Q13, 1)
			shp_lag_ptr++

			tmp2 = SUB32(LTP_pred_Q13, n_LTP_Q13) /* Q13 */
			tmp1 = ADD_LSHIFT32(tmp2, tmp1, 1)    /* Q13 */
			tmp1 = RSHIFT_ROUND(tmp1, 3)          /* Q10 */
		} else {
			tmp1 = RSHIFT_ROUND(tmp1, 2) /* Q10 */
		}

		r_Q10 = SUB32(x_sc_Q10[i], tmp1) /* residual error Q10 */

		/* Flip sign depending on dither */
		if NSQ.rand_seed < 0 {
			r_Q10 = -r_Q10
		}
		r_Q10 = LIMIT_32(r_Q10, -(31 << 10), 30<<10)

		/* Find two quantization level candidates and measure their rate-distortion */
		q1_Q10 = SUB32(r_Q10, offset_Q10)
		q1_Q0 = RSHIFT(q1_Q10, 10)
		if Lambda_Q10 > 2048 {
			/* For aggressive RDO, the bias becomes more than one pulse. */
			var rdo_offset int = Lambda_Q10/2 - 512
			if q1_Q10 > int32(rdo_offset) {
				q1_Q0 = RSHIFT(q1_Q10-int32(rdo_offset), 10)
			} else if q1_Q10 < int32(-rdo_offset) {
				q1_Q0 = RSHIFT(q1_Q10+int32(rdo_offset), 10)
			} else if q1_Q10 < 0 {
				q1_Q0 = -1
			} else {
				q1_Q0 = 0
			}
		}
		if q1_Q0 > 0 {
			q1_Q10 = SUB32(LSHIFT(q1_Q0, 10), QUANT_LEVEL_ADJUST_Q10)
			q1_Q10 = ADD32(q1_Q10, offset_Q10)
			q2_Q10 = ADD32(q1_Q10, 1024)
			rd1_Q20 = SMULBB(q1_Q10, Lambda_Q10)
			rd2_Q20 = SMULBB(q2_Q10, Lambda_Q10)
		} else if q1_Q0 == 0 {
			q1_Q10 = int32(offset_Q10)
			q2_Q10 = ADD32(q1_Q10, 1024-QUANT_LEVEL_ADJUST_Q10)
			rd1_Q20 = SMULBB(q1_Q10, Lambda_Q10)
			rd2_Q20 = SMULBB(q2_Q10, Lambda_Q10)
		} else if q1_Q0 == -1 {
			q2_Q10 = int32(offset_Q10)
			q1_Q10 = SUB32(q2_Q10, 1024-QUANT_LEVEL_ADJUST_Q10)
			rd1_Q20 = SMULBB(-q1_Q10, Lambda_Q10)
			rd2_Q20 = SMULBB(q2_Q10, Lambda_Q10)
		} else { /* Q1_Q0 < -1 */
			q1_Q10 = ADD32(LSHIFT(q1_Q0, 10), QUANT_LEVEL_ADJUST_Q10)
			q1_Q10 = ADD32(q1_Q10, offset_Q10)
			q2_Q10 = ADD32(q1_Q10, 1024)
			rd1_Q20 = SMULBB(-q1_Q10, Lambda_Q10)
			rd2_Q20 = SMULBB(-q2_Q10, Lambda_Q10)
		}
		rr_Q10 = SUB32(r_Q10, q1_Q10)
		rd1_Q20 = SMLABB(rd1_Q20, rr_Q10, rr_Q10)
		rr_Q10 = SUB32(r_Q10, q2_Q10)
		rd2_Q20 = SMLABB(rd2_Q20, rr_Q10, rr_Q10)

		if rd2_Q20 < rd1_Q20 {
			q1_Q10 = q2_Q10
		}

		pulses[i] = int8(RSHIFT_ROUND(q1_Q10, 10))

		/* Excitation */
		exc_Q14 = LSHIFT(q1_Q10, 4)
		if NSQ.rand_seed < 0 {
			exc_Q14 = -exc_Q14
		}

		/* Add predictions */
		LPC_exc_Q14 = ADD_LSHIFT32(exc_Q14, LTP_pred_Q13, 1)
		xq_Q14 = ADD_LSHIFT32(LPC_exc_Q14, LPC_pred_Q10, 4)

		/* Scale XQ back to normal level before saving */
		xq[i] = int16(SAT16(RSHIFT_ROUND(SMULWW(xq_Q14, Gain_Q10), 8)))

		/* Update states */
		psLPC_Q14++
		NSQ.sLPC_Q14[psLPC_Q14] = xq_Q14
		NSQ.sDiff_shp_Q14 = SUB_LSHIFT32(xq_Q14, x_sc_Q10[i], 4)
		sLF_AR_shp_Q14 = SUB_LSHIFT32(NSQ.sDiff_shp_Q14, n_AR_Q12, 2)
		NSQ.sLF_AR_shp_Q14 = sLF_AR_shp_Q14

		NSQ.sLTP_shp_Q14[NSQ.sLTP_shp_buf_idx] = SUB_LSHIFT32(sLF_AR_shp_Q14, n_LF_Q12, 2)
		sLTP_Q15[NSQ.sLTP_buf_idx] = LSHIFT(LPC_exc_Q14, 1)
		NSQ.sLTP_shp_buf_idx++
		NSQ.sLTP_buf_idx++

		/* Make dither dependent on quantized signal */
		NSQ.rand_seed = ADD32_ovflw(NSQ.rand_seed, pulses[i])
	}

	/* Update LPC synth buffer */
	copy(NSQ.sLPC_Q14[:NSQ_LPC_BUF_LENGTH], NSQ.sLPC_Q14[length:length+NSQ_LPC_BUF_LENGTH])
}

// C: static OPUS_INLINE void silk_nsq_scale_states(
//
//	const silk_encoder_state *psEncC,           /* I    Encoder State                   */
//	silk_nsq_state      *NSQ,                   /* I/O  NSQ state                       */
//	const opus_int16    x16[],                  /* I    input                           */
//	opus_int32          x_sc_Q10[],             /* O    input scaled with 1/Gain        */
//	const opus_int16    sLTP[],                 /* I    re-whitened LTP state in Q0     */
//	opus_int32          sLTP_Q15[],             /* O    LTP state matching scaled input */
//	opus_int            subfr,                  /* I    subframe number                 */
//	const opus_int      LTP_scale_Q14,          /* I                                    */
//	const opus_int32    Gains_Q16[ MAX_NB_SUBFR ], /* I                                 */
//	const opus_int      pitchL[ MAX_NB_SUBFR ], /* I    Pitch lag                       */
//	const opus_int      signal_type             /* I    Signal type                     */
//
// )
func nsqScaleStates(psEncC *EncoderState, NSQ *NsqState, x16 []int16, x_sc_Q10 []int32, sLTP []int16, sLTP_Q15 []int32, subfr int, LTP_scale_Q14 int, Gains_Q16 []int32, pitchL []int, signal_type int) {
	var i, lag int
	var gain_adj_Q16, inv_gain_Q31, inv_gain_Q26 int32

	lag = pitchL[subfr]
	inv_gain_Q31 = INVERSE32_varQ(max(Gains_Q16[subfr], 1), 47)
	silk_assert(inv_gain_Q31 != 0)

	/* Scale input */
	inv_gain_Q26 = RSHIFT_ROUND(inv_gain_Q31, 5)
	for i = 0; i < psEncC.subfr_length; i++ {
		x_sc_Q10[i] = SMULWW(x16[i], inv_gain_Q26)
	}

	/* After rewhitening the LTP state is un-scaled, so scale with inv_gain_Q16 */
	if NSQ.rewhite_flag != 0 {
		if subfr == 0 {
			/* Do LTP downscaling */
			inv_gain_Q31 = LSHIFT(SMULWB(inv_gain_Q31, LTP_scale_Q14), 2)
		}
		for i = NSQ.sLTP_buf_idx - lag - LTP_ORDER/2; i < NSQ.sLTP_buf_idx; i++ {
			silk_assert(i < MAX_FRAME_LENGTH)
			sLTP_Q15[i] = SMULWB(inv_gain_Q31, sLTP[i])
		}
	}

	/* Adjust for changing gain */
	if Gains_Q16[subfr] != NSQ.prev_gain_Q16 {
		gain_adj_Q16 = DIV32_varQ(NSQ.prev_gain_Q16, Gains_Q16[subfr], 16)

		/* Scale long-term shaping state */
		for i = NSQ.sLTP_shp_buf_idx - psEncC.ltp_mem_length; i < NSQ.sLTP_shp_buf_idx; i++ {
			NSQ.sLTP_shp_Q14[i] = SMULWW(gain_adj_Q16, NSQ.sLTP_shp_Q14[i])
		}

		/* Scale long-term prediction state */
		if signal_type == TYPE_VOICED && NSQ.rewhite_flag == 0 {
			for i = NSQ.sLTP_buf_idx - lag - LTP_ORDER/2; i < NSQ.sLTP_buf_idx; i++ {
				sLTP_Q15[i] = SMULWW(gain_adj_Q16, sLTP_Q15[i])
			}
		}

		NSQ.sLF_AR_shp_Q14 = SMULWW(gain_adj_Q16, NSQ.sLF_AR_shp_Q14)
		NSQ.sDiff_shp_Q14 = SMULWW(gain_adj_Q16, NSQ.sDiff_shp_Q14)

		/* Scale short-term prediction and shaping states */
		for i = 0; i < NSQ_LPC_BUF_LENGTH; i++ {
			NSQ.sLPC_Q14[i] = SMULWW(gain_adj_Q16, NSQ.sLPC_Q14[i])
		}
		for i = 0; i < MAX_SHAPE_LPC_ORDER; i++ {
			NSQ.sAR2_Q14[i] = SMULWW(gain_adj_Q16, NSQ.sAR2_Q14[i])
		}

		/* Save inverse gain */
		NSQ.prev_gain_Q16 = Gains_Q16[subfr]
	}
}
