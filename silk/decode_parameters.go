package silk

// C: silk/decode_parameters.c

/* Decode parameters from payload */
//
// C: void silk_decode_parameters(silk_decoder_state *psDec, silk_decoder_control *psDecCtrl, opus_int condCoding)
func (psDec *DecoderState) DecodeParameters(psDecCtrl *DecoderControl, condCoding int) {
	var i, k, Ix int
	var pNLSF_Q15, pNLSF0_Q15 [MAX_LPC_ORDER]int16
	var cbk_ptr_Q7 []int8

	/* Dequant Gains */
	GainsDequant(psDecCtrl.Gains_Q16[:], psDec.indices.GainsIndices[:],
		&psDec.LastGainIndex, b2i(condCoding == CODE_CONDITIONALLY), psDec.nb_subfr)

	/****************/
	/* Decode NLSFs */
	/****************/
	NLSFDecode(pNLSF_Q15[:], psDec.indices.NLSFIndices[:], psDec.psNLSF_CB)

	/* Convert NLSF parameters to AR prediction filter coefficients */
	NLSF2A(psDecCtrl.PredCoef_Q12[1][:], pNLSF_Q15[:], psDec.LPC_order, psDec.arch)

	/* If just reset, e.g., because internal Fs changed, do not allow interpolation */
	/* improves the case of packet loss in the first frame after a switch           */
	if psDec.first_frame_after_reset == 1 {
		psDec.indices.NLSFInterpCoef_Q2 = 4
	}

	if psDec.indices.NLSFInterpCoef_Q2 < 4 {
		/* Calculation of the interpolated NLSF0 vector from the interpolation factor, */
		/* the previous NLSF1, and the current NLSF1                                   */
		for i = 0; i < psDec.LPC_order; i++ {
			pNLSF0_Q15[i] = int16(int32(psDec.prevNLSF_Q15[i]) + RSHIFT(MUL(psDec.indices.NLSFInterpCoef_Q2,
				int32(pNLSF_Q15[i])-int32(psDec.prevNLSF_Q15[i])), 2))
		}

		/* Convert NLSF parameters to AR prediction filter coefficients */
		NLSF2A(psDecCtrl.PredCoef_Q12[0][:], pNLSF0_Q15[:], psDec.LPC_order, psDec.arch)
	} else {
		/* Copy LPC coefficients for first half from second half */
		copy(psDecCtrl.PredCoef_Q12[0][:psDec.LPC_order], psDecCtrl.PredCoef_Q12[1][:psDec.LPC_order])
	}

	copy(psDec.prevNLSF_Q15[:psDec.LPC_order], pNLSF_Q15[:psDec.LPC_order])

	/* After a packet loss do BWE of LPC coefs */
	if psDec.lossCnt != 0 {
		Bwexpander(psDecCtrl.PredCoef_Q12[0][:], psDec.LPC_order, BWE_AFTER_LOSS_Q16)
		Bwexpander(psDecCtrl.PredCoef_Q12[1][:], psDec.LPC_order, BWE_AFTER_LOSS_Q16)
	}

	if psDec.indices.signalType == TYPE_VOICED {
		/*********************/
		/* Decode pitch lags */
		/*********************/

		/* Decode pitch values */
		DecodePitch(psDec.indices.lagIndex, psDec.indices.contourIndex, psDecCtrl.pitchL[:], psDec.fs_kHz, psDec.nb_subfr)

		/* Decode Codebook Index */
		cbk_ptr_Q7 = LTP_vq_ptrs_Q7[psDec.indices.PERIndex] /* set pointer to start of codebook */

		for k = 0; k < psDec.nb_subfr; k++ {
			Ix = int(psDec.indices.LTPIndex[k])
			for i = 0; i < LTP_ORDER; i++ {
				psDecCtrl.LTPCoef_Q14[k*LTP_ORDER+i] = int16(LSHIFT(cbk_ptr_Q7[Ix*LTP_ORDER+i], 7))
			}
		}

		/**********************/
		/* Decode LTP scaling */
		/**********************/
		Ix = int(psDec.indices.LTP_scaleIndex)
		psDecCtrl.LTP_scale_Q14 = int(LTPScales_table_Q14[Ix])
	} else {
		clear(psDecCtrl.pitchL[:psDec.nb_subfr])
		clear(psDecCtrl.LTPCoef_Q14[:LTP_ORDER*psDec.nb_subfr])
		psDec.indices.PERIndex = 0
		psDecCtrl.LTP_scale_Q14 = 0
	}
}
