package silk

// C: silk/biquad_alt.c
//
// Second order ARMA filter
// Can handle slowly varying filter coefficients

// Second order ARMA filter, alternative implementation
//
// C: void silk_biquad_alt(
//
//	const opus_int16            *in,                /* I     input signal                                               */
//	const opus_int32            *B_Q28,             /* I     MA coefficients [3]                                        */
//	const opus_int32            *A_Q28,             /* I     AR coefficients [2]                                        */
//	opus_int32                  *S,                 /* I/O   State vector [2]                                           */
//	opus_int16                  *out,               /* O     output signal                                              */
//	const opus_int32            len,                /* I     signal length (must be even)                               */
//	opus_int                    stride              /* I     Operate on interleaved signal if > 1                       */
//
// )
func BiquadAlt(in []int16, B_Q28 []int32, A_Q28 []int32, S []int32, out []int16, len int32, stride int) {
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
		inval = int32(in[k*stride])
		out32_Q14 = LSHIFT(SMLAWB(S[0], B_Q28[0], inval), 2)

		S[0] = S[1] + RSHIFT_ROUND(SMULWB(out32_Q14, A0_L_Q28), 14)
		S[0] = SMLAWB(S[0], out32_Q14, A0_U_Q28)
		S[0] = SMLAWB(S[0], B_Q28[1], inval)

		S[1] = RSHIFT_ROUND(SMULWB(out32_Q14, A1_L_Q28), 14)
		S[1] = SMLAWB(S[1], out32_Q14, A1_U_Q28)
		S[1] = SMLAWB(S[1], B_Q28[2], inval)

		/* Scale back to Q0 and saturate */
		out[k*stride] = int16(SAT16(RSHIFT(out32_Q14+(1<<14)-1, 14)))
	}
}
