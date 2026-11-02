package silk

// C: silk/NLSF_VQ_weights_laroia.c

// R. Laroia, N. Phamdo and N. Farvardin, "Robust and Efficient Quantization of Speech LSP
// Parameters Using Structured Vector Quantization", Proc. IEEE Int. Conf. Acoust., Speech,
// Signal Processing, pp. 641-644, 1991.

// Laroia low complexity NLSF weights
//
// C: void silk_NLSF_VQ_weights_laroia(opus_int16 *pNLSFW_Q_OUT, const opus_int16 *pNLSF_Q15, const opus_int D)
func NLSFVQWeightsLaroia(
	pNLSFW_Q_OUT []int16, // O     Pointer to input vector weights [D]
	pNLSF_Q15 []int16, // I     Pointer to input vector         [D]
	D int, // I     Input vector dimension (even)
) {
	var k int
	var tmp1_int, tmp2_int int32

	silk_assert(D > 0)
	silk_assert((D & 1) == 0)

	// First value
	tmp1_int = int32(max_int(int(pNLSF_Q15[0]), 1))
	tmp1_int = DIV32_16(int32(1)<<(15+NLSF_W_Q), tmp1_int)
	tmp2_int = int32(max_int(int(pNLSF_Q15[1])-int(pNLSF_Q15[0]), 1))
	tmp2_int = DIV32_16(int32(1)<<(15+NLSF_W_Q), tmp2_int)
	pNLSFW_Q_OUT[0] = int16(min_int(int(tmp1_int+tmp2_int), int(int16_MAX)))
	silk_assert(pNLSFW_Q_OUT[0] > 0)

	// Main loop
	for k = 1; k < D-1; k += 2 {
		tmp1_int = int32(max_int(int(pNLSF_Q15[k+1])-int(pNLSF_Q15[k]), 1))
		tmp1_int = DIV32_16(int32(1)<<(15+NLSF_W_Q), tmp1_int)
		pNLSFW_Q_OUT[k] = int16(min_int(int(tmp1_int+tmp2_int), int(int16_MAX)))
		silk_assert(pNLSFW_Q_OUT[k] > 0)

		tmp2_int = int32(max_int(int(pNLSF_Q15[k+2])-int(pNLSF_Q15[k+1]), 1))
		tmp2_int = DIV32_16(int32(1)<<(15+NLSF_W_Q), tmp2_int)
		pNLSFW_Q_OUT[k+1] = int16(min_int(int(tmp1_int+tmp2_int), int(int16_MAX)))
		silk_assert(pNLSFW_Q_OUT[k+1] > 0)
	}

	// Last value
	tmp1_int = int32(max_int((1<<15)-int(pNLSF_Q15[D-1]), 1))
	tmp1_int = DIV32_16(int32(1)<<(15+NLSF_W_Q), tmp1_int)
	pNLSFW_Q_OUT[D-1] = int16(min_int(int(tmp1_int+tmp2_int), int(int16_MAX)))
	silk_assert(pNLSFW_Q_OUT[D-1] > 0)
}
