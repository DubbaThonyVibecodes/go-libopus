package silk

// C: silk/fixed/autocorr_FIX.c

// Compute autocorrelation
//
// C: void silk_autocorr(opus_int32 *results, opus_int *scale, const opus_int16 *inputData, const opus_int inputDataSize, const opus_int correlationCount)
func Autocorr(
	results []int32, // O    Result (length correlationCount)
	scale *int, // O    Scaling of the correlation vector
	inputData []int16, // I    Input data to correlate
	inputDataSize int, // I    Length of input
	correlationCount int, // I    Number of correlation taps to compute
) {
	var i, lz, nRightShifts, corrCount int
	var corr64 int64

	corrCount = min_int(inputDataSize, correlationCount)

	// compute energy (zero-lag correlation)
	corr64 = InnerProd16Aligned64(inputData, inputData, inputDataSize)

	// deal with all-zero input data
	corr64 += 1

	// number of leading zeros
	lz = int(CLZ64(corr64))

	// scaling: number of right shifts applied to correlations
	nRightShifts = 35 - lz
	*scale = nRightShifts

	if nRightShifts <= 0 {
		results[0] = LSHIFT(int32(CHECK_FIT32(corr64)), -nRightShifts)

		// compute remaining correlations based on int32 inner product
		for i = 1; i < corrCount; i++ {
			results[i] = LSHIFT(InnerProdAligned(inputData, inputData[i:], inputDataSize-i), -nRightShifts)
		}
	} else {
		results[0] = int32(CHECK_FIT32(RSHIFT64(corr64, nRightShifts)))

		// compute remaining correlations based on int64 inner product
		for i = 1; i < corrCount; i++ {
			results[i] = int32(CHECK_FIT32(RSHIFT64(InnerProd16Aligned64(inputData, inputData[i:], inputDataSize-i), nRightShifts)))
		}
	}
}
