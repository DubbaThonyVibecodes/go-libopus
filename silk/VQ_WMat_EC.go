package silk

// C: silk/VQ_WMat_EC.c

// Entropy constrained matrix-weighted VQ, hard-coded to 5-element vectors, for a single input data vector
//
// C: void silk_VQ_WMat_EC_c(opus_int8 *ind, opus_int32 *res_nrg_Q15, opus_int32 *rate_dist_Q8, opus_int *gain_Q7, const opus_int32 *XX_Q17, const opus_int32 *xX_Q17, const opus_int8 *cb_Q7, const opus_uint8 *cb_gain_Q7, const opus_uint8 *cl_Q5, const opus_int subfr_len, const opus_int32 max_gain_Q7, const opus_int L)
// C: #define silk_VQ_WMat_EC(ind, res_nrg_Q15, rate_dist_Q8, gain_Q7, XX_Q17, xX_Q17, cb_Q7, cb_gain_Q7, cl_Q5, subfr_len, max_gain_Q7, L, arch) ((void)(arch),silk_VQ_WMat_EC_c(...))
func VQWMatEC(ind *int8, res_nrg_Q15 *int32, rate_dist_Q8 *int32, gain_Q7 *int, XX_Q17 []int32, xX_Q17 []int32, cb_Q7 []int8, cb_gain_Q7 []uint8, cl_Q5 []uint8, subfr_len int, max_gain_Q7 int32, L int, arch int) {
	var k, gain_tmp_Q7 int
	var cb_row_Q7 []int8
	var neg_xX_Q24 [5]int32
	var sum1_Q15, sum2_Q24 int32
	var bits_res_Q8, bits_tot_Q8 int32

	/* Negate and convert to new Q domain */
	neg_xX_Q24[0] = -LSHIFT32(xX_Q17[0], 7)
	neg_xX_Q24[1] = -LSHIFT32(xX_Q17[1], 7)
	neg_xX_Q24[2] = -LSHIFT32(xX_Q17[2], 7)
	neg_xX_Q24[3] = -LSHIFT32(xX_Q17[3], 7)
	neg_xX_Q24[4] = -LSHIFT32(xX_Q17[4], 7)

	/* Loop over codebook */
	*rate_dist_Q8 = int32_MAX
	*res_nrg_Q15 = int32_MAX
	cb_row_Q7 = cb_Q7
	/* If things go really bad, at least *ind is set to something safe. */
	*ind = 0
	for k = 0; k < L; k++ {
		var penalty int32
		gain_tmp_Q7 = int(cb_gain_Q7[k])
		/* Weighted rate */
		/* Quantization error: 1 - 2 * xX * cb + cb' * XX * cb */
		sum1_Q15 = SILK_FIX_CONST(1.001, 15)

		/* Penalty for too large gain */
		penalty = LSHIFT32(max(SUB32(gain_tmp_Q7, max_gain_Q7), 0), 11)

		/* first row of XX_Q17 */
		sum2_Q24 = MLA(neg_xX_Q24[0], XX_Q17[1], cb_row_Q7[1])
		sum2_Q24 = MLA(sum2_Q24, XX_Q17[2], cb_row_Q7[2])
		sum2_Q24 = MLA(sum2_Q24, XX_Q17[3], cb_row_Q7[3])
		sum2_Q24 = MLA(sum2_Q24, XX_Q17[4], cb_row_Q7[4])
		sum2_Q24 = LSHIFT32(sum2_Q24, 1)
		sum2_Q24 = MLA(sum2_Q24, XX_Q17[0], cb_row_Q7[0])
		sum1_Q15 = SMLAWB(sum1_Q15, sum2_Q24, cb_row_Q7[0])

		/* second row of XX_Q17 */
		sum2_Q24 = MLA(neg_xX_Q24[1], XX_Q17[7], cb_row_Q7[2])
		sum2_Q24 = MLA(sum2_Q24, XX_Q17[8], cb_row_Q7[3])
		sum2_Q24 = MLA(sum2_Q24, XX_Q17[9], cb_row_Q7[4])
		sum2_Q24 = LSHIFT32(sum2_Q24, 1)
		sum2_Q24 = MLA(sum2_Q24, XX_Q17[6], cb_row_Q7[1])
		sum1_Q15 = SMLAWB(sum1_Q15, sum2_Q24, cb_row_Q7[1])

		/* third row of XX_Q17 */
		sum2_Q24 = MLA(neg_xX_Q24[2], XX_Q17[13], cb_row_Q7[3])
		sum2_Q24 = MLA(sum2_Q24, XX_Q17[14], cb_row_Q7[4])
		sum2_Q24 = LSHIFT32(sum2_Q24, 1)
		sum2_Q24 = MLA(sum2_Q24, XX_Q17[12], cb_row_Q7[2])
		sum1_Q15 = SMLAWB(sum1_Q15, sum2_Q24, cb_row_Q7[2])

		/* fourth row of XX_Q17 */
		sum2_Q24 = MLA(neg_xX_Q24[3], XX_Q17[19], cb_row_Q7[4])
		sum2_Q24 = LSHIFT32(sum2_Q24, 1)
		sum2_Q24 = MLA(sum2_Q24, XX_Q17[18], cb_row_Q7[3])
		sum1_Q15 = SMLAWB(sum1_Q15, sum2_Q24, cb_row_Q7[3])

		/* last row of XX_Q17 */
		sum2_Q24 = LSHIFT32(neg_xX_Q24[4], 1)
		sum2_Q24 = MLA(sum2_Q24, XX_Q17[24], cb_row_Q7[4])
		sum1_Q15 = SMLAWB(sum1_Q15, sum2_Q24, cb_row_Q7[4])

		/* find best */
		if sum1_Q15 >= 0 {
			/* Translate residual energy to bits using high-rate assumption (6 dB ==> 1 bit/sample) */
			bits_res_Q8 = SMULBB(subfr_len, Lin2log(sum1_Q15+penalty)-(15<<7))
			/* In the following line we reduce the codelength component by half ("-1"); seems to slightly improve quality */
			bits_tot_Q8 = ADD_LSHIFT32(bits_res_Q8, cl_Q5[k], 3-1)
			if bits_tot_Q8 <= *rate_dist_Q8 {
				*rate_dist_Q8 = bits_tot_Q8
				*res_nrg_Q15 = sum1_Q15 + penalty
				*ind = int8(k)
				*gain_Q7 = gain_tmp_Q7
			}
		}

		/* Go to next cbk vector */
		cb_row_Q7 = cb_row_Q7[LTP_ORDER:]
	}
}
