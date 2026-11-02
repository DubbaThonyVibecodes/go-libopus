package silk

// C: silk/stereo_quant_pred.c

// Quantize mid/side predictors
//
// C: void silk_stereo_quant_pred(
//
//	opus_int32                  pred_Q13[],                     /* I/O  Predictors (out: quantized)                 */
//	opus_int8                   ix[ 2 ][ 3 ]                    /* O    Quantization indices                        */
//
// )
func StereoQuantPred(pred_Q13 []int32, ix *[2][3]int8) {
	var i, j, n int
	var low_Q13, step_Q13, lvl_Q13, err_min_Q13, err_Q13 int32
	var quant_pred_Q13 int32 = 0

	/* Quantize */
	for n = 0; n < 2; n++ {
		/* Brute-force search over quantization levels */
		err_min_Q13 = int32_MAX
		for i = 0; i < STEREO_QUANT_TAB_SIZE-1; i++ {
			low_Q13 = int32(stereo_pred_quant_Q13[i])
			step_Q13 = SMULWB(int32(stereo_pred_quant_Q13[i+1])-low_Q13,
				SILK_FIX_CONST(0.5/STEREO_QUANT_SUB_STEPS, 16))
			for j = 0; j < STEREO_QUANT_SUB_STEPS; j++ {
				lvl_Q13 = SMLABB(low_Q13, step_Q13, 2*j+1)
				err_Q13 = abs(pred_Q13[n] - lvl_Q13)
				if err_Q13 < err_min_Q13 {
					err_min_Q13 = err_Q13
					quant_pred_Q13 = lvl_Q13
					ix[n][0] = int8(i)
					ix[n][1] = int8(j)
				} else {
					/* Error increasing, so we're past the optimum */
					goto done
				}
			}
		}
	done:
		ix[n][2] = int8(DIV32_16(ix[n][0], 3))
		ix[n][0] -= ix[n][2] * 3
		pred_Q13[n] = quant_pred_Q13
	}

	/* Subtract second from first predictor (helps when actually applying these) */
	pred_Q13[0] -= pred_Q13[1]
}
