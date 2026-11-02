package silk

// C: silk/fixed/LTP_analysis_filter_FIX.c

// C: void silk_LTP_analysis_filter_FIX(
//
//	opus_int16                      *LTP_res,                               /* O    LTP residual signal of length MAX_NB_SUBFR * ( pre_length + subfr_length )  */
//	const opus_int16                *x,                                     /* I    Pointer to input signal with at least max( pitchL ) preceding samples       */
//	const opus_int16                LTPCoef_Q14[ LTP_ORDER * MAX_NB_SUBFR ],/* I    LTP_ORDER LTP coefficients for each MAX_NB_SUBFR subframe                   */
//	const opus_int                  pitchL[ MAX_NB_SUBFR ],                 /* I    Pitch lag, one for each subframe                                            */
//	const opus_int32                invGains_Q16[ MAX_NB_SUBFR ],           /* I    Inverse quantization gains, one for each subframe                           */
//	const opus_int                  subfr_length,                           /* I    Length of each subframe                                                     */
//	const opus_int                  nb_subfr,                               /* I    Number of subframes                                                         */
//	const opus_int                  pre_length                              /* I    Length of the preceding samples starting at &x[0] for each subframe         */
//
// )
//
// The C x pointer is read behind by up to max( pitchL ) + LTP_ORDER / 2 samples
// ( x_lag_ptr = x_ptr - pitchL[ k ] ), which a Go slice cannot express, so the
// input is passed as the base buffer x plus x_idx, the index of the C x in it.
func LTPAnalysisFilterFIX(LTP_res []int16, x []int16, x_idx int, LTPCoef_Q14 []int16, pitchL []int, invGains_Q16 []int32, subfr_length int, nb_subfr int, pre_length int) {
	/* x_ptr and x_lag_ptr are indices into x */
	var x_ptr, x_lag_ptr int
	var Btmp_Q14 [LTP_ORDER]int16
	var LTP_res_ptr []int16
	var k, i int
	var LTP_est int32

	x_ptr = x_idx
	LTP_res_ptr = LTP_res
	for k = 0; k < nb_subfr; k++ {

		x_lag_ptr = x_ptr - pitchL[k]

		Btmp_Q14[0] = LTPCoef_Q14[k*LTP_ORDER]
		Btmp_Q14[1] = LTPCoef_Q14[k*LTP_ORDER+1]
		Btmp_Q14[2] = LTPCoef_Q14[k*LTP_ORDER+2]
		Btmp_Q14[3] = LTPCoef_Q14[k*LTP_ORDER+3]
		Btmp_Q14[4] = LTPCoef_Q14[k*LTP_ORDER+4]

		/* LTP analysis FIR filter */
		for i = 0; i < subfr_length+pre_length; i++ {
			LTP_res_ptr[i] = x[x_ptr+i]

			/* Long-term prediction */
			LTP_est = SMULBB(x[x_lag_ptr+LTP_ORDER/2], Btmp_Q14[0])
			LTP_est = SMLABB_ovflw(LTP_est, x[x_lag_ptr+1], Btmp_Q14[1])
			LTP_est = SMLABB_ovflw(LTP_est, x[x_lag_ptr+0], Btmp_Q14[2])
			LTP_est = SMLABB_ovflw(LTP_est, x[x_lag_ptr-1], Btmp_Q14[3])
			LTP_est = SMLABB_ovflw(LTP_est, x[x_lag_ptr-2], Btmp_Q14[4])

			LTP_est = RSHIFT_ROUND(LTP_est, 14) /* round and -> Q0*/

			/* Subtract long-term prediction */
			LTP_res_ptr[i] = int16(SAT16(int32(x[x_ptr+i]) - LTP_est))

			/* Scale residual */
			LTP_res_ptr[i] = int16(SMULWB(invGains_Q16[k], LTP_res_ptr[i]))

			x_lag_ptr++
		}

		/* Update pointers */
		LTP_res_ptr = LTP_res_ptr[subfr_length+pre_length:]
		x_ptr += subfr_length
	}
}
