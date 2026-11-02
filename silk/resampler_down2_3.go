package silk

// C: silk/resampler_down2_3.c

// Downsample by a factor 2/3, low quality
//
// C: void silk_resampler_down2_3(
//
//	opus_int32                  *S,                 /* I/O  State vector [ 6 ]                                          */
//	opus_int16                  *out,               /* O    Output signal [ floor(2*inLen/3) ]                          */
//	const opus_int16            *in,                /* I    Input signal [ inLen ]                                      */
//	opus_int32                  inLen               /* I    Number of input samples                                     */
//
// )
func ResamplerDown23(S []int32, out []int16, in []int16, inLen int32) {
	// C: #define ORDER_FIR 4 (file-local)
	const ORDER_FIR = 4

	var nSamplesIn, counter, res_Q6 int32
	var buf []int32
	var buf_ptr []int32

	buf = make([]int32, RESAMPLER_MAX_BATCH_SIZE_IN+ORDER_FIR)

	/* Copy buffered samples to start of buffer */
	copy(buf[:ORDER_FIR], S[:ORDER_FIR])

	/* Iterate over blocks of frameSizeIn input samples */
	for {
		nSamplesIn = min(inLen, RESAMPLER_MAX_BATCH_SIZE_IN)

		/* Second-order AR filter (output in Q8) */
		ResamplerPrivateAR2(S[ORDER_FIR:], buf[ORDER_FIR:], in,
			Resampler_2_3_COEFS_LQ[:], nSamplesIn)

		/* Interpolate filtered signal */
		buf_ptr = buf
		counter = nSamplesIn
		for counter > 2 {
			/* Inner product */
			res_Q6 = SMULWB(buf_ptr[0], Resampler_2_3_COEFS_LQ[2])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[1], Resampler_2_3_COEFS_LQ[3])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[2], Resampler_2_3_COEFS_LQ[5])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[3], Resampler_2_3_COEFS_LQ[4])

			/* Scale down, saturate and store in output array */
			out[0] = int16(SAT16(RSHIFT_ROUND(res_Q6, 6)))
			out = out[1:]

			res_Q6 = SMULWB(buf_ptr[1], Resampler_2_3_COEFS_LQ[4])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[2], Resampler_2_3_COEFS_LQ[5])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[3], Resampler_2_3_COEFS_LQ[3])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[4], Resampler_2_3_COEFS_LQ[2])

			/* Scale down, saturate and store in output array */
			out[0] = int16(SAT16(RSHIFT_ROUND(res_Q6, 6)))
			out = out[1:]

			buf_ptr = buf_ptr[3:]
			counter -= 3
		}

		in = in[nSamplesIn:]
		inLen -= nSamplesIn

		if inLen > 0 {
			/* More iterations to do; copy last part of filtered signal to beginning of buffer */
			copy(buf[:ORDER_FIR], buf[nSamplesIn:nSamplesIn+ORDER_FIR])
		} else {
			break
		}
	}

	/* Copy last part of filtered signal to the state for the next call */
	copy(S[:ORDER_FIR], buf[nSamplesIn:nSamplesIn+ORDER_FIR])
}
