package silk

// C: silk/decode_core.c

/**********************************************************/
/* Core decoder. Performs inverse NSQ operation LTP + LPC */
/**********************************************************/
//
// C: void silk_decode_core(silk_decoder_state *psDec, silk_decoder_control *psDecCtrl,
// opus_int16 xq[], const opus_int16 pulses[ MAX_FRAME_LENGTH ], int arch)
func (psDec *DecoderState) DecodeCore(psDecCtrl *DecoderControl, xq []int16, pulses []int16, arch int) {
	var i, k, start_idx, sLTP_buf_idx, NLSF_interpolation_flag, signalType int
	lag := 0
	var A_Q12, B_Q14, pxq []int16
	var A_Q12_tmp [MAX_LPC_ORDER]int16
	var sLTP []int16
	var sLTP_Q15 []int32
	var LTP_pred_Q13, LPC_pred_Q10, Gain_Q10, inv_gain_Q31, gain_adj_Q16, rand_seed, offset_Q10 int32
	/* pred_lag_ptr is an index into sLTP_Q15 because the C code reads before the pointer */
	var pred_lag_ptr int
	var pexc_Q14, pres_Q14 []int32
	var res_Q14 []int32
	var sLPC_Q14 []int32

	silk_assert(psDec.prev_gain_Q16 != 0)

	sLTP = make([]int16, psDec.ltp_mem_length)
	sLTP_Q15 = make([]int32, psDec.ltp_mem_length+psDec.frame_length)
	res_Q14 = make([]int32, psDec.subfr_length)
	sLPC_Q14 = make([]int32, psDec.subfr_length+MAX_LPC_ORDER)

	offset_Q10 = int32(Quantization_Offsets_Q10[psDec.indices.signalType>>1][psDec.indices.quantOffsetType])

	if psDec.indices.NLSFInterpCoef_Q2 < 1<<2 {
		NLSF_interpolation_flag = 1
	} else {
		NLSF_interpolation_flag = 0
	}

	/* Decode excitation */
	rand_seed = int32(psDec.indices.Seed)
	for i = 0; i < psDec.frame_length; i++ {
		rand_seed = RAND(rand_seed)
		psDec.exc_Q14[i] = LSHIFT(int32(pulses[i]), 14)
		if psDec.exc_Q14[i] > 0 {
			psDec.exc_Q14[i] -= QUANT_LEVEL_ADJUST_Q10 << 4
		} else if psDec.exc_Q14[i] < 0 {
			psDec.exc_Q14[i] += QUANT_LEVEL_ADJUST_Q10 << 4
		}
		psDec.exc_Q14[i] += offset_Q10 << 4
		if rand_seed < 0 {
			psDec.exc_Q14[i] = -psDec.exc_Q14[i]
		}

		rand_seed = ADD32_ovflw(rand_seed, pulses[i])
	}

	/* Copy LPC state */
	copy(sLPC_Q14[:MAX_LPC_ORDER], psDec.sLPC_Q14_buf[:])

	pexc_Q14 = psDec.exc_Q14[:]
	pxq = xq
	sLTP_buf_idx = psDec.ltp_mem_length
	/* Loop over subframes */
	for k = 0; k < psDec.nb_subfr; k++ {
		pres_Q14 = res_Q14
		A_Q12 = psDecCtrl.PredCoef_Q12[k>>1][:]

		/* Preload LPC coeficients to array on stack. Gives small performance gain */
		copy(A_Q12_tmp[:psDec.LPC_order], A_Q12[:psDec.LPC_order])
		B_Q14 = psDecCtrl.LTPCoef_Q14[k*LTP_ORDER:]
		signalType = int(psDec.indices.signalType)

		Gain_Q10 = RSHIFT(psDecCtrl.Gains_Q16[k], 6)
		inv_gain_Q31 = INVERSE32_varQ(psDecCtrl.Gains_Q16[k], 47)

		/* Calculate gain adjustment factor */
		if psDecCtrl.Gains_Q16[k] != psDec.prev_gain_Q16 {
			gain_adj_Q16 = DIV32_varQ(psDec.prev_gain_Q16, psDecCtrl.Gains_Q16[k], 16)

			/* Scale short term state */
			for i = 0; i < MAX_LPC_ORDER; i++ {
				sLPC_Q14[i] = SMULWW(gain_adj_Q16, sLPC_Q14[i])
			}
		} else {
			gain_adj_Q16 = int32(1) << 16
		}

		/* Save inv_gain */
		silk_assert(inv_gain_Q31 != 0)
		psDec.prev_gain_Q16 = psDecCtrl.Gains_Q16[k]

		/* Avoid abrupt transition from voiced PLC to unvoiced normal decoding */
		if psDec.lossCnt != 0 && psDec.prevSignalType == TYPE_VOICED &&
			int(psDec.indices.signalType) != TYPE_VOICED && k < MAX_NB_SUBFR/2 {

			clear(B_Q14[:LTP_ORDER])
			B_Q14[LTP_ORDER/2] = int16(SILK_FIX_CONST(0.25, 14))

			signalType = TYPE_VOICED
			psDecCtrl.pitchL[k] = psDec.lagPrev
		}

		if signalType == TYPE_VOICED {
			/* Voiced */
			lag = psDecCtrl.pitchL[k]

			/* Re-whitening */
			if k == 0 || (k == 2 && NLSF_interpolation_flag != 0) {
				/* Rewhiten with new A coefs */
				start_idx = psDec.ltp_mem_length - lag - psDec.LPC_order - LTP_ORDER/2
				silk_assert(start_idx > 0)

				if k == 2 {
					copy(psDec.outBuf[psDec.ltp_mem_length:psDec.ltp_mem_length+2*psDec.subfr_length], xq[:2*psDec.subfr_length])
				}

				LPCAnalysisFilter(sLTP[start_idx:], psDec.outBuf[start_idx+k*psDec.subfr_length:],
					A_Q12, int32(psDec.ltp_mem_length-start_idx), int32(psDec.LPC_order), arch)

				/* After rewhitening the LTP state is unscaled */
				if k == 0 {
					/* Do LTP downscaling to reduce inter-packet dependency */
					inv_gain_Q31 = LSHIFT(SMULWB(inv_gain_Q31, psDecCtrl.LTP_scale_Q14), 2)
				}
				for i = 0; i < lag+LTP_ORDER/2; i++ {
					sLTP_Q15[sLTP_buf_idx-i-1] = SMULWB(inv_gain_Q31, sLTP[psDec.ltp_mem_length-i-1])
				}
			} else {
				/* Update LTP state when Gain changes */
				if gain_adj_Q16 != int32(1)<<16 {
					for i = 0; i < lag+LTP_ORDER/2; i++ {
						sLTP_Q15[sLTP_buf_idx-i-1] = SMULWW(gain_adj_Q16, sLTP_Q15[sLTP_buf_idx-i-1])
					}
				}
			}
		}

		/* Long-term prediction */
		if signalType == TYPE_VOICED {
			/* Set up pointer */
			pred_lag_ptr = sLTP_buf_idx - lag + LTP_ORDER/2
			for i = 0; i < psDec.subfr_length; i++ {
				/* Unrolled loop */
				/* Avoids introducing a bias because silk_SMLAWB() always rounds to -inf */
				LTP_pred_Q13 = 2
				LTP_pred_Q13 = SMLAWB(LTP_pred_Q13, sLTP_Q15[pred_lag_ptr+0], B_Q14[0])
				LTP_pred_Q13 = SMLAWB(LTP_pred_Q13, sLTP_Q15[pred_lag_ptr-1], B_Q14[1])
				LTP_pred_Q13 = SMLAWB(LTP_pred_Q13, sLTP_Q15[pred_lag_ptr-2], B_Q14[2])
				LTP_pred_Q13 = SMLAWB(LTP_pred_Q13, sLTP_Q15[pred_lag_ptr-3], B_Q14[3])
				LTP_pred_Q13 = SMLAWB(LTP_pred_Q13, sLTP_Q15[pred_lag_ptr-4], B_Q14[4])
				pred_lag_ptr++

				/* Generate LPC excitation */
				pres_Q14[i] = ADD_LSHIFT32(pexc_Q14[i], LTP_pred_Q13, 1)

				/* Update states */
				sLTP_Q15[sLTP_buf_idx] = LSHIFT(pres_Q14[i], 1)
				sLTP_buf_idx++
			}
		} else {
			pres_Q14 = pexc_Q14
		}

		for i = 0; i < psDec.subfr_length; i++ {
			/* Short-term prediction */
			silk_assert(psDec.LPC_order == 10 || psDec.LPC_order == 16)
			/* Avoids introducing a bias because silk_SMLAWB() always rounds to -inf */
			LPC_pred_Q10 = int32(RSHIFT(psDec.LPC_order, 1))
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14[MAX_LPC_ORDER+i-1], A_Q12_tmp[0])
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14[MAX_LPC_ORDER+i-2], A_Q12_tmp[1])
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14[MAX_LPC_ORDER+i-3], A_Q12_tmp[2])
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14[MAX_LPC_ORDER+i-4], A_Q12_tmp[3])
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14[MAX_LPC_ORDER+i-5], A_Q12_tmp[4])
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14[MAX_LPC_ORDER+i-6], A_Q12_tmp[5])
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14[MAX_LPC_ORDER+i-7], A_Q12_tmp[6])
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14[MAX_LPC_ORDER+i-8], A_Q12_tmp[7])
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14[MAX_LPC_ORDER+i-9], A_Q12_tmp[8])
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14[MAX_LPC_ORDER+i-10], A_Q12_tmp[9])
			if psDec.LPC_order == 16 {
				LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14[MAX_LPC_ORDER+i-11], A_Q12_tmp[10])
				LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14[MAX_LPC_ORDER+i-12], A_Q12_tmp[11])
				LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14[MAX_LPC_ORDER+i-13], A_Q12_tmp[12])
				LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14[MAX_LPC_ORDER+i-14], A_Q12_tmp[13])
				LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14[MAX_LPC_ORDER+i-15], A_Q12_tmp[14])
				LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, sLPC_Q14[MAX_LPC_ORDER+i-16], A_Q12_tmp[15])
			}

			/* Add prediction to LPC excitation */
			sLPC_Q14[MAX_LPC_ORDER+i] = ADD_SAT32(pres_Q14[i], LSHIFT_SAT32(LPC_pred_Q10, 4))

			/* Scale with gain */
			pxq[i] = int16(SAT16(RSHIFT_ROUND(SMULWW(sLPC_Q14[MAX_LPC_ORDER+i], Gain_Q10), 8)))
		}

		/* Update LPC filter state */
		copy(sLPC_Q14[:MAX_LPC_ORDER], sLPC_Q14[psDec.subfr_length:psDec.subfr_length+MAX_LPC_ORDER])
		pexc_Q14 = pexc_Q14[psDec.subfr_length:]
		pxq = pxq[psDec.subfr_length:]
	}

	/* Save LPC state */
	copy(psDec.sLPC_Q14_buf[:], sLPC_Q14[:MAX_LPC_ORDER])
}
