package silk

// C: silk/stereo_MS_to_LR.c

// Convert adaptive Mid/Side representation to Left/Right stereo signal
//
// Buffer convention: exactly as in C, x1 and x2 point at a buffer whose
// first 2 elements are the history samples (element 0 is 2 samples before
// the frame start); the frame occupies x1[ 2 : 2 + frame_length ] on input
// and the converted L/R output is written to x1[ 1 : 1 + frame_length ] /
// x2[ 1 : 1 + frame_length ] (the C caller, dec_API.c, reads it from
// `&samplesOut1_tmp[ n ][ 1 ]`).
//
// C: void silk_stereo_MS_to_LR(
//
//	stereo_dec_state            *state,                         /* I/O  State                                       */
//	opus_int16                  x1[],                           /* I/O  Left input signal, becomes mid signal       */
//	opus_int16                  x2[],                           /* I/O  Right input signal, becomes side signal     */
//	const opus_int32            pred_Q13[],                     /* I    Predictors                                  */
//	opus_int                    fs_kHz,                         /* I    Samples rate (kHz)                          */
//	opus_int                    frame_length                    /* I    Number of samples                           */
//
// )
func (state *StereoDecState) StereoMSToLR(x1 []int16, x2 []int16, pred_Q13 []int32, fs_kHz int, frame_length int) {
	var n, denom_Q16, delta0_Q13, delta1_Q13 int
	var sum, diff, pred0_Q13, pred1_Q13 int32

	/* Buffering */
	copy(x1[:2], state.sMid[:])
	copy(x2[:2], state.sSide[:])
	copy(state.sMid[:], x1[frame_length:frame_length+2])
	copy(state.sSide[:], x2[frame_length:frame_length+2])

	/* Interpolate predictors and add prediction to side channel */
	pred0_Q13 = int32(state.pred_prev_Q13[0])
	pred1_Q13 = int32(state.pred_prev_Q13[1])
	denom_Q16 = int(DIV32_16(int32(1)<<16, STEREO_INTERP_LEN_MS*fs_kHz))
	delta0_Q13 = int(RSHIFT_ROUND(SMULBB(pred_Q13[0]-int32(state.pred_prev_Q13[0]), denom_Q16), 16))
	delta1_Q13 = int(RSHIFT_ROUND(SMULBB(pred_Q13[1]-int32(state.pred_prev_Q13[1]), denom_Q16), 16))
	for n = 0; n < STEREO_INTERP_LEN_MS*fs_kHz; n++ {
		pred0_Q13 += int32(delta0_Q13)
		pred1_Q13 += int32(delta1_Q13)
		sum = LSHIFT(ADD_LSHIFT(int32(x1[n])+int32(x1[n+2]), x1[n+1], 1), 9) /* Q11 */
		sum = SMLAWB(LSHIFT(int32(x2[n+1]), 8), sum, pred0_Q13)              /* Q8  */
		sum = SMLAWB(sum, LSHIFT(int32(x1[n+1]), 11), pred1_Q13)             /* Q8  */
		x2[n+1] = int16(SAT16(RSHIFT_ROUND(sum, 8)))
	}
	pred0_Q13 = pred_Q13[0]
	pred1_Q13 = pred_Q13[1]
	for n = STEREO_INTERP_LEN_MS * fs_kHz; n < frame_length; n++ {
		sum = LSHIFT(ADD_LSHIFT(int32(x1[n])+int32(x1[n+2]), x1[n+1], 1), 9) /* Q11 */
		sum = SMLAWB(LSHIFT(int32(x2[n+1]), 8), sum, pred0_Q13)              /* Q8  */
		sum = SMLAWB(sum, LSHIFT(int32(x1[n+1]), 11), pred1_Q13)             /* Q8  */
		x2[n+1] = int16(SAT16(RSHIFT_ROUND(sum, 8)))
	}
	state.pred_prev_Q13[0] = int16(pred_Q13[0])
	state.pred_prev_Q13[1] = int16(pred_Q13[1])

	/* Convert to left/right signals */
	for n = 0; n < frame_length; n++ {
		sum = int32(x1[n+1]) + int32(x2[n+1])
		diff = int32(x1[n+1]) - int32(x2[n+1])
		x1[n+1] = int16(SAT16(sum))
		x2[n+1] = int16(SAT16(diff))
	}
}
