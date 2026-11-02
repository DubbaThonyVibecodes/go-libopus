package silk

// C: silk/check_control_input.c

// Check encoder control struct
//
// C: opus_int check_control_input(silk_EncControlStruct *encControl)
func CheckControlInput(encControl *EncControlStruct) int {
	silk_assert(encControl != nil)

	if ((encControl.API_sampleRate != 8000) &&
		(encControl.API_sampleRate != 12000) &&
		(encControl.API_sampleRate != 16000) &&
		(encControl.API_sampleRate != 24000) &&
		(encControl.API_sampleRate != 32000) &&
		(encControl.API_sampleRate != 44100) &&
		(encControl.API_sampleRate != 48000)) ||
		((encControl.DesiredInternalSampleRate != 8000) &&
			(encControl.DesiredInternalSampleRate != 12000) &&
			(encControl.DesiredInternalSampleRate != 16000)) ||
		((encControl.MaxInternalSampleRate != 8000) &&
			(encControl.MaxInternalSampleRate != 12000) &&
			(encControl.MaxInternalSampleRate != 16000)) ||
		((encControl.MinInternalSampleRate != 8000) &&
			(encControl.MinInternalSampleRate != 12000) &&
			(encControl.MinInternalSampleRate != 16000)) ||
		(encControl.MinInternalSampleRate > encControl.DesiredInternalSampleRate) ||
		(encControl.MaxInternalSampleRate < encControl.DesiredInternalSampleRate) ||
		(encControl.MinInternalSampleRate > encControl.MaxInternalSampleRate) {
		silk_assert(false)
		return SILK_ENC_FS_NOT_SUPPORTED
	}
	if encControl.PayloadSize_ms != 10 &&
		encControl.PayloadSize_ms != 20 &&
		encControl.PayloadSize_ms != 40 &&
		encControl.PayloadSize_ms != 60 {
		silk_assert(false)
		return SILK_ENC_PACKET_SIZE_NOT_SUPPORTED
	}
	if encControl.PacketLossPercentage < 0 || encControl.PacketLossPercentage > 100 {
		silk_assert(false)
		return SILK_ENC_INVALID_LOSS_RATE
	}
	if encControl.UseDTX < 0 || encControl.UseDTX > 1 {
		silk_assert(false)
		return SILK_ENC_INVALID_DTX_SETTING
	}
	if encControl.UseCBR < 0 || encControl.UseCBR > 1 {
		silk_assert(false)
		return SILK_ENC_INVALID_CBR_SETTING
	}
	if encControl.UseInBandFEC < 0 || encControl.UseInBandFEC > 1 {
		silk_assert(false)
		return SILK_ENC_INVALID_INBAND_FEC_SETTING
	}
	if encControl.NChannelsAPI < 1 || encControl.NChannelsAPI > ENCODER_NUM_CHANNELS {
		silk_assert(false)
		return SILK_ENC_INVALID_NUMBER_OF_CHANNELS_ERROR
	}
	if encControl.NChannelsInternal < 1 || encControl.NChannelsInternal > ENCODER_NUM_CHANNELS {
		silk_assert(false)
		return SILK_ENC_INVALID_NUMBER_OF_CHANNELS_ERROR
	}
	if encControl.NChannelsInternal > encControl.NChannelsAPI {
		silk_assert(false)
		return SILK_ENC_INVALID_NUMBER_OF_CHANNELS_ERROR
	}
	if encControl.Complexity < 0 || encControl.Complexity > 10 {
		silk_assert(false)
		return SILK_ENC_INVALID_COMPLEXITY_SETTING
	}

	return SILK_NO_ERROR
}
