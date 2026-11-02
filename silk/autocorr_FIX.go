package silk

// C: silk/fixed/autocorr_FIX.c

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// Compute autocorrelation
//
// C: void silk_autocorr(opus_int32 *results, opus_int *scale, const opus_int16 *inputData, const opus_int inputDataSize, const opus_int correlationCount, int arch)
func Autocorr(
	results []int32, // O    Result (length correlationCount)
	scale *int, // O    Scaling of the correlation vector
	inputData []int16, // I    Input data to correlate
	inputDataSize int, // I    Length of input
	correlationCount int, // I    Number of correlation taps to compute
	arch int, // I    Run-time architecture
) {
	var corrCount int
	corrCount = min_int(inputDataSize, correlationCount)
	*scale = celt.Autocorr(inputData, results, nil, 0, corrCount-1, inputDataSize, arch)
}
