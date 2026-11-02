package silk

// C: silk/fixed/regularize_correlations_FIX.c

// Add noise to matrix diagonal
//
// C: void silk_regularize_correlations_FIX(opus_int32 *XX, opus_int32 *xx, opus_int32 noise, opus_int D)
func RegularizeCorrelationsFIX(
	XX []int32, // I/O  Correlation matrices
	xx []int32, // I/O  Correlation values
	noise int32, // I    Noise to add
	D int, // I    Dimension of XX
) {
	var i int
	for i = 0; i < D; i++ {
		*matrix_ptr(XX[0:], i, i, D) = ADD32(*matrix_ptr(XX[0:], i, i, D), noise)
	}
	xx[0] += noise
}
