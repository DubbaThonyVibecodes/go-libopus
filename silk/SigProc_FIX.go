package silk

// C: silk/SigProc_FIX.h — signal processing macros. The functions declared
// in that header live in their own files (resampler.go, lin2log.go, ...).

const SILK_MAX_ORDER_LPC = 24 // max order of the LPC analysis in schur() and k2a()

// Rotate a32 right by 'rot' bits. Negative rot values result in rotating
// left. Output is 32bit int.
//
// C: static inline opus_int32 silk_ROR32( opus_int32 a32, opus_int rot )
func ROR32(a32 int32, rot int) int32 {
	x := uint32(a32)
	r := uint32(rot)
	m := uint32(-rot)
	if rot == 0 {
		return a32
	} else if rot < 0 {
		return int32((x << m) | (x >> (32 - m)))
	}
	return int32((x << (32 - r)) | (x >> r))
}

// Fixed point macros

// C: silk_MUL(a32, b32) ((a32) * (b32)) — output has to be 32bit int
func MUL[A, B integer](a32 A, b32 B) int32 { return int32(a32) * int32(b32) }

// C: silk_MUL_uint(a32, b32) silk_MUL(a32, b32)
func MUL_uint[A, B integer](a32 A, b32 B) uint32 { return uint32(a32) * uint32(b32) }

// C: silk_MLA(a32, b32, c32) silk_ADD32((a32),((b32) * (c32)))
func MLA[A, B, C integer](a32 A, b32 B, c32 C) int32 { return ADD32(a32, int32(b32)*int32(c32)) }

// C: silk_MLA_uint(a32, b32, c32) silk_MLA(a32, b32, c32)
func MLA_uint[A, B, C integer](a32 A, b32 B, c32 C) uint32 {
	return uint32(a32) + uint32(b32)*uint32(c32)
}

// C: silk_SMULTT(a32, b32) (((a32) >> 16) * ((b32) >> 16))
func SMULTT[A, B integer](a32 A, b32 B) int32 { return (int32(a32) >> 16) * (int32(b32) >> 16) }

// C: silk_SMLATT(a32, b32, c32) silk_ADD32((a32),((b32) >> 16) * ((c32) >> 16))
func SMLATT[A, B, C integer](a32 A, b32 B, c32 C) int32 {
	return ADD32(a32, (int32(b32)>>16)*(int32(c32)>>16))
}

// C: silk_SMLALBB(a64, b16, c16) silk_ADD64((a64),(opus_int64)((opus_int32)(b16) * (opus_int32)(c16)))
func SMLALBB[A, B, C integer](a64 A, b16 B, c16 C) int64 {
	return ADD64(int64(a64), int64(int32(b16)*int32(c16)))
}

// C: silk_SMULL(a32, b32) ((opus_int64)(a32) * /*(opus_int64)*/(b32))
func SMULL[A, B integer](a32 A, b32 B) int64 { return int64(a32) * int64(b32) }

// Adds two signed 32-bit values in a way that can overflow, while not relying on undefined behaviour
// (just standard two's complement implementation-specific behaviour)
//
// C: silk_ADD32_ovflw(a, b) ((opus_int32)((opus_uint32)(a) + (opus_uint32)(b)))
func ADD32_ovflw[A, B integer](a A, b B) int32 { return int32(uint32(a) + uint32(b)) }

// C: silk_SUB32_ovflw(a, b) ((opus_int32)((opus_uint32)(a) - (opus_uint32)(b)))
func SUB32_ovflw[A, B integer](a A, b B) int32 { return int32(uint32(a) - uint32(b)) }

// Multiply-accumulate macros that allow overflow in the addition (ie, no asserts in debug mode)
//
// C: silk_MLA_ovflw(a32, b32, c32) silk_ADD32_ovflw((a32), (opus_uint32)(b32) * (opus_uint32)(c32))
func MLA_ovflw[A, B, C integer](a32 A, b32 B, c32 C) int32 {
	return ADD32_ovflw(a32, uint32(b32)*uint32(c32))
}

// C: silk_SMLABB_ovflw(a32, b32, c32) (silk_ADD32_ovflw((a32) , ((opus_int32)((opus_int16)(b32))) * (opus_int32)((opus_int16)(c32))))
func SMLABB_ovflw[A, B, C integer](a32 A, b32 B, c32 C) int32 {
	return ADD32_ovflw(a32, int32(int16(b32))*int32(int16(c32)))
}

// C: silk_DIV32_16(a32, b16) ((opus_int32)((a32) / (b16)))
func DIV32_16[A, B integer](a32 A, b16 B) int32 { return int32(int32(a32) / int32(b16)) }

// C: silk_DIV32(a32, b32) ((opus_int32)((a32) / (b32)))
func DIV32[A, B integer](a32 A, b32 B) int32 { return int32(int32(a32) / int32(b32)) }

// These macros enables checking for overflow in silk_API_Debug.h

// C: silk_ADD16(a, b) ((a) + (b))
func ADD16[A, B integer](a A, b B) int16 { return int16(a) + int16(b) }

// C: silk_ADD32(a, b) ((a) + (b))
func ADD32[A, B integer](a A, b B) int32 { return int32(a) + int32(b) }

// C: silk_ADD64(a, b) ((a) + (b))
func ADD64[A, B integer](a A, b B) int64 { return int64(a) + int64(b) }

// C: silk_SUB16(a, b) ((a) - (b))
func SUB16[A, B integer](a A, b B) int16 { return int16(a) - int16(b) }

// C: silk_SUB32(a, b) ((a) - (b))
func SUB32[A, B integer](a A, b B) int32 { return int32(a) - int32(b) }

// C: silk_SUB64(a, b) ((a) - (b))
func SUB64[A, B integer](a A, b B) int64 { return int64(a) - int64(b) }

// nc defeats constant folding so that a limit can be converted to any
// signed type parameter without a compile-time range check.
func nc(v int64) int64 { return v }

type signed interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

// C: silk_SAT8(a)
func SAT8[T signed](a T) T {
	hi, lo := T(nc(int64(int8_MAX))), T(nc(int64(int8_MIN)))
	if a > hi {
		return hi
	}
	if a < lo {
		return lo
	}
	return a
}

// C: silk_SAT16(a)
func SAT16[T signed](a T) T {
	hi, lo := T(nc(int64(int16_MAX))), T(nc(int64(int16_MIN)))
	if a > hi {
		return hi
	}
	if a < lo {
		return lo
	}
	return a
}

// C: silk_SAT32(a)
func SAT32[T signed](a T) T {
	hi, lo := T(nc(int64(int32_MAX))), T(nc(int64(int32_MIN)))
	if a > hi {
		return hi
	}
	if a < lo {
		return lo
	}
	return a
}

// C: silk_CHECK_FIT8/16/32(a) (a)
func CHECK_FIT8[T integer](a T) T  { return a }
func CHECK_FIT16[T integer](a T) T { return a }
func CHECK_FIT32[T integer](a T) T { return a }

// C: silk_ADD_SAT16(a, b) (opus_int16)silk_SAT16( silk_ADD32( (opus_int32)(a), (b) ) )
func ADD_SAT16[A, B integer](a A, b B) int16 { return int16(SAT16(ADD32(int32(a), b))) }

// C: silk_ADD_SAT64(a, b)
func ADD_SAT64[A, B integer](a A, b B) int64 {
	aa, bb := int64(a), int64(b)
	if ((uint64(aa) + uint64(bb)) & 0x8000000000000000) == 0 {
		if ((uint64(aa) & uint64(bb)) & 0x8000000000000000) != 0 {
			return int64_MIN
		}
		return aa + bb
	}
	if ((uint64(aa) | uint64(bb)) & 0x8000000000000000) == 0 {
		return int64_MAX
	}
	return aa + bb
}

// C: silk_SUB_SAT16(a, b) (opus_int16)silk_SAT16( silk_SUB32( (opus_int32)(a), (b) ) )
func SUB_SAT16[A, B integer](a A, b B) int16 { return int16(SAT16(SUB32(int32(a), b))) }

// C: silk_SUB_SAT64(a, b)
func SUB_SAT64[A, B integer](a A, b B) int64 {
	aa, bb := int64(a), int64(b)
	if ((uint64(aa) - uint64(bb)) & 0x8000000000000000) == 0 {
		if (uint64(aa) & (uint64(bb) ^ 0x8000000000000000) & 0x8000000000000000) != 0 {
			return int64_MIN
		}
		return aa - bb
	}
	if ((uint64(aa) ^ 0x8000000000000000) & uint64(bb) & 0x8000000000000000) != 0 {
		return int64_MAX
	}
	return aa - bb
}

// Saturation for positive input values
//
// C: silk_POS_SAT32(a) ((a) > silk_int32_MAX ? silk_int32_MAX : (a))
func POS_SAT32[T signed](a T) T {
	hi := T(nc(int64(int32_MAX)))
	if a > hi {
		return hi
	}
	return a
}

// Add with saturation for positive input values
//
// C: silk_ADD_POS_SAT8(a, b) ((((a)+(b)) & 0x80) ? silk_int8_MAX : ((a)+(b)))
func ADD_POS_SAT8[A, B integer](a A, b B) int8 {
	s := int32(a) + int32(b)
	if s&0x80 != 0 {
		return int8_MAX
	}
	return int8(s)
}

// C: silk_ADD_POS_SAT16(a, b) ((((a)+(b)) & 0x8000) ? silk_int16_MAX : ((a)+(b)))
func ADD_POS_SAT16[A, B integer](a A, b B) int16 {
	s := int32(a) + int32(b)
	if s&0x8000 != 0 {
		return int16_MAX
	}
	return int16(s)
}

// C: silk_ADD_POS_SAT32(a, b) ((((opus_uint32)(a)+(opus_uint32)(b)) & 0x80000000) ? silk_int32_MAX : ((a)+(b)))
func ADD_POS_SAT32[A, B integer](a A, b B) int32 {
	s := uint32(a) + uint32(b)
	if s&0x80000000 != 0 {
		return int32_MAX
	}
	return int32(s)
}

// C: silk_LSHIFT8(a, shift) ((opus_int8)((opus_uint8)(a)<<(shift)))   shift >= 0, shift < 8
func LSHIFT8[T integer](a T, shift int) int8 { return int8(uint8(a) << shift) }

// C: silk_LSHIFT16(a, shift) ((opus_int16)((opus_uint16)(a)<<(shift)))   shift >= 0, shift < 16
func LSHIFT16[T integer](a T, shift int) int16 { return int16(uint16(a) << shift) }

// C: silk_LSHIFT32(a, shift) ((opus_int32)((opus_uint32)(a)<<(shift)))   shift >= 0, shift < 32
func LSHIFT32[T integer](a T, shift int) int32 { return int32(uint32(a) << shift) }

// C: silk_LSHIFT64(a, shift) ((opus_int64)((opus_uint64)(a)<<(shift)))   shift >= 0, shift < 64
func LSHIFT64[T integer](a T, shift int) int64 { return int64(uint64(a) << shift) }

// C: silk_LSHIFT(a, shift) silk_LSHIFT32(a, shift)
func LSHIFT[T integer](a T, shift int) int32 { return LSHIFT32(a, shift) }

// C: silk_RSHIFT8/16/32/64(a, shift) ((a)>>(shift))
func RSHIFT8[T integer](a T, shift int) T  { return a >> shift }
func RSHIFT16[T integer](a T, shift int) T { return a >> shift }
func RSHIFT32[T integer](a T, shift int) T { return a >> shift }
func RSHIFT64[T integer](a T, shift int) T { return a >> shift }

// C: silk_RSHIFT(a, shift) silk_RSHIFT32(a, shift)
func RSHIFT[T integer](a T, shift int) T { return a >> shift }

// saturates before shifting
//
// C: silk_LSHIFT_SAT32(a, shift) (silk_LSHIFT32( silk_LIMIT( (a), silk_RSHIFT32( silk_int32_MIN, (shift) ), silk_RSHIFT32( silk_int32_MAX, (shift) ) ), (shift) ))
func LSHIFT_SAT32[T integer](a T, shift int) int32 {
	return LSHIFT32(LIMIT(int32(a), RSHIFT32(int32_MIN, shift), RSHIFT32(int32_MAX, shift)), shift)
}

// C: silk_LSHIFT_ovflw(a, shift) ((opus_int32)((opus_uint32)(a) << (shift)))   shift >= 0, allowed to overflow
func LSHIFT_ovflw[T integer](a T, shift int) int32 { return int32(uint32(a) << shift) }

// C: silk_LSHIFT_uint(a, shift) ((a) << (shift))
func LSHIFT_uint[T integer](a T, shift int) T { return a << shift }

// C: silk_RSHIFT_uint(a, shift) ((a) >> (shift))
func RSHIFT_uint[T integer](a T, shift int) T { return a >> shift }

// C: silk_ADD_LSHIFT(a, b, shift) ((a) + silk_LSHIFT((b), (shift)))
func ADD_LSHIFT[A, B integer](a A, b B, shift int) int32 { return int32(a) + LSHIFT(b, shift) }

// C: silk_ADD_LSHIFT32(a, b, shift) silk_ADD32((a), silk_LSHIFT32((b), (shift)))
func ADD_LSHIFT32[A, B integer](a A, b B, shift int) int32 { return ADD32(a, LSHIFT32(b, shift)) }

// C: silk_ADD_LSHIFT_uint(a, b, shift) ((a) + silk_LSHIFT_uint((b), (shift)))
func ADD_LSHIFT_uint[A, B integer](a A, b B, shift int) uint32 {
	return uint32(a) + uint32(LSHIFT_uint(b, shift))
}

// C: silk_ADD_RSHIFT(a, b, shift) ((a) + silk_RSHIFT((b), (shift)))
func ADD_RSHIFT[A, B integer](a A, b B, shift int) int32 { return int32(a) + int32(RSHIFT(b, shift)) }

// C: silk_ADD_RSHIFT32(a, b, shift) silk_ADD32((a), silk_RSHIFT32((b), (shift)))
func ADD_RSHIFT32[A, B integer](a A, b B, shift int) int32 { return ADD32(a, RSHIFT32(b, shift)) }

// C: silk_ADD_RSHIFT_uint(a, b, shift) ((a) + silk_RSHIFT_uint((b), (shift)))
func ADD_RSHIFT_uint[A, B integer](a A, b B, shift int) uint32 {
	return uint32(a) + uint32(RSHIFT_uint(b, shift))
}

// C: silk_SUB_LSHIFT32(a, b, shift) silk_SUB32((a), silk_LSHIFT32((b), (shift)))
func SUB_LSHIFT32[A, B integer](a A, b B, shift int) int32 { return SUB32(a, LSHIFT32(b, shift)) }

// C: silk_SUB_RSHIFT32(a, b, shift) silk_SUB32((a), silk_RSHIFT32((b), (shift)))
func SUB_RSHIFT32[A, B integer](a A, b B, shift int) int32 { return SUB32(a, RSHIFT32(b, shift)) }

// Requires that shift > 0
//
// C: silk_RSHIFT_ROUND(a, shift) ((shift) == 1 ? ((a) >> 1) + ((a) & 1) : (((a) >> ((shift) - 1)) + 1) >> 1)
func RSHIFT_ROUND[T integer](a T, shift int) T {
	if shift == 1 {
		return (a >> 1) + (a & 1)
	}
	return ((a >> (shift - 1)) + 1) >> 1
}

// C: silk_RSHIFT_ROUND64(a, shift)
func RSHIFT_ROUND64[T integer](a T, shift int) int64 {
	aa := int64(a)
	if shift == 1 {
		return (aa >> 1) + (aa & 1)
	}
	return ((aa >> (shift - 1)) + 1) >> 1
}

// Number of rightshift required to fit the multiplication
//
// C: silk_NSHIFT_MUL_32_32(a, b) ( -(31- (32-silk_CLZ32(silk_abs(a)) + (32-silk_CLZ32(silk_abs(b))))) )
func NSHIFT_MUL_32_32(a, b int32) int32 {
	return -(31 - (32 - CLZ32(abs(a)) + (32 - CLZ32(abs(b)))))
}

// C: silk_NSHIFT_MUL_16_16(a, b) ( -(15- (16-silk_CLZ16(silk_abs(a)) + (16-silk_CLZ16(silk_abs(b))))) )
func NSHIFT_MUL_16_16(a, b int16) int32 {
	return -(15 - (16 - CLZ16(abs(a)) + (16 - CLZ16(abs(b)))))
}

// silk_min / silk_max are the Go builtins min / max.

// Macro to convert floating-point constants to fixed-point. In C the
// product is computed in the constant's own precision (float for
// f-suffixed literals, double otherwise) and 0.5 is added in double.
//
// C: SILK_FIX_CONST( C, Q ) ((opus_int32)((C) * ((opus_int64)1 << (Q)) + 0.5))
func SILK_FIX_CONST[F float32 | float64](c F, q int) int32 {
	switch v := any(c).(type) {
	case float32:
		return int32(float64(v*float32(int64(1)<<q)) + 0.5)
	case float64:
		return int32(v*float64(int64(1)<<q) + 0.5)
	}
	return 0
}

// silk_min() versions with typecast in the function call
//
// C: static inline opus_int silk_min_int(opus_int a, opus_int b)
func min_int(a, b int) int { return min(a, b) }

// C: static inline opus_int16 silk_min_16(opus_int16 a, opus_int16 b)
func min_16(a, b int16) int16 { return min(a, b) }

// C: static inline opus_int32 silk_min_32(opus_int32 a, opus_int32 b)
func min_32(a, b int32) int32 { return min(a, b) }

// C: static inline opus_int64 silk_min_64(opus_int64 a, opus_int64 b)
func min_64(a, b int64) int64 { return min(a, b) }

// C: static inline opus_int silk_max_int(opus_int a, opus_int b)
func max_int(a, b int) int { return max(a, b) }

// C: static inline opus_int16 silk_max_16(opus_int16 a, opus_int16 b)
func max_16(a, b int16) int16 { return max(a, b) }

// C: static inline opus_int32 silk_max_32(opus_int32 a, opus_int32 b)
func max_32(a, b int32) int32 { return max(a, b) }

// C: static inline opus_int64 silk_max_64(opus_int64 a, opus_int64 b)
func max_64(a, b int64) int64 { return max(a, b) }

// C: silk_LIMIT( a, limit1, limit2)
func LIMIT[T integer](a, limit1, limit2 T) T {
	if limit1 > limit2 {
		if a > limit1 {
			return limit1
		}
		if a < limit2 {
			return limit2
		}
		return a
	}
	if a > limit2 {
		return limit2
	}
	if a < limit1 {
		return limit1
	}
	return a
}

// C: silk_LIMIT_int / silk_LIMIT_16 / silk_LIMIT_32 → silk_LIMIT
func LIMIT_int(a, limit1, limit2 int) int    { return LIMIT(a, limit1, limit2) }
func LIMIT_16(a, limit1, limit2 int16) int16 { return LIMIT(a, limit1, limit2) }
func LIMIT_32(a, limit1, limit2 int32) int32 { return LIMIT(a, limit1, limit2) }

// C: silk_abs(a) (((a) >  0)  ? (a) : -(a))   Be careful, silk_abs returns wrong when input equals to silk_intXX_MIN
func abs[T integer](a T) T {
	if a > 0 {
		return a
	}
	return -a
}

// C: silk_abs_int(a) (((a) ^ ((a) >> (8 * sizeof(a) - 1))) - ((a) >> (8 * sizeof(a) - 1)))
func abs_int(a int) int { return (a ^ (a >> 63)) - (a >> 63) }

// C: silk_abs_int32(a) (((a) ^ ((a) >> 31)) - ((a) >> 31))
func abs_int32(a int32) int32 { return (a ^ (a >> 31)) - (a >> 31) }

// C: silk_abs_int64(a) (((a) >  0)  ? (a) : -(a))
func abs_int64(a int64) int64 { return abs(a) }

// C: silk_sign(a) ((a) > 0 ? 1 : ( (a) < 0 ? -1 : 0 ))
func sign[T integer](a T) int {
	if a > 0 {
		return 1
	}
	if a < 0 {
		return -1
	}
	return 0
}

// PSEUDO-RANDOM GENERATOR
// Make sure to store the result as the seed for the next call (also in between
// frames), otherwise result won't be random at all. When only using some of the
// bits, take the most significant bits by right-shifting.
//
// C: #define RAND_MULTIPLIER 196314165
// C: #define RAND_INCREMENT  907633515
// C: silk_RAND(seed) (silk_MLA_ovflw((RAND_INCREMENT), (seed), (RAND_MULTIPLIER)))
const RAND_MULTIPLIER = 196314165
const RAND_INCREMENT = 907633515

func RAND(seed int32) int32 { return MLA_ovflw(RAND_INCREMENT, seed, RAND_MULTIPLIER) }

// silk_SMMUL: Signed top word multiply.
//
// C: silk_SMMUL(a32, b32) (opus_int32)silk_RSHIFT64(silk_SMULL((a32), (b32)), 32)
func SMMUL[A, B integer](a32 A, b32 B) int32 { return int32(RSHIFT64(SMULL(a32, b32), 32)) }
