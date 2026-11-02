package silk

// C: silk/fixed/k2a_FIX.c

// Step up function, converts reflection coefficients to prediction coefficients
//
// C: void silk_k2a(opus_int32 *A_Q24, const opus_int16 *rc_Q15, const opus_int32 order)
func K2a(
	A_Q24 []int32, // O    Prediction coefficients [order] Q24
	rc_Q15 []int16, // I    Reflection coefficients [order] Q15
	order int32, // I    Prediction order
) {
	var k, n int
	var rc, tmp1, tmp2 int32

	for k = 0; k < int(order); k++ {
		rc = int32(rc_Q15[k])
		for n = 0; n < (k+1)>>1; n++ {
			tmp1 = A_Q24[n]
			tmp2 = A_Q24[k-n-1]
			A_Q24[n] = SMLAWB(tmp1, LSHIFT(tmp2, 1), rc)
			A_Q24[k-n-1] = SMLAWB(tmp2, LSHIFT(tmp1, 1), rc)
		}
		A_Q24[k] = -LSHIFT(int32(rc_Q15[k]), 9)
	}
}
