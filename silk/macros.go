package silk

// C: silk/macros.h — inline header file for general platform.
// Each function reproduces the C macro exactly, including intermediate casts.
// Parameters are generic over integer types because the macros are untyped;
// results are opus_int32 unless noted.

// C: silk_SMULWB(a32, b32) ((((a32) >> 16) * (opus_int32)((opus_int16)(b32))) + ((((a32) & 0x0000FFFF) * (opus_int32)((opus_int16)(b32))) >> 16))
// (a32 * (opus_int32)((opus_int16)(b32))) >> 16 output have to be 32bit int
func SMULWB[A, B integer](a32 A, b32 B) int32 {
	a := int32(a32)
	b := int32(int16(b32))
	return ((a >> 16) * b) + (((a & 0x0000FFFF) * b) >> 16)
}

// C: silk_SMLAWB(a32, b32, c32) ((a32) + ((((b32) >> 16) * (opus_int32)((opus_int16)(c32))) + ((((b32) & 0x0000FFFF) * (opus_int32)((opus_int16)(c32))) >> 16)))
// a32 + (b32 * (opus_int32)((opus_int16)(c32))) >> 16 output have to be 32bit int
func SMLAWB[A, B, C integer](a32 A, b32 B, c32 C) int32 {
	b := int32(b32)
	c := int32(int16(c32))
	return int32(a32) + (((b >> 16) * c) + (((b & 0x0000FFFF) * c) >> 16))
}

// C: silk_SMULWT(a32, b32) (((a32) >> 16) * ((b32) >> 16) + ((((a32) & 0x0000FFFF) * ((b32) >> 16)) >> 16))
// (a32 * (b32 >> 16)) >> 16
func SMULWT[A, B integer](a32 A, b32 B) int32 {
	a := int32(a32)
	b := int32(b32)
	return (a>>16)*(b>>16) + (((a & 0x0000FFFF) * (b >> 16)) >> 16)
}

// C: silk_SMLAWT(a32, b32, c32) ((a32) + (((b32) >> 16) * ((c32) >> 16)) + ((((b32) & 0x0000FFFF) * ((c32) >> 16)) >> 16))
// a32 + (b32 * (c32 >> 16)) >> 16
func SMLAWT[A, B, C integer](a32 A, b32 B, c32 C) int32 {
	b := int32(b32)
	c := int32(c32)
	return int32(a32) + ((b >> 16) * (c >> 16)) + (((b & 0x0000FFFF) * (c >> 16)) >> 16)
}

// C: silk_SMULBB(a32, b32) ((opus_int32)((opus_int16)(a32)) * (opus_int32)((opus_int16)(b32)))
func SMULBB[A, B integer](a32 A, b32 B) int32 {
	return int32(int16(a32)) * int32(int16(b32))
}

// C: silk_SMLABB(a32, b32, c32) ((a32) + ((opus_int32)((opus_int16)(b32))) * (opus_int32)((opus_int16)(c32)))
func SMLABB[A, B, C integer](a32 A, b32 B, c32 C) int32 {
	return int32(a32) + int32(int16(b32))*int32(int16(c32))
}

// C: silk_SMULBT(a32, b32) ((opus_int32)((opus_int16)(a32)) * ((b32) >> 16))
func SMULBT[A, B integer](a32 A, b32 B) int32 {
	return int32(int16(a32)) * (int32(b32) >> 16)
}

// C: silk_SMLABT(a32, b32, c32) ((a32) + ((opus_int32)((opus_int16)(b32))) * ((c32) >> 16))
func SMLABT[A, B, C integer](a32 A, b32 B, c32 C) int32 {
	return int32(a32) + int32(int16(b32))*(int32(c32)>>16)
}

// C: silk_SMLAL(a64, b32, c32) (silk_ADD64((a64), ((opus_int64)(b32) * (opus_int64)(c32))))
func SMLAL[A, B, C integer](a64 A, b32 B, c32 C) int64 {
	return ADD64(int64(a64), int64(b32)*int64(c32))
}

// C: silk_SMULWW(a32, b32) silk_MLA(silk_SMULWB((a32), (b32)), (a32), silk_RSHIFT_ROUND((b32), 16))
// (a32 * b32) >> 16
func SMULWW[A, B integer](a32 A, b32 B) int32 {
	return MLA(SMULWB(a32, b32), int32(a32), RSHIFT_ROUND(int32(b32), 16))
}

// C: silk_SMLAWW(a32, b32, c32) silk_MLA(silk_SMLAWB((a32), (b32), (c32)), (b32), silk_RSHIFT_ROUND((c32), 16))
// a32 + ((b32 * c32) >> 16)
func SMLAWW[A, B, C integer](a32 A, b32 B, c32 C) int32 {
	return MLA(SMLAWB(a32, b32, c32), int32(b32), RSHIFT_ROUND(int32(c32), 16))
}

// C: silk_ADD_SAT32(a, b) — add with output saturated
func ADD_SAT32[A, B integer](a A, b B) int32 {
	aa, bb := int32(a), int32(b)
	if ((uint32(aa) + uint32(bb)) & 0x80000000) == 0 {
		if ((uint32(aa) & uint32(bb)) & 0x80000000) != 0 {
			return int32_MIN
		}
		return aa + bb
	}
	if ((uint32(aa) | uint32(bb)) & 0x80000000) == 0 {
		return int32_MAX
	}
	return aa + bb
}

// C: silk_SUB_SAT32(a, b) — subtract with output saturated
func SUB_SAT32[A, B integer](a A, b B) int32 {
	aa, bb := int32(a), int32(b)
	if ((uint32(aa) - uint32(bb)) & 0x80000000) == 0 {
		if (uint32(aa) & (uint32(bb) ^ 0x80000000) & 0x80000000) != 0 {
			return int32_MIN
		}
		return aa - bb
	}
	if ((uint32(aa) ^ 0x80000000) & uint32(bb) & 0x80000000) != 0 {
		return int32_MAX
	}
	return aa - bb
}

// C: static inline opus_int32 silk_CLZ16(opus_int16 in16)
func CLZ16(in16 int16) int32 {
	var out32 int32 = 0
	if in16 == 0 {
		return 16
	}
	// test nibbles
	if in16&int16(-256) != 0 { // 0xFF00
		if in16&int16(-4096) != 0 { // 0xF000
			in16 >>= 12
		} else {
			out32 += 4
			in16 >>= 8
		}
	} else {
		if in16&0x0FF0 != 0 { // 0xFFF0 with the high byte known zero
			out32 += 8
			in16 >>= 4
		} else {
			out32 += 12
		}
	}
	// test bits and return
	if in16&0xC != 0 {
		if in16&0x8 != 0 {
			return out32 + 0
		}
		return out32 + 1
	}
	if in16&0xE != 0 {
		return out32 + 2
	}
	return out32 + 3
}

// C: static inline opus_int32 silk_CLZ32(opus_int32 in32)
func CLZ32(in32 int32) int32 {
	// test highest 16 bits and convert to opus_int16
	if uint32(in32)&0xFFFF0000 != 0 {
		return CLZ16(int16(in32 >> 16))
	}
	return CLZ16(int16(in32)) + 16
}

// Row based matrix element access.
//
// C: matrix_ptr(Matrix_base_adr, row, column, N) *(Matrix_base_adr + ((row)*(N)+(column)))
func matrix_ptr[T any](m []T, row, column, N int) *T { return &m[row*N+column] }

// C: matrix_adr(Matrix_base_adr, row, column, N) (Matrix_base_adr + ((row)*(N)+(column)))
func matrix_adr[T any](m []T, row, column, N int) []T { return m[row*N+column:] }

// Column based.
//
// C: matrix_c_ptr(Matrix_base_adr, row, column, M) *(Matrix_base_adr + ((row)+(M)*(column)))
func matrix_c_ptr[T any](m []T, row, column, M int) *T { return &m[row+M*column] }
