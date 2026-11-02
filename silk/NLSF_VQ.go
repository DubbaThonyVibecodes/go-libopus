package silk

// C: silk/NLSF_VQ.c

// Compute quantization errors for an LPC_order element input vector for a VQ codebook
//
// C: void silk_NLSF_VQ(opus_int32 err_Q26[], const opus_int16 in_Q15[], const opus_uint8 pCB_Q8[], const opus_int K, const opus_int LPC_order)
func NLSFVQ(
	err_Q26 []int32, // O    Quantization errors [K]
	in_Q15 []int16, // I    Input vectors to be quantized [LPC_order]
	pCB_Q8 []uint8, // I    Codebook vectors [K*LPC_order]
	K int, // I    Number of codebook vectors
	LPC_order int, // I    Number of LPCs
) {
	var i, m int
	var diff_Q15, sum_error_Q30, sum_error_Q26 int32
	var pCB int // C: *pCB_Q8++ walks the codebook; kept as an index

	silk_assert(LPC_order <= 16)
	silk_assert((LPC_order & 1) == 0)

	// Loop over codebook
	for i = 0; i < K; i++ {
		sum_error_Q26 = 0
		for m = 0; m < LPC_order; m += 2 {
			// Compute weighted squared quantization error for index m
			diff_Q15 = SUB_LSHIFT32(in_Q15[m], int32(pCB_Q8[pCB]), 7) // range: [ -32767 : 32767 ]
			pCB++
			sum_error_Q30 = SMULBB(diff_Q15, diff_Q15)

			// Compute weighted squared quantization error for index m + 1
			diff_Q15 = SUB_LSHIFT32(in_Q15[m+1], int32(pCB_Q8[pCB]), 7) // range: [ -32767 : 32767 ]
			pCB++
			sum_error_Q30 = SMLABB(sum_error_Q30, diff_Q15, diff_Q15)

			sum_error_Q26 = ADD_RSHIFT32(sum_error_Q26, sum_error_Q30, 4)

			silk_assert(sum_error_Q26 >= 0)
			silk_assert(sum_error_Q30 >= 0)
		}
		err_Q26[i] = sum_error_Q26
	}
}
