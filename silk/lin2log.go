package silk

// C: silk/lin2log.c

// Approximation of 128 * log2() (very close inverse of silk_log2lin())
// Convert input to a log scale
//
// C: opus_int32 silk_lin2log(const opus_int32 inLin)
func Lin2log(
	inLin int32, // I  input in linear scale
) int32 {
	var lz, frac_Q7 int32

	CLZ_FRAC(inLin, &lz, &frac_Q7)

	// Piece-wise parabolic approximation
	return LSHIFT(31-lz, 7) + SMLAWB(frac_Q7, MUL(frac_Q7, 128-frac_Q7), 179)
}
