package celt

// C: celt/fixed_generic.h — generic fixed-point operations.
//
// Each function reproduces the C macro exactly, including the intermediate
// casts, so the result type is the type of the C expression:
// `(opus_val32)` → int32, `(opus_val16)` → int16, promoted-to-int → int32.
// Parameters are generic so that callers do not need to convert int16
// variables to int32 (the C macros are untyped).

// C: MULT16_16SU(a,b) ((opus_val32)(opus_val16)(a)*(opus_val32)(opus_uint16)(b))
func MULT16_16SU[A, B integer](a A, b B) int32 {
	return int32(int16(a)) * int32(uint16(b))
}

// C: MULT16_32_Q16(a,b) ((opus_val32)SHR((opus_int64)((opus_val16)(a))*(b),16))   (OPUS_FAST_INT64)
func MULT16_32_Q16[A, B integer](a A, b B) int32 {
	return int32((int64(int16(a)) * int64(b)) >> 16)
}

// C: MULT16_32_P16(a,b) ((opus_val32)PSHR((opus_int64)((opus_val16)(a))*(b),16))   (OPUS_FAST_INT64)
func MULT16_32_P16[A, B integer](a A, b B) int32 {
	return int32((int64(int16(a))*int64(b) + (int64(1) << 16 >> 1)) >> 16)
}

// C: MULT16_32_Q15(a,b) ((opus_val32)SHR((opus_int64)((opus_val16)(a))*(b),15))   (OPUS_FAST_INT64)
func MULT16_32_Q15[A, B integer](a A, b B) int32 {
	return int32((int64(int16(a)) * int64(b)) >> 15)
}

// C: MULT32_32_Q31(a,b) ((opus_val32)SHR((opus_int64)(a)*(opus_int64)(b),31))   (OPUS_FAST_INT64)
func MULT32_32_Q31[A, B integer](a A, b B) int32 {
	return int32((int64(a) * int64(b)) >> 31)
}

// C: QCONST16(x,bits) ((opus_val16)(.5+(x)*(((opus_val32)1)<<(bits))))
// x is a float literal in C (single precision), the product is computed in
// single precision and the .5 is added in double precision.
func QCONST16(x float32, bits uint) int16 {
	return int16(0.5 + float64(x*float32(int32(1)<<bits)))
}

// C: QCONST32(x,bits) ((opus_val32)(.5+(x)*(((opus_val32)1)<<(bits))))
func QCONST32(x float32, bits uint) int32 {
	return int32(0.5 + float64(x*float32(int32(1)<<bits)))
}

// C: NEG16(x) (-(x))
func NEG16[T integer](x T) T { return -x }

// C: NEG32(x) (-(x))
func NEG32[T integer](x T) T { return -x }

// C: EXTRACT16(x) ((opus_val16)(x))
func EXTRACT16[T integer](x T) int16 { return int16(x) }

// C: EXTEND32(x) ((opus_val32)(x))
func EXTEND32[T integer](x T) int32 { return int32(x) }

// C: SHR16(a,shift) ((a) >> (shift))   (promoted to int)
func SHR16[T integer](a T, shift int) int32 { return int32(a) >> uint(shift) }

// C: SHL16(a,shift) ((opus_int16)((opus_uint16)(a)<<(shift)))
func SHL16[T integer](a T, shift int) int16 { return int16(uint16(a) << uint(shift)) }

// C: SHR32(a,shift) ((a) >> (shift))
func SHR32[T integer](a T, shift int) int32 { return int32(a) >> uint(shift) }

// C: SHL32(a,shift) ((opus_int32)((opus_uint32)(a)<<(shift)))
func SHL32[T integer](a T, shift int) int32 { return int32(uint32(a) << uint(shift)) }

// C: PSHR32(a,shift) (SHR32((a)+((EXTEND32(1)<<((shift))>>1)),shift))
func PSHR32[T integer](a T, shift int) int32 {
	return SHR32(int32(a)+((int32(1)<<uint(shift))>>1), shift)
}

// C: VSHR32(a, shift) (((shift)>0) ? SHR32(a, shift) : SHL32(a, -(shift)))
func VSHR32[T integer](a T, shift int) int32 {
	if shift > 0 {
		return SHR32(a, shift)
	}
	return SHL32(a, -shift)
}

// C: SHR(a,shift) ((a) >> (shift))
func SHR[T integer](a T, shift int) int32 { return int32(a) >> uint(shift) }

// C: SHL(a,shift) SHL32(a,shift)
func SHL[T integer](a T, shift int) int32 { return SHL32(a, shift) }

// C: PSHR(a,shift) (SHR((a)+((EXTEND32(1)<<((shift))>>1)),shift))
func PSHR[T integer](a T, shift int) int32 {
	return SHR(int32(a)+((int32(1)<<uint(shift))>>1), shift)
}

// C: SATURATE(x,a) (((x)>(a) ? (a) : (x)<-(a) ? -(a) : (x)))
func SATURATE[T integer](x, a T) T {
	if x > a {
		return a
	}
	if x < -a {
		return -a
	}
	return x
}

// C: SATURATE16(x) (EXTRACT16((x)>32767 ? 32767 : (x)<-32768 ? -32768 : (x)))
func SATURATE16[T integer](x T) int16 {
	if int64(x) > 32767 {
		return 32767
	}
	if int64(x) < -32768 {
		return -32768
	}
	return EXTRACT16(x)
}

// C: ROUND16(x,a) (EXTRACT16(PSHR32((x),(a))))
func ROUND16[T integer](x T, a int) int16 { return EXTRACT16(PSHR32(x, a)) }

// C: HALF16(x) (SHR16(x,1))
func HALF16[T integer](x T) int32 { return SHR16(x, 1) }

// C: HALF32(x) (SHR32(x,1))
func HALF32[T integer](x T) int32 { return SHR32(x, 1) }

// C: ADD16(a,b) ((opus_val16)((opus_val16)(a)+(opus_val16)(b)))
func ADD16[A, B integer](a A, b B) int16 { return int16(int16(a) + int16(b)) }

// C: SUB16(a,b) ((opus_val16)(a)-(opus_val16)(b))   (promoted to int)
func SUB16[A, B integer](a A, b B) int32 { return int32(int16(a)) - int32(int16(b)) }

// C: ADD32(a,b) ((opus_val32)(a)+(opus_val32)(b))
func ADD32[A, B integer](a A, b B) int32 { return int32(a) + int32(b) }

// C: SUB32(a,b) ((opus_val32)(a)-(opus_val32)(b))
func SUB32[A, B integer](a A, b B) int32 { return int32(a) - int32(b) }

// C: MULT16_16_16(a,b) ((((opus_val16)(a))*((opus_val16)(b))))   (promoted to int)
func MULT16_16_16[A, B integer](a A, b B) int32 { return int32(int16(a)) * int32(int16(b)) }

// C: MULT16_16(a,b) (((opus_val32)(opus_val16)(a))*((opus_val32)(opus_val16)(b)))
func MULT16_16[A, B integer](a A, b B) int32 { return int32(int16(a)) * int32(int16(b)) }

// C: MAC16_16(c,a,b) (ADD32((c),MULT16_16((a),(b))))
func MAC16_16[C, A, B integer](c C, a A, b B) int32 { return ADD32(c, MULT16_16(a, b)) }

// 16x32 multiply, followed by a 15-bit shift right and 32-bit add.
// b must fit in 31 bits. Result fits in 32 bits.
//
// C: MAC16_32_Q15(c,a,b) ADD32((c),ADD32(MULT16_16((a),SHR((b),15)), SHR(MULT16_16((a),((b)&0x00007fff)),15)))
func MAC16_32_Q15[C, A, B integer](c C, a A, b B) int32 {
	bb := int32(b)
	return ADD32(c, ADD32(MULT16_16(a, SHR(bb, 15)), SHR(MULT16_16(a, bb&0x00007fff), 15)))
}

// 16x32 multiplication, followed by a 16-bit shift right and 32-bit add.
// Results fits in 32 bits.
//
// C: MAC16_32_Q16(c,a,b) ADD32((c),ADD32(MULT16_16((a),SHR((b),16)), SHR(MULT16_16SU((a),((b)&0x0000ffff)),16)))
func MAC16_32_Q16[C, A, B integer](c C, a A, b B) int32 {
	bb := int32(b)
	return ADD32(c, ADD32(MULT16_16(a, SHR(bb, 16)), SHR(MULT16_16SU(a, bb&0x0000ffff), 16)))
}

// C: MULT16_16_Q11_32(a,b) (SHR(MULT16_16((a),(b)),11))
func MULT16_16_Q11_32[A, B integer](a A, b B) int32 { return SHR(MULT16_16(a, b), 11) }

// C: MULT16_16_Q11(a,b) (SHR(MULT16_16((a),(b)),11))
func MULT16_16_Q11[A, B integer](a A, b B) int32 { return SHR(MULT16_16(a, b), 11) }

// C: MULT16_16_Q13(a,b) (SHR(MULT16_16((a),(b)),13))
func MULT16_16_Q13[A, B integer](a A, b B) int32 { return SHR(MULT16_16(a, b), 13) }

// C: MULT16_16_Q14(a,b) (SHR(MULT16_16((a),(b)),14))
func MULT16_16_Q14[A, B integer](a A, b B) int32 { return SHR(MULT16_16(a, b), 14) }

// C: MULT16_16_Q15(a,b) (SHR(MULT16_16((a),(b)),15))
func MULT16_16_Q15[A, B integer](a A, b B) int32 { return SHR(MULT16_16(a, b), 15) }

// C: MULT16_16_P13(a,b) (SHR(ADD32(4096,MULT16_16((a),(b))),13))
func MULT16_16_P13[A, B integer](a A, b B) int32 { return SHR(ADD32(4096, MULT16_16(a, b)), 13) }

// C: MULT16_16_P14(a,b) (SHR(ADD32(8192,MULT16_16((a),(b))),14))
func MULT16_16_P14[A, B integer](a A, b B) int32 { return SHR(ADD32(8192, MULT16_16(a, b)), 14) }

// C: MULT16_16_P15(a,b) (SHR(ADD32(16384,MULT16_16((a),(b))),15))
func MULT16_16_P15[A, B integer](a A, b B) int32 { return SHR(ADD32(16384, MULT16_16(a, b)), 15) }

// C: DIV32_16(a,b) ((opus_val16)(((opus_val32)(a))/((opus_val16)(b))))
func DIV32_16[A, B integer](a A, b B) int16 { return int16(int32(a) / int32(int16(b))) }

// C: DIV32(a,b) (((opus_val32)(a))/((opus_val32)(b)))
func DIV32[A, B integer](a A, b B) int32 { return int32(a) / int32(b) }

// C: static OPUS_INLINE opus_val16 SIG2WORD16_generic(celt_sig x)
func SIG2WORD16_generic(x int32) int16 {
	x = PSHR32(x, SIG_SHIFT)
	x = MAX32(x, -32768)
	x = MIN32(x, 32767)
	return EXTRACT16(x)
}

// C: #define SIG2WORD16(x) (SIG2WORD16_generic(x))
func SIG2WORD16(x int32) int16 { return SIG2WORD16_generic(x) }
