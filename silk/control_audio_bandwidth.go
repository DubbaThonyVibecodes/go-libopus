package silk

// C: silk/control_audio_bandwidth.c

// Control internal sampling rate
//
// C: opus_int silk_control_audio_bandwidth(silk_encoder_state *psEncC, silk_EncControlStruct *encControl)
func (psEncC *EncoderState) ControlAudioBandwidth(encControl *EncControlStruct) int {
	var fs_kHz int
	var fs_Hz int32

	fs_kHz = psEncC.fs_kHz
	fs_Hz = SMULBB(fs_kHz, 1000)
	if fs_Hz == 0 {
		/* Encoder has just been initialized */
		fs_Hz = min(int32(psEncC.desiredInternal_fs_Hz), psEncC.API_fs_Hz)
		fs_kHz = int(DIV32_16(fs_Hz, 1000))
	} else if fs_Hz > psEncC.API_fs_Hz || int(fs_Hz) > psEncC.maxInternal_fs_Hz || int(fs_Hz) < psEncC.minInternal_fs_Hz {
		/* Make sure internal rate is not higher than external rate or maximum allowed, or lower than minimum allowed */
		fs_Hz = psEncC.API_fs_Hz
		fs_Hz = min(fs_Hz, int32(psEncC.maxInternal_fs_Hz))
		fs_Hz = max(fs_Hz, int32(psEncC.minInternal_fs_Hz))
		fs_kHz = int(DIV32_16(fs_Hz, 1000))
	} else {
		/* State machine for the internal sampling rate switching */
		if psEncC.sLP.transition_frame_no >= TRANSITION_FRAMES {
			/* Stop transition phase */
			psEncC.sLP.mode = 0
		}
		if psEncC.allow_bandwidth_switch != 0 || encControl.OpusCanSwitch != 0 {
			/* Check if we should switch down */
			if int(SMULBB(psEncC.fs_kHz, 1000)) > psEncC.desiredInternal_fs_Hz {
				/* Switch down */
				if psEncC.sLP.mode == 0 {
					/* New transition */
					psEncC.sLP.transition_frame_no = TRANSITION_FRAMES

					/* Reset transition filter state */
					clear(psEncC.sLP.In_LP_State[:])
				}
				if encControl.OpusCanSwitch != 0 {
					/* Stop transition phase */
					psEncC.sLP.mode = 0

					/* Switch to a lower sample frequency */
					if psEncC.fs_kHz == 16 {
						fs_kHz = 12
					} else {
						fs_kHz = 8
					}
				} else {
					if psEncC.sLP.transition_frame_no <= 0 {
						encControl.SwitchReady = 1
						/* Make room for redundancy */
						encControl.MaxBits -= encControl.MaxBits * 5 / (encControl.PayloadSize_ms + 5)
					} else {
						/* Direction: down (at double speed) */
						psEncC.sLP.mode = -2
					}
				}
			} else if int(SMULBB(psEncC.fs_kHz, 1000)) < psEncC.desiredInternal_fs_Hz {
				/* Check if we should switch up */
				/* Switch up */
				if encControl.OpusCanSwitch != 0 {
					/* Switch to a higher sample frequency */
					if psEncC.fs_kHz == 8 {
						fs_kHz = 12
					} else {
						fs_kHz = 16
					}

					/* New transition */
					psEncC.sLP.transition_frame_no = 0

					/* Reset transition filter state */
					clear(psEncC.sLP.In_LP_State[:])

					/* Direction: up */
					psEncC.sLP.mode = 1
				} else {
					if psEncC.sLP.mode == 0 {
						encControl.SwitchReady = 1
						/* Make room for redundancy */
						encControl.MaxBits -= encControl.MaxBits * 5 / (encControl.PayloadSize_ms + 5)
					} else {
						/* Direction: up */
						psEncC.sLP.mode = 1
					}
				}
			} else {
				if psEncC.sLP.mode < 0 {
					psEncC.sLP.mode = 1
				}
			}
		}
	}

	return fs_kHz
}
