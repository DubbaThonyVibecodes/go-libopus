package silk

import (
	"unsafe"

	"github.com/DubbaThonyVibecodes/go-libopus/celt"
)

// C: silk/fixed/encode_frame_FIX.c

// C: void silk_encode_do_VAD_FIX(silk_encoder_state_FIX *psEnc)
func (psEnc *EncoderStateFIX) EncodeDoVADFIX() {
	/****************************/
	/* Voice Activity Detection */
	/****************************/
	psEnc.sCmn.VADGetSAQ8(psEnc.sCmn.inputBuf[1:], psEnc.sCmn.arch)

	/**************************************************/
	/* Convert speech activity into VAD and DTX flags */
	/**************************************************/
	if psEnc.sCmn.speech_activity_Q8 < int(SILK_FIX_CONST(SPEECH_ACTIVITY_DTX_THRES, 8)) {
		psEnc.sCmn.indices.signalType = TYPE_NO_VOICE_ACTIVITY
		psEnc.sCmn.noSpeechCounter++
		if psEnc.sCmn.noSpeechCounter < NB_SPEECH_FRAMES_BEFORE_DTX {
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
	var sNSQ_copy, sNSQ_copy2 NsqState
	var seed_copy, nBits, nBits_lower, nBits_upper, gainMult_lower, gainMult_upper int32
	var gainsID, gainsID_lower, gainsID_upper int32
	var gainMult_Q8 int16
	var ec_prevLagIndex_copy int16
	var ec_prevSignalType_copy int
	var LastGainIndex_copy2 int8

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
		var xfw_Q3 []int32
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
		psEnc.FindPitchLagsFIX(&sEncCtrl, res_pitch, x_frame, psEnc.sCmn.arch)

		/************************/
		/* Noise shape analysis */
		/************************/
		psEnc.NoiseShapeAnalysisFIX(&sEncCtrl, res_pitch_frame, x_frame, psEnc.sCmn.arch)

		/***************************************************/
		/* Find linear prediction coefficients (LPC + LTP) */
		/***************************************************/
		psEnc.FindPredCoefsFIX(&sEncCtrl, res_pitch, x_frame, condCoding)

		/****************************************/
		/* Process gains                        */
		/****************************************/
		psEnc.ProcessGainsFIX(&sEncCtrl, condCoding)

		/*****************************************/
		/* Prefiltering for noise shaper         */
		/*****************************************/
		xfw_Q3 = make([]int32, psEnc.sCmn.frame_length)
		psEnc.PrefilterFIX(&sEncCtrl, xfw_Q3, x_frame)

		/****************************************/
		/* Low Bitrate Redundant Encoding       */
		/****************************************/
		lBRREncodeFIX(psEnc, &sEncCtrl, xfw_Q3, condCoding)

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
		sNSQ_copy = psEnc.sCmn.sNSQ
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
					psEnc.sCmn.sNSQ = sNSQ_copy
					psEnc.sCmn.indices.Seed = int8(seed_copy)
					psEnc.sCmn.ec_prevLagIndex = ec_prevLagIndex_copy
					psEnc.sCmn.ec_prevSignalType = ec_prevSignalType_copy
				}

				/*****************************************/
				/* Noise shaping quantization            */
				/*****************************************/
				if psEnc.sCmn.nStatesDelayedDecision > 1 || psEnc.sCmn.warping_Q16 > 0 {
					psEnc.sCmn.NSQDelDec(&psEnc.sCmn.sNSQ, &psEnc.sCmn.indices, xfw_Q3, psEnc.sCmn.pulses[:],
						predCoefFlat(&sEncCtrl.PredCoef_Q12), sEncCtrl.LTPCoef_Q14[:], sEncCtrl.AR2_Q13[:], sEncCtrl.HarmShapeGain_Q14[:],
						sEncCtrl.Tilt_Q14[:], sEncCtrl.LF_shp_Q14[:], sEncCtrl.Gains_Q16[:], sEncCtrl.pitchL[:], sEncCtrl.Lambda_Q10, sEncCtrl.LTP_scale_Q14,
						psEnc.sCmn.arch)
				} else {
					psEnc.sCmn.NSQ(&psEnc.sCmn.sNSQ, &psEnc.sCmn.indices, xfw_Q3, psEnc.sCmn.pulses[:],
						predCoefFlat(&sEncCtrl.PredCoef_Q12), sEncCtrl.LTPCoef_Q14[:], sEncCtrl.AR2_Q13[:], sEncCtrl.HarmShapeGain_Q14[:],
						sEncCtrl.Tilt_Q14[:], sEncCtrl.LF_shp_Q14[:], sEncCtrl.Gains_Q16[:], sEncCtrl.pitchL[:], sEncCtrl.Lambda_Q10, sEncCtrl.LTP_scale_Q14,
						psEnc.sCmn.arch)
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
					psEnc.sCmn.sNSQ = sNSQ_copy2
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
			} else if nBits < int32(maxBits)-5 {
				found_lower = 1
				nBits_lower = nBits
				gainMult_lower = int32(gainMult_Q8)
				if gainsID != gainsID_lower {
					gainsID_lower = gainsID
					/* Copy part of the output state */
					sRangeEnc_copy2 = *psRangeEnc
					silk_assert(psRangeEnc.RangeBytes() <= 1275)
					copy(ec_buf_copy[:psRangeEnc.RangeBytes()], psRangeEnc.GetBuffer()[:psRangeEnc.RangeBytes()])
					sNSQ_copy2 = psEnc.sCmn.sNSQ
					LastGainIndex_copy2 = psEnc.sShape.LastGainIndex
				}
			} else {
				/* Within 5 bits of budget: close enough */
				break
			}

			if (found_lower & found_upper) == 0 {
				/* Adjust gain according to high-rate rate/distortion curve */
				var gain_factor_Q16 int32
				gain_factor_Q16 = Log2lin(LSHIFT(nBits-int32(maxBits), 7)/int32(psEnc.sCmn.frame_length) + SILK_FIX_CONST(16.0, 7))
				gain_factor_Q16 = min_32(gain_factor_Q16, SILK_FIX_CONST(2.0, 16))
				if nBits > int32(maxBits) {
					gain_factor_Q16 = max_32(gain_factor_Q16, SILK_FIX_CONST(1.3, 16))
				}
				gainMult_Q8 = int16(SMULWB(gain_factor_Q16, gainMult_Q8))
			} else {
				/* Adjust gain by interpolating */
				gainMult_Q8 = int16(gainMult_lower + DIV32_16(MUL(gainMult_upper-gainMult_lower, int32(maxBits)-nBits_lower), nBits_upper-nBits_lower))
				/* New gain multplier must be between 25% and 75% of old range (note that gainMult_upper < gainMult_lower) */
				if int32(gainMult_Q8) > ADD_RSHIFT32(gainMult_lower, gainMult_upper-gainMult_lower, 2) {
					gainMult_Q8 = int16(ADD_RSHIFT32(gainMult_lower, gainMult_upper-gainMult_lower, 2))
				} else if int32(gainMult_Q8) < SUB_RSHIFT32(gainMult_upper, gainMult_upper-gainMult_lower, 2) {
					gainMult_Q8 = int16(SUB_RSHIFT32(gainMult_upper, gainMult_upper-gainMult_lower, 2))
				}
			}

			for i = 0; i < psEnc.sCmn.nb_subfr; i++ {
				sEncCtrl.Gains_Q16[i] = LSHIFT_SAT32(SMULWB(sEncCtrl.GainsUnq_Q16[i], gainMult_Q8), 8)
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
// C: static OPUS_INLINE void silk_LBRR_encode_FIX(silk_encoder_state_FIX *psEnc, silk_encoder_control_FIX *psEncCtrl, const opus_int32 xfw_Q3[], opus_int condCoding)
func lBRREncodeFIX(psEnc *EncoderStateFIX, psEncCtrl *EncoderControlFIX, xfw_Q3 []int32, condCoding int) {
	var TempGains_Q16 [MAX_NB_SUBFR]int32
	psIndices_LBRR := &psEnc.sCmn.indices_LBRR[psEnc.sCmn.nFramesEncoded]
	var sNSQ_LBRR NsqState

	/*******************************************/
	/* Control use of inband LBRR              */
	/*******************************************/
	if psEnc.sCmn.LBRR_enabled != 0 && psEnc.sCmn.speech_activity_Q8 > int(SILK_FIX_CONST(LBRR_SPEECH_ACTIVITY_THRES, 8)) {
		psEnc.sCmn.LBRR_flags[psEnc.sCmn.nFramesEncoded] = 1

		/* Copy noise shaping quantizer state and quantization indices from regular encoding */
		sNSQ_LBRR = psEnc.sCmn.sNSQ
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
			psEnc.sCmn.NSQDelDec(&sNSQ_LBRR, psIndices_LBRR, xfw_Q3,
				psEnc.sCmn.pulses_LBRR[psEnc.sCmn.nFramesEncoded][:], predCoefFlat(&psEncCtrl.PredCoef_Q12), psEncCtrl.LTPCoef_Q14[:],
				psEncCtrl.AR2_Q13[:], psEncCtrl.HarmShapeGain_Q14[:], psEncCtrl.Tilt_Q14[:], psEncCtrl.LF_shp_Q14[:],
				psEncCtrl.Gains_Q16[:], psEncCtrl.pitchL[:], psEncCtrl.Lambda_Q10, psEncCtrl.LTP_scale_Q14, psEnc.sCmn.arch)
		} else {
			psEnc.sCmn.NSQ(&sNSQ_LBRR, psIndices_LBRR, xfw_Q3,
				psEnc.sCmn.pulses_LBRR[psEnc.sCmn.nFramesEncoded][:], predCoefFlat(&psEncCtrl.PredCoef_Q12), psEncCtrl.LTPCoef_Q14[:],
				psEncCtrl.AR2_Q13[:], psEncCtrl.HarmShapeGain_Q14[:], psEncCtrl.Tilt_Q14[:], psEncCtrl.LF_shp_Q14[:],
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
