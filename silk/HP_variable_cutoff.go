package silk

// C: silk/HP_variable_cutoff.c

// High-pass filter with cutoff frequency adaptation based on pitch lag statistics
//
// C: void silk_HP_variable_cutoff(
//
//	silk_encoder_state_Fxx          state_Fxx[]                         /* I/O  Encoder states                              */
//
// )
func HPVariableCutoff(state_Fxx []EncoderStateFIX) {
	var quality_Q15 int
	var pitch_freq_Hz_Q16, pitch_freq_log_Q7, delta_freq_Q7 int32
	psEncC1 := &state_Fxx[0].sCmn

	/* Adaptive cutoff frequency: estimate low end of pitch frequency range */
	if psEncC1.prevSignalType == TYPE_VOICED {
		/* difference, in log domain */
		pitch_freq_Hz_Q16 = DIV32_16(LSHIFT(MUL(psEncC1.fs_kHz, 1000), 16), psEncC1.prevLag)
		pitch_freq_log_Q7 = Lin2log(pitch_freq_Hz_Q16) - (16 << 7)

		/* adjustment based on quality */
		quality_Q15 = psEncC1.input_quality_bands_Q15[0]
		pitch_freq_log_Q7 = SMLAWB(pitch_freq_log_Q7, SMULWB(LSHIFT(-quality_Q15, 2), quality_Q15),
			pitch_freq_log_Q7-(Lin2log(SILK_FIX_CONST(float64(VARIABLE_HP_MIN_CUTOFF_HZ), 16))-(16<<7)))

		/* delta_freq = pitch_freq_log - psEnc->variable_HP_smth1; */
		delta_freq_Q7 = pitch_freq_log_Q7 - RSHIFT(psEncC1.variable_HP_smth1_Q15, 8)
		if delta_freq_Q7 < 0 {
			/* less smoothing for decreasing pitch frequency, to track something close to the minimum */
			delta_freq_Q7 = MUL(delta_freq_Q7, 3)
		}

		/* limit delta, to reduce impact of outliers in pitch estimation */
		delta_freq_Q7 = LIMIT_32(delta_freq_Q7, -SILK_FIX_CONST(VARIABLE_HP_MAX_DELTA_FREQ, 7), SILK_FIX_CONST(VARIABLE_HP_MAX_DELTA_FREQ, 7))

		/* update smoother */
		psEncC1.variable_HP_smth1_Q15 = SMLAWB(psEncC1.variable_HP_smth1_Q15,
			SMULBB(psEncC1.speech_activity_Q8, delta_freq_Q7), SILK_FIX_CONST(VARIABLE_HP_SMTH_COEF1, 16))

		/* limit frequency range */
		psEncC1.variable_HP_smth1_Q15 = LIMIT_32(psEncC1.variable_HP_smth1_Q15,
			LSHIFT(Lin2log(VARIABLE_HP_MIN_CUTOFF_HZ), 8),
			LSHIFT(Lin2log(VARIABLE_HP_MAX_CUTOFF_HZ), 8))
	}
}
