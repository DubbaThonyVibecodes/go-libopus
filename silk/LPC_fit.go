package silk

// C: silk/LPC_fit.c

// Convert int32 coefficients to int16 coefs and make sure there's no wrap-around.
// This logic is reused in _celt_lpc(). Any bug fixes should also be applied there.
//
// C: void silk_LPC_fit(opus_int16 *a_QOUT, opus_int32 *a_QIN, const opus_int QOUT, const opus_int QIN, const opus_int d)
func LPCFit(
	a_QOUT []int16, // O    Output signal
	a_QIN []int32, // I/O  Input signal
	QOUT int, // I    Input Q domain
	QIN int, // I    Input Q domain
	d int, // I    Filter order
) {
	var i, k int
	idx := 0
	var maxabs, absval, chirp_Q16 int32

	/* Limit the maximum absolute value of the prediction coefficients, so that they'll fit in int16 */
	for i = 0; i < 10; i++ {
		/* Find maximum absolute value and its index */
		maxabs = 0
		for k = 0; k < d; k++ {
			absval = abs(a_QIN[k])
			if absval > maxabs {
				maxabs = absval
				idx = k
			}
		}
		maxabs = RSHIFT_ROUND(maxabs, QIN-QOUT)

		if maxabs > int32(int16_MAX) {
			/* Reduce magnitude of prediction coefficients */
			maxabs = min(maxabs, 163838) /* ( silk_int32_MAX >> 14 ) + silk_int16_MAX = 163838 */
			chirp_Q16 = SILK_FIX_CONST(0.999, 16) - DIV32(LSHIFT(maxabs-int32(int16_MAX), 14),
				RSHIFT32(MUL(maxabs, idx+1), 2))
			Bwexpander32(a_QIN, d, chirp_Q16)
		} else {
			break
		}
	}

	if i == 10 {
		/* Reached the last iteration, clip the coefficients */
		for k = 0; k < d; k++ {
			a_QOUT[k] = int16(SAT16(RSHIFT_ROUND(a_QIN[k], QIN-QOUT)))
			a_QIN[k] = LSHIFT(int32(a_QOUT[k]), QIN-QOUT)
		}
	} else {
		for k = 0; k < d; k++ {
			a_QOUT[k] = int16(RSHIFT_ROUND(a_QIN[k], QIN-QOUT))
		}
	}
}
