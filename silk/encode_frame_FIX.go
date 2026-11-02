package silk

import (
	"unsafe"

	"github.com/DubbaThonyVibecodes/go-libopus/celt"
)

// C: silk/fixed/encode_frame_FIX.c

// C: void silk_encode_do_VAD_FIX(silk_encoder_state_FIX *psEnc, opus_int activity)
func (psEnc *EncoderStateFIX) EncodeDoVADFIX(activity int) {
	var activity_threshold int = int(SILK_FIX_CONST(SPEECH_ACTIVITY_DTX_THRES, 8))

	/****************************/
	/* Voice Activity Detection */
	/****************************/
	psEnc.sCmn.VADGetSAQ8(psEnc.sCmn.inputBuf[1:], psEnc.sCmn.arch)
	/* If Opus VAD is inactive and Silk VAD is active: lower Silk VAD to just under the threshold */
	if activity == VAD_NO_ACTIVITY && psEnc.sCmn.speech_activity_Q8 >= activity_threshold {
		psEnc.sCmn.speech_activity_Q8 = activity_threshold - 1
	}

	/**************************************************/
	/* Convert speech activity into VAD and DTX flags */
	/**************************************************/
	if psEnc.sCmn.speech_activity_Q8 < activity_threshold {
		psEnc.sCmn.indices.signalType = TYPE_NO_VOICE_ACTIVITY
		psEnc.sCmn.noSpeechCounter++
		if psEnc.sCmn.noSpeechCounter <= NB_SPEECH_FRAMES_BEFORE_DTX {
			psEnc.sCmn.inDTX = 0
		} else if psEnc.sCmn.noSpeechCounter > MAX_CONSECUTIVE_DTX+NB_SPEECH_FRAMES_BEFORE_DTX {
			psEnc.sCmn.noSpeechCounter = NB_SPEECH_FRAMES_BEFORE_DTX
			psEnc.sCmn.inDTX = 0
		}
		psEnc.sCmn.VAD_flags[psEnc.sCmn.nFramesEncoded] = 0
	} else {
		psEnc.sCmn.noSpeechCounter = 0
		psEnc.sCmn.inDTX = 0
		psEnc.sCmn.indices.signalType = TYPE_UNVOICED
		psEnc.sCmn.VAD_flags[psEnc.sCmn.nFramesEncoded] = 1
	}
}

/****************/
/* Encode frame */
/****************/
//
// C: opus_int silk_encode_frame_FIX(silk_encoder_state_FIX *psEnc, opus_int32 *pnBytesOut, ec_enc *psRangeEnc, opus_int condCoding, opus_int maxBits, opus_int useCBR)
func (psEnc *EncoderStateFIX) EncodeFrameFIX(pnBytesOut *int32, psRangeEnc *celt.EcEnc, condCoding int, maxBits int, useCBR int) int {
	var sEncCtrl EncoderControlFIX
	var i, iter, maxIter, found_upper, found_lower int
	var ret int = 0
	var x_frame []int16
	var sRangeEnc_copy, sRangeEnc_copy2 celt.EcEnc
	var sNSQ_copy []NsqState
	var seed_copy, nBits, nBits_lower, nBits_upper, gainMult_lower, gainMult_upper int32
	var gainsID, gainsID_lower, gainsID_upper int32
	var gainMult_Q8 int16
	var ec_prevLagIndex_copy int16
	var ec_prevSignalType_copy int
	var LastGainIndex_copy2 int8
	var gain_lock = [MAX_NB_SUBFR]int{0}
	var best_gain_mult [MAX_NB_SUBFR]int16
	var best_sum [MAX_NB_SUBFR]int
	var bits_margin int

	/* Using ALLOC() instead of a regular stack allocation to minimize real stack use when using the pseudostack.
	   This is useful on some embedded systems. */
	sNSQ_copy = make([]NsqState, 2)

	/* For CBR, 5 bits below budget is close enough. For VBR, allow up to 25% below the cap if we initially busted the budget. */
	if useCBR != 0 {
		bits_margin = 5
	} else {
		bits_margin = maxBits / 4
	}
	/* This is totally unnecessary but many compilers (including gcc) are too dumb to realise it */
	LastGainIndex_copy2, nBits_lower, nBits_upper, gainMult_lower, gainMult_upper = 0, 0, 0, 0, 0

	psEnc.sCmn.indices.Seed = int8(psEnc.sCmn.frameCounter & 3)
	psEnc.sCmn.frameCounter++

	/**************************************************************/
	/* Set up Input Pointers, and insert frame in input buffer   */
	/*************************************************************/
	/* start of frame to encode */
	x_frame = psEnc.x_buf[psEnc.sCmn.ltp_mem_length:]

	/***************************************/
	/* Ensure smooth bandwidth transitions */
	/***************************************/
	psEnc.sCmn.sLP.LPVariableCutoff(psEnc.sCmn.inputBuf[1:], psEnc.sCmn.frame_length)

	/*******************************************/
	/* Copy new frame to front of input buffer */
	/*******************************************/
	copy(x_frame[LA_SHAPE_MS*psEnc.sCmn.fs_kHz:LA_SHAPE_MS*psEnc.sCmn.fs_kHz+psEnc.sCmn.frame_length], psEnc.sCmn.inputBuf[1:1+psEnc.sCmn.frame_length])

	if psEnc.sCmn.prefillFlag == 0 {
		var res_pitch []int16
		var ec_buf_copy []uint8
		var res_pitch_frame []int16

		res_pitch = make([]int16, psEnc.sCmn.la_pitch+psEnc.sCmn.frame_length+
			psEnc.sCmn.ltp_mem_length)
		/* start of pitch LPC residual frame */
		res_pitch_frame = res_pitch[psEnc.sCmn.ltp_mem_length:]

		/*****************************************/
		/* Find pitch lags, initial LPC analysis */
		/*****************************************/
		/* C: x_frame - psEnc->sCmn.ltp_mem_length == psEnc->x_buf */
		psEnc.FindPitchLagsFIX(&sEncCtrl, res_pitch, psEnc.x_buf[:], psEnc.sCmn.arch)

		/************************/
		/* Noise shape analysis */
		/************************/
		psEnc.NoiseShapeAnalysisFIX(&sEncCtrl, res_pitch_frame, x_frame, psEnc.sCmn.arch)

		/***************************************************/
		/* Find linear prediction coefficients (LPC + LTP) */
		/***************************************************/
		/* C: res_pitch_frame == res_pitch + psEnc->sCmn.ltp_mem_length */
		psEnc.FindPredCoefsFIX(&sEncCtrl, res_pitch, psEnc.sCmn.ltp_mem_length, x_frame, condCoding)

		/****************************************/
		/* Process gains                        */
		/****************************************/
		psEnc.ProcessGainsFIX(&sEncCtrl, condCoding)

		/****************************************/
		/* Low Bitrate Redundant Encoding       */
		/****************************************/
		lBRREncodeFIX(psEnc, &sEncCtrl, x_frame, condCoding)

		/* Loop over quantizer and entropy coding to control bitrate */
		maxIter = 6
		gainMult_Q8 = int16(SILK_FIX_CONST(1.0, 8))
		found_lower = 0
		found_upper = 0
		gainsID = GainsID(psEnc.sCmn.indices.GainsIndices[:], psEnc.sCmn.nb_subfr)
		gainsID_lower = -1
		gainsID_upper = -1
		/* Copy part of the input state */
		/* (struct copy; the buf slice keeps pointing at the same backing array, like the C pointer) */
		sRangeEnc_copy = *psRangeEnc
		sNSQ_copy[0] = psEnc.sCmn.sNSQ
		seed_copy = int32(psEnc.sCmn.indices.Seed)
		ec_prevLagIndex_copy = psEnc.sCmn.ec_prevLagIndex
		ec_prevSignalType_copy = psEnc.sCmn.ec_prevSignalType
		ec_buf_copy = make([]uint8, 1275)
		for iter = 0; ; iter++ {
			if gainsID == gainsID_lower {
				nBits = nBits_lower
			} else if gainsID == gainsID_upper {
				nBits = nBits_upper
			} else {
				/* Restore part of the input state */
				if iter > 0 {
					*psRangeEnc = sRangeEnc_copy
					psEnc.sCmn.sNSQ = sNSQ_copy[0]
					psEnc.sCmn.indices.Seed = int8(seed_copy)
					psEnc.sCmn.ec_prevLagIndex = ec_prevLagIndex_copy
					psEnc.sCmn.ec_prevSignalType = ec_prevSignalType_copy
				}

				/*****************************************/
				/* Noise shaping quantization            */
				/*****************************************/
				if psEnc.sCmn.nStatesDelayedDecision > 1 || psEnc.sCmn.warping_Q16 > 0 {
					psEnc.sCmn.NSQDelDec(&psEnc.sCmn.sNSQ, &psEnc.sCmn.indices, x_frame, psEnc.sCmn.pulses[:],
						predCoefFlat(&sEncCtrl.PredCoef_Q12), sEncCtrl.LTPCoef_Q14[:], sEncCtrl.AR_Q13[:], sEncCtrl.HarmShapeGain_Q14[:],
						sEncCtrl.Tilt_Q14[:], sEncCtrl.LF_shp_Q14[:], sEncCtrl.Gains_Q16[:], sEncCtrl.pitchL[:], sEncCtrl.Lambda_Q10, sEncCtrl.LTP_scale_Q14,
						psEnc.sCmn.arch)
				} else {
					psEnc.sCmn.NSQ(&psEnc.sCmn.sNSQ, &psEnc.sCmn.indices, x_frame, psEnc.sCmn.pulses[:],
						predCoefFlat(&sEncCtrl.PredCoef_Q12), sEncCtrl.LTPCoef_Q14[:], sEncCtrl.AR_Q13[:], sEncCtrl.HarmShapeGain_Q14[:],
						sEncCtrl.Tilt_Q14[:], sEncCtrl.LF_shp_Q14[:], sEncCtrl.Gains_Q16[:], sEncCtrl.pitchL[:], sEncCtrl.Lambda_Q10, sEncCtrl.LTP_scale_Q14,
						psEnc.sCmn.arch)
				}

				if iter == maxIter && found_lower == 0 {
					sRangeEnc_copy2 = *psRangeEnc
				}

				/****************************************/
				/* Encode Parameters                    */
				/****************************************/
				psEnc.sCmn.EncodeIndices(psRangeEnc, psEnc.sCmn.nFramesEncoded, 0, condCoding)

				/****************************************/
				/* Encode Excitation Signal             */
				/****************************************/
				EncodePulses(psRangeEnc, int(psEnc.sCmn.indices.signalType), int(psEnc.sCmn.indices.quantOffsetType),
					psEnc.sCmn.pulses[:], psEnc.sCmn.frame_length)

				nBits = int32(psRangeEnc.Tell())

				/* If we still bust after the last iteration, do some damage control. */
				if iter == maxIter && found_lower == 0 && nBits > int32(maxBits) {
					*psRangeEnc = sRangeEnc_copy2

					/* Keep gains the same as the last frame. */
					psEnc.sShape.LastGainIndex = sEncCtrl.lastGainIndexPrev
					for i = 0; i < psEnc.sCmn.nb_subfr; i++ {
						psEnc.sCmn.indices.GainsIndices[i] = 4
					}
					if condCoding != CODE_CONDITIONALLY {
						psEnc.sCmn.indices.GainsIndices[0] = sEncCtrl.lastGainIndexPrev
					}
					psEnc.sCmn.ec_prevLagIndex = ec_prevLagIndex_copy
					psEnc.sCmn.ec_prevSignalType = ec_prevSignalType_copy
					/* Clear all pulses. */
					for i = 0; i < psEnc.sCmn.frame_length; i++ {
						psEnc.sCmn.pulses[i] = 0
					}

					psEnc.sCmn.EncodeIndices(psRangeEnc, psEnc.sCmn.nFramesEncoded, 0, condCoding)

					EncodePulses(psRangeEnc, int(psEnc.sCmn.indices.signalType), int(psEnc.sCmn.indices.quantOffsetType),
						psEnc.sCmn.pulses[:], psEnc.sCmn.frame_length)

					nBits = int32(psRangeEnc.Tell())
				}

				if useCBR == 0 && iter == 0 && nBits <= int32(maxBits) {
					break
				}
			}

			if iter == maxIter {
				if found_lower != 0 && (gainsID == gainsID_lower || nBits > int32(maxBits)) {
					/* Restore output state from earlier iteration that did meet the bitrate budget */
					*psRangeEnc = sRangeEnc_copy2
					silk_assert(sRangeEnc_copy2.RangeBytes() <= 1275)
					copy(psRangeEnc.GetBuffer()[:sRangeEnc_copy2.RangeBytes()], ec_buf_copy[:sRangeEnc_copy2.RangeBytes()])
					psEnc.sCmn.sNSQ = sNSQ_copy[1]
					psEnc.sShape.LastGainIndex = LastGainIndex_copy2
				}
				break
			}

			if nBits > int32(maxBits) {
				if found_lower == 0 && iter >= 2 {
					/* Adjust the quantizer's rate/distortion tradeoff and discard previous "upper" results */
					sEncCtrl.Lambda_Q10 = int(ADD_RSHIFT32(sEncCtrl.Lambda_Q10, sEncCtrl.Lambda_Q10, 1))
					found_upper = 0
					gainsID_upper = -1
				} else {
					found_upper = 1
					nBits_upper = nBits
					gainMult_upper = int32(gainMult_Q8)
					gainsID_upper = gainsID
				}
			} else if nBits < int32(maxBits-bits_margin) {
				found_lower = 1
				nBits_lower = nBits
				gainMult_lower = int32(gainMult_Q8)
				if gainsID != gainsID_lower {
					gainsID_lower = gainsID
					/* Copy part of the output state */
					sRangeEnc_copy2 = *psRangeEnc
					silk_assert(psRangeEnc.RangeBytes() <= 1275)
					copy(ec_buf_copy[:psRangeEnc.RangeBytes()], psRangeEnc.GetBuffer()[:psRangeEnc.RangeBytes()])
					sNSQ_copy[1] = psEnc.sCmn.sNSQ
					LastGainIndex_copy2 = psEnc.sShape.LastGainIndex
				}
			} else {
				/* Close enough */
				break
			}

			if found_lower == 0 && nBits > int32(maxBits) {
				var j int
				for i = 0; i < psEnc.sCmn.nb_subfr; i++ {
					var sum int = 0
					for j = i * psEnc.sCmn.subfr_length; j < (i+1)*psEnc.sCmn.subfr_length; j++ {
						sum += abs(int(psEnc.sCmn.pulses[j]))
					}
					if iter == 0 || (sum < best_sum[i] && gain_lock[i] == 0) {
						best_sum[i] = sum
						best_gain_mult[i] = gainMult_Q8
					} else {
						gain_lock[i] = 1
					}
				}
			}
			if (found_lower & found_upper) == 0 {
				/* Adjust gain according to high-rate rate/distortion curve */
				if nBits > int32(maxBits) {
					gainMult_Q8 = int16(min_32(1024, int32(gainMult_Q8)*3/2))
				} else {
					gainMult_Q8 = int16(max_32(64, int32(gainMult_Q8)*4/5))
				}
			} else {
				/* Adjust gain by interpolating */
				gainMult_Q8 = int16(gainMult_lower + DIV32_16(MUL(gainMult_upper-gainMult_lower, int32(maxBits)-nBits_lower), nBits_upper-nBits_lower))
				/* New gain multiplier must be between 25% and 75% of old range (note that gainMult_upper < gainMult_lower) */
				if int32(gainMult_Q8) > ADD_RSHIFT32(gainMult_lower, gainMult_upper-gainMult_lower, 2) {
					gainMult_Q8 = int16(ADD_RSHIFT32(gainMult_lower, gainMult_upper-gainMult_lower, 2))
				} else if int32(gainMult_Q8) < SUB_RSHIFT32(gainMult_upper, gainMult_upper-gainMult_lower, 2) {
					gainMult_Q8 = int16(SUB_RSHIFT32(gainMult_upper, gainMult_upper-gainMult_lower, 2))
				}
			}

			for i = 0; i < psEnc.sCmn.nb_subfr; i++ {
				var tmp int16
				if gain_lock[i] != 0 {
					tmp = best_gain_mult[i]
				} else {
					tmp = gainMult_Q8
				}
				sEncCtrl.Gains_Q16[i] = LSHIFT_SAT32(SMULWB(sEncCtrl.GainsUnq_Q16[i], tmp), 8)
			}

			/* Quantize gains */
			psEnc.sShape.LastGainIndex = sEncCtrl.lastGainIndexPrev
			GainsQuant(psEnc.sCmn.indices.GainsIndices[:], sEncCtrl.Gains_Q16[:],
				&psEnc.sShape.LastGainIndex, b2i(condCoding == CODE_CONDITIONALLY), psEnc.sCmn.nb_subfr)

			/* Unique identifier of gains vector */
			gainsID = GainsID(psEnc.sCmn.indices.GainsIndices[:], psEnc.sCmn.nb_subfr)
		}
	}

	/* Update input buffer */
	copy(psEnc.x_buf[:psEnc.sCmn.ltp_mem_length+LA_SHAPE_MS*psEnc.sCmn.fs_kHz],
		psEnc.x_buf[psEnc.sCmn.frame_length:psEnc.sCmn.frame_length+psEnc.sCmn.ltp_mem_length+LA_SHAPE_MS*psEnc.sCmn.fs_kHz])

	/* Exit without entropy coding */
	if psEnc.sCmn.prefillFlag != 0 {
		/* No payload */
		*pnBytesOut = 0
		return ret
	}

	/* Parameters needed for next frame */
	psEnc.sCmn.prevLag = sEncCtrl.pitchL[psEnc.sCmn.nb_subfr-1]
	psEnc.sCmn.prevSignalType = psEnc.sCmn.indices.signalType

	/****************************************/
	/* Finalize payload                     */
	/****************************************/
	psEnc.sCmn.first_frame_after_reset = 0
	/* Payload size */
	*pnBytesOut = RSHIFT(int32(psRangeEnc.Tell()+7), 3)

	return ret
}

// Low-Bitrate Redundancy (LBRR) encoding. Reuse all parameters but encode excitation at lower bitrate
//
// C: static OPUS_INLINE void silk_LBRR_encode_FIX(silk_encoder_state_FIX *psEnc, silk_encoder_control_FIX *psEncCtrl, const opus_int16 x16[], opus_int condCoding)
func lBRREncodeFIX(psEnc *EncoderStateFIX, psEncCtrl *EncoderControlFIX, x16 []int16, condCoding int) {
	var TempGains_Q16 [MAX_NB_SUBFR]int32
	psIndices_LBRR := &psEnc.sCmn.indices_LBRR[psEnc.sCmn.nFramesEncoded]
	var sNSQ_LBRR []NsqState

	/*******************************************/
	/* Control use of inband LBRR              */
	/*******************************************/
	if psEnc.sCmn.LBRR_enabled != 0 && psEnc.sCmn.speech_activity_Q8 > int(SILK_FIX_CONST(LBRR_SPEECH_ACTIVITY_THRES, 8)) {
		/* Using ALLOC() instead of a regular stack allocation to minimize real stack use when using the pseudostack.
		   This is useful on some embedded systems. */
		sNSQ_LBRR = make([]NsqState, 1)

		psEnc.sCmn.LBRR_flags[psEnc.sCmn.nFramesEncoded] = 1

		/* Copy noise shaping quantizer state and quantization indices from regular encoding */
		sNSQ_LBRR[0] = psEnc.sCmn.sNSQ
		*psIndices_LBRR = psEnc.sCmn.indices

		/* Save original gains */
		copy(TempGains_Q16[:psEnc.sCmn.nb_subfr], psEncCtrl.Gains_Q16[:psEnc.sCmn.nb_subfr])

		if psEnc.sCmn.nFramesEncoded == 0 || psEnc.sCmn.LBRR_flags[psEnc.sCmn.nFramesEncoded-1] == 0 {
			/* First frame in packet or previous frame not LBRR coded */
			psEnc.sCmn.LBRRprevLastGainIndex = psEnc.sShape.LastGainIndex

			/* Increase Gains to get target LBRR rate */
			psIndices_LBRR.GainsIndices[0] = int8(int(psIndices_LBRR.GainsIndices[0]) + psEnc.sCmn.LBRR_GainIncreases)
			psIndices_LBRR.GainsIndices[0] = int8(min_int(int(psIndices_LBRR.GainsIndices[0]), N_LEVELS_QGAIN-1))
		}

		/* Decode to get gains in sync with decoder         */
		/* Overwrite unquantized gains with quantized gains */
		GainsDequant(psEncCtrl.Gains_Q16[:], psIndices_LBRR.GainsIndices[:],
			&psEnc.sCmn.LBRRprevLastGainIndex, b2i(condCoding == CODE_CONDITIONALLY), psEnc.sCmn.nb_subfr)

		/*****************************************/
		/* Noise shaping quantization            */
		/*****************************************/
		if psEnc.sCmn.nStatesDelayedDecision > 1 || psEnc.sCmn.warping_Q16 > 0 {
			psEnc.sCmn.NSQDelDec(&sNSQ_LBRR[0], psIndices_LBRR, x16,
				psEnc.sCmn.pulses_LBRR[psEnc.sCmn.nFramesEncoded][:], predCoefFlat(&psEncCtrl.PredCoef_Q12), psEncCtrl.LTPCoef_Q14[:],
				psEncCtrl.AR_Q13[:], psEncCtrl.HarmShapeGain_Q14[:], psEncCtrl.Tilt_Q14[:], psEncCtrl.LF_shp_Q14[:],
				psEncCtrl.Gains_Q16[:], psEncCtrl.pitchL[:], psEncCtrl.Lambda_Q10, psEncCtrl.LTP_scale_Q14, psEnc.sCmn.arch)
		} else {
			psEnc.sCmn.NSQ(&sNSQ_LBRR[0], psIndices_LBRR, x16,
				psEnc.sCmn.pulses_LBRR[psEnc.sCmn.nFramesEncoded][:], predCoefFlat(&psEncCtrl.PredCoef_Q12), psEncCtrl.LTPCoef_Q14[:],
				psEncCtrl.AR_Q13[:], psEncCtrl.HarmShapeGain_Q14[:], psEncCtrl.Tilt_Q14[:], psEncCtrl.LF_shp_Q14[:],
				psEncCtrl.Gains_Q16[:], psEncCtrl.pitchL[:], psEncCtrl.Lambda_Q10, psEncCtrl.LTP_scale_Q14, psEnc.sCmn.arch)
		}

		/* Restore original gains */
		copy(psEncCtrl.Gains_Q16[:psEnc.sCmn.nb_subfr], TempGains_Q16[:psEnc.sCmn.nb_subfr])
	}
}

// predCoefFlat returns the C view `psEncCtrl->PredCoef_Q12[ 0 ]` of the
// [2][MAX_LPC_ORDER] coefficient array: a flat slice of 2*MAX_LPC_ORDER
// elements, because silk_NSQ / silk_NSQ_del_dec take
// `const opus_int16 PredCoef_Q12[ 2 * MAX_LPC_ORDER ]` and index the second
// row through the first. The two rows are contiguous in memory exactly as in C.
func predCoefFlat(p *[2][MAX_LPC_ORDER]int16) []int16 {
	return unsafe.Slice(&p[0][0], 2*MAX_LPC_ORDER)
}
