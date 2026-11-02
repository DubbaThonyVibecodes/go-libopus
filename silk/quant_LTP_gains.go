package silk

// C: silk/quant_LTP_gains.c

// C: void silk_quant_LTP_gains(opus_int16 B_Q14[], opus_int8 cbk_index[], opus_int8 *periodicity_index, opus_int32 *sum_log_gain_Q7, opus_int *pred_gain_dB_Q7, const opus_int32 XX_Q17[], const opus_int32 xX_Q17[], const opus_int subfr_len, const opus_int nb_subfr, int arch)
func QuantLTPGains(
	B_Q14 []int16, // O    Quantized LTP gains
	cbk_index []int8, // O    Codebook Index
	periodicity_index *int8, // O    Periodicity Index
	sum_log_gain_Q7 *int32, // I/O  Cumulative max prediction gain
	pred_gain_dB_Q7 *int, // O    LTP prediction gain
	XX_Q17 []int32, // I    Correlation matrix in Q18
	xX_Q17 []int32, // I    Correlation vector in Q18
	subfr_len int, // I    Number of samples per subframe
	nb_subfr int, // I    Number of subframes
	arch int, // I    Run-time architecture
) {
	var j, k, cbk_size int
	var temp_idx [MAX_NB_SUBFR]int8
	var cl_ptr_Q5 []uint8
	var cbk_ptr_Q7 []int8
	var cbk_gain_ptr_Q7 []uint8
	var XX_Q17_ptr, xX_Q17_ptr []int32
	var res_nrg_Q15_subfr, res_nrg_Q15, rate_dist_Q7_subfr, rate_dist_Q7, min_rate_dist_Q7 int32
	var sum_log_gain_tmp_Q7, best_sum_log_gain_Q7, max_gain_Q7 int32
	var gain_Q7 int

	/***************************************************/
	/* iterate over different codebooks with different */
	/* rates/distortions, and choose best */
	/***************************************************/
	min_rate_dist_Q7 = int32_MAX
	best_sum_log_gain_Q7 = 0
	for k = 0; k < 3; k++ {
		/* Safety margin for pitch gain control, to take into account factors
		   such as state rescaling/rewhitening. */
		var gain_safety int32 = SILK_FIX_CONST(0.4, 7)

		cl_ptr_Q5 = LTP_gain_BITS_Q5_ptrs[k]
		cbk_ptr_Q7 = LTP_vq_ptrs_Q7[k]
		cbk_gain_ptr_Q7 = LTP_vq_gain_ptrs_Q7[k]
		cbk_size = int(LTP_vq_sizes[k])

		/* Set up pointers to first subframe */
		XX_Q17_ptr = XX_Q17
		xX_Q17_ptr = xX_Q17

		res_nrg_Q15 = 0
		rate_dist_Q7 = 0
		sum_log_gain_tmp_Q7 = *sum_log_gain_Q7
		for j = 0; j < nb_subfr; j++ {
			max_gain_Q7 = Log2lin((SILK_FIX_CONST(MAX_SUM_LOG_GAIN_DB/6.0, 7)-sum_log_gain_tmp_Q7)+
				SILK_FIX_CONST(7.0, 7)) - gain_safety
			VQWMatEC(
				&temp_idx[j],        /* O    index of best codebook vector                           */
				&res_nrg_Q15_subfr,  /* O    residual energy                                         */
				&rate_dist_Q7_subfr, /* O    best weighted quantization error + mu * rate            */
				&gain_Q7,            /* O    sum of absolute LTP coefficients                        */
				XX_Q17_ptr,          /* I    correlation matrix                                      */
				xX_Q17_ptr,          /* I    correlation vector                                      */
				cbk_ptr_Q7,          /* I    codebook                                                */
				cbk_gain_ptr_Q7,     /* I    codebook effective gains                                */
				cl_ptr_Q5,           /* I    code length for each codebook vector                    */
				subfr_len,           /* I    number of samples per subframe                          */
				max_gain_Q7,         /* I    maximum sum of absolute LTP coefficients                */
				cbk_size,            /* I    number of vectors in codebook                           */
				arch,                /* I    Run-time architecture                                   */
			)

			res_nrg_Q15 = ADD_POS_SAT32(res_nrg_Q15, res_nrg_Q15_subfr)
			rate_dist_Q7 = ADD_POS_SAT32(rate_dist_Q7, rate_dist_Q7_subfr)
			sum_log_gain_tmp_Q7 = max(0, sum_log_gain_tmp_Q7+
				Lin2log(gain_safety+int32(gain_Q7))-SILK_FIX_CONST(7.0, 7))

			XX_Q17_ptr = XX_Q17_ptr[LTP_ORDER*LTP_ORDER:]
			xX_Q17_ptr = xX_Q17_ptr[LTP_ORDER:]
		}

		if rate_dist_Q7 <= min_rate_dist_Q7 {
			min_rate_dist_Q7 = rate_dist_Q7
			*periodicity_index = int8(k)
			copy(cbk_index[:nb_subfr], temp_idx[:nb_subfr])
			best_sum_log_gain_Q7 = sum_log_gain_tmp_Q7
		}
	}

	cbk_ptr_Q7 = LTP_vq_ptrs_Q7[*periodicity_index]
	for j = 0; j < nb_subfr; j++ {
		for k = 0; k < LTP_ORDER; k++ {
			B_Q14[j*LTP_ORDER+k] = int16(LSHIFT(cbk_ptr_Q7[int(cbk_index[j])*LTP_ORDER+k], 7))
		}
	}

	if nb_subfr == 2 {
		res_nrg_Q15 = RSHIFT32(res_nrg_Q15, 1)
	} else {
		res_nrg_Q15 = RSHIFT32(res_nrg_Q15, 2)
	}

	*sum_log_gain_Q7 = best_sum_log_gain_Q7
	*pred_gain_dB_Q7 = int(SMULBB(-3, Lin2log(res_nrg_Q15)-(15<<7)))
}
