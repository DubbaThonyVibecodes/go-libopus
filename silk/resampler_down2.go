package silk

// C: silk/resampler_down2.c

// Downsample by a factor 2
//
// C: void silk_resampler_down2(
//
//	opus_int32                  *S,                 /* I/O  State vector [ 2 ]                                          */
//	opus_int16                  *out,               /* O    Output signal [ len ]                                       */
//	const opus_int16            *in,                /* I    Input signal [ floor(len/2) ]                               */
//	opus_int32                  inLen               /* I    Number of input samples                                     */
//
// )
func ResamplerDown2(S []int32, out []int16, in []int16, inLen int32) {
	var k int32
	len2 := RSHIFT32(inLen, 1)
	var in32, out32, Y, X int32

	silk_assert(resampler_down2_0 > 0)
	silk_assert(resampler_down2_1 < 0)

	/* Internal variables and state are in Q10 format */
	for k = 0; k < len2; k++ {
		/* Convert to Q10 */
		in32 = LSHIFT(int32(in[2*k]), 10)

		/* All-pass section for even input sample */
		Y = SUB32(in32, S[0])
		X = SMLAWB(Y, Y, resampler_down2_1)
		out32 = ADD32(S[0], X)
		S[0] = ADD32(in32, X)

		/* Convert to Q10 */
		in32 = LSHIFT(int32(in[2*k+1]), 10)

		/* All-pass section for odd input sample, and add to output of previous section */
		Y = SUB32(in32, S[1])
		X = SMULWB(Y, resampler_down2_0)
		out32 = ADD32(out32, S[1])
		out32 = ADD32(out32, X)
		S[1] = ADD32(in32, X)

		/* Add, convert back to int16 and store to output */
		out[k] = int16(SAT16(RSHIFT_ROUND(out32, 11)))
	}
}
