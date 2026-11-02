package silk

// C: silk/CNG.c

/* Generates excitation for CNG LPC synthesis */
//
// C: static OPUS_INLINE void silk_CNG_exc(opus_int32 exc_Q14[], opus_int32 exc_buf_Q14[],
// opus_int length, opus_int32 *rand_seed)
func cngExc(
	exc_Q14 []int32, /* O    CNG excitation signal Q10                   */
	exc_buf_Q14 []int32, /* I    Random samples buffer Q10                   */
	length int, /* I    Length                                      */
	rand_seed *int32, /* I/O  Seed to random index generator              */
) {
	var seed int32
	var i, idx, exc_mask int

	exc_mask = CNG_BUF_MASK_MAX
	for exc_mask > length {
		exc_mask = RSHIFT(exc_mask, 1)
	}

	seed = *rand_seed
	for i = 0; i < length; i++ {
		seed = RAND(seed)
		idx = int(RSHIFT(seed, 24)) & exc_mask
		silk_assert(idx >= 0)
		silk_assert(idx <= CNG_BUF_MASK_MAX)
		exc_Q14[i] = exc_buf_Q14[idx]
	}
	*rand_seed = seed
}

// C: void silk_CNG_Reset(silk_decoder_state *psDec)
func (psDec *DecoderState) CNGReset() {
	var i, NLSF_step_Q15, NLSF_acc_Q15 int

	NLSF_step_Q15 = int(DIV32_16(int16_MAX, psDec.LPC_order+1))
	NLSF_acc_Q15 = 0
	for i = 0; i < psDec.LPC_order; i++ {
		NLSF_acc_Q15 += NLSF_step_Q15
		psDec.sCNG.CNG_smth_NLSF_Q15[i] = int16(NLSF_acc_Q15)
	}
	psDec.sCNG.CNG_smth_Gain_Q16 = 0
	psDec.sCNG.rand_seed = 3176576
}

/* Updates CNG estimate, and applies the CNG when packet was lost   */
//
// C: void silk_CNG(silk_decoder_state *psDec, silk_decoder_control *psDecCtrl, opus_int16 frame[], opus_int length)
func (psDec *DecoderState) CNG(psDecCtrl *DecoderControl, frame []int16, length int) {
	var i, subfr int
	var LPC_pred_Q10, max_Gain_Q16, gain_Q16, gain_Q10 int32
	var A_Q12 [MAX_LPC_ORDER]int16
	psCNG := &psDec.sCNG

	if psDec.fs_kHz != psCNG.fs_kHz {
		/* Reset state */
		psDec.CNGReset()

		psCNG.fs_kHz = psDec.fs_kHz
	}
	if psDec.lossCnt == 0 && psDec.prevSignalType == TYPE_NO_VOICE_ACTIVITY {
		/* Update CNG parameters */

		/* Smoothing of LSF's  */
		for i = 0; i < psDec.LPC_order; i++ {
			psCNG.CNG_smth_NLSF_Q15[i] += int16(SMULWB(int32(psDec.prevNLSF_Q15[i])-int32(psCNG.CNG_smth_NLSF_Q15[i]), CNG_NLSF_SMTH_Q16))
		}
		/* Find the subframe with the highest gain */
		max_Gain_Q16 = 0
		subfr = 0
		for i = 0; i < psDec.nb_subfr; i++ {
			if psDecCtrl.Gains_Q16[i] > max_Gain_Q16 {
				max_Gain_Q16 = psDecCtrl.Gains_Q16[i]
				subfr = i
			}
		}
		/* Update CNG excitation buffer with excitation from this subframe */
		copy(psCNG.CNG_exc_buf_Q14[psDec.subfr_length:][:(psDec.nb_subfr-1)*psDec.subfr_length], psCNG.CNG_exc_buf_Q14[:(psDec.nb_subfr-1)*psDec.subfr_length])
		copy(psCNG.CNG_exc_buf_Q14[:psDec.subfr_length], psDec.exc_Q14[subfr*psDec.subfr_length:][:psDec.subfr_length])

		/* Smooth gains */
		for i = 0; i < psDec.nb_subfr; i++ {
			psCNG.CNG_smth_Gain_Q16 += SMULWB(psDecCtrl.Gains_Q16[i]-psCNG.CNG_smth_Gain_Q16, CNG_GAIN_SMTH_Q16)
		}
	}

	/* Add CNG when packet is lost or during DTX */
	if psDec.lossCnt != 0 {
		var CNG_sig_Q14 []int32
		CNG_sig_Q14 = make([]int32, length+MAX_LPC_ORDER)

		/* Generate CNG excitation */
		gain_Q16 = SMULWW(psDec.sPLC.randScale_Q14, psDec.sPLC.prevGain_Q16[1])
		if gain_Q16 >= (1<<21) || psCNG.CNG_smth_Gain_Q16 > (1<<23) {
			gain_Q16 = SMULTT(gain_Q16, gain_Q16)
			gain_Q16 = SUB_LSHIFT32(SMULTT(psCNG.CNG_smth_Gain_Q16, psCNG.CNG_smth_Gain_Q16), gain_Q16, 5)
			gain_Q16 = LSHIFT32(SQRT_APPROX(gain_Q16), 16)
		} else {
			gain_Q16 = SMULWW(gain_Q16, gain_Q16)
			gain_Q16 = SUB_LSHIFT32(SMULWW(psCNG.CNG_smth_Gain_Q16, psCNG.CNG_smth_Gain_Q16), gain_Q16, 5)
			gain_Q16 = LSHIFT32(SQRT_APPROX(gain_Q16), 8)
		}
		gain_Q10 = RSHIFT(gain_Q16, 6)

		cngExc(CNG_sig_Q14[MAX_LPC_ORDER:], psCNG.CNG_exc_buf_Q14[:], length, &psCNG.rand_seed)

		/* Convert CNG NLSF to filter representation */
		NLSF2A(A_Q12[:], psCNG.CNG_smth_NLSF_Q15[:], psDec.LPC_order, psDec.arch)

		/* Generate CNG signal, by synthesis filtering */
		copy(CNG_sig_Q14[:MAX_LPC_ORDER], psCNG.CNG_synth_state[:])
		for i = 0; i < length; i++ {
			silk_assert(psDec.LPC_order == 10 || psDec.LPC_order == 16)
			/* Avoids introducing a bias because silk_SMLAWB() always rounds to -inf */
			LPC_pred_Q10 = int32(RSHIFT(psDec.LPC_order, 1))
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, CNG_sig_Q14[MAX_LPC_ORDER+i-1], A_Q12[0])
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, CNG_sig_Q14[MAX_LPC_ORDER+i-2], A_Q12[1])
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, CNG_sig_Q14[MAX_LPC_ORDER+i-3], A_Q12[2])
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, CNG_sig_Q14[MAX_LPC_ORDER+i-4], A_Q12[3])
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, CNG_sig_Q14[MAX_LPC_ORDER+i-5], A_Q12[4])
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, CNG_sig_Q14[MAX_LPC_ORDER+i-6], A_Q12[5])
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, CNG_sig_Q14[MAX_LPC_ORDER+i-7], A_Q12[6])
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, CNG_sig_Q14[MAX_LPC_ORDER+i-8], A_Q12[7])
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, CNG_sig_Q14[MAX_LPC_ORDER+i-9], A_Q12[8])
			LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, CNG_sig_Q14[MAX_LPC_ORDER+i-10], A_Q12[9])
			if psDec.LPC_order == 16 {
				LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, CNG_sig_Q14[MAX_LPC_ORDER+i-11], A_Q12[10])
				LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, CNG_sig_Q14[MAX_LPC_ORDER+i-12], A_Q12[11])
				LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, CNG_sig_Q14[MAX_LPC_ORDER+i-13], A_Q12[12])
				LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, CNG_sig_Q14[MAX_LPC_ORDER+i-14], A_Q12[13])
				LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, CNG_sig_Q14[MAX_LPC_ORDER+i-15], A_Q12[14])
				LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, CNG_sig_Q14[MAX_LPC_ORDER+i-16], A_Q12[15])
			}

			/* Update states */
			CNG_sig_Q14[MAX_LPC_ORDER+i] = ADD_SAT32(CNG_sig_Q14[MAX_LPC_ORDER+i], LSHIFT_SAT32(LPC_pred_Q10, 4))

			/* Scale with Gain and add to input signal */
			frame[i] = int16(ADD_SAT16(frame[i], SAT16(RSHIFT_ROUND(SMULWW(CNG_sig_Q14[MAX_LPC_ORDER+i], gain_Q10), 8))))

		}
		copy(psCNG.CNG_synth_state[:], CNG_sig_Q14[length:length+MAX_LPC_ORDER])
	} else {
		clear(psCNG.CNG_synth_state[:psDec.LPC_order])
	}
}
