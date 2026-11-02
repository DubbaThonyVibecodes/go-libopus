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
	var nrg_tmp uint32
	var nrg int32

	// Do a first run with the maximum shift we could have.
	shft = int(31 - CLZ32(int32(len)))
	// Let's be conservative with rounding and start with nrg=len.
	nrg = int32(len)
	for i = 0; i < len-1; i += 2 {
		nrg_tmp = uint32(SMULBB(x[i], x[i]))
		nrg_tmp = uint32(SMLABB_ovflw(nrg_tmp, x[i+1], x[i+1]))
		nrg = int32(ADD_RSHIFT_uint(nrg, nrg_tmp, shft))
	}
	if i < len {
		// One sample left to process
		nrg_tmp = uint32(SMULBB(x[i], x[i]))
		nrg = int32(ADD_RSHIFT_uint(nrg, nrg_tmp, shft))
	}
	silk_assert(nrg >= 0)
	// Make sure the result will fit in a 32-bit signed integer with two bits
	// of headroom.
	shft = int(max_32(0, int32(shft)+3-CLZ32(nrg)))
	nrg = 0
	for i = 0; i < len-1; i += 2 {
		nrg_tmp = uint32(SMULBB(x[i], x[i]))
		nrg_tmp = uint32(SMLABB_ovflw(nrg_tmp, x[i+1], x[i+1]))
		nrg = int32(ADD_RSHIFT_uint(nrg, nrg_tmp, shft))
	}
	if i < len {
		// One sample left to process
		nrg_tmp = uint32(SMULBB(x[i], x[i]))
		nrg = int32(ADD_RSHIFT_uint(nrg, nrg_tmp, shft))
	}

	silk_assert(nrg >= 0)

	// Output arguments
	*shift = shft
	*energy = nrg
}
