package silk

// C: silk/NSQ_del_dec.c

// C: NSQ_del_dec_struct
type nsqDelDecStruct struct {
	sLPC_Q14  [MAX_SUB_FRAME_LENGTH + NSQ_LPC_BUF_LENGTH]int32
	RandState [DECISION_DELAY]int32
	Q_Q10     [DECISION_DELAY]int32
	Xq_Q14    [DECISION_DELAY]int32
	Pred_Q15  [DECISION_DELAY]int32
	Shape_Q14 [DECISION_DELAY]int32
	sAR2_Q14  [MAX_SHAPE_LPC_ORDER]int32
	LF_AR_Q14 int32
	Seed      int32
	SeedInit  int32
	RD_Q10    int32
}

// C: NSQ_sample_struct
type nsqSampleStruct struct {
	Q_Q10        int32
	RD_Q10       int32
	xq_Q14       int32
	LF_AR_Q14    int32
	sLTP_shp_Q14 int32
	LPC_exc_Q14  int32
}

// C: typedef NSQ_sample_struct  NSQ_sample_pair[ 2 ];
type nsqSamplePair [2]nsqSampleStruct

// C: void silk_NSQ_del_dec_c(
//
//	const silk_encoder_state    *psEncC,                                    /* I/O  Encoder State                   */
//	silk_nsq_state              *NSQ,                                       /* I/O  NSQ state                       */
//	SideInfoIndices             *psIndices,                                 /* I/O  Quantization Indices            */
//	const opus_int32            x_Q3[],                                     /* I    Prefiltered input signal        */
//	opus_int8                   pulses[],                                   /* O    Quantized pulse signal          */
//	const opus_int16            PredCoef_Q12[ 2 * MAX_LPC_ORDER ],          /* I    Short term prediction coefs     */
//	const opus_int16            LTPCoef_Q14[ LTP_ORDER * MAX_NB_SUBFR ],    /* I    Long term prediction coefs      */
//	const opus_int16            AR2_Q13[ MAX_NB_SUBFR * MAX_SHAPE_LPC_ORDER ], /* I Noise shaping coefs             */
//	const opus_int              HarmShapeGain_Q14[ MAX_NB_SUBFR ],          /* I    Long term shaping coefs         */
//	const opus_int              Tilt_Q14[ MAX_NB_SUBFR ],                   /* I    Spectral tilt                   */
//	const opus_int32            LF_shp_Q14[ MAX_NB_SUBFR ],                 /* I    Low frequency shaping coefs     */
//	const opus_int32            Gains_Q16[ MAX_NB_SUBFR ],                  /* I    Quantization step sizes         */
//	const opus_int              pitchL[ MAX_NB_SUBFR ],                     /* I    Pitch lags                      */
//	const opus_int              Lambda_Q10,                                 /* I    Rate/distortion tradeoff        */
//	const opus_int              LTP_scale_Q14                               /* I    LTP state scaling               */
//
// )
// C: #define silk_NSQ_del_dec(psEncC, NSQ, psIndices, x_Q3, pulses, PredCoef_Q12, LTPCoef_Q14, AR2_Q13, HarmShapeGain_Q14, Tilt_Q14, LF_shp_Q14, Gains_Q16, pitchL, Lambda_Q10, LTP_scale_Q14, arch) ((void)(arch),silk_NSQ_del_dec_c(...))
func (psEncC *EncoderState) NSQDelDec(NSQ *NsqState, psIndices *SideInfoIndices, x_Q3 []int32, pulses []int8, PredCoef_Q12 []int16, LTPCoef_Q14 []int16, AR2_Q13 []int16, HarmShapeGain_Q14 []int, Tilt_Q14 []int, LF_shp_Q14 []int32, Gains_Q16 []int32, pitchL []int, Lambda_Q10 int, LTP_scale_Q14 int, arch int) {
	var i, k, lag, start_idx, LSF_interpolation_flag, Winner_ind, subfr int
	var last_smple_idx, smpl_buf_idx, decisionDelay int
	var A_Q12, B_Q14, AR_shp_Q13 []int16
	/* The C code advances the pulses and pxq pointers per subframe and then reads   */
	/* behind them ( pulses[ i - decisionDelay ] ), so both are kept as indices into */
	/* the pulses slice and NSQ.xq respectively.                                     */
	var pulses_idx, pxq int
	var sLTP_Q15 []int32
	var sLTP []int16
	var HarmShapeFIRPacked_Q14 int32
	var offset_Q10 int
	var RDmin_Q10, Gain_Q10 int32
	var x_sc_Q10 []int32
	var delayedGain_Q10 []int32
	var psDelDec []nsqDelDecStruct
	var psDD *nsqDelDecStruct

	/* Set unvoiced lag to the previous one, overwrite later for voiced */
	lag = NSQ.lagPrev

	silk_assert(NSQ.prev_gain_Q16 != 0)

	/* Initialize delayed decision states */
	psDelDec = make([]nsqDelDecStruct, psEncC.nStatesDelayedDecision)
	clear(psDelDec[:psEncC.nStatesDelayedDecision])
	for k = 0; k < psEncC.nStatesDelayedDecision; k++ {
		psDD = &psDelDec[k]
		psDD.Seed = int32((k + int(psIndices.Seed)) & 3)
		psDD.SeedInit = psDD.Seed
		psDD.RD_Q10 = 0
		psDD.LF_AR_Q14 = NSQ.sLF_AR_shp_Q14
		psDD.Shape_Q14[0] = NSQ.sLTP_shp_Q14[psEncC.ltp_mem_length-1]
		copy(psDD.sLPC_Q14[:NSQ_LPC_BUF_LENGTH], NSQ.sLPC_Q14[:NSQ_LPC_BUF_LENGTH])
		psDD.sAR2_Q14 = NSQ.sAR2_Q14
	}

	offset_Q10 = int(Quantization_Offsets_Q10[psIndices.signalType>>1][psIndices.quantOffsetType])
	smpl_buf_idx = 0 /* index of oldest samples */

	decisionDelay = min_int(DECISION_DELAY, psEncC.subfr_length)

	/* For voiced frames limit the decision delay to lower than the pitch lag */
	if psIndices.signalType == TYPE_VOICED {
		for k = 0; k < psEncC.nb_subfr; k++ {
			decisionDelay = min_int(decisionDelay, pitchL[k]-LTP_ORDER/2-1)
		}
	} else {
		if lag > 0 {
			decisionDelay = min_int(decisionDelay, lag-LTP_ORDER/2-1)
		}
	}

	if psIndices.NLSFInterpCoef_Q2 == 4 {
		LSF_interpolation_flag = 0
	} else {
		LSF_interpolation_flag = 1
	}

	sLTP_Q15 = make([]int32, psEncC.ltp_mem_length+psEncC.frame_length)
	sLTP = make([]int16, psEncC.ltp_mem_length+psEncC.frame_length)
	x_sc_Q10 = make([]int32, psEncC.subfr_length)
	delayedGain_Q10 = make([]int32, DECISION_DELAY)
	/* Set up pointers to start of sub frame */
	pxq = psEncC.ltp_mem_length
	NSQ.sLTP_shp_buf_idx = psEncC.ltp_mem_length
	NSQ.sLTP_buf_idx = psEncC.ltp_mem_length
	subfr = 0
	for k = 0; k < psEncC.nb_subfr; k++ {
		A_Q12 = PredCoef_Q12[((k>>1)|(1-LSF_interpolation_flag))*MAX_LPC_ORDER:]
		B_Q14 = LTPCoef_Q14[k*LTP_ORDER:]
		AR_shp_Q13 = AR2_Q13[k*MAX_SHAPE_LPC_ORDER:]

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
				if k == 2 {
					/* RESET DELAYED DECISIONS */
					/* Find winner */
					RDmin_Q10 = psDelDec[0].RD_Q10
					Winner_ind = 0
					for i = 1; i < psEncC.nStatesDelayedDecision; i++ {
						if psDelDec[i].RD_Q10 < RDmin_Q10 {
							RDmin_Q10 = psDelDec[i].RD_Q10
							Winner_ind = i
						}
					}
					for i = 0; i < psEncC.nStatesDelayedDecision; i++ {
						if i != Winner_ind {
							psDelDec[i].RD_Q10 += (int32_MAX >> 4)
							silk_assert(psDelDec[i].RD_Q10 >= 0)
						}
					}

					/* Copy final part of signals from winner state to output and long-term filter states */
					psDD = &psDelDec[Winner_ind]
					last_smple_idx = smpl_buf_idx + decisionDelay
					for i = 0; i < decisionDelay; i++ {
						last_smple_idx = (last_smple_idx - 1) & DECISION_DELAY_MASK
						pulses[pulses_idx+i-decisionDelay] = int8(RSHIFT_ROUND(psDD.Q_Q10[last_smple_idx], 10))
						NSQ.xq[pxq+i-decisionDelay] = int16(SAT16(RSHIFT_ROUND(
							SMULWW(psDD.Xq_Q14[last_smple_idx], Gains_Q16[1]), 14)))
						NSQ.sLTP_shp_Q14[NSQ.sLTP_shp_buf_idx-decisionDelay+i] = psDD.Shape_Q14[last_smple_idx]
					}

					subfr = 0
				}

				/* Rewhiten with new A coefs */
				start_idx = psEncC.ltp_mem_length - lag - psEncC.predictLPCOrder - LTP_ORDER/2
				silk_assert(start_idx > 0)

				LPCAnalysisFilter(sLTP[start_idx:], NSQ.xq[start_idx+k*psEncC.subfr_length:],
					A_Q12, int32(psEncC.ltp_mem_length-start_idx), int32(psEncC.predictLPCOrder), psEncC.arch)

				NSQ.sLTP_buf_idx = psEncC.ltp_mem_length
				NSQ.rewhite_flag = 1
			}
		}

		nsqDelDecScaleStates(psEncC, NSQ, psDelDec, x_Q3, x_sc_Q10, sLTP, sLTP_Q15, k,
			psEncC.nStatesDelayedDecision, LTP_scale_Q14, Gains_Q16, pitchL, int(psIndices.signalType), decisionDelay)

		noiseShapeQuantizerDelDec(NSQ, psDelDec, int(psIndices.signalType), x_sc_Q10, pulses, pulses_idx, NSQ.xq[:], pxq, sLTP_Q15,
			delayedGain_Q10, A_Q12, B_Q14, AR_shp_Q13, lag, HarmShapeFIRPacked_Q14, Tilt_Q14[k], LF_shp_Q14[k],
			Gains_Q16[k], Lambda_Q10, offset_Q10, psEncC.subfr_length, subfr, psEncC.shapingLPCOrder,
			psEncC.predictLPCOrder, psEncC.warping_Q16, psEncC.nStatesDelayedDecision, &smpl_buf_idx, decisionDelay, psEncC.arch)
		subfr++

		x_Q3 = x_Q3[psEncC.subfr_length:]
		pulses_idx += psEncC.subfr_length
		pxq += psEncC.subfr_length
	}

	/* Find winner */
	RDmin_Q10 = psDelDec[0].RD_Q10
	Winner_ind = 0
	for k = 1; k < psEncC.nStatesDelayedDecision; k++ {
		if psDelDec[k].RD_Q10 < RDmin_Q10 {
			RDmin_Q10 = psDelDec[k].RD_Q10
			Winner_ind = k
		}
	}

	/* Copy final part of signals from winner state to output and long-term filter states */
	psDD = &psDelDec[Winner_ind]
	psIndices.Seed = int8(psDD.SeedInit)
	last_smple_idx = smpl_buf_idx + decisionDelay
	Gain_Q10 = RSHIFT32(Gains_Q16[psEncC.nb_subfr-1], 6)
	for i = 0; i < decisionDelay; i++ {
		last_smple_idx = (last_smple_idx - 1) & DECISION_DELAY_MASK
		pulses[pulses_idx+i-decisionDelay] = int8(RSHIFT_ROUND(psDD.Q_Q10[last_smple_idx], 10))
		NSQ.xq[pxq+i-decisionDelay] = int16(SAT16(RSHIFT_ROUND(
			SMULWW(psDD.Xq_Q14[last_smple_idx], Gain_Q10), 8)))
		NSQ.sLTP_shp_Q14[NSQ.sLTP_shp_buf_idx-decisionDelay+i] = psDD.Shape_Q14[last_smple_idx]
	}
	copy(NSQ.sLPC_Q14[:NSQ_LPC_BUF_LENGTH], psDD.sLPC_Q14[psEncC.subfr_length:psEncC.subfr_length+NSQ_LPC_BUF_LENGTH])
	NSQ.sAR2_Q14 = psDD.sAR2_Q14

	/* Update states */
	NSQ.sLF_AR_shp_Q14 = psDD.LF_AR_Q14
	NSQ.lagPrev = pitchL[psEncC.nb_subfr-1]

	/* Save quantized speech signal */
	/* DEBUG_STORE_DATA( enc.pcm, &NSQ->xq[psEncC->ltp_mem_length], psEncC->frame_length * sizeof( opus_int16 ) ) */
	copy(NSQ.xq[:psEncC.ltp_mem_length], NSQ.xq[psEncC.frame_length:psEncC.frame_length+psEncC.ltp_mem_length])
	copy(NSQ.sLTP_shp_Q14[:psEncC.ltp_mem_length], NSQ.sLTP_shp_Q14[psEncC.frame_length:psEncC.frame_length+psEncC.ltp_mem_length])
}

/******************************************/
/* Noise shape quantizer for one subframe */
/******************************************/

// C: static OPUS_INLINE void silk_noise_shape_quantizer_del_dec(
//
//	silk_nsq_state      *NSQ,                   /* I/O  NSQ state                           */
//	NSQ_del_dec_struct  psDelDec[],             /* I/O  Delayed decision states             */
//	opus_int            signalType,             /* I    Signal type                         */
//	const opus_int32    x_Q10[],                /* I                                        */
//	opus_int8           pulses[],               /* O                                        */
//	opus_int16          xq[],                   /* O                                        */
//	opus_int32          sLTP_Q15[],             /* I/O  LTP filter state                    */
//	opus_int32          delayedGain_Q10[],      /* I/O  Gain delay buffer                   */
//	const opus_int16    a_Q12[],                /* I    Short term prediction coefs         */
//	const opus_int16    b_Q14[],                /* I    Long term prediction coefs          */
//	const opus_int16    AR_shp_Q13[],           /* I    Noise shaping coefs                 */
//	opus_int            lag,                    /* I    Pitch lag                           */
//	opus_int32          HarmShapeFIRPacked_Q14, /* I                                        */
//	opus_int            Tilt_Q14,               /* I    Spectral tilt                       */
//	opus_int32          LF_shp_Q14,             /* I                                        */
//	opus_int32          Gain_Q16,               /* I                                        */
//	opus_int            Lambda_Q10,             /* I                                        */
//	opus_int            offset_Q10,             /* I                                        */
//	opus_int            length,                 /* I    Input length                        */
//	opus_int            subfr,                  /* I    Subframe number                     */
//	opus_int            shapingLPCOrder,        /* I    Shaping LPC filter order            */
//	opus_int            predictLPCOrder,        /* I    Prediction filter order             */
//	opus_int            warping_Q16,            /* I                                        */
//	opus_int            nStatesDelayedDecision, /* I    Number of states in decision tree   */
//	opus_int            *smpl_buf_idx,          /* I    Index to newest samples in buffers  */
//	opus_int            decisionDelay,          /* I                                        */
//	int                 arch                    /* I                                        */
//
// )
//
// The C pulses[] and xq[] pointers are written at negative offsets
// ( pulses[ i - decisionDelay ] ), so they are passed as the base slices plus
// the indices pulses_idx / xq_idx of the C pointers.
func noiseShapeQuantizerDelDec(NSQ *NsqState, psDelDec []nsqDelDecStruct, signalType int, x_Q10 []int32, pulses []int8, pulses_idx int, xq []int16, xq_idx int, sLTP_Q15 []int32, delayedGain_Q10 []int32, a_Q12 []int16, b_Q14 []int16, AR_shp_Q13 []int16, lag int, HarmShapeFIRPacked_Q14 int32, Tilt_Q14 int, LF_shp_Q14 int32, Gain_Q16 int32, Lambda_Q10 int, offset_Q10 int, length int, subfr int, shapingLPCOrder int, predictLPCOrder int, warping_Q16 int, nStatesDelayedDecision int, smpl_buf_idx *int, decisionDelay int, arch int) {
	var i, j, k, Winner_ind, RDmin_ind, RDmax_ind, last_smple_idx int
	var Winner_rand_state int32
	var LTP_pred_Q14, LPC_pred_Q14, n_AR_Q14, n_LTP_Q14 int32
	var n_LF_Q14, r_Q10, rr_Q10, rd1_Q10, rd2_Q10, RDmin_Q10, RDmax_Q10 int32
	var q1_Q0, q1_Q10, q2_Q10, exc_Q14, LPC_exc_Q14, xq_Q14, Gain_Q10 int32
	var tmp1, tmp2, sLF_AR_shp_Q14 int32
	/* The C pointers pred_lag_ptr, shp_lag_ptr and psLPC_Q14 are read at negative */
	/* offsets, so they are kept as indices into sLTP_Q15, NSQ.sLTP_shp_Q14 and    */
	/* psDD.sLPC_Q14.                                                               */
	var pred_lag_ptr, shp_lag_ptr, psLPC_Q14 int
	var psSampleState []nsqSamplePair
	var psDD *nsqDelDecStruct
	var psSS *nsqSamplePair

	silk_assert(nStatesDelayedDecision > 0)
	psSampleState = make([]nsqSamplePair, nStatesDelayedDecision)

	shp_lag_ptr = NSQ.sLTP_shp_buf_idx - lag + HARM_SHAPE_FIR_TAPS/2
	pred_lag_ptr = NSQ.sLTP_buf_idx - lag + LTP_ORDER/2
	Gain_Q10 = RSHIFT(Gain_Q16, 6)

	for i = 0; i < length; i++ {
		/* Perform common calculations used in all states */

		/* Long-term prediction */
		if signalType == TYPE_VOICED {
			/* Unrolled loop */
			/* Avoids introducing a bias because silk_SMLAWB() always rounds to -inf */
			LTP_pred_Q14 = 2
			LTP_pred_Q14 = SMLAWB(LTP_pred_Q14, sLTP_Q15[pred_lag_ptr-0], b_Q14[0])
			LTP_pred_Q14 = SMLAWB(LTP_pred_Q14, sLTP_Q15[pred_lag_ptr-1], b_Q14[1])
			LTP_pred_Q14 = SMLAWB(LTP_pred_Q14, sLTP_Q15[pred_lag_ptr-2], b_Q14[2])
			LTP_pred_Q14 = SMLAWB(LTP_pred_Q14, sLTP_Q15[pred_lag_ptr-3], b_Q14[3])
			LTP_pred_Q14 = SMLAWB(LTP_pred_Q14, sLTP_Q15[pred_lag_ptr-4], b_Q14[4])
			LTP_pred_Q14 = LSHIFT(LTP_pred_Q14, 1) /* Q13 -> Q14 */
			pred_lag_ptr++
		} else {
			LTP_pred_Q14 = 0
		}

		/* Long-term shaping */
		if lag > 0 {
			/* Symmetric, packed FIR coefficients */
			n_LTP_Q14 = SMULWB(ADD32(NSQ.sLTP_shp_Q14[shp_lag_ptr+0], NSQ.sLTP_shp_Q14[shp_lag_ptr-2]), HarmShapeFIRPacked_Q14)
			n_LTP_Q14 = SMLAWT(n_LTP_Q14, NSQ.sLTP_shp_Q14[shp_lag_ptr-1], HarmShapeFIRPacked_Q14)
			n_LTP_Q14 = SUB_LSHIFT32(LTP_pred_Q14, n_LTP_Q14, 2) /* Q12 -> Q14 */
			shp_lag_ptr++
		} else {
			n_LTP_Q14 = 0
		}

		for k = 0; k < nStatesDelayedDecision; k++ {
			/* Delayed decision state */
			psDD = &psDelDec[k]

			/* Sample state */
			psSS = &psSampleState[k]

			/* Generate dither */
			psDD.Seed = RAND(psDD.Seed)

			/* Pointer used in short term prediction and shaping */
			psLPC_Q14 = NSQ_LPC_BUF_LENGTH - 1 + i
			/* Short-term prediction */
			LPC_pred_Q14 = noiseShapeQuantizerShortPrediction(psDD.sLPC_Q14[:], psLPC_Q14, a_Q12, nil, predictLPCOrder, arch)
			LPC_pred_Q14 = LSHIFT(LPC_pred_Q14, 4) /* Q10 -> Q14 */

			/* Noise shape feedback */
			silk_assert((shapingLPCOrder & 1) == 0) /* check that order is even */
			/* Output of lowpass section */
			tmp2 = SMLAWB(psDD.sLPC_Q14[psLPC_Q14], psDD.sAR2_Q14[0], warping_Q16)
			/* Output of allpass section */
			tmp1 = SMLAWB(psDD.sAR2_Q14[0], psDD.sAR2_Q14[1]-tmp2, warping_Q16)
			psDD.sAR2_Q14[0] = tmp2
			n_AR_Q14 = int32(RSHIFT(shapingLPCOrder, 1))
			n_AR_Q14 = SMLAWB(n_AR_Q14, tmp2, AR_shp_Q13[0])
			/* Loop over allpass sections */
			for j = 2; j < shapingLPCOrder; j += 2 {
				/* Output of allpass section */
				tmp2 = SMLAWB(psDD.sAR2_Q14[j-1], psDD.sAR2_Q14[j+0]-tmp1, warping_Q16)
				psDD.sAR2_Q14[j-1] = tmp1
				n_AR_Q14 = SMLAWB(n_AR_Q14, tmp1, AR_shp_Q13[j-1])
				/* Output of allpass section */
				tmp1 = SMLAWB(psDD.sAR2_Q14[j+0], psDD.sAR2_Q14[j+1]-tmp2, warping_Q16)
				psDD.sAR2_Q14[j+0] = tmp2
				n_AR_Q14 = SMLAWB(n_AR_Q14, tmp2, AR_shp_Q13[j])
			}
			psDD.sAR2_Q14[shapingLPCOrder-1] = tmp1
			n_AR_Q14 = SMLAWB(n_AR_Q14, tmp1, AR_shp_Q13[shapingLPCOrder-1])

			n_AR_Q14 = LSHIFT(n_AR_Q14, 1)                        /* Q11 -> Q12 */
			n_AR_Q14 = SMLAWB(n_AR_Q14, psDD.LF_AR_Q14, Tilt_Q14) /* Q12 */
			n_AR_Q14 = LSHIFT(n_AR_Q14, 2)                        /* Q12 -> Q14 */

			n_LF_Q14 = SMULWB(psDD.Shape_Q14[*smpl_buf_idx], LF_shp_Q14) /* Q12 */
			n_LF_Q14 = SMLAWT(n_LF_Q14, psDD.LF_AR_Q14, LF_shp_Q14)      /* Q12 */
			n_LF_Q14 = LSHIFT(n_LF_Q14, 2)                               /* Q12 -> Q14 */

			/* Input minus prediction plus noise feedback                       */
			/* r = x[ i ] - LTP_pred - LPC_pred + n_AR + n_Tilt + n_LF + n_LTP  */
			tmp1 = ADD32(n_AR_Q14, n_LF_Q14)      /* Q14 */
			tmp2 = ADD32(n_LTP_Q14, LPC_pred_Q14) /* Q13 */
			tmp1 = SUB32(tmp2, tmp1)              /* Q13 */
			tmp1 = RSHIFT_ROUND(tmp1, 4)          /* Q10 */

			r_Q10 = SUB32(x_Q10[i], tmp1) /* residual error Q10 */

			/* Flip sign depending on dither */
			if psDD.Seed < 0 {
				r_Q10 = -r_Q10
			}
			r_Q10 = LIMIT_32(r_Q10, -(31 << 10), 30<<10)

			/* Find two quantization level candidates and measure their rate-distortion */
			q1_Q10 = SUB32(r_Q10, offset_Q10)
			q1_Q0 = RSHIFT(q1_Q10, 10)
			if q1_Q0 > 0 {
				q1_Q10 = SUB32(LSHIFT(q1_Q0, 10), QUANT_LEVEL_ADJUST_Q10)
				q1_Q10 = ADD32(q1_Q10, offset_Q10)
				q2_Q10 = ADD32(q1_Q10, 1024)
				rd1_Q10 = SMULBB(q1_Q10, Lambda_Q10)
				rd2_Q10 = SMULBB(q2_Q10, Lambda_Q10)
			} else if q1_Q0 == 0 {
				q1_Q10 = int32(offset_Q10)
				q2_Q10 = ADD32(q1_Q10, 1024-QUANT_LEVEL_ADJUST_Q10)
				rd1_Q10 = SMULBB(q1_Q10, Lambda_Q10)
				rd2_Q10 = SMULBB(q2_Q10, Lambda_Q10)
			} else if q1_Q0 == -1 {
				q2_Q10 = int32(offset_Q10)
				q1_Q10 = SUB32(q2_Q10, 1024-QUANT_LEVEL_ADJUST_Q10)
				rd1_Q10 = SMULBB(-q1_Q10, Lambda_Q10)
				rd2_Q10 = SMULBB(q2_Q10, Lambda_Q10)
			} else { /* q1_Q0 < -1 */
				q1_Q10 = ADD32(LSHIFT(q1_Q0, 10), QUANT_LEVEL_ADJUST_Q10)
				q1_Q10 = ADD32(q1_Q10, offset_Q10)
				q2_Q10 = ADD32(q1_Q10, 1024)
				rd1_Q10 = SMULBB(-q1_Q10, Lambda_Q10)
				rd2_Q10 = SMULBB(-q2_Q10, Lambda_Q10)
			}
			rr_Q10 = SUB32(r_Q10, q1_Q10)
			rd1_Q10 = RSHIFT(SMLABB(rd1_Q10, rr_Q10, rr_Q10), 10)
			rr_Q10 = SUB32(r_Q10, q2_Q10)
			rd2_Q10 = RSHIFT(SMLABB(rd2_Q10, rr_Q10, rr_Q10), 10)

			if rd1_Q10 < rd2_Q10 {
				psSS[0].RD_Q10 = ADD32(psDD.RD_Q10, rd1_Q10)
				psSS[1].RD_Q10 = ADD32(psDD.RD_Q10, rd2_Q10)
				psSS[0].Q_Q10 = q1_Q10
				psSS[1].Q_Q10 = q2_Q10
			} else {
				psSS[0].RD_Q10 = ADD32(psDD.RD_Q10, rd2_Q10)
				psSS[1].RD_Q10 = ADD32(psDD.RD_Q10, rd1_Q10)
				psSS[0].Q_Q10 = q2_Q10
				psSS[1].Q_Q10 = q1_Q10
			}

			/* Update states for best quantization */

			/* Quantized excitation */
			exc_Q14 = LSHIFT32(psSS[0].Q_Q10, 4)
			if psDD.Seed < 0 {
				exc_Q14 = -exc_Q14
			}

			/* Add predictions */
			LPC_exc_Q14 = ADD32(exc_Q14, LTP_pred_Q14)
			xq_Q14 = ADD32(LPC_exc_Q14, LPC_pred_Q14)

			/* Update states */
			sLF_AR_shp_Q14 = SUB32(xq_Q14, n_AR_Q14)
			psSS[0].sLTP_shp_Q14 = SUB32(sLF_AR_shp_Q14, n_LF_Q14)
			psSS[0].LF_AR_Q14 = sLF_AR_shp_Q14
			psSS[0].LPC_exc_Q14 = LPC_exc_Q14
			psSS[0].xq_Q14 = xq_Q14

			/* Update states for second best quantization */

			/* Quantized excitation */
			exc_Q14 = LSHIFT32(psSS[1].Q_Q10, 4)
			if psDD.Seed < 0 {
				exc_Q14 = -exc_Q14
			}

			/* Add predictions */
			LPC_exc_Q14 = ADD32(exc_Q14, LTP_pred_Q14)
			xq_Q14 = ADD32(LPC_exc_Q14, LPC_pred_Q14)

			/* Update states */
			sLF_AR_shp_Q14 = SUB32(xq_Q14, n_AR_Q14)
			psSS[1].sLTP_shp_Q14 = SUB32(sLF_AR_shp_Q14, n_LF_Q14)
			psSS[1].LF_AR_Q14 = sLF_AR_shp_Q14
			psSS[1].LPC_exc_Q14 = LPC_exc_Q14
			psSS[1].xq_Q14 = xq_Q14
		}

		*smpl_buf_idx = (*smpl_buf_idx - 1) & DECISION_DELAY_MASK              /* Index to newest samples              */
		last_smple_idx = (*smpl_buf_idx + decisionDelay) & DECISION_DELAY_MASK /* Index to decisionDelay old samples   */

		/* Find winner */
		RDmin_Q10 = psSampleState[0][0].RD_Q10
		Winner_ind = 0
		for k = 1; k < nStatesDelayedDecision; k++ {
			if psSampleState[k][0].RD_Q10 < RDmin_Q10 {
				RDmin_Q10 = psSampleState[k][0].RD_Q10
				Winner_ind = k
			}
		}

		/* Increase RD values of expired states */
		Winner_rand_state = psDelDec[Winner_ind].RandState[last_smple_idx]
		for k = 0; k < nStatesDelayedDecision; k++ {
			if psDelDec[k].RandState[last_smple_idx] != Winner_rand_state {
				psSampleState[k][0].RD_Q10 = ADD32(psSampleState[k][0].RD_Q10, int32_MAX>>4)
				psSampleState[k][1].RD_Q10 = ADD32(psSampleState[k][1].RD_Q10, int32_MAX>>4)
				silk_assert(psSampleState[k][0].RD_Q10 >= 0)
			}
		}

		/* Find worst in first set and best in second set */
		RDmax_Q10 = psSampleState[0][0].RD_Q10
		RDmin_Q10 = psSampleState[0][1].RD_Q10
		RDmax_ind = 0
		RDmin_ind = 0
		for k = 1; k < nStatesDelayedDecision; k++ {
			/* find worst in first set */
			if psSampleState[k][0].RD_Q10 > RDmax_Q10 {
				RDmax_Q10 = psSampleState[k][0].RD_Q10
				RDmax_ind = k
			}
			/* find best in second set */
			if psSampleState[k][1].RD_Q10 < RDmin_Q10 {
				RDmin_Q10 = psSampleState[k][1].RD_Q10
				RDmin_ind = k
			}
		}

		/* Replace a state if best from second set outperforms worst in first set */
		if RDmin_Q10 < RDmax_Q10 {
			/* C: silk_memcpy( ( (opus_int32 *)&psDelDec[ RDmax_ind ] ) + i,              */
			/*                 ( (opus_int32 *)&psDelDec[ RDmin_ind ] ) + i,              */
			/*                 sizeof( NSQ_del_dec_struct ) - i * sizeof( opus_int32) ); */
			/* i.e. copy the whole struct except its first i int32s, which are           */
			/* sLPC_Q14[ 0 .. i-1 ] (sLPC_Q14 is the first member).                       */
			psDDmax, psDDmin := &psDelDec[RDmax_ind], &psDelDec[RDmin_ind]
			copy(psDDmax.sLPC_Q14[i:], psDDmin.sLPC_Q14[i:])
			psDDmax.RandState = psDDmin.RandState
			psDDmax.Q_Q10 = psDDmin.Q_Q10
			psDDmax.Xq_Q14 = psDDmin.Xq_Q14
			psDDmax.Pred_Q15 = psDDmin.Pred_Q15
			psDDmax.Shape_Q14 = psDDmin.Shape_Q14
			psDDmax.sAR2_Q14 = psDDmin.sAR2_Q14
			psDDmax.LF_AR_Q14 = psDDmin.LF_AR_Q14
			psDDmax.Seed = psDDmin.Seed
			psDDmax.SeedInit = psDDmin.SeedInit
			psDDmax.RD_Q10 = psDDmin.RD_Q10
			psSampleState[RDmax_ind][0] = psSampleState[RDmin_ind][1]
		}

		/* Write samples from winner to output and long-term filter states */
		psDD = &psDelDec[Winner_ind]
		if subfr > 0 || i >= decisionDelay {
			pulses[pulses_idx+i-decisionDelay] = int8(RSHIFT_ROUND(psDD.Q_Q10[last_smple_idx], 10))
			xq[xq_idx+i-decisionDelay] = int16(SAT16(RSHIFT_ROUND(
				SMULWW(psDD.Xq_Q14[last_smple_idx], delayedGain_Q10[last_smple_idx]), 8)))
			NSQ.sLTP_shp_Q14[NSQ.sLTP_shp_buf_idx-decisionDelay] = psDD.Shape_Q14[last_smple_idx]
			sLTP_Q15[NSQ.sLTP_buf_idx-decisionDelay] = psDD.Pred_Q15[last_smple_idx]
		}
		NSQ.sLTP_shp_buf_idx++
		NSQ.sLTP_buf_idx++

		/* Update states */
		for k = 0; k < nStatesDelayedDecision; k++ {
			psDD = &psDelDec[k]
			psSS := &psSampleState[k][0]
			psDD.LF_AR_Q14 = psSS.LF_AR_Q14
			psDD.sLPC_Q14[NSQ_LPC_BUF_LENGTH+i] = psSS.xq_Q14
			psDD.Xq_Q14[*smpl_buf_idx] = psSS.xq_Q14
			psDD.Q_Q10[*smpl_buf_idx] = psSS.Q_Q10
			psDD.Pred_Q15[*smpl_buf_idx] = LSHIFT32(psSS.LPC_exc_Q14, 1)
			psDD.Shape_Q14[*smpl_buf_idx] = psSS.sLTP_shp_Q14
			psDD.Seed = ADD32_ovflw(psDD.Seed, RSHIFT_ROUND(psSS.Q_Q10, 10))
			psDD.RandState[*smpl_buf_idx] = psDD.Seed
			psDD.RD_Q10 = psSS.RD_Q10
		}
		delayedGain_Q10[*smpl_buf_idx] = Gain_Q10
	}
	/* Update LPC states */
	for k = 0; k < nStatesDelayedDecision; k++ {
		psDD = &psDelDec[k]
		copy(psDD.sLPC_Q14[:NSQ_LPC_BUF_LENGTH], psDD.sLPC_Q14[length:length+NSQ_LPC_BUF_LENGTH])
	}
}

// C: static OPUS_INLINE void silk_nsq_del_dec_scale_states(
//
//	const silk_encoder_state *psEncC,               /* I    Encoder State                       */
//	silk_nsq_state      *NSQ,                       /* I/O  NSQ state                           */
//	NSQ_del_dec_struct  psDelDec[],                 /* I/O  Delayed decision states             */
//	const opus_int32    x_Q3[],                     /* I    Input in Q3                         */
//	opus_int32          x_sc_Q10[],                 /* O    Input scaled with 1/Gain in Q10     */
//	const opus_int16    sLTP[],                     /* I    Re-whitened LTP state in Q0         */
//	opus_int32          sLTP_Q15[],                 /* O    LTP state matching scaled input     */
//	opus_int            subfr,                      /* I    Subframe number                     */
//	opus_int            nStatesDelayedDecision,     /* I    Number of del dec states            */
//	const opus_int      LTP_scale_Q14,              /* I    LTP state scaling                   */
//	const opus_int32    Gains_Q16[ MAX_NB_SUBFR ],  /* I                                        */
//	const opus_int      pitchL[ MAX_NB_SUBFR ],     /* I    Pitch lag                           */
//	const opus_int      signal_type,                /* I    Signal type                         */
//	const opus_int      decisionDelay               /* I    Decision delay                      */
//
// )
func nsqDelDecScaleStates(psEncC *EncoderState, NSQ *NsqState, psDelDec []nsqDelDecStruct, x_Q3 []int32, x_sc_Q10 []int32, sLTP []int16, sLTP_Q15 []int32, subfr int, nStatesDelayedDecision int, LTP_scale_Q14 int, Gains_Q16 []int32, pitchL []int, signal_type int, decisionDelay int) {
	var i, k, lag int
	var gain_adj_Q16, inv_gain_Q31, inv_gain_Q23 int32
	var psDD *nsqDelDecStruct

	lag = pitchL[subfr]
	inv_gain_Q31 = INVERSE32_varQ(max(Gains_Q16[subfr], 1), 47)
	silk_assert(inv_gain_Q31 != 0)

	/* Calculate gain adjustment factor */
	if Gains_Q16[subfr] != NSQ.prev_gain_Q16 {
		gain_adj_Q16 = DIV32_varQ(NSQ.prev_gain_Q16, Gains_Q16[subfr], 16)
	} else {
		gain_adj_Q16 = int32(1) << 16
	}

	/* Scale input */
	inv_gain_Q23 = RSHIFT_ROUND(inv_gain_Q31, 8)
	for i = 0; i < psEncC.subfr_length; i++ {
		x_sc_Q10[i] = SMULWW(x_Q3[i], inv_gain_Q23)
	}

	/* Save inverse gain */
	NSQ.prev_gain_Q16 = Gains_Q16[subfr]

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
	if gain_adj_Q16 != int32(1)<<16 {
		/* Scale long-term shaping state */
		for i = NSQ.sLTP_shp_buf_idx - psEncC.ltp_mem_length; i < NSQ.sLTP_shp_buf_idx; i++ {
			NSQ.sLTP_shp_Q14[i] = SMULWW(gain_adj_Q16, NSQ.sLTP_shp_Q14[i])
		}

		/* Scale long-term prediction state */
		if signal_type == TYPE_VOICED && NSQ.rewhite_flag == 0 {
			for i = NSQ.sLTP_buf_idx - lag - LTP_ORDER/2; i < NSQ.sLTP_buf_idx-decisionDelay; i++ {
				sLTP_Q15[i] = SMULWW(gain_adj_Q16, sLTP_Q15[i])
			}
		}

		for k = 0; k < nStatesDelayedDecision; k++ {
			psDD = &psDelDec[k]

			/* Scale scalar states */
			psDD.LF_AR_Q14 = SMULWW(gain_adj_Q16, psDD.LF_AR_Q14)

			/* Scale short-term prediction and shaping states */
			for i = 0; i < NSQ_LPC_BUF_LENGTH; i++ {
				psDD.sLPC_Q14[i] = SMULWW(gain_adj_Q16, psDD.sLPC_Q14[i])
			}
			for i = 0; i < MAX_SHAPE_LPC_ORDER; i++ {
				psDD.sAR2_Q14[i] = SMULWW(gain_adj_Q16, psDD.sAR2_Q14[i])
			}
			for i = 0; i < DECISION_DELAY; i++ {
				psDD.Pred_Q15[i] = SMULWW(gain_adj_Q16, psDD.Pred_Q15[i])
				psDD.Shape_Q14[i] = SMULWW(gain_adj_Q16, psDD.Shape_Q14[i])
			}
		}
	}
}
