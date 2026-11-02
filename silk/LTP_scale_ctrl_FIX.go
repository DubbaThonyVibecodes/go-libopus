package silk

// C: silk/fixed/LTP_scale_ctrl_FIX.c

// Calculation of LTP state scaling
//
// C: void silk_LTP_scale_ctrl_FIX(silk_encoder_state_FIX *psEnc, silk_encoder_control_FIX *psEncCtrl, opus_int condCoding)
func (psEnc *EncoderStateFIX) LTPScaleCtrlFIX(psEncCtrl *EncoderControlFIX, condCoding int) {
	var round_loss int

	if condCoding == CODE_INDEPENDENTLY {
		/* Only scale if first frame in packet */
		round_loss = psEnc.sCmn.PacketLoss_perc + psEnc.sCmn.nFramesPerPacket
		psEnc.sCmn.indices.LTP_scaleIndex = int8(LIMIT(
			SMULWB(SMULBB(round_loss, psEncCtrl.LTPredCodGain_Q7), SILK_FIX_CONST(0.1, 9)), 0, 2))
	} else {
		/* Default is minimum scaling */
		psEnc.sCmn.indices.LTP_scaleIndex = 0
	}
	psEncCtrl.LTP_scale_Q14 = int(LTPScales_table_Q14[psEnc.sCmn.indices.LTP_scaleIndex])
}
