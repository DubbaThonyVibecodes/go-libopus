package silk

// C: silk/fixed/warped_autocorrelation_FIX.c

const QC = 10
const QS = 14

// Autocorrelations for a warped frequency axis
//
// C: void silk_warped_autocorrelation_FIX(opus_int32 *corr, opus_int *scale, const opus_int16 *input, const opus_int warping_Q16, const opus_int length, const opus_int order)
func WarpedAutocorrelationFIX(
	corr []int32, // O    Result [order + 1]
	scale *int, // O    Scaling of the correlation vector
	input []int16, // I    Input data to correlate
	warping_Q16 int, // I    Warping coefficient
	length int, // I    Length of input
	order int, // I    Correlation order (even)
) {
	var n, i, lsh int
	var tmp1_QS, tmp2_QS int32
	var state_QS = [MAX_SHAPE_LPC_ORDER + 1]int32{0}
	var corr_QC = [MAX_SHAPE_LPC_ORDER + 1]int64{0}

	// Order must be even
	silk_assert((order & 1) == 0)
	silk_assert(2*QS-QC >= 0)

	// Loop over samples
	for n = 0; n < length; n++ {
		tmp1_QS = LSHIFT32(int32(input[n]), QS)
		// Loop over allpass sections
		for i = 0; i < order; i += 2 {
			// Output of allpass section
			tmp2_QS = SMLAWB(state_QS[i], state_QS[i+1]-tmp1_QS, warping_Q16)
			state_QS[i] = tmp1_QS
			corr_QC[i] += RSHIFT64(SMULL(tmp1_QS, state_QS[0]), 2*QS-QC)
			// Output of allpass section
			tmp1_QS = SMLAWB(state_QS[i+1], state_QS[i+2]-tmp2_QS, warping_Q16)
			state_QS[i+1] = tmp2_QS
			corr_QC[i+1] += RSHIFT64(SMULL(tmp2_QS, state_QS[0]), 2*QS-QC)
		}
		state_QS[order] = tmp1_QS
		corr_QC[order] += RSHIFT64(SMULL(tmp1_QS, state_QS[0]), 2*QS-QC)
	}

	lsh = int(CLZ64(corr_QC[0])) - 35
	lsh = LIMIT(lsh, -12-QC, 30-QC)
	*scale = -(QC + lsh)
	silk_assert(*scale >= -30 && *scale <= 12)
	if lsh >= 0 {
		for i = 0; i < order+1; i++ {
			corr[i] = int32(CHECK_FIT32(LSHIFT64(corr_QC[i], lsh)))
		}
	} else {
		for i = 0; i < order+1; i++ {
			corr[i] = int32(CHECK_FIT32(RSHIFT64(corr_QC[i], -lsh)))
		}
	}
	silk_assert(corr_QC[0] >= 0) // If breaking, decrease QC
}
