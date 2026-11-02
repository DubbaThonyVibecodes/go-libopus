package silk

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// C: silk/stereo_decode_pred.c

// Decode mid/side predictors
//
// C: void silk_stereo_decode_pred(
//
//	ec_dec                      *psRangeDec,                    /* I/O  Compressor data structure                   */
//	opus_int32                  pred_Q13[]                      /* O    Predictors                                  */
//
// )
func StereoDecodePred(psRangeDec *celt.EcDec, pred_Q13 []int32) {
	var n int
	var ix [2][3]int
	var low_Q13, step_Q13 int32

	/* Entropy decoding */
	n = psRangeDec.DecIcdf(stereo_pred_joint_iCDF[:], 8)
	ix[0][2] = int(DIV32_16(n, 5))
	ix[1][2] = n - 5*ix[0][2]
	for n = 0; n < 2; n++ {
		ix[n][0] = psRangeDec.DecIcdf(uniform3_iCDF[:], 8)
		ix[n][1] = psRangeDec.DecIcdf(uniform5_iCDF[:], 8)
	}

	/* Dequantize */
	for n = 0; n < 2; n++ {
		ix[n][0] += 3 * ix[n][2]
		low_Q13 = int32(stereo_pred_quant_Q13[ix[n][0]])
		step_Q13 = SMULWB(int32(stereo_pred_quant_Q13[ix[n][0]+1])-low_Q13,
			SILK_FIX_CONST(0.5/STEREO_QUANT_SUB_STEPS, 16))
		pred_Q13[n] = SMLABB(low_Q13, step_Q13, 2*ix[n][1]+1)
	}

	/* Subtract second from first predictor (helps when actually applying these) */
	pred_Q13[0] -= pred_Q13[1]
}

// Decode mid-only flag
//
// C: void silk_stereo_decode_mid_only(
//
//	ec_dec                      *psRangeDec,                    /* I/O  Compressor data structure                   */
//	opus_int                    *decode_only_mid                /* O    Flag that only mid channel has been coded   */
//
// )
func StereoDecodeMidOnly(psRangeDec *celt.EcDec, decode_only_mid *int) {
	/* Decode flag that only mid channel is coded */
	*decode_only_mid = psRangeDec.DecIcdf(stereo_only_code_mid_iCDF[:], 8)
}
