package silk

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// C: silk/fixed/vector_ops_FIX.c

// Copy and multiply a vector by a constant
//
// C: void silk_scale_copy_vector16(opus_int16 *data_out, const opus_int16 *data_in, opus_int32 gain_Q16, const opus_int dataSize)
func ScaleCopyVector16(
	data_out []int16,
	data_in []int16,
	gain_Q16 int32, // I    Gain in Q16
	dataSize int, // I    Length
) {
	var i int
	var tmp32 int32

	for i = 0; i < dataSize; i++ {
		tmp32 = SMULWB(gain_Q16, data_in[i])
		data_out[i] = int16(CHECK_FIT16(tmp32))
	}
}

// Multiply a vector by a constant
//
// C: void silk_scale_vector32_Q26_lshift_18(opus_int32 *data1, opus_int32 gain_Q26, opus_int dataSize)
func ScaleVector32Q26Lshift18(
	data1 []int32, // I/O  Q0/Q18
	gain_Q26 int32, // I    Q26
	dataSize int, // I    length
) {
	var i int

	for i = 0; i < dataSize; i++ {
		data1[i] = int32(CHECK_FIT32(RSHIFT64(SMULL(data1[i], gain_Q26), 8))) // OUTPUT: Q18
	}
}

// sum = for(i=0;i<len;i++)inVec1[i]*inVec2[i];      ---        inner product
// Note for ARM asm:
//   - inVec1 and inVec2 should be at least 2 byte aligned.
//   - len should be positive 16bit integer.
//   - only when len>6, memory access can be reduced by half.
//
// C: opus_int32 silk_inner_prod_aligned(const opus_int16 *const inVec1, const opus_int16 *const inVec2, const opus_int len, int arch)
func InnerProdAligned(
	inVec1 []int16, //    I input vector 1
	inVec2 []int16, //    I input vector 2
	len int, //    I vector lengths
	arch int, //    I Run-time architecture
) int32 {
	return celt.InnerProd(inVec1, inVec2, len, arch)
}

// C: opus_int64 silk_inner_prod16_aligned_64_c(const opus_int16 *inVec1, const opus_int16 *inVec2, const opus_int len)
// C: #define silk_inner_prod16_aligned_64(inVec1, inVec2, len, arch) ((void)(arch),silk_inner_prod16_aligned_64_c(inVec1, inVec2, len))
func InnerProd16Aligned64(
	inVec1 []int16, //    I input vector 1
	inVec2 []int16, //    I input vector 2
	len int, //    I vector lengths
	arch int, //    I Run-time architecture
) int64 {
	var i int
	var sum int64 = 0
	for i = 0; i < len; i++ {
		sum = SMLALBB(sum, inVec1[i], inVec2[i])
	}
	return sum
}
