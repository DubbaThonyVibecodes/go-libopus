package silk

// C: silk/fixed/process_gains_FIX.c

// Processing of gains
//
// C: void silk_process_gains_FIX(silk_encoder_state_FIX *psEnc, silk_encoder_control_FIX *psEncCtrl, opus_int condCoding)
func (psEnc *EncoderStateFIX) ProcessGainsFIX(psEncCtrl *EncoderControlFIX, condCoding int) {
	psShapeSt := &psEnc.sShape
	var k int
	var s_Q16, InvMaxSqrVal_Q16, gain, gain_squared, ResNrg, ResNrgPart, quant_offset_Q10 int32

	/* Gain reduction when LTP coding gain is high */
	if psEnc.sCmn.indices.signalType == TYPE_VOICED {
		/*s = -0.5f * silk_sigmoid( 0.25f * ( psEncCtrl->LTPredCodGain - 12.0f ) ); */
		s_Q16 = -int32(SigmQ15(RSHIFT_ROUND(psEncCtrl.LTPredCodGain_Q7-int(SILK_FIX_CONST(12.0, 7)), 4)))
		for k = 0; k < psEnc.sCmn.nb_subfr; k++ {
			psEncCtrl.Gains_Q16[k] = SMLAWB(psEncCtrl.Gains_Q16[k], psEncCtrl.Gains_Q16[k], s_Q16)
		}
	}

	/* Limit the quantized signal */
	/* InvMaxSqrVal = pow( 2.0f, 0.33f * ( 21.0f - SNR_dB ) ) / subfr_length; */
	InvMaxSqrVal_Q16 = DIV32_16(Log2lin(
		SMULWB(SILK_FIX_CONST(21+16/0.33, 7)-int32(psEnc.sCmn.SNR_dB_Q7), SILK_FIX_CONST(0.33, 16))), psEnc.sCmn.subfr_length)

	for k = 0; k < psEnc.sCmn.nb_subfr; k++ {
		/* Soft limit on ratio residual energy and squared gains */
		ResNrg = psEncCtrl.ResNrg[k]
		ResNrgPart = SMULWW(ResNrg, InvMaxSqrVal_Q16)
		if psEncCtrl.ResNrgQ[k] > 0 {
			ResNrgPart = RSHIFT_ROUND(ResNrgPart, psEncCtrl.ResNrgQ[k])
		} else {
			if ResNrgPart >= RSHIFT(int32_MAX, -psEncCtrl.ResNrgQ[k]) {
				ResNrgPart = int32_MAX
			} else {
				ResNrgPart = LSHIFT(ResNrgPart, -psEncCtrl.ResNrgQ[k])
			}
		}
		gain = psEncCtrl.Gains_Q16[k]
		gain_squared = ADD_SAT32(ResNrgPart, SMMUL(gain, gain))
		if gain_squared < int32(int16_MAX) {
			/* recalculate with higher precision */
			gain_squared = SMLAWW(LSHIFT(ResNrgPart, 16), gain, gain)
			silk_assert(gain_squared > 0)
			gain = SQRT_APPROX(gain_squared) /* Q8   */
			gain = min(gain, int32_MAX>>8)
			psEncCtrl.Gains_Q16[k] = LSHIFT_SAT32(gain, 8) /* Q16  */
		} else {
			gain = SQRT_APPROX(gain_squared) /* Q0   */
			gain = min(gain, int32_MAX>>16)
			psEncCtrl.Gains_Q16[k] = LSHIFT_SAT32(gain, 16) /* Q16  */
		}
	}

	/* Save unquantized gains and gain Index */
	copy(psEncCtrl.GainsUnq_Q16[:psEnc.sCmn.nb_subfr], psEncCtrl.Gains_Q16[:psEnc.sCmn.nb_subfr])
	psEncCtrl.lastGainIndexPrev = psShapeSt.LastGainIndex

	/* Quantize gains */
	GainsQuant(psEnc.sCmn.indices.GainsIndices[:], psEncCtrl.Gains_Q16[:],
		&psShapeSt.LastGainIndex, b2i(condCoding == CODE_CONDITIONALLY), psEnc.sCmn.nb_subfr)

	/* Set quantizer offset for voiced signals. Larger offset when LTP coding gain is low or tilt is high (ie low-pass) */
	if psEnc.sCmn.indices.signalType == TYPE_VOICED {
		if psEncCtrl.LTPredCodGain_Q7+RSHIFT(psEnc.sCmn.input_tilt_Q15, 8) > int(SILK_FIX_CONST(1.0, 7)) {
			psEnc.sCmn.indices.quantOffsetType = 0
		} else {
			psEnc.sCmn.indices.quantOffsetType = 1
		}
	}

	/* Quantizer boundary adjustment */
	quant_offset_Q10 = int32(Quantization_Offsets_Q10[psEnc.sCmn.indices.signalType>>1][psEnc.sCmn.indices.quantOffsetType])
	psEncCtrl.Lambda_Q10 = int(SILK_FIX_CONST(LAMBDA_OFFSET, 10) +
		SMULBB(SILK_FIX_CONST(LAMBDA_DELAYED_DECISIONS, 10), psEnc.sCmn.nStatesDelayedDecision) +
		SMULWB(SILK_FIX_CONST(LAMBDA_SPEECH_ACT, 18), psEnc.sCmn.speech_activity_Q8) +
		SMULWB(SILK_FIX_CONST(LAMBDA_INPUT_QUALITY, 12), psEncCtrl.input_quality_Q14) +
		SMULWB(SILK_FIX_CONST(LAMBDA_CODING_QUALITY, 12), psEncCtrl.coding_quality_Q14) +
		SMULWB(SILK_FIX_CONST(LAMBDA_QUANT_OFFSET, 16), quant_offset_Q10))

	silk_assert(psEncCtrl.Lambda_Q10 > 0)
	silk_assert(psEncCtrl.Lambda_Q10 < int(SILK_FIX_CONST(2.0, 10)))
}
