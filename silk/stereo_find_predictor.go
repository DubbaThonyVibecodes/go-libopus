package silk

// C: silk/stereo_find_predictor.c

// Find least-squares prediction gain for one signal based on another and quantize it
//
// C: opus_int32 silk_stereo_find_predictor(                          /* O    Returns predictor in Q13                    */
//
//	opus_int32                  *ratio_Q14,                     /* O    Ratio of residual and mid energies          */
//	const opus_int16            x[],                            /* I    Basis signal                                */
//	const opus_int16            y[],                            /* I    Target signal                               */
//	opus_int32                  mid_res_amp_Q0[],               /* I/O  Smoothed mid, residual norms                */
//	opus_int                    length,                         /* I    Number of samples                           */
//	opus_int                    smooth_coef_Q16                 /* I    Smoothing coefficient                       */
//
// )
func StereoFindPredictor(ratio_Q14 *int32, x []int16, y []int16, mid_res_amp_Q0 []int32, length int, smooth_coef_Q16 int) int32 {
	var scale, scale1, scale2 int
	var nrgx, nrgy, corr, pred_Q13, pred2_Q10 int32

	/* Find  predictor */
	SumSqrShift(&nrgx, &scale1, x, length)
	SumSqrShift(&nrgy, &scale2, y, length)
	scale = max_int(scale1, scale2)
	scale = scale + (scale & 1) /* make even */
	nrgy = RSHIFT32(nrgy, scale-scale2)
	nrgx = RSHIFT32(nrgx, scale-scale1)
	nrgx = int32(max_int(int(nrgx), 1))
	corr = InnerProdAlignedScale(x, y, scale, length)
	pred_Q13 = DIV32_varQ(corr, nrgx, 13)
	pred_Q13 = LIMIT(pred_Q13, -(1 << 14), 1<<14)
	pred2_Q10 = SMULWB(pred_Q13, pred_Q13)

	/* Faster update for signals with large prediction parameters */
	smooth_coef_Q16 = max_int(smooth_coef_Q16, int(abs(pred2_Q10)))

	/* Smoothed mid and residual norms */
	silk_assert(smooth_coef_Q16 < 32768)
	scale = RSHIFT(scale, 1)
	mid_res_amp_Q0[0] = SMLAWB(mid_res_amp_Q0[0], LSHIFT(SQRT_APPROX(nrgx), scale)-mid_res_amp_Q0[0],
		smooth_coef_Q16)
	/* Residual energy = nrgy - 2 * pred * corr + pred^2 * nrgx */
	nrgy = SUB_LSHIFT32(nrgy, SMULWB(corr, pred_Q13), 3+1)
	nrgy = ADD_LSHIFT32(nrgy, SMULWB(nrgx, pred2_Q10), 6)
	mid_res_amp_Q0[1] = SMLAWB(mid_res_amp_Q0[1], LSHIFT(SQRT_APPROX(nrgy), scale)-mid_res_amp_Q0[1],
		smooth_coef_Q16)

	/* Ratio of smoothed residual and mid norms */
	*ratio_Q14 = DIV32_varQ(mid_res_amp_Q0[1], max(mid_res_amp_Q0[0], 1), 14)
	*ratio_Q14 = LIMIT(*ratio_Q14, 0, 32767)

	return pred_Q13
}
