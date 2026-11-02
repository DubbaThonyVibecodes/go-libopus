package silk

// C: silk/fixed/corrMatrix_FIX.c

// **********************************************************************
// Correlation Matrix Computations for LS estimate.
// **********************************************************************

// Calculates correlation vector X'*t
//
// C: void silk_corrVector_FIX(const opus_int16 *x, const opus_int16 *t, const opus_int L, const opus_int order, opus_int32 *Xt, const opus_int rshifts, int arch)
func CorrVectorFIX(
	x []int16, // I    x vector [L + order - 1] used to form data matrix X
	t []int16, // I    Target vector [L]
	L int, // I    Length of vectors
	order int, // I    Max lag for correlation
	Xt []int32, // O    Pointer to X'*t correlation vector [order]
	rshifts int, // I    Right shifts of correlations
	arch int, // I    Run-time architecture
) {
	var lag, i int
	var ptr1 int // C: const opus_int16 *ptr1; index into x[] (is decremented, so kept as an index)
	var ptr2 []int16
	var inner_prod int32

	ptr1 = order - 1 // Points to first sample of column 0 of X: X[:,0]
	ptr2 = t
	// Calculate X'*t
	if rshifts > 0 {
		// Right shifting used
		for lag = 0; lag < order; lag++ {
			inner_prod = 0
			for i = 0; i < L; i++ {
				inner_prod = ADD_RSHIFT32(inner_prod, SMULBB(x[ptr1+i], ptr2[i]), rshifts)
			}
			Xt[lag] = inner_prod // X[:,lag]'*t
			ptr1--               // Go to next column of X
		}
	} else {
		silk_assert(rshifts == 0)
		for lag = 0; lag < order; lag++ {
			Xt[lag] = InnerProdAligned(x[ptr1:], ptr2, L, arch) // X[:,lag]'*t
			ptr1--                                              // Go to next column of X
		}
	}
}

// Calculates correlation matrix X'*X
//
// C: void silk_corrMatrix_FIX(const opus_int16 *x, const opus_int L, const opus_int order, opus_int32 *XX, opus_int32 *nrg, opus_int *rshifts, int arch)
func CorrMatrixFIX(
	x []int16, // I    x vector [L + order - 1] used to form data matrix X
	L int, // I    Length of vectors
	order int, // I    Max lag for correlation
	XX []int32, // O    Pointer to X'*X correlation matrix [ order x order ]
	nrg *int32, // O    Energy of x vector
	rshifts *int, // O    Right shifts of correlations and energy
	arch int, // I    Run-time architecture
) {
	var i, j, lag int
	var energy int32
	var ptr1, ptr2 int // C: const opus_int16 *ptr1, *ptr2; indices into x[] (read before the pointer)

	// Calculate energy to find shift used to fit in 32 bits
	SumSqrShift(nrg, rshifts, x, L+order-1)
	energy = *nrg

	// Calculate energy of first column (0) of X: X[:,0]'*X[:,0]
	// Remove contribution of first order - 1 samples
	for i = 0; i < order-1; i++ {
		energy -= RSHIFT32(SMULBB(x[i], x[i]), *rshifts)
	}

	// Calculate energy of remaining columns of X: X[:,j]'*X[:,j]
	// Fill out the diagonal of the correlation matrix
	*matrix_ptr(XX, 0, 0, order) = energy
	silk_assert(energy >= 0)
	ptr1 = order - 1 // First sample of column 0 of X
	for j = 1; j < order; j++ {
		energy = SUB32(energy, RSHIFT32(SMULBB(x[ptr1+L-j], x[ptr1+L-j]), *rshifts))
		energy = ADD32(energy, RSHIFT32(SMULBB(x[ptr1-j], x[ptr1-j]), *rshifts))
		*matrix_ptr(XX, j, j, order) = energy
		silk_assert(energy >= 0)
	}

	ptr2 = order - 2 // First sample of column 1 of X
	// Calculate the remaining elements of the correlation matrix
	if *rshifts > 0 {
		// Right shifting used
		for lag = 1; lag < order; lag++ {
			// Inner product of column 0 and column lag: X[:,0]'*X[:,lag]
			energy = 0
			for i = 0; i < L; i++ {
				energy += RSHIFT32(SMULBB(x[ptr1+i], x[ptr2+i]), *rshifts)
			}
			// Calculate remaining off diagonal: X[:,j]'*X[:,j + lag]
			*matrix_ptr(XX, lag, 0, order) = energy
			*matrix_ptr(XX, 0, lag, order) = energy
			for j = 1; j < (order - lag); j++ {
				energy = SUB32(energy, RSHIFT32(SMULBB(x[ptr1+L-j], x[ptr2+L-j]), *rshifts))
				energy = ADD32(energy, RSHIFT32(SMULBB(x[ptr1-j], x[ptr2-j]), *rshifts))
				*matrix_ptr(XX, lag+j, j, order) = energy
				*matrix_ptr(XX, j, lag+j, order) = energy
			}
			ptr2-- // Update pointer to first sample of next column (lag) in X
		}
	} else {
		for lag = 1; lag < order; lag++ {
			// Inner product of column 0 and column lag: X[:,0]'*X[:,lag]
			energy = InnerProdAligned(x[ptr1:], x[ptr2:], L, arch)
			*matrix_ptr(XX, lag, 0, order) = energy
			*matrix_ptr(XX, 0, lag, order) = energy
			// Calculate remaining off diagonal: X[:,j]'*X[:,j + lag]
			for j = 1; j < (order - lag); j++ {
				energy = SUB32(energy, SMULBB(x[ptr1+L-j], x[ptr2+L-j]))
				energy = SMLABB(energy, x[ptr1-j], x[ptr2-j])
				*matrix_ptr(XX, lag+j, j, order) = energy
				*matrix_ptr(XX, j, lag+j, order) = energy
			}
			ptr2-- // Update pointer to first sample of next column (lag) in X
		}
	}
}
