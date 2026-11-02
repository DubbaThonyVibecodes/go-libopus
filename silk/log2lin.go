package silk

// C: silk/log2lin.c

// Approximation of 2^() (very close inverse of silk_lin2log())
// Convert input to a linear scale
//
// C: opus_int32 silk_log2lin(const opus_int32 inLog_Q7)
func Log2lin(
	inLog_Q7 int32, // I  input on log scale
) int32 {
	var out, frac_Q7 int32

	if inLog_Q7 < 0 {
		return 0
	}

	out = LSHIFT(1, int(RSHIFT(inLog_Q7, 7)))
	frac_Q7 = inLog_Q7 & 0x7F
	if inLog_Q7 < 2048 {
		// Piece-wise parabolic approximation
		out = ADD_RSHIFT32(out, MUL(out, SMLAWB(frac_Q7, SMULBB(frac_Q7, 128-frac_Q7), -174)), 7)
	} else {
		// Piece-wise parabolic approximation
		out = MLA(out, RSHIFT(out, 7), SMLAWB(frac_Q7, SMULBB(frac_Q7, 128-frac_Q7), -174))
	}
	return out
}
