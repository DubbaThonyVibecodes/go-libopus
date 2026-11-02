package silk

// C: silk/stereo_LR_to_MS.c

// Convert Left/Right stereo signal to adaptive Mid/Side representation
//
// Buffer convention (differs from the C pointer): the C caller passes
// `&inputBuf[ 2 ]` and the function reads/writes x1[ -2 ], x1[ -1 ] and
// x2[ -1 ]. Go cannot index before a slice start, so x1 and x2 MUST be
// slices whose element 0 is 2 samples BEFORE the frame start (the Go caller
// passes `inputBuf[:]`, not `inputBuf[2:]`). The frame itself therefore
// lives at x1[ 2 : 2 + frame_length ]; every C index `x1[ n - 2 ]` is
// `x1[ n ]` here and every C `x2[ n - 1 ]` is `x2[ n + 1 ]`. The C local
// `mid = &x1[ -2 ]` is simply `mid = x1`.
//
// C: void silk_stereo_LR_to_MS(
//
//	stereo_enc_state            *state,                         /* I/O  State                                       */
//	opus_int16                  x1[],                           /* I/O  Left input signal, becomes mid signal       */
//	opus_int16                  x2[],                           /* I/O  Right input signal, becomes side signal     */
//	opus_int8                   ix[ 2 ][ 3 ],                   /* O    Quantization indices                        */
//	opus_int8                   *mid_only_flag,                 /* O    Flag: only mid signal coded                 */
//	opus_int32                  mid_side_rates_bps[],           /* O    Bitrates for mid and side signals           */
//	opus_int32                  total_rate_bps,                 /* I    Total bitrate                               */
//	opus_int                    prev_speech_act_Q8,             /* I    Speech activity level in previous frame     */
//	opus_int                    toMono,                         /* I    Last frame before a stereo->mono transition */
//	opus_int                    fs_kHz,                         /* I    Sample rate (kHz)                           */
//	opus_int                    frame_length                    /* I    Number of samples                           */
//
// )
func (state *StereoEncState) StereoLRToMS(x1 []int16, x2 []int16, ix *[2][3]int8, mid_only_flag *int8, mid_side_rates_bps []int32, total_rate_bps int32, prev_speech_act_Q8 int, toMono int, fs_kHz int, frame_length int) {
	var n, is10msFrame, denom_Q16, delta0_Q13, delta1_Q13 int
	var sum, diff, smooth_coef_Q16, pred0_Q13, pred1_Q13 int32
	var pred_Q13 [2]int32
	var LP_ratio_Q14, HP_ratio_Q14, frac_Q16, frac_3_Q16, min_mid_rate_bps, width_Q14, w_Q24, deltaw_Q24 int32
	var side []int16
	var LP_mid []int16
	var HP_mid []int16
	var LP_side []int16
	var HP_side []int16
	mid := x1 /* C: opus_int16 *mid = &x1[ -2 ]; x1[ 0 ] already is 2 samples before the frame */

	side = make([]int16, frame_length+2)
	/* Convert to basic mid/side signals */
	for n = 0; n < frame_length+2; n++ {
		sum = int32(x1[n]) + int32(x2[n])  /* C: x1[ n - 2 ] + (opus_int32)x2[ n - 2 ] */
		diff = int32(x1[n]) - int32(x2[n]) /* C: x1[ n - 2 ] - (opus_int32)x2[ n - 2 ] */
		mid[n] = int16(RSHIFT_ROUND(sum, 1))
		side[n] = int16(SAT16(RSHIFT_ROUND(diff, 1)))
	}

	/* Buffering */
	copy(mid[:2], state.sMid[:])
	copy(side[:2], state.sSide[:])
	copy(state.sMid[:], mid[frame_length:frame_length+2])
	copy(state.sSide[:], side[frame_length:frame_length+2])

	/* LP and HP filter mid signal */
	LP_mid = make([]int16, frame_length)
	HP_mid = make([]int16, frame_length)
	for n = 0; n < frame_length; n++ {
		sum = RSHIFT_ROUND(ADD_LSHIFT(int32(mid[n])+int32(mid[n+2]), mid[n+1], 1), 2)
		LP_mid[n] = int16(sum)
		HP_mid[n] = int16(int32(mid[n+1]) - sum)
	}

	/* LP and HP filter side signal */
	LP_side = make([]int16, frame_length)
	HP_side = make([]int16, frame_length)
	for n = 0; n < frame_length; n++ {
		sum = RSHIFT_ROUND(ADD_LSHIFT(int32(side[n])+int32(side[n+2]), side[n+1], 1), 2)
		LP_side[n] = int16(sum)
		HP_side[n] = int16(int32(side[n+1]) - sum)
	}

	/* Find energies and predictors */
	is10msFrame = b2i(frame_length == 10*fs_kHz)
	if is10msFrame != 0 {
		smooth_coef_Q16 = SILK_FIX_CONST(STEREO_RATIO_SMOOTH_COEF/2, 16)
	} else {
		smooth_coef_Q16 = SILK_FIX_CONST(STEREO_RATIO_SMOOTH_COEF, 16)
	}
	smooth_coef_Q16 = SMULWB(SMULBB(prev_speech_act_Q8, prev_speech_act_Q8), smooth_coef_Q16)

	pred_Q13[0] = StereoFindPredictor(&LP_ratio_Q14, LP_mid, LP_side, state.mid_side_amp_Q0[0:], frame_length, int(smooth_coef_Q16))
	pred_Q13[1] = StereoFindPredictor(&HP_ratio_Q14, HP_mid, HP_side, state.mid_side_amp_Q0[2:], frame_length, int(smooth_coef_Q16))
	/* Ratio of the norms of residual and mid signals */
	frac_Q16 = SMLABB(HP_ratio_Q14, LP_ratio_Q14, 3)
	frac_Q16 = min(frac_Q16, SILK_FIX_CONST(1.0, 16))

	/* Determine bitrate distribution between mid and side, and possibly reduce stereo width */
	if is10msFrame != 0 { /* Subtract approximate bitrate for coding stereo parameters */
		total_rate_bps -= 1200
	} else {
		total_rate_bps -= 600
	}
	if total_rate_bps < 1 {
		total_rate_bps = 1
	}
	min_mid_rate_bps = SMLABB(2000, fs_kHz, 900)
	silk_assert(min_mid_rate_bps < 32767)
	/* Default bitrate distribution: 8 parts for Mid and (5+3*frac) parts for Side. so: mid_rate = ( 8 / ( 13 + 3 * frac ) ) * total_ rate */
	frac_3_Q16 = MUL(3, frac_Q16)
	mid_side_rates_bps[0] = DIV32_varQ(total_rate_bps, SILK_FIX_CONST(8.0+5, 16)+frac_3_Q16, 16+3)
	/* If Mid bitrate below minimum, reduce stereo width */
	if mid_side_rates_bps[0] < min_mid_rate_bps {
		mid_side_rates_bps[0] = min_mid_rate_bps
		mid_side_rates_bps[1] = total_rate_bps - mid_side_rates_bps[0]
		/* width = 4 * ( 2 * side_rate - min_rate ) / ( ( 1 + 3 * frac ) * min_rate ) */
		width_Q14 = DIV32_varQ(LSHIFT(mid_side_rates_bps[1], 1)-min_mid_rate_bps,
			SMULWB(SILK_FIX_CONST(1.0, 16)+frac_3_Q16, min_mid_rate_bps), 14+2)
		width_Q14 = LIMIT(width_Q14, 0, SILK_FIX_CONST(1.0, 14))
	} else {
		mid_side_rates_bps[1] = total_rate_bps - mid_side_rates_bps[0]
		width_Q14 = SILK_FIX_CONST(1.0, 14)
	}

	/* Smoother */
	state.smth_width_Q14 = int16(SMLAWB(state.smth_width_Q14, width_Q14-int32(state.smth_width_Q14), smooth_coef_Q16))

	/* At very low bitrates or for inputs that are nearly amplitude panned, switch to panned-mono coding */
	*mid_only_flag = 0
	if toMono != 0 {
		/* Last frame before stereo->mono transition; collapse stereo width */
		width_Q14 = 0
		pred_Q13[0] = 0
		pred_Q13[1] = 0
		StereoQuantPred(pred_Q13[:], ix)
	} else if state.width_prev_Q14 == 0 &&
		(8*total_rate_bps < 13*min_mid_rate_bps || SMULWB(frac_Q16, state.smth_width_Q14) < SILK_FIX_CONST(0.05, 14)) {
		/* Code as panned-mono; previous frame already had zero width */
		/* Scale down and quantize predictors */
		pred_Q13[0] = RSHIFT(SMULBB(state.smth_width_Q14, pred_Q13[0]), 14)
		pred_Q13[1] = RSHIFT(SMULBB(state.smth_width_Q14, pred_Q13[1]), 14)
		StereoQuantPred(pred_Q13[:], ix)
		/* Collapse stereo width */
		width_Q14 = 0
		pred_Q13[0] = 0
		pred_Q13[1] = 0
		mid_side_rates_bps[0] = total_rate_bps
		mid_side_rates_bps[1] = 0
		*mid_only_flag = 1
	} else if state.width_prev_Q14 != 0 &&
		(8*total_rate_bps < 11*min_mid_rate_bps || SMULWB(frac_Q16, state.smth_width_Q14) < SILK_FIX_CONST(0.02, 14)) {
		/* Transition to zero-width stereo */
		/* Scale down and quantize predictors */
		pred_Q13[0] = RSHIFT(SMULBB(state.smth_width_Q14, pred_Q13[0]), 14)
		pred_Q13[1] = RSHIFT(SMULBB(state.smth_width_Q14, pred_Q13[1]), 14)
		StereoQuantPred(pred_Q13[:], ix)
		/* Collapse stereo width */
		width_Q14 = 0
		pred_Q13[0] = 0
		pred_Q13[1] = 0
	} else if int32(state.smth_width_Q14) > SILK_FIX_CONST(0.95, 14) {
		/* Full-width stereo coding */
		StereoQuantPred(pred_Q13[:], ix)
		width_Q14 = SILK_FIX_CONST(1.0, 14)
	} else {
		/* Reduced-width stereo coding; scale down and quantize predictors */
		pred_Q13[0] = RSHIFT(SMULBB(state.smth_width_Q14, pred_Q13[0]), 14)
		pred_Q13[1] = RSHIFT(SMULBB(state.smth_width_Q14, pred_Q13[1]), 14)
		StereoQuantPred(pred_Q13[:], ix)
		width_Q14 = int32(state.smth_width_Q14)
	}

	/* Make sure to keep on encoding until the tapered output has been transmitted */
	if *mid_only_flag == 1 {
		state.silent_side_len = int16(int(state.silent_side_len) + frame_length - STEREO_INTERP_LEN_MS*fs_kHz)
		if int(state.silent_side_len) < LA_SHAPE_MS*fs_kHz {
			*mid_only_flag = 0
		} else {
			/* Limit to avoid wrapping around */
			state.silent_side_len = 10000
		}
	} else {
		state.silent_side_len = 0
	}

	if *mid_only_flag == 0 && mid_side_rates_bps[1] < 1 {
		mid_side_rates_bps[1] = 1
		mid_side_rates_bps[0] = int32(max_int(1, int(total_rate_bps-mid_side_rates_bps[1])))
	}

	/* Interpolate predictors and subtract prediction from side channel */
	pred0_Q13 = -int32(state.pred_prev_Q13[0])
	pred1_Q13 = -int32(state.pred_prev_Q13[1])
	w_Q24 = LSHIFT(state.width_prev_Q14, 10)
	denom_Q16 = int(DIV32_16(int32(1)<<16, STEREO_INTERP_LEN_MS*fs_kHz))
	delta0_Q13 = -int(RSHIFT_ROUND(SMULBB(pred_Q13[0]-int32(state.pred_prev_Q13[0]), denom_Q16), 16))
	delta1_Q13 = -int(RSHIFT_ROUND(SMULBB(pred_Q13[1]-int32(state.pred_prev_Q13[1]), denom_Q16), 16))
	deltaw_Q24 = LSHIFT(SMULWB(width_Q14-int32(state.width_prev_Q14), denom_Q16), 10)
	for n = 0; n < STEREO_INTERP_LEN_MS*fs_kHz; n++ {
		pred0_Q13 += int32(delta0_Q13)
		pred1_Q13 += int32(delta1_Q13)
		w_Q24 += deltaw_Q24
		sum = LSHIFT(ADD_LSHIFT(int32(mid[n])+int32(mid[n+2]), mid[n+1], 1), 9) /* Q11 */
		sum = SMLAWB(SMULWB(w_Q24, side[n+1]), sum, pred0_Q13)                  /* Q8  */
		sum = SMLAWB(sum, LSHIFT(int32(mid[n+1]), 11), pred1_Q13)               /* Q8  */
		x2[n+1] = int16(SAT16(RSHIFT_ROUND(sum, 8)))                            /* C: x2[ n - 1 ] */
	}

	pred0_Q13 = -pred_Q13[0]
	pred1_Q13 = -pred_Q13[1]
	w_Q24 = LSHIFT(width_Q14, 10)
	for n = STEREO_INTERP_LEN_MS * fs_kHz; n < frame_length; n++ {
		sum = LSHIFT(ADD_LSHIFT(int32(mid[n])+int32(mid[n+2]), mid[n+1], 1), 9) /* Q11 */
		sum = SMLAWB(SMULWB(w_Q24, side[n+1]), sum, pred0_Q13)                  /* Q8  */
		sum = SMLAWB(sum, LSHIFT(int32(mid[n+1]), 11), pred1_Q13)               /* Q8  */
		x2[n+1] = int16(SAT16(RSHIFT_ROUND(sum, 8)))                            /* C: x2[ n - 1 ] */
	}
	state.pred_prev_Q13[0] = int16(pred_Q13[0])
	state.pred_prev_Q13[1] = int16(pred_Q13[1])
	state.width_prev_Q14 = int16(width_Q14)
}
