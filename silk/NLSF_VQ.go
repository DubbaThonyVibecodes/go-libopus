package silk

// C: silk/NLSF_VQ.c

// Compute quantization errors for an LPC_order element input vector for a VQ codebook
//
// C: void silk_NLSF_VQ(opus_int32 err_Q24[], const opus_int16 in_Q15[], const opus_uint8 pCB_Q8[], const opus_int16 pWght_Q9[], const opus_int K, const opus_int LPC_order)
func NLSFVQ(
	err_Q24 []int32, // O    Quantization errors [K]
	in_Q15 []int16, // I    Input vectors to be quantized [LPC_order]
	pCB_Q8 []uint8, // I    Codebook vectors [K*LPC_order]
	pWght_Q9 []int16, // I    Codebook weights [K*LPC_order]
	K int, // I    Number of codebook vectors
	LPC_order int, // I    Number of LPCs
) {
	var i, m int
	var diff_Q15, diffw_Q24, sum_error_Q24, pred_Q24 int32
	var w_Q9_ptr []int16
	var cb_Q8_ptr []uint8

	silk_assert((LPC_order & 1) == 0)

	// Loop over codebook
	cb_Q8_ptr = pCB_Q8
	w_Q9_ptr = pWght_Q9
	for i = 0; i < K; i++ {
		sum_error_Q24 = 0
		pred_Q24 = 0
		for m = LPC_order - 2; m >= 0; m -= 2 {
			// Compute weighted absolute predictive quantization error for index m + 1
			diff_Q15 = SUB_LSHIFT32(in_Q15[m+1], int32(cb_Q8_ptr[m+1]), 7) // range: [ -32767 : 32767 ]
			diffw_Q24 = SMULBB(diff_Q15, w_Q9_ptr[m+1])
			sum_error_Q24 = ADD32(sum_error_Q24, abs(SUB_RSHIFT32(diffw_Q24, pred_Q24, 1)))
			pred_Q24 = diffw_Q24

			// Compute weighted absolute predictive quantization error for index m
			diff_Q15 = SUB_LSHIFT32(in_Q15[m], int32(cb_Q8_ptr[m]), 7) // range: [ -32767 : 32767 ]
			diffw_Q24 = SMULBB(diff_Q15, w_Q9_ptr[m])
			sum_error_Q24 = ADD32(sum_error_Q24, abs(SUB_RSHIFT32(diffw_Q24, pred_Q24, 1)))
			pred_Q24 = diffw_Q24

			silk_assert(sum_error_Q24 >= 0)
		}
		err_Q24[i] = sum_error_Q24
		cb_Q8_ptr = cb_Q8_ptr[LPC_order:]
		w_Q9_ptr = w_Q9_ptr[LPC_order:]
	}
}
