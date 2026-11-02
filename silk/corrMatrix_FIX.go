package silk

// C: silk/fixed/corrMatrix_FIX.c

// **********************************************************************
// Correlation Matrix Computations for LS estimate.
// **********************************************************************

// Calculates correlation vector X'*t
//
// C: void silk_corrVector_FIX(const opus_int16 *x, const opus_int16 *t, const opus_int L, const opus_int order, opus_int32 *Xt, const opus_int rshifts)
func CorrVectorFIX(
	x []int16, // I    x vector [L + order - 1] used to form data matrix X
	t []int16, // I    Target vector [L]
	L int, // I    Length of vectors
	order int, // I    Max lag for correlation
	Xt []int32, // O    Pointer to X'*t correlation vector [order]
	rshifts int, // I    Right shifts of correlations
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
				inner_prod += RSHIFT32(SMULBB(x[ptr1+i], ptr2[i]), rshifts)
			}
			Xt[lag] = inner_prod // X[:,lag]'*t
			ptr1--               // Go to next column of X
		}
	} else {
		silk_assert(rshifts == 0)
		for lag = 0; lag < order; lag++ {
			Xt[lag] = InnerProdAligned(x[ptr1:], ptr2, L) // X[:,lag]'*t
			ptr1--                                        // Go to next column of X
		}
	}
}

// Calculates correlation matrix X'*X
//
// C: void silk_corrMatrix_FIX(const opus_int16 *x, const opus_int L, const opus_int order, const opus_int head_room, opus_int32 *XX, opus_int *rshifts)
func CorrMatrixFIX(
	x []int16, // I    x vector [L + order - 1] used to form data matrix X
	L int, // I    Length of vectors
	order int, // I    Max lag for correlation
	head_room int, // I    Desired headroom
	XX []int32, // O    Pointer to X'*X correlation matrix [ order x order ]
	rshifts *int, // I/O  Right shifts of correlations
) {
	var i, j, lag, rshifts_local, head_room_rshifts int
	var energy int32
	var ptr1, ptr2 int // C: const opus_int16 *ptr1, *ptr2; indices into x[] (read before the pointer)

	// Calculate energy to find shift used to fit in 32 bits
	SumSqrShift(&energy, &rshifts_local, x, L+order-1)
	// Add shifts to get the desired head room
	head_room_rshifts = max(head_room-int(CLZ32(energy)), 0)

	energy = RSHIFT32(energy, head_room_rshifts)
	rshifts_local += head_room_rshifts

	// Calculate energy of first column (0) of X: X[:,0]'*X[:,0]
	// Remove contribution of first order - 1 samples
	for i = 0; i < order-1; i++ {
		energy -= RSHIFT32(SMULBB(x[i], x[i]), rshifts_local)
	}
	if rshifts_local < *rshifts {
		// Adjust energy
		energy = RSHIFT32(energy, *rshifts-rshifts_local)
		rshifts_local = *rshifts
	}

	// Calculate energy of remaining columns of X: X[:,j]'*X[:,j]
	// Fill out the diagonal of the correlation matrix
	*matrix_ptr(XX, 0, 0, order) = energy
	ptr1 = order - 1 // First sample of column 0 of X
	for j = 1; j < order; j++ {
		energy = SUB32(energy, RSHIFT32(SMULBB(x[ptr1+L-j], x[ptr1+L-j]), rshifts_local))
		energy = ADD32(energy, RSHIFT32(SMULBB(x[ptr1-j], x[ptr1-j]), rshifts_local))
		*matrix_ptr(XX, j, j, order) = energy
	}

	ptr2 = order - 2 // First sample of column 1 of X
	// Calculate the remaining elements of the correlation matrix
	if rshifts_local > 0 {
		// Right shifting used
		for lag = 1; lag < order; lag++ {
			// Inner product of column 0 and column lag: X[:,0]'*X[:,lag]
			energy = 0
			for i = 0; i < L; i++ {
				energy += RSHIFT32(SMULBB(x[ptr1+i], x[ptr2+i]), rshifts_local)
			}
			// Calculate remaining off diagonal: X[:,j]'*X[:,j + lag]
			*matrix_ptr(XX, lag, 0, order) = energy
			*matrix_ptr(XX, 0, lag, order) = energy
			for j = 1; j < (order - lag); j++ {
				energy = SUB32(energy, RSHIFT32(SMULBB(x[ptr1+L-j], x[ptr2+L-j]), rshifts_local))
				energy = ADD32(energy, RSHIFT32(SMULBB(x[ptr1-j], x[ptr2-j]), rshifts_local))
				*matrix_ptr(XX, lag+j, j, order) = energy
				*matrix_ptr(XX, j, lag+j, order) = energy
			}
			ptr2-- // Update pointer to first sample of next column (lag) in X
		}
	} else {
		for lag = 1; lag < order; lag++ {
			// Inner product of column 0 and column lag: X[:,0]'*X[:,lag]
			energy = InnerProdAligned(x[ptr1:], x[ptr2:], L)
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
	*rshifts = rshifts_local
}
