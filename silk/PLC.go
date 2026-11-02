package silk

// C: silk/PLC.c (constants from PLC.h are in PLC_h.go)

const NB_ATT = 2

var HARM_ATT_Q15 = [NB_ATT]int16{32440, 31130}              /* 0.99, 0.95 */
var PLC_RAND_ATTENUATE_V_Q15 = [NB_ATT]int16{31130, 26214}  /* 0.95, 0.8 */
var PLC_RAND_ATTENUATE_UV_Q15 = [NB_ATT]int16{32440, 29491} /* 0.99, 0.9 */

// C: void silk_PLC_Reset(silk_decoder_state *psDec)
func (psDec *DecoderState) PLCReset() {
	psDec.sPLC.pitchL_Q8 = LSHIFT(psDec.frame_length, 8-1)
	psDec.sPLC.prevGain_Q16[0] = SILK_FIX_CONST(1.0, 16)
	psDec.sPLC.prevGain_Q16[1] = SILK_FIX_CONST(1.0, 16)
	psDec.sPLC.subfr_length = 20
	psDec.sPLC.nb_subfr = 2
}

// C: void silk_PLC(silk_decoder_state *psDec, silk_decoder_control *psDecCtrl, opus_int16 frame[], opus_int lost)
func (psDec *DecoderState) PLC(psDecCtrl *DecoderControl, frame []int16, lost int) {
	/* PLC control function */
	if psDec.fs_kHz != psDec.sPLC.fs_kHz {
		psDec.PLCReset()
		psDec.sPLC.fs_kHz = psDec.fs_kHz
	}

	if lost != 0 {
		/****************************/
		/* Generate Signal          */
		/****************************/
		plcConceal(psDec, psDecCtrl, frame)

		psDec.lossCnt++
	} else {
		/****************************/
		/* Update state             */
		/****************************/
		plcUpdate(psDec, psDecCtrl)
	}
}

/**************************************************/
/* Update state of PLC                            */
/**************************************************/
//
// C: static inline void silk_PLC_update(silk_decoder_state *psDec, silk_decoder_control *psDecCtrl)
func plcUpdate(
	psDec *DecoderState, /* I/O Decoder state        */
	psDecCtrl *DecoderControl, /* I/O Decoder control      */
) {
	var LTP_Gain_Q14, temp_LTP_Gain_Q14 int32
	var i, j int
	var psPLC *PLCStruct

	psPLC = &psDec.sPLC

	/* Update parameters used in case of packet loss */
	psDec.prevSignalType = int(psDec.indices.signalType)
	LTP_Gain_Q14 = 0
	if psDec.indices.signalType == TYPE_VOICED {
		/* Find the parameters for the last subframe which contains a pitch pulse */
		for j = 0; j*psDec.subfr_length < psDecCtrl.pitchL[psDec.nb_subfr-1]; j++ {
			if j == psDec.nb_subfr {
				break
			}
			temp_LTP_Gain_Q14 = 0
			for i = 0; i < LTP_ORDER; i++ {
				temp_LTP_Gain_Q14 += int32(psDecCtrl.LTPCoef_Q14[(psDec.nb_subfr-1-j)*LTP_ORDER+i])
			}
			if temp_LTP_Gain_Q14 > LTP_Gain_Q14 {
				LTP_Gain_Q14 = temp_LTP_Gain_Q14
				copy(psPLC.LTPCoef_Q14[:],
					psDecCtrl.LTPCoef_Q14[SMULBB(psDec.nb_subfr-1-j, LTP_ORDER):][:LTP_ORDER])

				psPLC.pitchL_Q8 = LSHIFT(psDecCtrl.pitchL[psDec.nb_subfr-1-j], 8)
			}
		}

		clear(psPLC.LTPCoef_Q14[:])
		psPLC.LTPCoef_Q14[LTP_ORDER/2] = int16(LTP_Gain_Q14)

		/* Limit LT coefs */
		if LTP_Gain_Q14 < V_PITCH_GAIN_START_MIN_Q14 {
			var scale_Q10 int
			var tmp int32

			tmp = LSHIFT(V_PITCH_GAIN_START_MIN_Q14, 10)
			scale_Q10 = int(DIV32(tmp, max(LTP_Gain_Q14, 1)))
			for i = 0; i < LTP_ORDER; i++ {
				psPLC.LTPCoef_Q14[i] = int16(RSHIFT(SMULBB(psPLC.LTPCoef_Q14[i], scale_Q10), 10))
			}
		} else if LTP_Gain_Q14 > V_PITCH_GAIN_START_MAX_Q14 {
			var scale_Q14 int
			var tmp int32

			tmp = LSHIFT(V_PITCH_GAIN_START_MAX_Q14, 14)
			scale_Q14 = int(DIV32(tmp, max(LTP_Gain_Q14, 1)))
			for i = 0; i < LTP_ORDER; i++ {
				psPLC.LTPCoef_Q14[i] = int16(RSHIFT(SMULBB(psPLC.LTPCoef_Q14[i], scale_Q14), 14))
			}
		}
	} else {
		psPLC.pitchL_Q8 = LSHIFT(SMULBB(psDec.fs_kHz, 18), 8)
		clear(psPLC.LTPCoef_Q14[:])
	}

	/* Save LPC coeficients */
	copy(psPLC.prevLPC_Q12[:psDec.LPC_order], psDecCtrl.PredCoef_Q12[1][:psDec.LPC_order])
	psPLC.prevLTP_scale_Q14 = int16(psDecCtrl.LTP_scale_Q14)

	/* Save last two gains */
	copy(psPLC.prevGain_Q16[:], psDecCtrl.Gains_Q16[psDec.nb_subfr-2:psDec.nb_subfr])

	psPLC.subfr_length = psDec.subfr_length
	psPLC.nb_subfr = psDec.nb_subfr
}

// C: static inline void silk_PLC_conceal(silk_decoder_state *psDec, silk_decoder_control *psDecCtrl, opus_int16 frame[])
func plcConceal(
	psDec *DecoderState, /* I/O Decoder state        */
	psDecCtrl *DecoderControl, /* I/O Decoder control      */
	frame []int16, /* O LPC residual signal    */
) {
	var i, j, k int
	var lag, idx, sLTP_buf_idx, shift1, shift2 int
	var rand_seed, harm_Gain_Q15, rand_Gain_Q15, inv_gain_Q30 int32
	var energy1, energy2 int32
	var rand_ptr []int32
	/* pred_lag_ptr is an index into sLTP_Q14 because the C code reads before the pointer */
	var pred_lag_ptr int
	var LPC_pred_Q10, LTP_pred_Q12 int32
	var rand_scale_Q14 int16
	var B_Q14, exc_buf_ptr []int16
	var sLPC_Q14_ptr []int32
	var exc_buf []int16
	var A_Q12 [MAX_LPC_ORDER]int16
	var sLTP []int16
	var sLTP_Q14 []int32
	psPLC := &psDec.sPLC
	var prevGain_Q10 [2]int32

	exc_buf = make([]int16, 2*psPLC.subfr_length)
	sLTP = make([]int16, psDec.ltp_mem_length)
	sLTP_Q14 = make([]int32, psDec.ltp_mem_length+psDec.frame_length)

	prevGain_Q10[0] = RSHIFT(psPLC.prevGain_Q16[0], 6)
	prevGain_Q10[1] = RSHIFT(psPLC.prevGain_Q16[1], 6)

	if psDec.first_frame_after_reset != 0 {
		clear(psPLC.prevLPC_Q12[:])
	}

	/* Find random noise component */
	/* Scale previous excitation signal */
	exc_buf_ptr = exc_buf
	for k = 0; k < 2; k++ {
		for i = 0; i < psPLC.subfr_length; i++ {
			exc_buf_ptr[i] = int16(SAT16(RSHIFT(
				SMULWW(psDec.exc_Q14[i+(k+psPLC.nb_subfr-2)*psPLC.subfr_length], prevGain_Q10[k]), 8)))
		}
		exc_buf_ptr = exc_buf_ptr[psPLC.subfr_length:]
	}
	/* Find the subframe with lowest energy of the last two and use that as random noise generator */
	SumSqrShift(&energy1, &shift1, exc_buf, psPLC.subfr_length)
	SumSqrShift(&energy2, &shift2, exc_buf[psPLC.subfr_length:], psPLC.subfr_length)

	if RSHIFT(energy1, shift2) < RSHIFT(energy2, shift1) {
		/* First sub-frame has lowest energy */
		rand_ptr = psDec.exc_Q14[max_int(0, (psPLC.nb_subfr-1)*psPLC.subfr_length-RAND_BUF_SIZE):]
	} else {
		/* Second sub-frame has lowest energy */
		rand_ptr = psDec.exc_Q14[max_int(0, psPLC.nb_subfr*psPLC.subfr_length-RAND_BUF_SIZE):]
	}

	/* Set up Gain to random noise component */
	B_Q14 = psPLC.LTPCoef_Q14[:]
	rand_scale_Q14 = psPLC.randScale_Q14

	/* Set up attenuation gains */
	harm_Gain_Q15 = int32(HARM_ATT_Q15[min_int(NB_ATT-1, psDec.lossCnt)])
	if psDec.prevSignalType == TYPE_VOICED {
		rand_Gain_Q15 = int32(PLC_RAND_ATTENUATE_V_Q15[min_int(NB_ATT-1, psDec.lossCnt)])
	} else {
		rand_Gain_Q15 = int32(PLC_RAND_ATTENUATE_UV_Q15[min_int(NB_ATT-1, psDec.lossCnt)])
	}

	/* LPC concealment. Apply BWE to previous LPC */
	Bwexpander(psPLC.prevLPC_Q12[:], psDec.LPC_order, SILK_FIX_CONST(BWE_COEF, 16))

	/* Preload LPC coeficients to array on stack. Gives small performance gain */
	copy(A_Q12[:psDec.LPC_order], psPLC.prevLPC_Q12[:psDec.LPC_order])

	/* First Lost frame */
	if psDec.lossCnt == 0 {
		rand_scale_Q14 = 1 << 14

		/* Reduce random noise Gain for voiced frames */
		if psDec.prevSignalType == TYPE_VOICED {
			for i = 0; i < LTP_ORDER; i++ {
				rand_scale_Q14 -= B_Q14[i]
			}
			rand_scale_Q14 = max_16(3277, rand_scale_Q14) /* 0.2 */
			rand_scale_Q14 = int16(RSHIFT(SMULBB(rand_scale_Q14, psPLC.prevLTP_scale_Q14), 14))
		} else {
			/* Reduce random noise for unvoiced frames with high LPC gain */
			var invGain_Q30, down_scale_Q30 int32

			invGain_Q30 = LPCInversePredGain(psPLC.prevLPC_Q12[:], psDec.LPC_order)

			down_scale_Q30 = min_32(RSHIFT(int32(1)<<30, LOG2_INV_LPC_GAIN_HIGH_THRES), invGain_Q30)
			down_scale_Q30 = max_32(RSHIFT(int32(1)<<30, LOG2_INV_LPC_GAIN_LOW_THRES), down_scale_Q30)
			down_scale_Q30 = LSHIFT(down_scale_Q30, LOG2_INV_LPC_GAIN_HIGH_THRES)

			rand_Gain_Q15 = RSHIFT(SMULWB(down_scale_Q30, rand_Gain_Q15), 14)
		}
	}

	rand_seed = psPLC.rand_seed
	lag = int(RSHIFT_ROUND(psPLC.pitchL_Q8, 8))
	sLTP_buf_idx = psDec.ltp_mem_length

	/* Rewhiten LTP state */
	idx = psDec.ltp_mem_length - lag - psDec.LPC_order - LTP_ORDER/2
	silk_assert(idx > 0)
	LPCAnalysisFilter(sLTP[idx:], psDec.outBuf[idx:], A_Q12[:], int32(psDec.ltp_mem_length-idx), int32(psDec.LPC_order))
	/* Scale LTP state */
	inv_gain_Q30 = INVERSE32_varQ(psPLC.prevGain_Q16[1], 46)
	inv_gain_Q30 = min(inv_gain_Q30, int32_MAX>>1)
	for i = idx + psDec.LPC_order; i < psDec.ltp_mem_length; i++ {
		sLTP_Q14[i] = SMULWB(inv_gain_Q30, sLTP[i])
	}

	/***************************/
	/* LTP synthesis filtering */
	/***************************/
	for k = 0; k < psDec.nb_subfr; k++ {
		/* Set up pointer */
		pred_lag_ptr = sLTP_buf_idx - lag + LTP_ORDER/2
		for i = 0; i < psDec.subfr_length; i++ {
			/* Unrolled loop */
			/* Avoids introducing a bias because silk_SMLAWB() always rounds to -inf */
			LTP_pred_Q12 = 2
			LTP_pred_Q12 = SMLAWB(LTP_pred_Q12, sLTP_Q14[pred_lag_ptr+0], B_Q14[0])
			LTP_pred_Q12 = SMLAWB(LTP_pred_Q12, sLTP_Q14[pred_lag_ptr-1], B_Q14[1])
			LTP_pred_Q12 = SMLAWB(LTP_pred_Q12, sLTP_Q14[pred_lag_ptr-2], B_Q14[2])
			LTP_pred_Q12 = SMLAWB(LTP_pred_Q12, sLTP_Q14[pred_lag_ptr-3], B_Q14[3])
			LTP_pred_Q12 = SMLAWB(LTP_pred_Q12, sLTP_Q14[pred_lag_ptr-4], B_Q14[4])
			pred_lag_ptr++

			/* Generate LPC excitation */
			rand_seed = RAND(rand_seed)
			idx = int(RSHIFT(rand_seed, 25) & RAND_BUF_MASK)
			sLTP_Q14[sLTP_buf_idx] = LSHIFT32(SMLAWB(LTP_pred_Q12, rand_ptr[idx], rand_scale_Q14), 2)
			sLTP_buf_idx++
		}

		/* Gradually reduce LTP gain */
		for j = 0; j < LTP_ORDER; j++ {
			B_Q14[j] = int16(RSHIFT(SMULBB(harm_Gain_Q15, B_Q14[j]), 15))
		}
		/* Gradually reduce excitation gain */
		rand_scale_Q14 = int16(RSHIFT(SMULBB(rand_scale_Q14, rand_Gain_Q15), 15))

		/* Slowly increase pitch lag */
		psPLC.pitchL_Q8 = SMLAWB(psPLC.pitchL_Q8, psPLC.pitchL_Q8, PITCH_DRIFT_FAC_Q16)
		psPLC.pitchL_Q8 = min_32(psPLC.pitchL_Q8, LSHIFT(SMULBB(MAX_PITCH_LAG_MS, psDec.fs_kHz), 8))
		lag = int(RSHIFT_ROUND(psPLC.pitchL_Q8, 8))
	}

	/***************************/
	/* LPC synthesis filtering */
	/***************************/
	sLPC_Q14_ptr = sLTP_Q14[psDec.ltp_mem_length-MAX_LPC_ORDER:]

	/* Copy LPC state */
	copy(sLPC_Q14_ptr[:MAX_LPC_ORDER], psDec.sLPC_Q14_buf[:])

	silk_assert(psDec.LPC_order >= 10) /* check that unrolling works */
	for i = 0; i < psDec.frame_length; i++ {
		/* partly unrolled */
		/* Avoids introducing a bias because silk_SMLAWB() always rounds to -inf */
		LPC_pred_Q10 = int32(RSHIFT(psDec.LPC_order, 1))
		LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14_ptr[MAX_LPC_ORDER+i-1], A_Q12[0])
		LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14_ptr[MAX_LPC_ORDER+i-2], A_Q12[1])
		LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14_ptr[MAX_LPC_ORDER+i-3], A_Q12[2])
		LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14_ptr[MAX_LPC_ORDER+i-4], A_Q12[3])
		LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14_ptr[MAX_LPC_ORDER+i-5], A_Q12[4])
		LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14_ptr[MAX_LPC_ORDER+i-6], A_Q12[5])
		LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14_ptr[MAX_LPC_ORDER+i-7], A_Q12[6])
		LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14_ptr[MAX_LPC_ORDER+i-8], A_Q12[7])
		LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14_ptr[MAX_LPC_ORDER+i-9], A_Q12[8])
		LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14_ptr[MAX_LPC_ORDER+i-10], A_Q12[9])
		for j = 10; j < psDec.LPC_order; j++ {
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14_ptr[MAX_LPC_ORDER+i-j-1], A_Q12[j])
		}

		/* Add prediction to LPC excitation */
		sLPC_Q14_ptr[MAX_LPC_ORDER+i] = ADD_LSHIFT32(sLPC_Q14_ptr[MAX_LPC_ORDER+i], LPC_pred_Q10, 4)

		/* Scale with Gain */
		frame[i] = int16(SAT16(SAT16(RSHIFT_ROUND(SMULWW(sLPC_Q14_ptr[MAX_LPC_ORDER+i], prevGain_Q10[1]), 8))))
	}

	/* Save LPC state */
	copy(psDec.sLPC_Q14_buf[:], sLPC_Q14_ptr[psDec.frame_length:psDec.frame_length+MAX_LPC_ORDER])

	/**************************************/
	/* Update states                      */
	/**************************************/
	psPLC.rand_seed = rand_seed
	psPLC.randScale_Q14 = rand_scale_Q14
	for i = 0; i < MAX_NB_SUBFR; i++ {
		psDecCtrl.pitchL[i] = lag
	}
}

/* Glues concealed frames with new good received frames */
//
// C: void silk_PLC_glue_frames(silk_decoder_state *psDec, opus_int16 frame[], opus_int length)
func (psDec *DecoderState) PLCGlueFrames(frame []int16, length int) {
	var i, energy_shift int
	var energy int32
	var psPLC *PLCStruct
	psPLC = &psDec.sPLC

	if psDec.lossCnt != 0 {
		/* Calculate energy in concealed residual */
		SumSqrShift(&psPLC.conc_energy, &psPLC.conc_energy_shift, frame, length)

		psPLC.last_frame_lost = 1
	} else {
		if psDec.sPLC.last_frame_lost != 0 {
			/* Calculate residual in decoded signal if last frame was lost */
			SumSqrShift(&energy, &energy_shift, frame, length)

			/* Normalize energies */
			if energy_shift > psPLC.conc_energy_shift {
				psPLC.conc_energy = RSHIFT(psPLC.conc_energy, energy_shift-psPLC.conc_energy_shift)
			} else if energy_shift < psPLC.conc_energy_shift {
				energy = RSHIFT(energy, psPLC.conc_energy_shift-energy_shift)
			}

			/* Fade in the energy difference */
			if energy > psPLC.conc_energy {
				var frac_Q24, LZ int32
				var gain_Q16, slope_Q16 int32

				LZ = CLZ32(psPLC.conc_energy)
				LZ = LZ - 1
				psPLC.conc_energy = LSHIFT(psPLC.conc_energy, int(LZ))
				energy = RSHIFT(energy, int(max_32(24-LZ, 0)))

				frac_Q24 = DIV32(psPLC.conc_energy, max(energy, 1))

				gain_Q16 = LSHIFT(SQRT_APPROX(frac_Q24), 4)
				slope_Q16 = DIV32_16((int32(1)<<16)-gain_Q16, length)
				/* Make slope 4x steeper to avoid missing onsets after DTX */
				slope_Q16 = LSHIFT(slope_Q16, 2)

				for i = 0; i < length; i++ {
					frame[i] = int16(SMULWB(gain_Q16, frame[i]))
					gain_Q16 += slope_Q16
					if gain_Q16 > int32(1)<<16 {
						break
					}
				}
			}
		}
		psPLC.last_frame_lost = 0
	}
}
