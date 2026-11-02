package silk

// C: silk/resampler_private_up2_HQ.c

// Upsample by a factor 2, high quality
// Uses 2nd order allpass filters for the 2x upsampling, followed by a
// notch filter just above Nyquist.
//
// C: void silk_resampler_private_up2_HQ(
//
//	opus_int32                      *S,             /* I/O  Resampler state [ 6 ]       */
//	opus_int16                      *out,           /* O    Output signal [ 2 * len ]   */
//	const opus_int16                *in,            /* I    Input signal [ len ]        */
//	opus_int32                      len             /* I    Number of input samples     */
//
// )
func ResamplerPrivateUp2HQ(S []int32, out []int16, in []int16, len int32) {
	var k int32
	var in32, out32_1, out32_2, Y, X int32

	silk_assert(resampler_up2_hq_0[0] > 0)
	silk_assert(resampler_up2_hq_0[1] > 0)
	silk_assert(resampler_up2_hq_0[2] < 0)
	silk_assert(resampler_up2_hq_1[0] > 0)
	silk_assert(resampler_up2_hq_1[1] > 0)
	silk_assert(resampler_up2_hq_1[2] < 0)

	/* Internal variables and state are in Q10 format */
	for k = 0; k < len; k++ {
		/* Convert to Q10 */
		in32 = LSHIFT(int32(in[k]), 10)

		/* First all-pass section for even output sample */
		Y = SUB32(in32, S[0])
		X = SMULWB(Y, resampler_up2_hq_0[0])
		out32_1 = ADD32(S[0], X)
		S[0] = ADD32(in32, X)

		/* Second all-pass section for even output sample */
		Y = SUB32(out32_1, S[1])
		X = SMULWB(Y, resampler_up2_hq_0[1])
		out32_2 = ADD32(S[1], X)
		S[1] = ADD32(out32_1, X)

		/* Third all-pass section for even output sample */
		Y = SUB32(out32_2, S[2])
		X = SMLAWB(Y, Y, resampler_up2_hq_0[2])
		out32_1 = ADD32(S[2], X)
		S[2] = ADD32(out32_2, X)

		/* Apply gain in Q15, convert back to int16 and store to output */
		out[2*k] = int16(SAT16(RSHIFT_ROUND(out32_1, 10)))

		/* First all-pass section for odd output sample */
		Y = SUB32(in32, S[3])
		X = SMULWB(Y, resampler_up2_hq_1[0])
		out32_1 = ADD32(S[3], X)
		S[3] = ADD32(in32, X)

		/* Second all-pass section for odd output sample */
		Y = SUB32(out32_1, S[4])
		X = SMULWB(Y, resampler_up2_hq_1[1])
		out32_2 = ADD32(S[4], X)
		S[4] = ADD32(out32_1, X)

		/* Third all-pass section for odd output sample */
		Y = SUB32(out32_2, S[5])
		X = SMLAWB(Y, Y, resampler_up2_hq_1[2])
		out32_1 = ADD32(S[5], X)
		S[5] = ADD32(out32_2, X)

		/* Apply gain in Q15, convert back to int16 and store to output */
		out[2*k+1] = int16(SAT16(RSHIFT_ROUND(out32_1, 10)))
	}
}

// C: void silk_resampler_private_up2_HQ_wrapper(
//
//	void                            *SS,            /* I/O  Resampler state (unused)    */
//	opus_int16                      *out,           /* O    Output signal [ 2 * len ]   */
//	const opus_int16                *in,            /* I    Input signal [ len ]        */
//	opus_int32                      len             /* I    Number of input samples     */
//
// )
func (SS *ResamplerStateStruct) ResamplerPrivateUp2HQWrapper(out []int16, in []int16, len int32) {
	S := SS
	ResamplerPrivateUp2HQ(S.sIIR[:], out, in, len)
}
