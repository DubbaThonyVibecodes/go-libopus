package silk

// C: silk/init_decoder.c

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

/************************/
/* Reset Decoder State  */
/************************/
//
// C: opus_int silk_reset_decoder(silk_decoder_state *psDec)
func (psDec *DecoderState) ResetDecoder() int {
	/* Clear the entire encoder state, except anything copied */
	/* C: silk_memset( &psDec->SILK_DECODER_STATE_RESET_START, 0, sizeof( silk_decoder_state ) - offset );
	 * SILK_DECODER_STATE_RESET_START is prev_gain_Q16, which without ENABLE_OSCE is the
	 * first field, so this clears the whole struct. */
	*psDec = DecoderState{}

	/* Used to deactivate LSF interpolation */
	psDec.first_frame_after_reset = 1
	psDec.prev_gain_Q16 = 65536
	psDec.arch = celt.SelectArch()

	/* Reset CNG state */
	psDec.CNGReset()

	/* Reset PLC state */
	psDec.PLCReset()

	return 0
}

/************************/
/* Init Decoder State   */
/************************/
//
// C: opus_int silk_init_decoder(silk_decoder_state *psDec)
func (psDec *DecoderState) InitDecoder() int {
	/* Clear the entire encoder state, except anything copied */
	*psDec = DecoderState{}

	psDec.ResetDecoder()

	return 0
}
