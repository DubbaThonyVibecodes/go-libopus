package silk

// C: silk/resampler_private_AR2.c

// Second order AR filter with single delay elements
//
// C: void silk_resampler_private_AR2(
//
//	opus_int32                      S[],            /* I/O  State vector [ 2 ]          */
//	opus_int32                      out_Q8[],       /* O    Output signal               */
//	const opus_int16                in[],           /* I    Input signal                */
//	const opus_int16                A_Q14[],        /* I    AR coefficients, Q14        */
//	opus_int32                      len             /* I    Signal length               */
//
// )
func ResamplerPrivateAR2(S []int32, out_Q8 []int32, in []int16, A_Q14 []int16, len int32) {
	var k int32
	var out32 int32

	for k = 0; k < len; k++ {
		out32 = ADD_LSHIFT32(S[0], int32(in[k]), 8)
		out_Q8[k] = out32
		out32 = LSHIFT(out32, 2)
		S[0] = SMLAWB(S[1], out32, A_Q14[0])
		S[1] = SMULWB(out32, A_Q14[1])
	}
}
