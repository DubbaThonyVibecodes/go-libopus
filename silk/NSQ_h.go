package silk

// C: silk/NSQ.h

// C: static OPUS_INLINE opus_int32 silk_noise_shape_quantizer_short_prediction_c(const opus_int32 *buf32, const opus_int16 *coef16, opus_int order)
// C: #define silk_noise_shape_quantizer_short_prediction(in, coef, coefRev, order, arch)  ((void)arch,silk_noise_shape_quantizer_short_prediction_c(in, coef, order))
//
// The C reads buf32 at negative offsets, so buf32 is passed as the backing
// slice plus the index of the C pointer; coefRev is unused (nil).
func noiseShapeQuantizerShortPrediction(buf32 []int32, buf32_idx int, coef16 []int16, coefRev []int32, order int, arch int) int32 {
	var out int32
	silk_assert(order == 10 || order == 16)

	/* Avoids introducing a bias because silk_SMLAWB() always rounds to -inf */
	out = int32(RSHIFT(order, 1))
	out = SMLAWB(out, buf32[buf32_idx-0], coef16[0])
	out = SMLAWB(out, buf32[buf32_idx-1], coef16[1])
	out = SMLAWB(out, buf32[buf32_idx-2], coef16[2])
	out = SMLAWB(out, buf32[buf32_idx-3], coef16[3])
	out = SMLAWB(out, buf32[buf32_idx-4], coef16[4])
	out = SMLAWB(out, buf32[buf32_idx-5], coef16[5])
	out = SMLAWB(out, buf32[buf32_idx-6], coef16[6])
	out = SMLAWB(out, buf32[buf32_idx-7], coef16[7])
	out = SMLAWB(out, buf32[buf32_idx-8], coef16[8])
	out = SMLAWB(out, buf32[buf32_idx-9], coef16[9])

	if order == 16 {
		out = SMLAWB(out, buf32[buf32_idx-10], coef16[10])
		out = SMLAWB(out, buf32[buf32_idx-11], coef16[11])
		out = SMLAWB(out, buf32[buf32_idx-12], coef16[12])
		out = SMLAWB(out, buf32[buf32_idx-13], coef16[13])
		out = SMLAWB(out, buf32[buf32_idx-14], coef16[14])
		out = SMLAWB(out, buf32[buf32_idx-15], coef16[15])
	}
	return out
}

// C: static OPUS_INLINE opus_int32 silk_NSQ_noise_shape_feedback_loop_c(const opus_int32 *data0, opus_int32 *data1, const opus_int16 *coef, opus_int order)
// C: #define silk_NSQ_noise_shape_feedback_loop(data0, data1, coef, order, arch)  ((void)arch,silk_NSQ_noise_shape_feedback_loop_c(data0, data1, coef, order))
//
// data0 points at a single value (the caller passes &NSQ->sDiff_shp_Q14).
func nsqNoiseShapeFeedbackLoop(data0 *int32, data1 []int32, coef []int16, order int, arch int) int32 {
	var out int32
	var tmp1, tmp2 int32
	var j int

	tmp2 = *data0
	tmp1 = data1[0]
	data1[0] = tmp2

	out = int32(RSHIFT(order, 1))
	out = SMLAWB(out, tmp2, coef[0])

	for j = 2; j < order; j += 2 {
		tmp2 = data1[j-1]
		data1[j-1] = tmp1
		out = SMLAWB(out, tmp1, coef[j-1])
		tmp1 = data1[j+0]
		data1[j+0] = tmp2
		out = SMLAWB(out, tmp2, coef[j])
	}
	data1[order-1] = tmp1
	out = SMLAWB(out, tmp1, coef[order-1])
	/* Q11 -> Q12 */
	out = LSHIFT32(out, 1)
	return out
}
