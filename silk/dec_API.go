package silk

import (
	"unsafe"

	"github.com/DubbaThonyVibecodes/go-libopus/celt"
)

// C: silk/dec_API.c
//
// The decoder super struct silk_decoder is Decoder in structs.go.

/*********************/
/* Decoder functions */
/*********************/

// GetDecoderSize returns the C allocation size for API parity; the Go state
// is allocated with new.
//
// C: opus_int silk_Get_Decoder_Size(opus_int *decSizeBytes)
func GetDecoderSize(decSizeBytes *int) int {
	ret := SILK_NO_ERROR

	*decSizeBytes = int(unsafe.Sizeof(Decoder{}))

	return ret
}

/* Reset decoder state */
//
// C: opus_int silk_InitDecoder(void *decState)
func (decState *Decoder) InitDecoder() int {
	var n int
	ret := SILK_NO_ERROR
	channel_state := decState.channel_state[:]

	for n = 0; n < DECODER_NUM_CHANNELS; n++ {
		ret = channel_state[n].InitDecoder()
	}
	decState.sStereo = StereoDecState{}
	/* Not strictly needed, but it's cleaner that way */
	decState.prev_decode_only_middle = 0

	return ret
}

/* Decode a frame */
//
// C: opus_int silk_Decode(void *decState, silk_DecControlStruct *decControl, opus_int lostFlag,
// opus_int newPacketFlag, ec_dec *psRangeDec, opus_int16 *samplesOut, opus_int32 *nSamplesOut, int arch)
func (decState *Decoder) Decode(decControl *DecControlStruct, lostFlag int, newPacketFlag int, psRangeDec *celt.EcDec, samplesOut []int16, nSamplesOut *int32, arch int) int {
	var i, n int
	decode_only_middle := 0
	ret := SILK_NO_ERROR
	var nSamplesOutDec, LBRR_symbol int32
	var samplesOut1_tmp [2][]int16
	var samplesOut1_tmp_storage1 []int16
	var samplesOut1_tmp_storage2 []int16
	var samplesOut2_tmp []int16
	MS_pred_Q13 := [2]int32{0}
	var resample_out_ptr []int16
	psDec := decState
	channel_state := psDec.channel_state[:]
	var has_side int
	var stereo_to_mono int
	var delay_stack_alloc int

	silk_assert(decControl.NChannelsInternal == 1 || decControl.NChannelsInternal == 2)

	/**********************************/
	/* Test if first frame in payload */
	/**********************************/
	if newPacketFlag != 0 {
		for n = 0; n < int(decControl.NChannelsInternal); n++ {
			channel_state[n].nFramesDecoded = 0 /* Used to count frames in packet */
		}
	}

	/* If Mono -> Stereo transition in bitstream: init state of second channel */
	if int(decControl.NChannelsInternal) > psDec.nChannelsInternal {
		ret += channel_state[1].InitDecoder()
	}

	stereo_to_mono = b2i(decControl.NChannelsInternal == 1 && psDec.nChannelsInternal == 2 &&
		(decControl.InternalSampleRate == 1000*int32(channel_state[0].fs_kHz)))

	if channel_state[0].nFramesDecoded == 0 {
		for n = 0; n < int(decControl.NChannelsInternal); n++ {
			var fs_kHz_dec int
			if decControl.PayloadSize_ms == 0 {
				/* Assuming packet loss, use 10 ms */
				channel_state[n].nFramesPerPacket = 1
				channel_state[n].nb_subfr = 2
			} else if decControl.PayloadSize_ms == 10 {
				channel_state[n].nFramesPerPacket = 1
				channel_state[n].nb_subfr = 2
			} else if decControl.PayloadSize_ms == 20 {
				channel_state[n].nFramesPerPacket = 1
				channel_state[n].nb_subfr = 4
			} else if decControl.PayloadSize_ms == 40 {
				channel_state[n].nFramesPerPacket = 2
				channel_state[n].nb_subfr = 4
			} else if decControl.PayloadSize_ms == 60 {
				channel_state[n].nFramesPerPacket = 3
				channel_state[n].nb_subfr = 4
			} else {
				silk_assert(false)
				return SILK_DEC_INVALID_FRAME_SIZE
			}
			fs_kHz_dec = int(decControl.InternalSampleRate>>10) + 1
			if fs_kHz_dec != 8 && fs_kHz_dec != 12 && fs_kHz_dec != 16 {
				silk_assert(false)
				return SILK_DEC_INVALID_SAMPLING_FREQUENCY
			}
			ret += channel_state[n].DecoderSetFs(fs_kHz_dec, decControl.API_sampleRate)
		}
	}

	if decControl.NChannelsAPI == 2 && decControl.NChannelsInternal == 2 && (psDec.nChannelsAPI == 1 || psDec.nChannelsInternal == 1) {
		clear(psDec.sStereo.pred_prev_Q13[:])
		clear(psDec.sStereo.sSide[:])
		channel_state[1].resampler_state = channel_state[0].resampler_state
	}
	psDec.nChannelsAPI = int(decControl.NChannelsAPI)
	psDec.nChannelsInternal = int(decControl.NChannelsInternal)

	if decControl.API_sampleRate > int32(MAX_API_FS_KHZ)*1000 || decControl.API_sampleRate < 8000 {
		ret = SILK_DEC_INVALID_SAMPLING_FREQUENCY
		return ret
	}

	if lostFlag != FLAG_PACKET_LOST && channel_state[0].nFramesDecoded == 0 {
		/* First decoder call for this payload */
		/* Decode VAD flags and LBRR flag */
		for n = 0; n < int(decControl.NChannelsInternal); n++ {
			for i = 0; i < channel_state[n].nFramesPerPacket; i++ {
				channel_state[n].VAD_flags[i] = psRangeDec.DecBitLogp(1)
			}
			channel_state[n].LBRR_flag = psRangeDec.DecBitLogp(1)
		}
		/* Decode LBRR flags */
		for n = 0; n < int(decControl.NChannelsInternal); n++ {
			clear(channel_state[n].LBRR_flags[:])
			if channel_state[n].LBRR_flag != 0 {
				if channel_state[n].nFramesPerPacket == 1 {
					channel_state[n].LBRR_flags[0] = 1
				} else {
					LBRR_symbol = int32(psRangeDec.DecIcdf(LBRR_flags_iCDF_ptr[channel_state[n].nFramesPerPacket-2], 8)) + 1
					for i = 0; i < channel_state[n].nFramesPerPacket; i++ {
						channel_state[n].LBRR_flags[i] = int(RSHIFT(LBRR_symbol, i)) & 1
					}
				}
			}
		}

		if lostFlag == FLAG_DECODE_NORMAL {
			/* Regular decoding: skip all LBRR data */
			for i = 0; i < channel_state[0].nFramesPerPacket; i++ {
				for n = 0; n < int(decControl.NChannelsInternal); n++ {
					if channel_state[n].LBRR_flags[i] != 0 {
						var pulses [MAX_FRAME_LENGTH]int16
						var condCoding int

						if decControl.NChannelsInternal == 2 && n == 0 {
							StereoDecodePred(psRangeDec, MS_pred_Q13[:])
							if channel_state[1].LBRR_flags[i] == 0 {
								StereoDecodeMidOnly(psRangeDec, &decode_only_middle)
							}
						}
						/* Use conditional coding if previous frame available */
						if i > 0 && channel_state[n].LBRR_flags[i-1] != 0 {
							condCoding = CODE_CONDITIONALLY
						} else {
							condCoding = CODE_INDEPENDENTLY
						}
						channel_state[n].DecodeIndices(psRangeDec, i, 1, condCoding)
						DecodePulses(psRangeDec, pulses[:], int(channel_state[n].indices.signalType),
							int(channel_state[n].indices.quantOffsetType), channel_state[n].frame_length)
					}
				}
			}
		}
	}

	/* Get MS predictor index */
	if decControl.NChannelsInternal == 2 {
		if lostFlag == FLAG_DECODE_NORMAL ||
			(lostFlag == FLAG_DECODE_LBRR && channel_state[0].LBRR_flags[channel_state[0].nFramesDecoded] == 1) {
			StereoDecodePred(psRangeDec, MS_pred_Q13[:])
			/* For LBRR data, decode mid-only flag only if side-channel's LBRR flag is false */
			if (lostFlag == FLAG_DECODE_NORMAL && channel_state[1].VAD_flags[channel_state[0].nFramesDecoded] == 0) ||
				(lostFlag == FLAG_DECODE_LBRR && channel_state[1].LBRR_flags[channel_state[0].nFramesDecoded] == 0) {
				StereoDecodeMidOnly(psRangeDec, &decode_only_middle)
			} else {
				decode_only_middle = 0
			}
		} else {
			for n = 0; n < 2; n++ {
				MS_pred_Q13[n] = int32(psDec.sStereo.pred_prev_Q13[n])
			}
		}
	}

	/* Reset side channel decoder prediction memory for first frame with side coding */
	if decControl.NChannelsInternal == 2 && decode_only_middle == 0 && psDec.prev_decode_only_middle == 1 {
		clear(psDec.channel_state[1].outBuf[:])
		clear(psDec.channel_state[1].sLPC_Q14_buf[:])
		psDec.channel_state[1].lagPrev = 100
		psDec.channel_state[1].LastGainIndex = 10
		psDec.channel_state[1].prevSignalType = TYPE_NO_VOICE_ACTIVITY
		psDec.channel_state[1].first_frame_after_reset = 1
	}

	/* Check if the temp buffer fits into the output PCM buffer. If it fits,
	   we can delay allocating the temp buffer until after the SILK peak stack
	   usage. We need to use a < and not a <= because of the two extra samples. */
	delay_stack_alloc = b2i(decControl.InternalSampleRate*decControl.NChannelsInternal <
		decControl.API_sampleRate*decControl.NChannelsAPI)
	if delay_stack_alloc == 0 {
		samplesOut1_tmp_storage1 = make([]int16,
			int(decControl.NChannelsInternal)*(channel_state[0].frame_length+2))
	}
	if delay_stack_alloc != 0 {
		samplesOut1_tmp[0] = samplesOut
		samplesOut1_tmp[1] = samplesOut[channel_state[0].frame_length+2:]
	} else {
		samplesOut1_tmp[0] = samplesOut1_tmp_storage1
		samplesOut1_tmp[1] = samplesOut1_tmp_storage1[channel_state[0].frame_length+2:]
	}

	if lostFlag == FLAG_DECODE_NORMAL {
		has_side = b2i(decode_only_middle == 0)
	} else {
		has_side = b2i(psDec.prev_decode_only_middle == 0 ||
			(decControl.NChannelsInternal == 2 && lostFlag == FLAG_DECODE_LBRR && channel_state[1].LBRR_flags[channel_state[1].nFramesDecoded] == 1))
	}
	/* Call decoder for one frame */
	for n = 0; n < int(decControl.NChannelsInternal); n++ {
		if n == 0 || has_side != 0 {
			var FrameIndex int
			var condCoding int

			FrameIndex = channel_state[0].nFramesDecoded - n
			/* Use independent coding if no previous frame available */
			if FrameIndex <= 0 {
				condCoding = CODE_INDEPENDENTLY
			} else if lostFlag == FLAG_DECODE_LBRR {
				if channel_state[n].LBRR_flags[FrameIndex-1] != 0 {
					condCoding = CODE_CONDITIONALLY
				} else {
					condCoding = CODE_INDEPENDENTLY
				}
			} else if n > 0 && psDec.prev_decode_only_middle != 0 {
				/* If we skipped a side frame in this packet, we don't
				   need LTP scaling; the LTP state is well-defined. */
				condCoding = CODE_INDEPENDENTLY_NO_LTP_SCALING
			} else {
				condCoding = CODE_CONDITIONALLY
			}
			ret += channel_state[n].DecodeFrame(psRangeDec, samplesOut1_tmp[n][2:], &nSamplesOutDec, lostFlag, condCoding, arch)
		} else {
			clear(samplesOut1_tmp[n][2 : 2+int(nSamplesOutDec)])
		}
		channel_state[n].nFramesDecoded++
	}

	if decControl.NChannelsAPI == 2 && decControl.NChannelsInternal == 2 {
		/* Convert Mid/Side to Left/Right */
		psDec.sStereo.StereoMSToLR(samplesOut1_tmp[0], samplesOut1_tmp[1], MS_pred_Q13[:], channel_state[0].fs_kHz, int(nSamplesOutDec))
	} else {
		/* Buffering */
		copy(samplesOut1_tmp[0][:2], psDec.sStereo.sMid[:])
		copy(psDec.sStereo.sMid[:], samplesOut1_tmp[0][nSamplesOutDec:nSamplesOutDec+2])
	}

	/* Number of output samples */
	*nSamplesOut = DIV32(nSamplesOutDec*decControl.API_sampleRate, SMULBB(channel_state[0].fs_kHz, 1000))

	/* Set up pointers to temp buffers */
	if decControl.NChannelsAPI == 2 {
		samplesOut2_tmp = make([]int16, *nSamplesOut)
	}
	if decControl.NChannelsAPI == 2 {
		resample_out_ptr = samplesOut2_tmp
	} else {
		resample_out_ptr = samplesOut
	}

	if delay_stack_alloc != 0 {
		samplesOut1_tmp_storage2 = make([]int16,
			int(decControl.NChannelsInternal)*(channel_state[0].frame_length+2))
	}
	if delay_stack_alloc != 0 {
		copy(samplesOut1_tmp_storage2, samplesOut[:int(decControl.NChannelsInternal)*(channel_state[0].frame_length+2)])
		samplesOut1_tmp[0] = samplesOut1_tmp_storage2
		samplesOut1_tmp[1] = samplesOut1_tmp_storage2[channel_state[0].frame_length+2:]
	}
	for n = 0; n < int(min(decControl.NChannelsAPI, decControl.NChannelsInternal)); n++ {

		/* Resample decoded signal to API_sampleRate */
		ret += channel_state[n].resampler_state.Resampler(resample_out_ptr, samplesOut1_tmp[n][1:], nSamplesOutDec)

		/* Interleave if stereo output and stereo stream */
		if decControl.NChannelsAPI == 2 {
			for i = 0; i < int(*nSamplesOut); i++ {
				samplesOut[n+2*i] = resample_out_ptr[i]
			}
		}
	}

	/* Create two channel output from mono stream */
	if decControl.NChannelsAPI == 2 && decControl.NChannelsInternal == 1 {
		if stereo_to_mono != 0 {
			/* Resample right channel for newly collapsed stereo just in case
			   we weren't doing collapsing when switching to mono */
			ret += channel_state[1].resampler_state.Resampler(resample_out_ptr, samplesOut1_tmp[0][1:], nSamplesOutDec)

			for i = 0; i < int(*nSamplesOut); i++ {
				samplesOut[1+2*i] = resample_out_ptr[i]
			}
		} else {
			for i = 0; i < int(*nSamplesOut); i++ {
				samplesOut[1+2*i] = samplesOut[0+2*i]
			}
		}
	}

	/* Export pitch lag, measured at 48 kHz sampling rate */
	if channel_state[0].prevSignalType == TYPE_VOICED {
		mult_tab := [3]int{6, 4, 3}
		decControl.PrevPitchLag = channel_state[0].lagPrev * mult_tab[(channel_state[0].fs_kHz-8)>>2]
	} else {
		decControl.PrevPitchLag = 0
	}

	if lostFlag == FLAG_PACKET_LOST {
		/* On packet loss, remove the gain clamping to prevent having the energy "bounce back"
		   if we lose packets when the energy is going down */
		for i = 0; i < psDec.nChannelsInternal; i++ {
			psDec.channel_state[i].LastGainIndex = 10
		}
	} else {
		psDec.prev_decode_only_middle = decode_only_middle
	}
	return ret
}
