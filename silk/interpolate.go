package silk

// C: silk/interpolate.c

/* Interpolate two vectors */
//
// C: void silk_interpolate(opus_int16 xi[ MAX_LPC_ORDER ], const opus_int16 x0[ MAX_LPC_ORDER ],
// const opus_int16 x1[ MAX_LPC_ORDER ], const opus_int ifact_Q2, const opus_int d)
func Interpolate(xi []int16, x0 []int16, x1 []int16, ifact_Q2 int, d int) {
	var i int

	silk_assert(ifact_Q2 >= 0)
	silk_assert(ifact_Q2 <= 4)

	for i = 0; i < d; i++ {
		xi[i] = int16(ADD_RSHIFT(x0[i], SMULBB(int32(x1[i])-int32(x0[i]), ifact_Q2), 2))
	}
}
