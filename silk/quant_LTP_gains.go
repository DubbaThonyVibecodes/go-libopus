package silk

// C: silk/quant_LTP_gains.c

// C: void silk_quant_LTP_gains(opus_int16 B_Q14[], opus_int8 cbk_index[], opus_int8 *periodicity_index, const opus_int32 W_Q18[], opus_int mu_Q9, opus_int lowComplexity, const opus_int nb_subfr)
func QuantLTPGains(B_Q14 []int16, cbk_index []int8, periodicity_index *int8, W_Q18 []int32, mu_Q9 int, lowComplexity int, nb_subfr int) {
	var j, k, cbk_size int
	var temp_idx [MAX_NB_SUBFR]int8
	var cl_ptr_Q5 []uint8
	var cbk_ptr_Q7 []int8
	var b_Q14_ptr []int16
	var W_Q18_ptr []int32
	var rate_dist_Q14_subfr, rate_dist_Q14, min_rate_dist_Q14 int32

	/***************************************************/
	/* iterate over different codebooks with different */
	/* rates/distortions, and choose best */
	/***************************************************/
	min_rate_dist_Q14 = int32_MAX
	for k = 0; k < 3; k++ {
		cl_ptr_Q5 = LTP_gain_BITS_Q5_ptrs[k]
		cbk_ptr_Q7 = LTP_vq_ptrs_Q7[k]
		cbk_size = int(LTP_vq_sizes[k])

		/* Set up pointer to first subframe */
		W_Q18_ptr = W_Q18
		b_Q14_ptr = B_Q14

		rate_dist_Q14 = 0
		for j = 0; j < nb_subfr; j++ {
			VQWMatEC(
				&temp_idx[j],         /* O    index of best codebook vector                           */
				&rate_dist_Q14_subfr, /* O    best weighted quantization error + mu * rate            */
				b_Q14_ptr,            /* I    input vector to be quantized                            */
				W_Q18_ptr,            /* I    weighting matrix                                        */
				cbk_ptr_Q7,           /* I    codebook                                                */
				cl_ptr_Q5,            /* I    code length for each codebook vector                    */
				mu_Q9,                /* I    tradeoff between weighted error and rate                */
				cbk_size,             /* I    number of vectors in codebook                           */
			)

			rate_dist_Q14 = ADD_POS_SAT32(rate_dist_Q14, rate_dist_Q14_subfr)

			b_Q14_ptr = b_Q14_ptr[LTP_ORDER:]
			W_Q18_ptr = W_Q18_ptr[LTP_ORDER*LTP_ORDER:]
		}

		/* Avoid never finding a codebook */
		rate_dist_Q14 = min(int32_MAX-1, rate_dist_Q14)

		if rate_dist_Q14 < min_rate_dist_Q14 {
			min_rate_dist_Q14 = rate_dist_Q14
			*periodicity_index = int8(k)
			copy(cbk_index[:nb_subfr], temp_idx[:nb_subfr])
		}

		/* Break early in low-complexity mode if rate distortion is below threshold */
		if lowComplexity != 0 && (rate_dist_Q14 < int32(LTP_gain_middle_avg_RD_Q14)) {
			break
		}
	}

	cbk_ptr_Q7 = LTP_vq_ptrs_Q7[*periodicity_index]
	for j = 0; j < nb_subfr; j++ {
		for k = 0; k < LTP_ORDER; k++ {
			B_Q14[j*LTP_ORDER+k] = int16(LSHIFT(cbk_ptr_Q7[int(cbk_index[j])*LTP_ORDER+k], 7))
		}
	}
}
