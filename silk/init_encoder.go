package silk

// C: silk/init_encoder.c

/*********************************/
/* Initialize Silk Encoder state */
/*********************************/
//
// C: opus_int silk_init_encoder(silk_encoder_state_Fxx *psEnc, int arch)
func (psEnc *EncoderStateFIX) InitEncoder(arch int) int {
	var ret int = 0

	/* Clear the entire encoder state */
	*psEnc = EncoderStateFIX{}

	psEnc.sCmn.arch = arch

	psEnc.sCmn.variable_HP_smth1_Q15 = LSHIFT(Lin2log(SILK_FIX_CONST(float64(VARIABLE_HP_MIN_CUTOFF_HZ), 16))-(16<<7), 8)
	psEnc.sCmn.variable_HP_smth2_Q15 = psEnc.sCmn.variable_HP_smth1_Q15

	/* Used to deactivate LSF interpolation, pitch prediction */
	psEnc.sCmn.first_frame_after_reset = 1

	/* Initialize Silk VAD */
	ret += psEnc.sCmn.sVAD.VADInit()

	return ret
}
