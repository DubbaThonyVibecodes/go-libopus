package silk

// C: silk/sum_sqr_shift.c

// Compute number of bits to right shift the sum of squares of a vector
// of int16s to make it fit in an int32
//
// C: void silk_sum_sqr_shift(opus_int32 *energy, opus_int *shift, const opus_int16 *x, opus_int len)
func SumSqrShift(
	energy *int32, // O   Energy of x, after shifting to the right
	shift *int, // O   Number of bits right shift applied to energy
	x []int16, // I   Input vector
	len int, // I   Length of input vector
) {
	var i, shft int
	var nrg_tmp, nrg int32

	nrg = 0
	shft = 0
	len--
	for i = 0; i < len; i += 2 {
		nrg = SMLABB_ovflw(nrg, x[i], x[i])
		nrg = SMLABB_ovflw(nrg, x[i+1], x[i+1])
		if nrg < 0 {
			// Scale down
			nrg = int32(RSHIFT_uint(uint32(nrg), 2))
			shft = 2
			i += 2
			break
		}
	}
	for ; i < len; i += 2 {
		nrg_tmp = SMULBB(x[i], x[i])
		nrg_tmp = SMLABB_ovflw(nrg_tmp, x[i+1], x[i+1])
		nrg = int32(ADD_RSHIFT_uint(nrg, uint32(nrg_tmp), shft))
		if nrg < 0 {
			// Scale down
			nrg = int32(RSHIFT_uint(uint32(nrg), 2))
			shft += 2
		}
	}
	if i == len {
		// One sample left to process
		nrg_tmp = SMULBB(x[i], x[i])
		nrg = int32(ADD_RSHIFT_uint(nrg, nrg_tmp, shft))
	}

	// Make sure to have at least one extra leading zero (two leading zeros in total)
	if uint32(nrg)&0xC0000000 != 0 {
		nrg = int32(RSHIFT_uint(uint32(nrg), 2))
		shft += 2
	}

	// Output arguments
	*shift = shft
	*energy = nrg
}
