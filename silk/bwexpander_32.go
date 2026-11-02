package silk

// C: silk/bwexpander_32.c

// Chirp (bandwidth expand) LP AR filter.
// This logic is reused in _celt_lpc(). Any bug fixes should also be applied there.
//
// C: void silk_bwexpander_32(opus_int32 *ar, const opus_int d, opus_int32 chirp_Q16)
func Bwexpander32(
	ar []int32, // I/O  AR filter to be expanded (without leading 1)
	d int, // I    Length of ar
	chirp_Q16 int32, // I    Chirp factor in Q16
) {
	var i int
	var chirp_minus_one_Q16 int32 = chirp_Q16 - 65536

	for i = 0; i < d-1; i++ {
		ar[i] = SMULWW(chirp_Q16, ar[i])
		chirp_Q16 += RSHIFT_ROUND(MUL(chirp_Q16, chirp_minus_one_Q16), 16)
	}
	ar[d-1] = SMULWW(chirp_Q16, ar[d-1])
}
