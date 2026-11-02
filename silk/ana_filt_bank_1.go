package silk

// C: silk/ana_filt_bank_1.c

// Coefficients for 2-band filter bank based on first-order allpass filters
// (C: static opus_int16 A_fb1_20 / A_fb1_21; unexported here)
var a_fb1_20 int16 = 5394 << 1
var a_fb1_21 int16 = -24290 /* (opus_int16)(20623 << 1) */

// Split signal into two decimated bands using first-order allpass filters
//
// C: void silk_ana_filt_bank_1(
//
//	const opus_int16            *in,                /* I    Input signal [N]                                            */
//	opus_int32                  *S,                 /* I/O  State vector [2]                                            */
//	opus_int16                  *outL,              /* O    Low band [N/2]                                              */
//	opus_int16                  *outH,              /* O    High band [N/2]                                             */
//	const opus_int32            N                   /* I    Number of input samples                                     */
//
// )
func AnaFiltBank1(in []int16, S []int32, outL []int16, outH []int16, N int32) {
	var k int
	N2 := int(RSHIFT(N, 1))
	var in32, X, Y, out_1, out_2 int32

	/* Internal variables and state are in Q10 format */
	for k = 0; k < N2; k++ {
		/* Convert to Q10 */
		in32 = LSHIFT(int32(in[2*k]), 10)

		/* All-pass section for even input sample */
		Y = SUB32(in32, S[0])
		X = SMLAWB(Y, Y, a_fb1_21)
		out_1 = ADD32(S[0], X)
		S[0] = ADD32(in32, X)

		/* Convert to Q10 */
		in32 = LSHIFT(int32(in[2*k+1]), 10)

		/* All-pass section for odd input sample, and add to output of previous section */
		Y = SUB32(in32, S[1])
		X = SMULWB(Y, a_fb1_20)
		out_2 = ADD32(S[1], X)
		S[1] = ADD32(in32, X)

		/* Add/subtract, convert back to int16 and store to output */
		outL[k] = int16(SAT16(RSHIFT_ROUND(ADD32(out_2, out_1), 11)))
		outH[k] = int16(SAT16(RSHIFT_ROUND(SUB32(out_2, out_1), 11)))
	}
}
