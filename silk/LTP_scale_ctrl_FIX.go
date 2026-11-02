package silk

// C: silk/fixed/LTP_scale_ctrl_FIX.c

// Calculation of LTP state scaling
//
// C: void silk_LTP_scale_ctrl_FIX(silk_encoder_state_FIX *psEnc, silk_encoder_control_FIX *psEncCtrl, opus_int condCoding)
func (psEnc *EncoderStateFIX) LTPScaleCtrlFIX(psEncCtrl *EncoderControlFIX, condCoding int) {
	var round_loss int

	if condCoding == CODE_INDEPENDENTLY {
		/* Only scale if first frame in packet */
		round_loss = psEnc.sCmn.PacketLoss_perc * psEnc.sCmn.nFramesPerPacket
		if psEnc.sCmn.LBRR_flag != 0 {
			/* LBRR reduces the effective loss. In practice, it does not square the loss because
			   losses aren't independent, but that still seems to work best. We also never go below 2%. */
			round_loss = 2 + int(SMULBB(round_loss, round_loss)/100)
		}
		psEnc.sCmn.indices.LTP_scaleIndex = int8(b2i(SMULBB(psEncCtrl.LTPredCodGain_Q7, round_loss) > Log2lin(int32(128*7+2900-psEnc.sCmn.SNR_dB_Q7))))
		psEnc.sCmn.indices.LTP_scaleIndex += int8(b2i(SMULBB(psEncCtrl.LTPredCodGain_Q7, round_loss) > Log2lin(int32(128*7+3900-psEnc.sCmn.SNR_dB_Q7))))
	} else {
		/* Default is minimum scaling */
		psEnc.sCmn.indices.LTP_scaleIndex = 0
	}
	psEncCtrl.LTP_scale_Q14 = int(LTPScales_table_Q14[psEnc.sCmn.indices.LTP_scaleIndex])
}
