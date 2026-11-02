package silk

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// C: silk/stereo_encode_pred.c

// Entropy code the mid/side quantization indices
//
// C: void silk_stereo_encode_pred(
//
//	ec_enc                      *psRangeEnc,                    /* I/O  Compressor data structure                   */
//	opus_int8                   ix[ 2 ][ 3 ]                    /* I    Quantization indices                        */
//
// )
func StereoEncodePred(psRangeEnc *celt.EcEnc, ix *[2][3]int8) {
	var n int

	/* Entropy coding */
	n = 5*int(ix[0][2]) + int(ix[1][2])
	silk_assert(n < 25)
	psRangeEnc.EncIcdf(n, stereo_pred_joint_iCDF[:], 8)
	for n = 0; n < 2; n++ {
		silk_assert(ix[n][0] < 3)
		silk_assert(ix[n][1] < STEREO_QUANT_SUB_STEPS)
		psRangeEnc.EncIcdf(int(ix[n][0]), uniform3_iCDF[:], 8)
		psRangeEnc.EncIcdf(int(ix[n][1]), uniform5_iCDF[:], 8)
	}
}

// Entropy code the mid-only flag
//
// C: void silk_stereo_encode_mid_only(
//
//	ec_enc                      *psRangeEnc,                    /* I/O  Compressor data structure                   */
//	opus_int8                   mid_only_flag
//
// )
func StereoEncodeMidOnly(psRangeEnc *celt.EcEnc, mid_only_flag int8) {
	/* Encode flag that only mid channel is coded */
	psRangeEnc.EncIcdf(int(mid_only_flag), stereo_only_code_mid_iCDF[:], 8)
}
