package silk

// C: silk/Inlines.h — inline signal processing functions.

// count leading zeros of opus_int64
//
// C: static inline opus_int32 silk_CLZ64( opus_int64 in )
func CLZ64(in int64) int32 {
	var in_upper int32
	in_upper = int32(RSHIFT64(in, 32))
	if in_upper == 0 {
		// Search in the lower 32 bits
		return 32 + CLZ32(int32(in))
	}
	// Search in the upper 32 bits
	return CLZ32(in_upper)
}

// get number of leading zeros and fractional part (the bits right after the leading one
//
// C: static inline void silk_CLZ_FRAC(opus_int32 in, opus_int32 *lz, opus_int32 *frac_Q7)
func CLZ_FRAC(in int32, lz *int32, frac_Q7 *int32) {
	lzeros := CLZ32(in)
	*lz = lzeros
	*frac_Q7 = ROR32(in, int(24-lzeros)) & 0x7f
}

// Approximation of square root
// Accuracy: < +/- 10%  for output values > 15
//
//	< +/- 2.5% for output values > 120
//
// C: static inline opus_int32 silk_SQRT_APPROX( opus_int32 x )
func SQRT_APPROX(x int32) int32 {
	var y, lz, frac_Q7 int32

	if x <= 0 {
		return 0
	}

	CLZ_FRAC(x, &lz, &frac_Q7)

	if lz&1 != 0 {
		y = 32768
	} else {
		y = 46214 // 46214 = sqrt(2) * 32768
	}

	// get scaling right
	y >>= RSHIFT(lz, 1)

	// increment using fractional part of input
	y = SMLAWB(y, y, SMULBB(213, frac_Q7))

	return y
}

// Divide two int32 values and return result as int32 in a given Q-domain
//
// C: static inline opus_int32 silk_DIV32_varQ(const opus_int32 a32, const opus_int32 b32, const opus_int Qres)
func DIV32_varQ(a32, b32 int32, Qres int) int32 {
	var a_headrm, b_headrm, lshift int
	var b32_inv, a32_nrm, b32_nrm, result int32

	silk_assert(b32 != 0)
	silk_assert(Qres >= 0)

	// Compute number of bits head room and normalize inputs
	a_headrm = int(CLZ32(abs(a32))) - 1
	a32_nrm = LSHIFT(a32, a_headrm) // Q: a_headrm
	b_headrm = int(CLZ32(abs(b32))) - 1
	b32_nrm = LSHIFT(b32, b_headrm) // Q: b_headrm

	// Inverse of b32, with 14 bits of precision
	b32_inv = DIV32_16(int32_MAX>>2, RSHIFT(b32_nrm, 16)) // Q: 29 + 16 - b_headrm

	// First approximation
	result = SMULWB(a32_nrm, b32_inv) // Q: 29 + a_headrm - b_headrm

	// Compute residual by subtracting product of denominator and first approximation
	// It's OK to overflow because the final value of a32_nrm should always be small
	a32_nrm = SUB32_ovflw(a32_nrm, LSHIFT_ovflw(SMMUL(b32_nrm, result), 3)) // Q: a_headrm

	// Refinement
	result = SMLAWB(result, a32_nrm, b32_inv) // Q: 29 + a_headrm - b_headrm

	// Convert to Qres domain
	lshift = 29 + a_headrm - b_headrm - Qres
	if lshift < 0 {
		return LSHIFT_SAT32(result, -lshift)
	}
	if lshift < 32 {
		return RSHIFT(result, lshift)
	}
	// Avoid undefined result
	return 0
}

// Invert int32 value and return result as int32 in a given Q-domain
//
// C: static inline opus_int32 silk_INVERSE32_varQ(const opus_int32 b32, const opus_int Qres)
func INVERSE32_varQ(b32 int32, Qres int) int32 {
	var b_headrm, lshift int
	var b32_inv, b32_nrm, err_Q32, result int32

	silk_assert(b32 != 0)
	silk_assert(Qres > 0)

	// Compute number of bits head room and normalize input
	b_headrm = int(CLZ32(abs(b32))) - 1
	b32_nrm = LSHIFT(b32, b_headrm) // Q: b_headrm

	// Inverse of b32, with 14 bits of precision
	b32_inv = DIV32_16(int32_MAX>>2, RSHIFT(b32_nrm, 16)) // Q: 29 + 16 - b_headrm

	// First approximation
	result = LSHIFT(b32_inv, 16) // Q: 61 - b_headrm

	// Compute residual by subtracting product of denominator and first approximation from one
	err_Q32 = LSHIFT((int32(1)<<29)-SMULWB(b32_nrm, b32_inv), 3) // Q32

	// Refinement
	result = SMLAWW(result, err_Q32, b32_inv) // Q: 61 - b_headrm

	// Convert to Qres domain
	lshift = 61 - b_headrm - Qres
	if lshift <= 0 {
		return LSHIFT_SAT32(result, -lshift)
	}
	if lshift < 32 {
		return RSHIFT(result, lshift)
	}
	// Avoid undefined result
	return 0
}
