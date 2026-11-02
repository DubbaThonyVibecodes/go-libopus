package silk

// C: silk/control_codec.c

// Control encoder
//
// C: opus_int silk_control_encoder(silk_encoder_state_Fxx *psEnc, silk_EncControlStruct *encControl, const opus_int32 TargetRate_bps, const opus_int allow_bw_switch, const opus_int channelNb, const opus_int force_fs_kHz)
func (psEnc *EncoderStateFIX) ControlEncoder(encControl *EncControlStruct, TargetRate_bps int32, allow_bw_switch int, channelNb int, force_fs_kHz int) int {
	var fs_kHz, ret int = 0, 0

	psEnc.sCmn.useDTX = encControl.UseDTX
	psEnc.sCmn.useCBR = encControl.UseCBR
	psEnc.sCmn.API_fs_Hz = encControl.API_sampleRate
	psEnc.sCmn.maxInternal_fs_Hz = int(encControl.MaxInternalSampleRate)
	psEnc.sCmn.minInternal_fs_Hz = int(encControl.MinInternalSampleRate)
	psEnc.sCmn.desiredInternal_fs_Hz = int(encControl.DesiredInternalSampleRate)
	psEnc.sCmn.useInBandFEC = encControl.UseInBandFEC
	psEnc.sCmn.nChannelsAPI = int(encControl.NChannelsAPI)
	psEnc.sCmn.nChannelsInternal = int(encControl.NChannelsInternal)
	psEnc.sCmn.allow_bandwidth_switch = allow_bw_switch
	psEnc.sCmn.channelNb = channelNb

	if psEnc.sCmn.controlled_since_last_payload != 0 && psEnc.sCmn.prefillFlag == 0 {
		if psEnc.sCmn.API_fs_Hz != psEnc.sCmn.prev_API_fs_Hz && psEnc.sCmn.fs_kHz > 0 {
			/* Change in API sampling rate in the middle of encoding a packet */
			ret += setupResamplers(psEnc, psEnc.sCmn.fs_kHz)
		}
		return ret
	}

	/* Beyond this point we know that there are no previously coded frames in the payload buffer */

	/********************************************/
	/* Determine internal sampling rate         */
	/********************************************/
	fs_kHz = psEnc.sCmn.ControlAudioBandwidth(encControl)
	if force_fs_kHz != 0 {
		fs_kHz = force_fs_kHz
	}
	/********************************************/
	/* Prepare resampler and buffered data      */
	/********************************************/
	ret += setupResamplers(psEnc, fs_kHz)

	/********************************************/
	/* Set internal sampling frequency          */
	/********************************************/
	ret += setupFs(psEnc, fs_kHz, encControl.PayloadSize_ms)

	/********************************************/
	/* Set encoding complexity                  */
	/********************************************/
	ret += setupComplexity(&psEnc.sCmn, encControl.Complexity)

	/********************************************/
	/* Set packet loss rate measured by farend  */
	/********************************************/
	psEnc.sCmn.PacketLoss_perc = encControl.PacketLossPercentage

	/********************************************/
	/* Set LBRR usage                           */
	/********************************************/
	ret += setupLBRR(&psEnc.sCmn, TargetRate_bps)

	psEnc.sCmn.controlled_since_last_payload = 1

	return ret
}

// C: static opus_int silk_setup_resamplers(silk_encoder_state_Fxx *psEnc, opus_int fs_kHz)
func setupResamplers(psEnc *EncoderStateFIX, fs_kHz int) int {
	var ret int = SILK_NO_ERROR

	if psEnc.sCmn.fs_kHz != fs_kHz || psEnc.sCmn.prev_API_fs_Hz != psEnc.sCmn.API_fs_Hz {
		if psEnc.sCmn.fs_kHz == 0 {
			/* Initialize the resampler for enc_API.c preparing resampling from API_fs_Hz to fs_kHz */
			ret += psEnc.sCmn.resampler_state.ResamplerInit(psEnc.sCmn.API_fs_Hz, int32(fs_kHz*1000), 1)
		} else {
			var x_buf_API_fs_Hz []int16
			var temp_resampler_state *ResamplerStateStruct
			x_bufFIX := psEnc.x_buf[:]
			var api_buf_samples int32
			var old_buf_samples int32
			var buf_length_ms int32

			buf_length_ms = LSHIFT(psEnc.sCmn.nb_subfr*5, 1) + LA_SHAPE_MS
			old_buf_samples = buf_length_ms * int32(psEnc.sCmn.fs_kHz)

			/* Initialize resampler for temporary resampling of x_buf data to API_fs_Hz */
			temp_resampler_state = new(ResamplerStateStruct)
			ret += temp_resampler_state.ResamplerInit(SMULBB(psEnc.sCmn.fs_kHz, 1000), psEnc.sCmn.API_fs_Hz, 0)

			/* Calculate number of samples to temporarily upsample */
			api_buf_samples = buf_length_ms * DIV32_16(psEnc.sCmn.API_fs_Hz, 1000)

			/* Temporary resampling of x_buf data to API_fs_Hz */
			x_buf_API_fs_Hz = make([]int16, api_buf_samples)
			ret += temp_resampler_state.Resampler(x_buf_API_fs_Hz, x_bufFIX, old_buf_samples)

			/* Initialize the resampler for enc_API.c preparing resampling from API_fs_Hz to fs_kHz */
			ret += psEnc.sCmn.resampler_state.ResamplerInit(psEnc.sCmn.API_fs_Hz, SMULBB(fs_kHz, 1000), 1)

			/* Correct resampler state by resampling buffered data from API_fs_Hz to fs_kHz */
			ret += psEnc.sCmn.resampler_state.Resampler(x_bufFIX, x_buf_API_fs_Hz, api_buf_samples)
		}
	}

	psEnc.sCmn.prev_API_fs_Hz = psEnc.sCmn.API_fs_Hz

	return ret
}

// C: static opus_int silk_setup_fs(silk_encoder_state_Fxx *psEnc, opus_int fs_kHz, opus_int PacketSize_ms)
func setupFs(psEnc *EncoderStateFIX, fs_kHz int, PacketSize_ms int) int {
	var ret int = SILK_NO_ERROR

	/* Set packet size */
	if PacketSize_ms != psEnc.sCmn.PacketSize_ms {
		if (PacketSize_ms != 10) &&
			(PacketSize_ms != 20) &&
			(PacketSize_ms != 40) &&
			(PacketSize_ms != 60) {
			ret = SILK_ENC_PACKET_SIZE_NOT_SUPPORTED
		}
		if PacketSize_ms <= 10 {
			psEnc.sCmn.nFramesPerPacket = 1
			if PacketSize_ms == 10 {
				psEnc.sCmn.nb_subfr = 2
			} else {
				psEnc.sCmn.nb_subfr = 1
			}
			psEnc.sCmn.frame_length = int(SMULBB(PacketSize_ms, fs_kHz))
			psEnc.sCmn.pitch_LPC_win_length = int(SMULBB(FIND_PITCH_LPC_WIN_MS_2_SF, fs_kHz))
			if psEnc.sCmn.fs_kHz == 8 {
				psEnc.sCmn.pitch_contour_iCDF = pitch_contour_10_ms_NB_iCDF[:]
			} else {
				psEnc.sCmn.pitch_contour_iCDF = pitch_contour_10_ms_iCDF[:]
			}
		} else {
			psEnc.sCmn.nFramesPerPacket = int(DIV32_16(PacketSize_ms, MAX_FRAME_LENGTH_MS))
			psEnc.sCmn.nb_subfr = MAX_NB_SUBFR
			psEnc.sCmn.frame_length = int(SMULBB(20, fs_kHz))
			psEnc.sCmn.pitch_LPC_win_length = int(SMULBB(FIND_PITCH_LPC_WIN_MS, fs_kHz))
			if psEnc.sCmn.fs_kHz == 8 {
				psEnc.sCmn.pitch_contour_iCDF = pitch_contour_NB_iCDF[:]
			} else {
				psEnc.sCmn.pitch_contour_iCDF = pitch_contour_iCDF[:]
			}
		}
		psEnc.sCmn.PacketSize_ms = PacketSize_ms
		psEnc.sCmn.TargetRate_bps = 0 /* trigger new SNR computation */
	}

	/* Set internal sampling frequency */
	silk_assert(fs_kHz == 8 || fs_kHz == 12 || fs_kHz == 16)
	silk_assert(psEnc.sCmn.nb_subfr == 2 || psEnc.sCmn.nb_subfr == 4)
	if psEnc.sCmn.fs_kHz != fs_kHz {
		/* reset part of the state */
		psEnc.sShape = ShapeStateFIX{}
		psEnc.sPrefilt = PrefilterStateFIX{}
		psEnc.sCmn.sNSQ = NsqState{}
		clear(psEnc.sCmn.prev_NLSFq_Q15[:])
		clear(psEnc.sCmn.sLP.In_LP_State[:])
		psEnc.sCmn.inputBufIx = 0
		psEnc.sCmn.nFramesEncoded = 0
		psEnc.sCmn.TargetRate_bps = 0 /* trigger new SNR computation */

		/* Initialize non-zero parameters */
		psEnc.sCmn.prevLag = 100
		psEnc.sCmn.first_frame_after_reset = 1
		psEnc.sPrefilt.lagPrev = 100
		psEnc.sShape.LastGainIndex = 10
		psEnc.sCmn.sNSQ.lagPrev = 100
		psEnc.sCmn.sNSQ.prev_gain_Q16 = 65536
		psEnc.sCmn.prevSignalType = TYPE_NO_VOICE_ACTIVITY

		psEnc.sCmn.fs_kHz = fs_kHz
		if psEnc.sCmn.fs_kHz == 8 {
			if psEnc.sCmn.nb_subfr == MAX_NB_SUBFR {
				psEnc.sCmn.pitch_contour_iCDF = pitch_contour_NB_iCDF[:]
			} else {
				psEnc.sCmn.pitch_contour_iCDF = pitch_contour_10_ms_NB_iCDF[:]
			}
		} else {
			if psEnc.sCmn.nb_subfr == MAX_NB_SUBFR {
				psEnc.sCmn.pitch_contour_iCDF = pitch_contour_iCDF[:]
			} else {
				psEnc.sCmn.pitch_contour_iCDF = pitch_contour_10_ms_iCDF[:]
			}
		}
		if psEnc.sCmn.fs_kHz == 8 || psEnc.sCmn.fs_kHz == 12 {
			psEnc.sCmn.predictLPCOrder = MIN_LPC_ORDER
			psEnc.sCmn.psNLSF_CB = &NLSF_CB_NB_MB
		} else {
			psEnc.sCmn.predictLPCOrder = MAX_LPC_ORDER
			psEnc.sCmn.psNLSF_CB = &NLSF_CB_WB
		}
		psEnc.sCmn.subfr_length = SUB_FRAME_LENGTH_MS * fs_kHz
		psEnc.sCmn.frame_length = int(SMULBB(psEnc.sCmn.subfr_length, psEnc.sCmn.nb_subfr))
		psEnc.sCmn.ltp_mem_length = int(SMULBB(LTP_MEM_LENGTH_MS, fs_kHz))
		psEnc.sCmn.la_pitch = int(SMULBB(LA_PITCH_MS, fs_kHz))
		psEnc.sCmn.max_pitch_lag = int(SMULBB(18, fs_kHz))
		if psEnc.sCmn.nb_subfr == MAX_NB_SUBFR {
			psEnc.sCmn.pitch_LPC_win_length = int(SMULBB(FIND_PITCH_LPC_WIN_MS, fs_kHz))
		} else {
			psEnc.sCmn.pitch_LPC_win_length = int(SMULBB(FIND_PITCH_LPC_WIN_MS_2_SF, fs_kHz))
		}
		if psEnc.sCmn.fs_kHz == 16 {
			psEnc.sCmn.mu_LTP_Q9 = int(SILK_FIX_CONST(MU_LTP_QUANT_WB, 9))
			psEnc.sCmn.pitch_lag_low_bits_iCDF = uniform8_iCDF[:]
		} else if psEnc.sCmn.fs_kHz == 12 {
			psEnc.sCmn.mu_LTP_Q9 = int(SILK_FIX_CONST(MU_LTP_QUANT_MB, 9))
			psEnc.sCmn.pitch_lag_low_bits_iCDF = uniform6_iCDF[:]
		} else {
			psEnc.sCmn.mu_LTP_Q9 = int(SILK_FIX_CONST(MU_LTP_QUANT_NB, 9))
			psEnc.sCmn.pitch_lag_low_bits_iCDF = uniform4_iCDF[:]
		}
	}

	/* Check that settings are valid */
	silk_assert((psEnc.sCmn.subfr_length * psEnc.sCmn.nb_subfr) == psEnc.sCmn.frame_length)

	return ret
}

// C: static opus_int silk_setup_complexity(silk_encoder_state *psEncC, opus_int Complexity)
func setupComplexity(psEncC *EncoderState, Complexity int) int {
	var ret int = 0

	/* Set encoding complexity */
	silk_assert(Complexity >= 0 && Complexity <= 10)
	if Complexity < 2 {
		psEncC.pitchEstimationComplexity = SILK_PE_MIN_COMPLEX
		psEncC.pitchEstimationThreshold_Q16 = SILK_FIX_CONST(0.8, 16)
		psEncC.pitchEstimationLPCOrder = 6
		psEncC.shapingLPCOrder = 8
		psEncC.la_shape = 3 * psEncC.fs_kHz
		psEncC.nStatesDelayedDecision = 1
		psEncC.useInterpolatedNLSFs = 0
		psEncC.LTPQuantLowComplexity = 1
		psEncC.NLSF_MSVQ_Survivors = 2
		psEncC.warping_Q16 = 0
	} else if Complexity < 4 {
		psEncC.pitchEstimationComplexity = SILK_PE_MID_COMPLEX
		psEncC.pitchEstimationThreshold_Q16 = SILK_FIX_CONST(0.76, 16)
		psEncC.pitchEstimationLPCOrder = 8
		psEncC.shapingLPCOrder = 10
		psEncC.la_shape = 5 * psEncC.fs_kHz
		psEncC.nStatesDelayedDecision = 1
		psEncC.useInterpolatedNLSFs = 0
		psEncC.LTPQuantLowComplexity = 0
		psEncC.NLSF_MSVQ_Survivors = 4
		psEncC.warping_Q16 = 0
	} else if Complexity < 6 {
		psEncC.pitchEstimationComplexity = SILK_PE_MID_COMPLEX
		psEncC.pitchEstimationThreshold_Q16 = SILK_FIX_CONST(0.74, 16)
		psEncC.pitchEstimationLPCOrder = 10
		psEncC.shapingLPCOrder = 12
		psEncC.la_shape = 5 * psEncC.fs_kHz
		psEncC.nStatesDelayedDecision = 2
		psEncC.useInterpolatedNLSFs = 1
		psEncC.LTPQuantLowComplexity = 0
		psEncC.NLSF_MSVQ_Survivors = 8
		psEncC.warping_Q16 = psEncC.fs_kHz * int(SILK_FIX_CONST(WARPING_MULTIPLIER, 16))
	} else if Complexity < 8 {
		psEncC.pitchEstimationComplexity = SILK_PE_MID_COMPLEX
		psEncC.pitchEstimationThreshold_Q16 = SILK_FIX_CONST(0.72, 16)
		psEncC.pitchEstimationLPCOrder = 12
		psEncC.shapingLPCOrder = 14
		psEncC.la_shape = 5 * psEncC.fs_kHz
		psEncC.nStatesDelayedDecision = 3
		psEncC.useInterpolatedNLSFs = 1
		psEncC.LTPQuantLowComplexity = 0
		psEncC.NLSF_MSVQ_Survivors = 16
		psEncC.warping_Q16 = psEncC.fs_kHz * int(SILK_FIX_CONST(WARPING_MULTIPLIER, 16))
	} else {
		psEncC.pitchEstimationComplexity = SILK_PE_MAX_COMPLEX
		psEncC.pitchEstimationThreshold_Q16 = SILK_FIX_CONST(0.7, 16)
		psEncC.pitchEstimationLPCOrder = 16
		psEncC.shapingLPCOrder = 16
		psEncC.la_shape = 5 * psEncC.fs_kHz
		psEncC.nStatesDelayedDecision = MAX_DEL_DEC_STATES
		psEncC.useInterpolatedNLSFs = 1
		psEncC.LTPQuantLowComplexity = 0
		psEncC.NLSF_MSVQ_Survivors = 32
		psEncC.warping_Q16 = psEncC.fs_kHz * int(SILK_FIX_CONST(WARPING_MULTIPLIER, 16))
	}

	/* Do not allow higher pitch estimation LPC order than predict LPC order */
	psEncC.pitchEstimationLPCOrder = min_int(psEncC.pitchEstimationLPCOrder, psEncC.predictLPCOrder)
	psEncC.shapeWinLength = SUB_FRAME_LENGTH_MS*psEncC.fs_kHz + 2*psEncC.la_shape
	psEncC.Complexity = Complexity

	silk_assert(psEncC.pitchEstimationLPCOrder <= MAX_FIND_PITCH_LPC_ORDER)
	silk_assert(psEncC.shapingLPCOrder <= MAX_SHAPE_LPC_ORDER)
	silk_assert(psEncC.nStatesDelayedDecision <= MAX_DEL_DEC_STATES)
	silk_assert(psEncC.warping_Q16 <= 32767)
	silk_assert(psEncC.la_shape <= LA_SHAPE_MAX)
	silk_assert(psEncC.shapeWinLength <= SHAPE_LPC_WIN_MAX)
	silk_assert(psEncC.NLSF_MSVQ_Survivors <= NLSF_VQ_MAX_SURVIVORS)

	return ret
}

// C: static OPUS_INLINE opus_int silk_setup_LBRR(silk_encoder_state *psEncC, const opus_int32 TargetRate_bps)
func setupLBRR(psEncC *EncoderState, TargetRate_bps int32) int {
	var LBRR_in_previous_packet int
	var ret int = SILK_NO_ERROR
	var LBRR_rate_thres_bps int32

	LBRR_in_previous_packet = psEncC.LBRR_enabled
	psEncC.LBRR_enabled = 0
	if psEncC.useInBandFEC != 0 && psEncC.PacketLoss_perc > 0 {
		if psEncC.fs_kHz == 8 {
			LBRR_rate_thres_bps = LBRR_NB_MIN_RATE_BPS
		} else if psEncC.fs_kHz == 12 {
			LBRR_rate_thres_bps = LBRR_MB_MIN_RATE_BPS
		} else {
			LBRR_rate_thres_bps = LBRR_WB_MIN_RATE_BPS
		}
		LBRR_rate_thres_bps = SMULWB(MUL(LBRR_rate_thres_bps, 125-min(psEncC.PacketLoss_perc, 25)), SILK_FIX_CONST(0.01, 16))

		if TargetRate_bps > LBRR_rate_thres_bps {
			/* Set gain increase for coding LBRR excitation */
			if LBRR_in_previous_packet == 0 {
				/* Previous packet did not have LBRR, and was therefore coded at a higher bitrate */
				psEncC.LBRR_GainIncreases = 7
			} else {
				psEncC.LBRR_GainIncreases = max_int(7-int(SMULWB(int32(psEncC.PacketLoss_perc), SILK_FIX_CONST(0.4, 16))), 2)
			}
			psEncC.LBRR_enabled = 1
		}
	}

	return ret
}
