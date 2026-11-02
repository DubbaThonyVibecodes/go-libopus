package silk

// C: silk/inner_prod_aligned.c

// C: opus_int32 silk_inner_prod_aligned_scale(const opus_int16 *const inVec1, const opus_int16 *const inVec2, const opus_int scale, const opus_int len)
func InnerProdAlignedScale(
	inVec1 []int16, //    I input vector 1
	inVec2 []int16, //    I input vector 2
	scale int, //    I number of bits to shift
	len int, //    I vector lengths
) int32 {
	var i int
	var sum int32 = 0
	for i = 0; i < len; i++ {
		sum = ADD_RSHIFT32(sum, SMULBB(inVec1[i], inVec2[i]), scale)
	}
	return sum
}
