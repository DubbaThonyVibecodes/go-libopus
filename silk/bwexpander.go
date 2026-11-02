package silk

// C: silk/bwexpander.c

// Chirp (bandwidth expand) LP AR filter
//
// C: void silk_bwexpander(opus_int16 *ar, const opus_int d, opus_int32 chirp_Q16)
func Bwexpander(
	ar []int16, // I/O  AR filter to be expanded (without leading 1)
	d int, // I    Length of ar
	chirp_Q16 int32, // I    Chirp factor (typically in the range 0 to 1)
) {
	var i int
	var chirp_minus_one_Q16 int32 = chirp_Q16 - 65536

	// NB: Dont use silk_SMULWB, instead of silk_RSHIFT_ROUND( silk_MUL(), 16 ), below.
	// Bias in silk_SMULWB can lead to unstable filters
	for i = 0; i < d-1; i++ {
		ar[i] = int16(RSHIFT_ROUND(MUL(chirp_Q16, ar[i]), 16))
		chirp_Q16 += RSHIFT_ROUND(MUL(chirp_Q16, chirp_minus_one_Q16), 16)
	}
	ar[d-1] = int16(RSHIFT_ROUND(MUL(chirp_Q16, ar[d-1]), 16))
}
