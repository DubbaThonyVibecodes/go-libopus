package silk

// C: silk/VQ_WMat_EC.c

// Entropy constrained matrix-weighted VQ, hard-coded to 5-element vectors, for a single input data vector
//
// C: void silk_VQ_WMat_EC(opus_int8 *ind, opus_int32 *rate_dist_Q14, const opus_int16 *in_Q14, const opus_int32 *W_Q18, const opus_int8 *cb_Q7, const opus_uint8 *cl_Q5, const opus_int mu_Q9, opus_int L)
func VQWMatEC(ind *int8, rate_dist_Q14 *int32, in_Q14 []int16, W_Q18 []int32, cb_Q7 []int8, cl_Q5 []uint8, mu_Q9 int, L int) {
	var k int
	var cb_row_Q7 []int8
	var diff_Q14 [5]int16
	var sum1_Q14, sum2_Q16 int32

	/* Loop over codebook */
	*rate_dist_Q14 = int32_MAX
	cb_row_Q7 = cb_Q7
	for k = 0; k < L; k++ {
		diff_Q14[0] = int16(int32(in_Q14[0]) - LSHIFT(cb_row_Q7[0], 7))
		diff_Q14[1] = int16(int32(in_Q14[1]) - LSHIFT(cb_row_Q7[1], 7))
		diff_Q14[2] = int16(int32(in_Q14[2]) - LSHIFT(cb_row_Q7[2], 7))
		diff_Q14[3] = int16(int32(in_Q14[3]) - LSHIFT(cb_row_Q7[3], 7))
		diff_Q14[4] = int16(int32(in_Q14[4]) - LSHIFT(cb_row_Q7[4], 7))

		/* Weighted rate */
		sum1_Q14 = SMULBB(mu_Q9, cl_Q5[k])

		silk_assert(sum1_Q14 >= 0)

		/* first row of W_Q18 */
		sum2_Q16 = SMULWB(W_Q18[1], diff_Q14[1])
		sum2_Q16 = SMLAWB(sum2_Q16, W_Q18[2], diff_Q14[2])
		sum2_Q16 = SMLAWB(sum2_Q16, W_Q18[3], diff_Q14[3])
		sum2_Q16 = SMLAWB(sum2_Q16, W_Q18[4], diff_Q14[4])
		sum2_Q16 = LSHIFT(sum2_Q16, 1)
		sum2_Q16 = SMLAWB(sum2_Q16, W_Q18[0], diff_Q14[0])
		sum1_Q14 = SMLAWB(sum1_Q14, sum2_Q16, diff_Q14[0])

		/* second row of W_Q18 */
		sum2_Q16 = SMULWB(W_Q18[7], diff_Q14[2])
		sum2_Q16 = SMLAWB(sum2_Q16, W_Q18[8], diff_Q14[3])
		sum2_Q16 = SMLAWB(sum2_Q16, W_Q18[9], diff_Q14[4])
		sum2_Q16 = LSHIFT(sum2_Q16, 1)
		sum2_Q16 = SMLAWB(sum2_Q16, W_Q18[6], diff_Q14[1])
		sum1_Q14 = SMLAWB(sum1_Q14, sum2_Q16, diff_Q14[1])

		/* third row of W_Q18 */
		sum2_Q16 = SMULWB(W_Q18[13], diff_Q14[3])
		sum2_Q16 = SMLAWB(sum2_Q16, W_Q18[14], diff_Q14[4])
		sum2_Q16 = LSHIFT(sum2_Q16, 1)
		sum2_Q16 = SMLAWB(sum2_Q16, W_Q18[12], diff_Q14[2])
		sum1_Q14 = SMLAWB(sum1_Q14, sum2_Q16, diff_Q14[2])

		/* fourth row of W_Q18 */
		sum2_Q16 = SMULWB(W_Q18[19], diff_Q14[4])
		sum2_Q16 = LSHIFT(sum2_Q16, 1)
		sum2_Q16 = SMLAWB(sum2_Q16, W_Q18[18], diff_Q14[3])
		sum1_Q14 = SMLAWB(sum1_Q14, sum2_Q16, diff_Q14[3])

		/* last row of W_Q18 */
		sum2_Q16 = SMULWB(W_Q18[24], diff_Q14[4])
		sum1_Q14 = SMLAWB(sum1_Q14, sum2_Q16, diff_Q14[4])

		silk_assert(sum1_Q14 >= 0)

		/* find best */
		if sum1_Q14 < *rate_dist_Q14 {
			*rate_dist_Q14 = sum1_Q14
			*ind = int8(k)
		}

		/* Go to next cbk vector */
		cb_row_Q7 = cb_row_Q7[LTP_ORDER:]
	}
}
