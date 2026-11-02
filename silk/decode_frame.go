package silk

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// C: silk/decode_frame.c

/****************/
/* Decode frame */
/****************/
//
// C: opus_int silk_decode_frame(silk_decoder_state *psDec, ec_dec *psRangeDec, opus_int16 pOut[],
// opus_int32 *pN, opus_int lostFlag, opus_int condCoding, int arch)
func (psDec *DecoderState) DecodeFrame(psRangeDec *celt.EcDec, pOut []int16, pN *int32, lostFlag int, condCoding int, arch int) int {
	var psDecCtrl *DecoderControl
	var L, mv_len int
	ret := 0

	L = psDec.frame_length
	psDecCtrl = new(DecoderControl)
	psDecCtrl.LTP_scale_Q14 = 0

	/* Safety checks */
	silk_assert(L > 0 && L <= MAX_FRAME_LENGTH)

	if lostFlag == FLAG_DECODE_NORMAL ||
		(lostFlag == FLAG_DECODE_LBRR && psDec.LBRR_flags[psDec.nFramesDecoded] == 1) {
		var pulses []int16
		pulses = make([]int16, (L+SHELL_CODEC_FRAME_LENGTH-1) &
			^(SHELL_CODEC_FRAME_LENGTH-1))
		/*********************************************/
		/* Decode quantization indices of side info  */
		/*********************************************/
		psDec.DecodeIndices(psRangeDec, psDec.nFramesDecoded, lostFlag, condCoding)

		/*********************************************/
		/* Decode quantization indices of excitation */
		/*********************************************/
		DecodePulses(psRangeDec, pulses, int(psDec.indices.signalType),
			int(psDec.indices.quantOffsetType), psDec.frame_length)

		/********************************************/
		/* Decode parameters and pulse signal       */
		/********************************************/
		psDec.DecodeParameters(psDecCtrl, condCoding)

		/********************************************************/
		/* Run inverse NSQ                                      */
		/********************************************************/
		psDec.DecodeCore(psDecCtrl, pOut, pulses, arch)

		/********************************************************/
		/* Update PLC state                                     */
		/********************************************************/
		psDec.PLC(psDecCtrl, pOut, 0, arch)

		psDec.lossCnt = 0
		psDec.prevSignalType = int(psDec.indices.signalType)
		silk_assert(psDec.prevSignalType >= 0 && psDec.prevSignalType <= 2)

		/* A frame has been decoded without errors */
		psDec.first_frame_after_reset = 0
	} else {
		/* Handle packet loss by extrapolation */
		psDec.indices.signalType = int8(psDec.prevSignalType)
		psDec.PLC(psDecCtrl, pOut, 1, arch)
	}

	/*************************/
	/* Update output buffer. */
	/*************************/
	silk_assert(psDec.ltp_mem_length >= psDec.frame_length)
	mv_len = psDec.ltp_mem_length - psDec.frame_length
	copy(psDec.outBuf[:mv_len], psDec.outBuf[psDec.frame_length:psDec.frame_length+mv_len])
	copy(psDec.outBuf[mv_len:mv_len+psDec.frame_length], pOut[:psDec.frame_length])

	/************************************************/
	/* Comfort noise generation / estimation        */
	/************************************************/
	psDec.CNG(psDecCtrl, pOut, L)

	/****************************************************************/
	/* Ensure smooth connection of extrapolated and good frames     */
	/****************************************************************/
	psDec.PLCGlueFrames(pOut, L)

	/* Update some decoder state variables */
	psDec.lagPrev = psDecCtrl.pitchL[psDec.nb_subfr-1]

	/* Set output frame length */
	*pN = int32(L)

	return ret
}
