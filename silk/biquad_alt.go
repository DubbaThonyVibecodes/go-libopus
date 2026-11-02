package silk

// C: silk/biquad_alt.c
//
// Second order ARMA filter
// Can handle slowly varying filter coefficients

// Second order ARMA filter, alternative implementation
//
// C: void silk_biquad_alt_stride1(
//
//	const opus_int16            *in,                /* I     input signal                                               */
//	const opus_int32            *B_Q28,             /* I     MA coefficients [3]                                        */
//	const opus_int32            *A_Q28,             /* I     AR coefficients [2]                                        */
//	opus_int32                  *S,                 /* I/O   State vector [2]                                           */
//	opus_int16                  *out,               /* O     output signal                                              */
//	const opus_int32            len                 /* I     signal length (must be even)                               */
//
// )
func BiquadAltStride1(in []int16, B_Q28 []int32, A_Q28 []int32, S []int32, out []int16, len int32) {
	/* DIRECT FORM II TRANSPOSED (uses 2 element state vector) */
	var k int
	var inval, A0_U_Q28, A0_L_Q28, A1_U_Q28, A1_L_Q28, out32_Q14 int32

	/* Negate A_Q28 values and split in two parts */
	A0_L_Q28 = (-A_Q28[0]) & 0x00003FFF /* lower part */
	A0_U_Q28 = RSHIFT(-A_Q28[0], 14)    /* upper part */
	A1_L_Q28 = (-A_Q28[1]) & 0x00003FFF /* lower part */
	A1_U_Q28 = RSHIFT(-A_Q28[1], 14)    /* upper part */

	for k = 0; k < int(len); k++ {
		/* S[ 0 ], S[ 1 ]: Q12 */
		inval = int32(in[k])
		out32_Q14 = LSHIFT(SMLAWB(S[0], B_Q28[0], inval), 2)

		S[0] = S[1] + RSHIFT_ROUND(SMULWB(out32_Q14, A0_L_Q28), 14)
		S[0] = SMLAWB(S[0], out32_Q14, A0_U_Q28)
		S[0] = SMLAWB(S[0], B_Q28[1], inval)

		S[1] = RSHIFT_ROUND(SMULWB(out32_Q14, A1_L_Q28), 14)
		S[1] = SMLAWB(S[1], out32_Q14, A1_U_Q28)
		S[1] = SMLAWB(S[1], B_Q28[2], inval)

		/* Scale back to Q0 and saturate */
		out[k] = int16(SAT16(RSHIFT(out32_Q14+(1<<14)-1, 14)))
	}
}

// C: void silk_biquad_alt_stride2_c(
//
//	const opus_int16            *in,                /* I     input signal                                               */
//	const opus_int32            *B_Q28,             /* I     MA coefficients [3]                                        */
//	const opus_int32            *A_Q28,             /* I     AR coefficients [2]                                        */
//	opus_int32                  *S,                 /* I/O   State vector [4]                                           */
//	opus_int16                  *out,               /* O     output signal                                              */
//	const opus_int32            len                 /* I     signal length (must be even)                               */
//
// )
// C: #define silk_biquad_alt_stride2(in, B_Q28, A_Q28, S, out, len, arch) ((void)(arch), silk_biquad_alt_stride2_c(in, B_Q28, A_Q28, S, out, len))
func BiquadAltStride2(in []int16, B_Q28 []int32, A_Q28 []int32, S []int32, out []int16, len int32, arch int) {
	/* DIRECT FORM II TRANSPOSED (uses 2 element state vector) */
	var k int
	var A0_U_Q28, A0_L_Q28, A1_U_Q28, A1_L_Q28 int32
	var out32_Q14 [2]int32

	/* Negate A_Q28 values and split in two parts */
	A0_L_Q28 = (-A_Q28[0]) & 0x00003FFF /* lower part */
	A0_U_Q28 = RSHIFT(-A_Q28[0], 14)    /* upper part */
	A1_L_Q28 = (-A_Q28[1]) & 0x00003FFF /* lower part */
	A1_U_Q28 = RSHIFT(-A_Q28[1], 14)    /* upper part */

	for k = 0; k < int(len); k++ {
		/* S[ 0 ], S[ 1 ], S[ 2 ], S[ 3 ]: Q12 */
		out32_Q14[0] = LSHIFT(SMLAWB(S[0], B_Q28[0], in[2*k+0]), 2)
		out32_Q14[1] = LSHIFT(SMLAWB(S[2], B_Q28[0], in[2*k+1]), 2)

		S[0] = S[1] + RSHIFT_ROUND(SMULWB(out32_Q14[0], A0_L_Q28), 14)
		S[2] = S[3] + RSHIFT_ROUND(SMULWB(out32_Q14[1], A0_L_Q28), 14)
		S[0] = SMLAWB(S[0], out32_Q14[0], A0_U_Q28)
		S[2] = SMLAWB(S[2], out32_Q14[1], A0_U_Q28)
		S[0] = SMLAWB(S[0], B_Q28[1], in[2*k+0])
		S[2] = SMLAWB(S[2], B_Q28[1], in[2*k+1])

		S[1] = RSHIFT_ROUND(SMULWB(out32_Q14[0], A1_L_Q28), 14)
		S[3] = RSHIFT_ROUND(SMULWB(out32_Q14[1], A1_L_Q28), 14)
		S[1] = SMLAWB(S[1], out32_Q14[0], A1_U_Q28)
		S[3] = SMLAWB(S[3], out32_Q14[1], A1_U_Q28)
		S[1] = SMLAWB(S[1], B_Q28[2], in[2*k+0])
		S[3] = SMLAWB(S[3], B_Q28[2], in[2*k+1])

		/* Scale back to Q0 and saturate */
		out[2*k+0] = int16(SAT16(RSHIFT(out32_Q14[0]+(1<<14)-1, 14)))
		out[2*k+1] = int16(SAT16(RSHIFT(out32_Q14[1]+(1<<14)-1, 14)))
	}
}
