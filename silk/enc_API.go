package silk

import (
	"unsafe"

	"github.com/DubbaThonyVibecodes/go-libopus/celt"
)

// C: silk/enc_API.c

/****************************************/
/* Encoder functions                    */
/****************************************/

// C: opus_int silk_Get_Encoder_Size(opus_int *encSizeBytes)
//
// O    Returns error code
func GetEncoderSize(encSizeBytes *int) int {
	var ret int = SILK_NO_ERROR

	*encSizeBytes = int(unsafe.Sizeof(Encoder{}))

	return ret
}

/*************************/
/* Init or Reset encoder */
/*************************/
//
// C: opus_int silk_InitEncoder(void *encState, int arch, silk_EncControlStruct *encStatus)
//
// O    Returns error code
func (encState *Encoder) InitEncoder(arch int, encStatus *EncControlStruct) int {
	var psEnc *Encoder
	var n int
	var ret int = SILK_NO_ERROR

	psEnc = encState

	/* Reset encoder */
	*psEnc = Encoder{}
	for n = 0; n < ENCODER_NUM_CHANNELS; n++ {
		ret += psEnc.state_Fxx[n].InitEncoder(arch)
		if ret != 0 {
			silk_assert(false)
		}
	}

	psEnc.nChannelsAPI = 1
	psEnc.nChannelsInternal = 1

	/* Read control structure */
	ret += queryEncoder(encState, encStatus)
	if ret != 0 {
		silk_assert(false)
	}

	return ret
}

/***************************************/
/* Read control structure from encoder */
/***************************************/
//
// C: static opus_int silk_QueryEncoder(const void *encState, silk_EncControlStruct *encStatus)
//
// O    Returns error code
func queryEncoder(encState *Encoder, encStatus *EncControlStruct) int {
	var ret int = SILK_NO_ERROR
	var state_Fxx []EncoderStateFIX
	psEnc := encState

	state_Fxx = psEnc.state_Fxx[:]

	encStatus.NChannelsAPI = int32(psEnc.nChannelsAPI)
	encStatus.NChannelsInternal = int32(psEnc.nChannelsInternal)
	encStatus.API_sampleRate = state_Fxx[0].sCmn.API_fs_Hz
	encStatus.MaxInternalSampleRate = int32(state_Fxx[0].sCmn.maxInternal_fs_Hz)
	encStatus.MinInternalSampleRate = int32(state_Fxx[0].sCmn.minInternal_fs_Hz)
	encStatus.DesiredInternalSampleRate = int32(state_Fxx[0].sCmn.desiredInternal_fs_Hz)
	encStatus.PayloadSize_ms = state_Fxx[0].sCmn.PacketSize_ms
	encStatus.BitRate = state_Fxx[0].sCmn.TargetRate_bps
	encStatus.PacketLossPercentage = state_Fxx[0].sCmn.PacketLoss_perc
	encStatus.Complexity = state_Fxx[0].sCmn.Complexity
	encStatus.UseInBandFEC = state_Fxx[0].sCmn.useInBandFEC
	encStatus.UseDTX = state_Fxx[0].sCmn.useDTX
	encStatus.UseCBR = state_Fxx[0].sCmn.useCBR
	encStatus.InternalSampleRate = SMULBB(state_Fxx[0].sCmn.fs_kHz, 1000)
	encStatus.AllowBandwidthSwitch = state_Fxx[0].sCmn.allow_bandwidth_switch
	encStatus.InWBmodeWithoutVariableLP = b2i(state_Fxx[0].sCmn.fs_kHz == 16 && state_Fxx[0].sCmn.sLP.mode == 0)

	return ret
}

/**************************/
/* Encode frame with Silk */
/**************************/
/* Note: if prefillFlag is set, the input must contain 10 ms of audio, irrespective of what                     */
/* encControl->payloadSize_ms is set to                                                                         */
//
// C: opus_int silk_Encode(void *encState, silk_EncControlStruct *encControl, const opus_int16 *samplesIn, opus_int nSamplesIn, ec_enc *psRangeEnc, opus_int32 *nBytesOut, const opus_int prefillFlag, opus_int activity)
//
// O    Returns error code
func (encState *Encoder) Encode(encControl *EncControlStruct, samplesIn []int16, nSamplesIn int, psRangeEnc *celt.EcEnc, nBytesOut *int32, prefillFlag int, activity int) int {
	var n, i, nBits, flags int
	var tmp_payloadSize_ms, tmp_complexity, ret int = 0, 0, 0
	var nSamplesToBuffer, nSamplesToBufferMax, nBlocksOf10ms int
	var nSamplesFromInput, nSamplesFromInputMax int = 0, 0
	var speech_act_thr_for_switch_Q8 int
	var TargetRate_bps, channelRate_bps, LBRR_symbol, sum int32
	var MStargetRates_bps [2]int32
	psEnc := encState
	var buf []int16
	var transition, curr_block, tot_blocks int

	if encControl.ReducedDependency != 0 {
		psEnc.state_Fxx[0].sCmn.first_frame_after_reset = 1
		psEnc.state_Fxx[1].sCmn.first_frame_after_reset = 1
	}
	psEnc.state_Fxx[1].sCmn.nFramesEncoded = 0
	psEnc.state_Fxx[0].sCmn.nFramesEncoded = 0

	/* Check values in encoder control structure */
	if ret = CheckControlInput(encControl); ret != 0 {
		silk_assert(false)
		return ret
	}

	encControl.SwitchReady = 0

	if int(encControl.NChannelsInternal) > psEnc.nChannelsInternal {
		/* Mono -> Stereo transition: init state of second channel and stereo state */
		ret += psEnc.state_Fxx[1].InitEncoder(psEnc.state_Fxx[0].sCmn.arch)
		clear(psEnc.sStereo.pred_prev_Q13[:])
		clear(psEnc.sStereo.sSide[:])
		psEnc.sStereo.mid_side_amp_Q0[0] = 0
		psEnc.sStereo.mid_side_amp_Q0[1] = 1
		psEnc.sStereo.mid_side_amp_Q0[2] = 0
		psEnc.sStereo.mid_side_amp_Q0[3] = 1
		psEnc.sStereo.width_prev_Q14 = 0
		psEnc.sStereo.smth_width_Q14 = int16(SILK_FIX_CONST(1.0, 14))
		if psEnc.nChannelsAPI == 2 {
			psEnc.state_Fxx[1].sCmn.resampler_state = psEnc.state_Fxx[0].sCmn.resampler_state
			psEnc.state_Fxx[1].sCmn.In_HP_State = psEnc.state_Fxx[0].sCmn.In_HP_State
		}
	}

	transition = b2i((encControl.PayloadSize_ms != psEnc.state_Fxx[0].sCmn.PacketSize_ms) || (psEnc.nChannelsInternal != int(encControl.NChannelsInternal)))

	psEnc.nChannelsAPI = int(encControl.NChannelsAPI)
	psEnc.nChannelsInternal = int(encControl.NChannelsInternal)

	nBlocksOf10ms = int(DIV32(100*nSamplesIn, encControl.API_sampleRate))
	if nBlocksOf10ms > 1 {
		tot_blocks = nBlocksOf10ms >> 1
	} else {
		tot_blocks = 1
	}
	curr_block = 0
	if prefillFlag != 0 {
		var save_LP LPState
		/* Only accept input length of 10 ms */
		if nBlocksOf10ms != 1 {
			silk_assert(false)
			return SILK_ENC_INPUT_INVALID_NO_OF_SAMPLES
		}
		if prefillFlag == 2 {
			save_LP = psEnc.state_Fxx[0].sCmn.sLP
			/* Save the sampling rate so the bandwidth switching code can keep handling transitions. */
			save_LP.saved_fs_kHz = int32(psEnc.state_Fxx[0].sCmn.fs_kHz)
		}
		/* Reset Encoder */
		for n = 0; n < int(encControl.NChannelsInternal); n++ {
			ret = psEnc.state_Fxx[n].InitEncoder(psEnc.state_Fxx[n].sCmn.arch)
			/* Restore the variable LP state. */
			if prefillFlag == 2 {
				psEnc.state_Fxx[n].sCmn.sLP = save_LP
			}
			silk_assert(ret == 0)
		}
		tmp_payloadSize_ms = encControl.PayloadSize_ms
		encControl.PayloadSize_ms = 10
		tmp_complexity = encControl.Complexity
		encControl.Complexity = 0
		for n = 0; n < int(encControl.NChannelsInternal); n++ {
			psEnc.state_Fxx[n].sCmn.controlled_since_last_payload = 0
			psEnc.state_Fxx[n].sCmn.prefillFlag = 1
		}
	} else {
		/* Only accept input lengths that are a multiple of 10 ms */
		if nBlocksOf10ms*int(encControl.API_sampleRate) != 100*nSamplesIn || nSamplesIn < 0 {
			silk_assert(false)
			return SILK_ENC_INPUT_INVALID_NO_OF_SAMPLES
		}
		/* Make sure no more than one packet can be produced */
		if 1000*int32(nSamplesIn) > int32(encControl.PayloadSize_ms)*encControl.API_sampleRate {
			silk_assert(false)
			return SILK_ENC_INPUT_INVALID_NO_OF_SAMPLES
		}
	}

	for n = 0; n < int(encControl.NChannelsInternal); n++ {
		/* Force the side channel to the same rate as the mid */
		var force_fs_kHz int
		if n == 1 {
			force_fs_kHz = psEnc.state_Fxx[0].sCmn.fs_kHz
		} else {
			force_fs_kHz = 0
		}
		if ret = psEnc.state_Fxx[n].ControlEncoder(encControl, psEnc.allowBandwidthSwitch, n, force_fs_kHz); ret != 0 {
			silk_assert(false)
			return ret
		}
		if psEnc.state_Fxx[n].sCmn.first_frame_after_reset != 0 || transition != 0 {
			for i = 0; i < psEnc.state_Fxx[0].sCmn.nFramesPerPacket; i++ {
				psEnc.state_Fxx[n].sCmn.LBRR_flags[i] = 0
			}
		}
		psEnc.state_Fxx[n].sCmn.inDTX = psEnc.state_Fxx[n].sCmn.useDTX
	}
	silk_assert(encControl.NChannelsInternal == 1 || psEnc.state_Fxx[0].sCmn.fs_kHz == psEnc.state_Fxx[1].sCmn.fs_kHz)

	/* Input buffering/resampling and encoding */
	nSamplesToBufferMax =
		10 * nBlocksOf10ms * psEnc.state_Fxx[0].sCmn.fs_kHz
	nSamplesFromInputMax =
		int(DIV32_16(int32(nSamplesToBufferMax)*
			psEnc.state_Fxx[0].sCmn.API_fs_Hz,
			psEnc.state_Fxx[0].sCmn.fs_kHz*1000))
	buf = make([]int16, nSamplesFromInputMax)
	for {
		nSamplesToBuffer = psEnc.state_Fxx[0].sCmn.frame_length - psEnc.state_Fxx[0].sCmn.inputBufIx
		nSamplesToBuffer = min(nSamplesToBuffer, nSamplesToBufferMax)
		nSamplesFromInput = int(DIV32_16(int32(nSamplesToBuffer)*psEnc.state_Fxx[0].sCmn.API_fs_Hz, psEnc.state_Fxx[0].sCmn.fs_kHz*1000))
		/* Resample and write to buffer */
		if encControl.NChannelsAPI == 2 && encControl.NChannelsInternal == 2 {
			id := psEnc.state_Fxx[0].sCmn.nFramesEncoded
			for n = 0; n < nSamplesFromInput; n++ {
				buf[n] = samplesIn[2*n]
			}
			/* Making sure to start both resamplers from the same state when switching from mono to stereo */
			if psEnc.nPrevChannelsInternal == 1 && id == 0 {
				psEnc.state_Fxx[1].sCmn.resampler_state = psEnc.state_Fxx[0].sCmn.resampler_state
			}

			ret += psEnc.state_Fxx[0].sCmn.resampler_state.Resampler(
				psEnc.state_Fxx[0].sCmn.inputBuf[psEnc.state_Fxx[0].sCmn.inputBufIx+2:], buf, int32(nSamplesFromInput))
			psEnc.state_Fxx[0].sCmn.inputBufIx += nSamplesToBuffer

			nSamplesToBuffer = psEnc.state_Fxx[1].sCmn.frame_length - psEnc.state_Fxx[1].sCmn.inputBufIx
			nSamplesToBuffer = min(nSamplesToBuffer, 10*nBlocksOf10ms*psEnc.state_Fxx[1].sCmn.fs_kHz)
			for n = 0; n < nSamplesFromInput; n++ {
				buf[n] = samplesIn[2*n+1]
			}
			ret += psEnc.state_Fxx[1].sCmn.resampler_state.Resampler(
				psEnc.state_Fxx[1].sCmn.inputBuf[psEnc.state_Fxx[1].sCmn.inputBufIx+2:], buf, int32(nSamplesFromInput))

			psEnc.state_Fxx[1].sCmn.inputBufIx += nSamplesToBuffer
		} else if encControl.NChannelsAPI == 2 && encControl.NChannelsInternal == 1 {
			/* Combine left and right channels before resampling */
			for n = 0; n < nSamplesFromInput; n++ {
				sum = int32(samplesIn[2*n]) + int32(samplesIn[2*n+1])
				buf[n] = int16(RSHIFT_ROUND(sum, 1))
			}
			ret += psEnc.state_Fxx[0].sCmn.resampler_state.Resampler(
				psEnc.state_Fxx[0].sCmn.inputBuf[psEnc.state_Fxx[0].sCmn.inputBufIx+2:], buf, int32(nSamplesFromInput))
			/* On the first mono frame, average the results for the two resampler states  */
			if psEnc.nPrevChannelsInternal == 2 && psEnc.state_Fxx[0].sCmn.nFramesEncoded == 0 {
				ret += psEnc.state_Fxx[1].sCmn.resampler_state.Resampler(
					psEnc.state_Fxx[1].sCmn.inputBuf[psEnc.state_Fxx[1].sCmn.inputBufIx+2:], buf, int32(nSamplesFromInput))
				for n = 0; n < psEnc.state_Fxx[0].sCmn.frame_length; n++ {
					psEnc.state_Fxx[0].sCmn.inputBuf[psEnc.state_Fxx[0].sCmn.inputBufIx+n+2] =
						int16(RSHIFT(int(psEnc.state_Fxx[0].sCmn.inputBuf[psEnc.state_Fxx[0].sCmn.inputBufIx+n+2])+
							int(psEnc.state_Fxx[1].sCmn.inputBuf[psEnc.state_Fxx[1].sCmn.inputBufIx+n+2]), 1))
				}
			}
			psEnc.state_Fxx[0].sCmn.inputBufIx += nSamplesToBuffer
		} else {
			silk_assert(encControl.NChannelsAPI == 1 && encControl.NChannelsInternal == 1)
			copy(buf[:nSamplesFromInput], samplesIn[:nSamplesFromInput])
			ret += psEnc.state_Fxx[0].sCmn.resampler_state.Resampler(
				psEnc.state_Fxx[0].sCmn.inputBuf[psEnc.state_Fxx[0].sCmn.inputBufIx+2:], buf, int32(nSamplesFromInput))
			psEnc.state_Fxx[0].sCmn.inputBufIx += nSamplesToBuffer
		}

		samplesIn = samplesIn[nSamplesFromInput*int(encControl.NChannelsAPI):]
		nSamplesIn -= nSamplesFromInput

		/* Default */
		psEnc.allowBandwidthSwitch = 0

		/* Silk encoder */
		if psEnc.state_Fxx[0].sCmn.inputBufIx >= psEnc.state_Fxx[0].sCmn.frame_length {
			/* Enough data in input buffer, so encode */
			silk_assert(psEnc.state_Fxx[0].sCmn.inputBufIx == psEnc.state_Fxx[0].sCmn.frame_length)
			silk_assert(encControl.NChannelsInternal == 1 || psEnc.state_Fxx[1].sCmn.inputBufIx == psEnc.state_Fxx[1].sCmn.frame_length)

			/* Deal with LBRR data */
			if psEnc.state_Fxx[0].sCmn.nFramesEncoded == 0 && prefillFlag == 0 {
				/* Create space at start of payload for VAD and FEC flags */
				iCDF := [2]uint8{0, 0}
				iCDF[0] = uint8(256 - RSHIFT(256, (psEnc.state_Fxx[0].sCmn.nFramesPerPacket+1)*int(encControl.NChannelsInternal)))
				psRangeEnc.EncIcdf(0, iCDF[:], 8)

				/* Encode any LBRR data from previous packet */
				/* Encode LBRR flags */
				for n = 0; n < int(encControl.NChannelsInternal); n++ {
					LBRR_symbol = 0
					for i = 0; i < psEnc.state_Fxx[n].sCmn.nFramesPerPacket; i++ {
						LBRR_symbol |= LSHIFT(psEnc.state_Fxx[n].sCmn.LBRR_flags[i], i)
					}
					psEnc.state_Fxx[n].sCmn.LBRR_flag = int8(b2i(LBRR_symbol > 0))
					if LBRR_symbol != 0 && psEnc.state_Fxx[n].sCmn.nFramesPerPacket > 1 {
						psRangeEnc.EncIcdf(int(LBRR_symbol-1), LBRR_flags_iCDF_ptr[psEnc.state_Fxx[n].sCmn.nFramesPerPacket-2], 8)
					}
				}

				/* Code LBRR indices and excitation signals */
				for i = 0; i < psEnc.state_Fxx[0].sCmn.nFramesPerPacket; i++ {
					for n = 0; n < int(encControl.NChannelsInternal); n++ {
						if psEnc.state_Fxx[n].sCmn.LBRR_flags[i] != 0 {
							var condCoding int

							if encControl.NChannelsInternal == 2 && n == 0 {
								StereoEncodePred(psRangeEnc, &psEnc.sStereo.predIx[i])
								/* For LBRR data there's no need to code the mid-only flag if the side-channel LBRR flag is set */
								if psEnc.state_Fxx[1].sCmn.LBRR_flags[i] == 0 {
									StereoEncodeMidOnly(psRangeEnc, psEnc.sStereo.mid_only_flags[i])
								}
							}
							/* Use conditional coding if previous frame available */
							if i > 0 && psEnc.state_Fxx[n].sCmn.LBRR_flags[i-1] != 0 {
								condCoding = CODE_CONDITIONALLY
							} else {
								condCoding = CODE_INDEPENDENTLY
							}
							psEnc.state_Fxx[n].sCmn.EncodeIndices(psRangeEnc, i, 1, condCoding)
							EncodePulses(psRangeEnc, int(psEnc.state_Fxx[n].sCmn.indices_LBRR[i].signalType), int(psEnc.state_Fxx[n].sCmn.indices_LBRR[i].quantOffsetType),
								psEnc.state_Fxx[n].sCmn.pulses_LBRR[i][:], psEnc.state_Fxx[n].sCmn.frame_length)
						}
					}
				}

				/* Reset LBRR flags */
				for n = 0; n < int(encControl.NChannelsInternal); n++ {
					clear(psEnc.state_Fxx[n].sCmn.LBRR_flags[:])
				}

				psEnc.nBitsUsedLBRR = int32(psRangeEnc.Tell())
			}

			HPVariableCutoff(psEnc.state_Fxx[:])

			/* Total target bits for packet */
			nBits = int(DIV32_16(MUL(encControl.BitRate, encControl.PayloadSize_ms), 1000))
			/* Subtract bits used for LBRR */
			if prefillFlag == 0 {
				nBits -= int(psEnc.nBitsUsedLBRR)
			}
			/* Divide by number of uncoded frames left in packet */
			nBits = int(DIV32_16(nBits, psEnc.state_Fxx[0].sCmn.nFramesPerPacket))
			/* Convert to bits/second */
			if encControl.PayloadSize_ms == 10 {
				TargetRate_bps = SMULBB(nBits, 100)
			} else {
				TargetRate_bps = SMULBB(nBits, 50)
			}
			/* Subtract fraction of bits in excess of target in previous frames and packets */
			TargetRate_bps -= DIV32_16(MUL(psEnc.nBitsExceeded, 1000), BITRESERVOIR_DECAY_TIME_MS)
			if prefillFlag == 0 && psEnc.state_Fxx[0].sCmn.nFramesEncoded > 0 {
				/* Compare actual vs target bits so far in this packet */
				var bitsBalance int32 = int32(psRangeEnc.Tell()) - psEnc.nBitsUsedLBRR - int32(nBits*psEnc.state_Fxx[0].sCmn.nFramesEncoded)
				TargetRate_bps -= DIV32_16(MUL(bitsBalance, 1000), BITRESERVOIR_DECAY_TIME_MS)
			}
			/* Never exceed input bitrate */
			TargetRate_bps = LIMIT(TargetRate_bps, encControl.BitRate, 5000)

			/* Convert Left/Right to Mid/Side */
			if encControl.NChannelsInternal == 2 {
				/* C passes &inputBuf[ 2 ] here; silk_stereo_LR_to_MS reads x1[ -2 ], x1[ -1 ]
				   (the two history samples at inputBuf[ 0 ], inputBuf[ 1 ]). The Go port of
				   StereoLRToMS takes slices that start 2 samples BEFORE the frame, i.e.
				   element 0 of x1/x2 is inputBuf[ 0 ], so the whole inputBuf is passed
				   here (not inputBuf[ 2: ] as for the other uses of &inputBuf[ 2 ]). */
				psEnc.sStereo.StereoLRToMS(psEnc.state_Fxx[0].sCmn.inputBuf[:], psEnc.state_Fxx[1].sCmn.inputBuf[:],
					&psEnc.sStereo.predIx[psEnc.state_Fxx[0].sCmn.nFramesEncoded], &psEnc.sStereo.mid_only_flags[psEnc.state_Fxx[0].sCmn.nFramesEncoded],
					MStargetRates_bps[:], TargetRate_bps, psEnc.state_Fxx[0].sCmn.speech_activity_Q8, encControl.ToMono,
					psEnc.state_Fxx[0].sCmn.fs_kHz, psEnc.state_Fxx[0].sCmn.frame_length)
				if psEnc.sStereo.mid_only_flags[psEnc.state_Fxx[0].sCmn.nFramesEncoded] == 0 {
					/* Reset side channel encoder memory for first frame with side coding */
					if psEnc.prev_decode_only_middle == 1 {
						psEnc.state_Fxx[1].sShape = ShapeStateFIX{}
						psEnc.state_Fxx[1].sCmn.sNSQ = NsqState{}
						clear(psEnc.state_Fxx[1].sCmn.prev_NLSFq_Q15[:])
						clear(psEnc.state_Fxx[1].sCmn.sLP.In_LP_State[:])
						psEnc.state_Fxx[1].sCmn.prevLag = 100
						psEnc.state_Fxx[1].sCmn.sNSQ.lagPrev = 100
						psEnc.state_Fxx[1].sShape.LastGainIndex = 10
						psEnc.state_Fxx[1].sCmn.prevSignalType = TYPE_NO_VOICE_ACTIVITY
						psEnc.state_Fxx[1].sCmn.sNSQ.prev_gain_Q16 = 65536
						psEnc.state_Fxx[1].sCmn.first_frame_after_reset = 1
					}
					psEnc.state_Fxx[1].EncodeDoVADFIX(activity)
				} else {
					psEnc.state_Fxx[1].sCmn.VAD_flags[psEnc.state_Fxx[0].sCmn.nFramesEncoded] = 0
				}
				if prefillFlag == 0 {
					StereoEncodePred(psRangeEnc, &psEnc.sStereo.predIx[psEnc.state_Fxx[0].sCmn.nFramesEncoded])
					if psEnc.state_Fxx[1].sCmn.VAD_flags[psEnc.state_Fxx[0].sCmn.nFramesEncoded] == 0 {
						StereoEncodeMidOnly(psRangeEnc, psEnc.sStereo.mid_only_flags[psEnc.state_Fxx[0].sCmn.nFramesEncoded])
					}
				}
			} else {
				/* Buffering */
				copy(psEnc.state_Fxx[0].sCmn.inputBuf[:2], psEnc.sStereo.sMid[:2])
				copy(psEnc.sStereo.sMid[:2], psEnc.state_Fxx[0].sCmn.inputBuf[psEnc.state_Fxx[0].sCmn.frame_length:psEnc.state_Fxx[0].sCmn.frame_length+2])
			}
			psEnc.state_Fxx[0].EncodeDoVADFIX(activity)

			/* Encode */
			for n = 0; n < int(encControl.NChannelsInternal); n++ {
				var maxBits, useCBR int

				/* Handling rate constraints */
				maxBits = encControl.MaxBits
				if tot_blocks == 2 && curr_block == 0 {
					maxBits = maxBits * 3 / 5
				} else if tot_blocks == 3 {
					if curr_block == 0 {
						maxBits = maxBits * 2 / 5
					} else if curr_block == 1 {
						maxBits = maxBits * 3 / 4
					}
				}
				useCBR = b2i(encControl.UseCBR != 0 && curr_block == tot_blocks-1)

				if encControl.NChannelsInternal == 1 {
					channelRate_bps = TargetRate_bps
				} else {
					channelRate_bps = MStargetRates_bps[n]
					if n == 0 && MStargetRates_bps[1] > 0 {
						useCBR = 0
						/* Give mid up to 1/2 of the max bits for that frame */
						maxBits -= encControl.MaxBits / (tot_blocks * 2)
					}
				}

				if channelRate_bps > 0 {
					var condCoding int

					psEnc.state_Fxx[n].sCmn.ControlSNR(channelRate_bps)

					/* Use independent coding if no previous frame available */
					if psEnc.state_Fxx[0].sCmn.nFramesEncoded-n <= 0 {
						condCoding = CODE_INDEPENDENTLY
					} else if n > 0 && psEnc.prev_decode_only_middle != 0 {
						/* If we skipped a side frame in this packet, we don't
						   need LTP scaling; the LTP state is well-defined. */
						condCoding = CODE_INDEPENDENTLY_NO_LTP_SCALING
					} else {
						condCoding = CODE_CONDITIONALLY
					}
					if ret = psEnc.state_Fxx[n].EncodeFrameFIX(nBytesOut, psRangeEnc, condCoding, maxBits, useCBR); ret != 0 {
						silk_assert(false)
					}
				}
				psEnc.state_Fxx[n].sCmn.controlled_since_last_payload = 0
				psEnc.state_Fxx[n].sCmn.inputBufIx = 0
				psEnc.state_Fxx[n].sCmn.nFramesEncoded++
			}
			psEnc.prev_decode_only_middle = int(psEnc.sStereo.mid_only_flags[psEnc.state_Fxx[0].sCmn.nFramesEncoded-1])

			/* Insert VAD and FEC flags at beginning of bitstream */
			if *nBytesOut > 0 && psEnc.state_Fxx[0].sCmn.nFramesEncoded == psEnc.state_Fxx[0].sCmn.nFramesPerPacket {
				flags = 0
				for n = 0; n < int(encControl.NChannelsInternal); n++ {
					for i = 0; i < psEnc.state_Fxx[n].sCmn.nFramesPerPacket; i++ {
						flags = int(LSHIFT(flags, 1))
						flags |= int(psEnc.state_Fxx[n].sCmn.VAD_flags[i])
					}
					flags = int(LSHIFT(flags, 1))
					flags |= int(psEnc.state_Fxx[n].sCmn.LBRR_flag)
				}
				if prefillFlag == 0 {
					psRangeEnc.EncPatchInitialBits(uint32(flags), uint((psEnc.state_Fxx[0].sCmn.nFramesPerPacket+1)*int(encControl.NChannelsInternal)))
				}

				/* Return zero bytes if all channels DTXed */
				if psEnc.state_Fxx[0].sCmn.inDTX != 0 && (encControl.NChannelsInternal == 1 || psEnc.state_Fxx[1].sCmn.inDTX != 0) {
					*nBytesOut = 0
				}

				psEnc.nBitsExceeded += *nBytesOut * 8
				psEnc.nBitsExceeded -= DIV32_16(MUL(encControl.BitRate, encControl.PayloadSize_ms), 1000)
				psEnc.nBitsExceeded = LIMIT(psEnc.nBitsExceeded, 0, 10000)

				/* Update flag indicating if bandwidth switching is allowed */
				speech_act_thr_for_switch_Q8 = int(SMLAWB(SILK_FIX_CONST(SPEECH_ACTIVITY_DTX_THRES, 8),
					SILK_FIX_CONST((1-SPEECH_ACTIVITY_DTX_THRES)/MAX_BANDWIDTH_SWITCH_DELAY_MS, 16+8), psEnc.timeSinceSwitchAllowed_ms))
				if psEnc.state_Fxx[0].sCmn.speech_activity_Q8 < speech_act_thr_for_switch_Q8 {
					psEnc.allowBandwidthSwitch = 1
					psEnc.timeSinceSwitchAllowed_ms = 0
				} else {
					psEnc.allowBandwidthSwitch = 0
					psEnc.timeSinceSwitchAllowed_ms += encControl.PayloadSize_ms
				}
			}

			if nSamplesIn == 0 {
				break
			}
		} else {
			break
		}
		curr_block++
	}

	psEnc.nPrevChannelsInternal = int(encControl.NChannelsInternal)

	encControl.AllowBandwidthSwitch = psEnc.allowBandwidthSwitch
	encControl.InWBmodeWithoutVariableLP = b2i(psEnc.state_Fxx[0].sCmn.fs_kHz == 16 && psEnc.state_Fxx[0].sCmn.sLP.mode == 0)
	encControl.InternalSampleRate = SMULBB(psEnc.state_Fxx[0].sCmn.fs_kHz, 1000)
	if encControl.ToMono != 0 {
		encControl.StereoWidth_Q14 = 0
	} else {
		encControl.StereoWidth_Q14 = int(psEnc.sStereo.smth_width_Q14)
	}
	if prefillFlag != 0 {
		encControl.PayloadSize_ms = tmp_payloadSize_ms
		encControl.Complexity = tmp_complexity
		for n = 0; n < int(encControl.NChannelsInternal); n++ {
			psEnc.state_Fxx[n].sCmn.controlled_since_last_payload = 0
			psEnc.state_Fxx[n].sCmn.prefillFlag = 0
		}
	}

	encControl.SignalType = int(psEnc.state_Fxx[0].sCmn.indices.signalType)
	encControl.Offset = int(Quantization_Offsets_Q10[psEnc.state_Fxx[0].sCmn.indices.signalType>>1][psEnc.state_Fxx[0].sCmn.indices.quantOffsetType])
	return ret
}
