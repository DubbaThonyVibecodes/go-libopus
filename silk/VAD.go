package silk

// C: silk/VAD.c

/**********************************/
/* Initialization of the Silk VAD */
/**********************************/
//
// C: opus_int silk_VAD_Init(                                         /* O    Return value, 0 if success                  */
//
//	silk_VAD_state              *psSilk_VAD                     /* I/O  Pointer to Silk VAD state                   */
//
// )
func (psSilk_VAD *VADState) VADInit() int {
	var b int
	var ret int = 0

	/* reset state memory */
	*psSilk_VAD = VADState{}

	/* init noise levels */
	/* Initialize array with approx pink noise levels (psd proportional to inverse of frequency) */
	for b = 0; b < VAD_N_BANDS; b++ {
		psSilk_VAD.NoiseLevelBias[b] = max_32(DIV32_16(VAD_NOISE_LEVELS_BIAS, b+1), 1)
	}

	/* Initialize state */
	for b = 0; b < VAD_N_BANDS; b++ {
		psSilk_VAD.NL[b] = MUL(100, psSilk_VAD.NoiseLevelBias[b])
		psSilk_VAD.inv_NL[b] = DIV32(int32_MAX, psSilk_VAD.NL[b])
	}
	psSilk_VAD.counter = 15

	/* init smoothed energy-to-noise ratio*/
	for b = 0; b < VAD_N_BANDS; b++ {
		psSilk_VAD.NrgRatioSmth_Q8[b] = 100 * 256 /* 100 * 256 --> 20 dB SNR */
	}

	return ret
}

// Weighting factors for tilt measure
var tiltWeights = [VAD_N_BANDS]int32{30000, 6000, -12000, -12000}

/***************************************/
/* Get the speech activity level in Q8 */
/***************************************/
//
// C: opus_int silk_VAD_GetSA_Q8(                                     /* O    Return value, 0 if success                  */
//
//	silk_encoder_state          *psEncC,                        /* I/O  Encoder state                               */
//	const opus_int16            pIn[]                           /* I    PCM input                                   */
//
// )
func (psEncC *EncoderState) VADGetSAQ8(pIn []int16) int {
	var SA_Q15, pSNR_dB_Q7, input_tilt int
	var decimated_framelength, dec_subframe_length, dec_subframe_offset, SNR_Q7, i, b, s int
	var sumSquared, smooth_coef_Q16 int32
	var HPstateTmp int16
	var X [VAD_N_BANDS][MAX_FRAME_LENGTH / 2]int16
	var Xnrg [VAD_N_BANDS]int32
	var NrgToNoiseRatio_Q8 [VAD_N_BANDS]int32
	var speech_nrg, x_tmp int32
	var ret int = 0
	psSilk_VAD := &psEncC.sVAD

	/* Safety checks */
	silk_assert(VAD_N_BANDS == 4)
	silk_assert(MAX_FRAME_LENGTH >= psEncC.frame_length)
	silk_assert(psEncC.frame_length <= 512)
	silk_assert(psEncC.frame_length == 8*RSHIFT(psEncC.frame_length, 3))

	/***********************/
	/* Filter and Decimate */
	/***********************/
	/* 0-8 kHz to 0-4 kHz and 4-8 kHz */
	AnaFiltBank1(pIn, psSilk_VAD.AnaState[0:], X[0][0:], X[3][0:], int32(psEncC.frame_length))

	/* 0-4 kHz to 0-2 kHz and 2-4 kHz */
	AnaFiltBank1(X[0][0:], psSilk_VAD.AnaState1[0:], X[0][0:], X[2][0:], int32(RSHIFT(psEncC.frame_length, 1)))

	/* 0-2 kHz to 0-1 kHz and 1-2 kHz */
	AnaFiltBank1(X[0][0:], psSilk_VAD.AnaState2[0:], X[0][0:], X[1][0:], int32(RSHIFT(psEncC.frame_length, 2)))

	/*********************************************/
	/* HP filter on lowest band (differentiator) */
	/*********************************************/
	decimated_framelength = RSHIFT(psEncC.frame_length, 3)
	X[0][decimated_framelength-1] = RSHIFT(X[0][decimated_framelength-1], 1)
	HPstateTmp = X[0][decimated_framelength-1]
	for i = decimated_framelength - 1; i > 0; i-- {
		X[0][i-1] = RSHIFT(X[0][i-1], 1)
		X[0][i] -= X[0][i-1]
	}
	X[0][0] -= psSilk_VAD.HPstate
	psSilk_VAD.HPstate = HPstateTmp

	/*************************************/
	/* Calculate the energy in each band */
	/*************************************/
	for b = 0; b < VAD_N_BANDS; b++ {
		/* Find the decimated framelength in the non-uniformly divided bands */
		decimated_framelength = RSHIFT(psEncC.frame_length, min_int(VAD_N_BANDS-b, VAD_N_BANDS-1))

		/* Split length into subframe lengths */
		dec_subframe_length = RSHIFT(decimated_framelength, VAD_INTERNAL_SUBFRAMES_LOG2)
		dec_subframe_offset = 0

		/* Compute energy per sub-frame */
		/* initialize with summed energy of last subframe */
		Xnrg[b] = psSilk_VAD.XnrgSubfr[b]
		for s = 0; s < VAD_INTERNAL_SUBFRAMES; s++ {
			sumSquared = 0
			for i = 0; i < dec_subframe_length; i++ {
				/* The energy will be less than dec_subframe_length * ( silk_int16_MIN / 8 ) ^ 2.            */
				/* Therefore we can accumulate with no risk of overflow (unless dec_subframe_length > 128)  */
				x_tmp = int32(RSHIFT(X[b][i+dec_subframe_offset], 3))
				sumSquared = SMLABB(sumSquared, x_tmp, x_tmp)

				/* Safety check */
				silk_assert(sumSquared >= 0)
			}

			/* Add/saturate summed energy of current subframe */
			if s < VAD_INTERNAL_SUBFRAMES-1 {
				Xnrg[b] = ADD_POS_SAT32(Xnrg[b], sumSquared)
			} else {
				/* Look-ahead subframe */
				Xnrg[b] = ADD_POS_SAT32(Xnrg[b], RSHIFT(sumSquared, 1))
			}

			dec_subframe_offset += dec_subframe_length
		}
		psSilk_VAD.XnrgSubfr[b] = sumSquared
	}

	/********************/
	/* Noise estimation */
	/********************/
	vadGetNoiseLevels(Xnrg[0:], psSilk_VAD)

	/***********************************************/
	/* Signal-plus-noise to noise ratio estimation */
	/***********************************************/
	sumSquared = 0
	input_tilt = 0
	for b = 0; b < VAD_N_BANDS; b++ {
		speech_nrg = Xnrg[b] - psSilk_VAD.NL[b]
		if speech_nrg > 0 {
			/* Divide, with sufficient resolution */
			if (uint32(Xnrg[b]) & 0xFF800000) == 0 { /* C: ( Xnrg[ b ] & 0xFF800000 ) == 0 */
				NrgToNoiseRatio_Q8[b] = DIV32(LSHIFT(Xnrg[b], 8), psSilk_VAD.NL[b]+1)
			} else {
				NrgToNoiseRatio_Q8[b] = DIV32(Xnrg[b], RSHIFT(psSilk_VAD.NL[b], 8)+1)
			}

			/* Convert to log domain */
			SNR_Q7 = int(Lin2log(NrgToNoiseRatio_Q8[b]) - 8*128)

			/* Sum-of-squares */
			sumSquared = SMLABB(sumSquared, SNR_Q7, SNR_Q7) /* Q14 */

			/* Tilt measure */
			if speech_nrg < (int32(1) << 20) {
				/* Scale down SNR value for small subband speech energies */
				SNR_Q7 = int(SMULWB(LSHIFT(SQRT_APPROX(speech_nrg), 6), SNR_Q7))
			}
			input_tilt = int(SMLAWB(input_tilt, tiltWeights[b], SNR_Q7))
		} else {
			NrgToNoiseRatio_Q8[b] = 256
		}
	}

	/* Mean-of-squares */
	sumSquared = DIV32_16(sumSquared, VAD_N_BANDS) /* Q14 */

	/* Root-mean-square approximation, scale to dBs, and write to output pointer */
	pSNR_dB_Q7 = int(int16(3 * SQRT_APPROX(sumSquared))) /* Q7 */

	/*********************************/
	/* Speech Probability Estimation */
	/*********************************/
	SA_Q15 = SigmQ15(int(SMULWB(VAD_SNR_FACTOR_Q16, pSNR_dB_Q7) - VAD_NEGATIVE_OFFSET_Q5))

	/**************************/
	/* Frequency Tilt Measure */
	/**************************/
	psEncC.input_tilt_Q15 = int(LSHIFT(SigmQ15(input_tilt)-16384, 1))

	/**************************************************/
	/* Scale the sigmoid output based on power levels */
	/**************************************************/
	speech_nrg = 0
	for b = 0; b < VAD_N_BANDS; b++ {
		/* Accumulate signal-without-noise energies, higher frequency bands have more weight */
		speech_nrg += int32(b+1) * RSHIFT(Xnrg[b]-psSilk_VAD.NL[b], 4)
	}

	/* Power scaling */
	if speech_nrg <= 0 {
		SA_Q15 = RSHIFT(SA_Q15, 1)
	} else if speech_nrg < 32768 {
		if psEncC.frame_length == 10*psEncC.fs_kHz {
			speech_nrg = LSHIFT_SAT32(speech_nrg, 16)
		} else {
			speech_nrg = LSHIFT_SAT32(speech_nrg, 15)
		}

		/* square-root */
		speech_nrg = SQRT_APPROX(speech_nrg)
		SA_Q15 = int(SMULWB(32768+speech_nrg, SA_Q15))
	}

	/* Copy the resulting speech activity in Q8 */
	psEncC.speech_activity_Q8 = min_int(RSHIFT(SA_Q15, 7), int(uint8_MAX))

	/***********************************/
	/* Energy Level and SNR estimation */
	/***********************************/
	/* Smoothing coefficient */
	smooth_coef_Q16 = SMULWB(VAD_SNR_SMOOTH_COEF_Q18, SMULWB(int32(SA_Q15), SA_Q15))

	if psEncC.frame_length == 10*psEncC.fs_kHz {
		smooth_coef_Q16 >>= 1
	}

	for b = 0; b < VAD_N_BANDS; b++ {
		/* compute smoothed energy-to-noise ratio per band */
		psSilk_VAD.NrgRatioSmth_Q8[b] = SMLAWB(psSilk_VAD.NrgRatioSmth_Q8[b],
			NrgToNoiseRatio_Q8[b]-psSilk_VAD.NrgRatioSmth_Q8[b], smooth_coef_Q16)

		/* signal to noise ratio in dB per band */
		SNR_Q7 = int(3 * (Lin2log(psSilk_VAD.NrgRatioSmth_Q8[b]) - 8*128))
		/* quality = sigmoid( 0.25 * ( SNR_dB - 16 ) ); */
		psEncC.input_quality_bands_Q15[b] = SigmQ15(RSHIFT(SNR_Q7-16*128, 4))
	}

	return ret
}

/**************************/
/* Noise level estimation */
/**************************/
//
// C: static inline void silk_VAD_GetNoiseLevels(
//
//	const opus_int32            pX[ VAD_N_BANDS ],  /* I    subband energies                            */
//	silk_VAD_state              *psSilk_VAD         /* I/O  Pointer to Silk VAD state                   */
//
// )
func vadGetNoiseLevels(pX []int32, psSilk_VAD *VADState) {
	var k int
	var nl, nrg, inv_nrg int32
	var coef, min_coef int

	/* Initially faster smoothing */
	if psSilk_VAD.counter < 1000 { /* 1000 = 20 sec */
		min_coef = int(DIV32_16(int16_MAX, RSHIFT(psSilk_VAD.counter, 4)+1))
	} else {
		min_coef = 0
	}

	for k = 0; k < VAD_N_BANDS; k++ {
		/* Get old noise level estimate for current band */
		nl = psSilk_VAD.NL[k]
		silk_assert(nl >= 0)

		/* Add bias */
		nrg = ADD_POS_SAT32(pX[k], psSilk_VAD.NoiseLevelBias[k])
		silk_assert(nrg > 0)

		/* Invert energies */
		inv_nrg = DIV32(int32_MAX, nrg)
		silk_assert(inv_nrg >= 0)

		/* Less update when subband energy is high */
		if nrg > LSHIFT(nl, 3) {
			coef = VAD_NOISE_LEVEL_SMOOTH_COEF_Q16 >> 3
		} else if nrg < nl {
			coef = VAD_NOISE_LEVEL_SMOOTH_COEF_Q16
		} else {
			coef = int(SMULWB(SMULWW(inv_nrg, nl), VAD_NOISE_LEVEL_SMOOTH_COEF_Q16<<1))
		}

		/* Initially faster smoothing */
		coef = max_int(coef, min_coef)

		/* Smooth inverse energies */
		psSilk_VAD.inv_NL[k] = SMLAWB(psSilk_VAD.inv_NL[k], inv_nrg-psSilk_VAD.inv_NL[k], coef)
		silk_assert(psSilk_VAD.inv_NL[k] >= 0)

		/* Compute noise level by inverting again */
		nl = DIV32(int32_MAX, psSilk_VAD.inv_NL[k])
		silk_assert(nl >= 0)

		/* Limit noise levels (guarantee 7 bits of head room) */
		nl = min(nl, 0x00FFFFFF)

		/* Store as part of state */
		psSilk_VAD.NL[k] = nl
	}

	/* Increment frame counter */
	psSilk_VAD.counter++
}
