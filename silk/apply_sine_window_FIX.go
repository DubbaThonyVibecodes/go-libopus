package silk

// C: silk/fixed/apply_sine_window_FIX.c

// Apply sine window to signal vector.
// Window types:
//    1 -> sine window from 0 to pi/2
//    2 -> sine window from pi/2 to pi
// Every other sample is linearly interpolated, for speed.
// Window length must be between 16 and 120 (incl) and a multiple of 4.

// Matlab code for table:
//
//	for k=16:9*4:16+2*9*4, fprintf(' %7.d,', -round(65536*pi ./ (k:4:k+8*4))); fprintf('\n'); end
var freq_table_Q16 = [27]int16{
	12111, 9804, 8235, 7100, 6239, 5565, 5022, 4575, 4202,
	3885, 3612, 3375, 3167, 2984, 2820, 2674, 2542, 2422,
	2313, 2214, 2123, 2038, 1961, 1889, 1822, 1760, 1702,
}

// C: void silk_apply_sine_window(opus_int16 px_win[], const opus_int16 px[], const opus_int win_type, const opus_int length)
func ApplySineWindow(
	px_win []int16, // O    Pointer to windowed signal
	px []int16, // I    Pointer to input signal
	win_type int, // I    Selects a window type
	length int, // I    Window length, multiple of 4
) {
	var k, f_Q16, c_Q16 int
	var S0_Q16, S1_Q16 int32

	silk_assert(win_type == 1 || win_type == 2)

	// Length must be in a range from 16 to 120 and a multiple of 4
	silk_assert(length >= 16 && length <= 120)
	silk_assert((length & 3) == 0)

	// Frequency
	k = (length >> 2) - 4
	silk_assert(k >= 0 && k <= 26)
	f_Q16 = int(freq_table_Q16[k])

	// Factor used for cosine approximation
	c_Q16 = int(SMULWB(int32(f_Q16), -f_Q16))
	silk_assert(c_Q16 >= -32768)

	// initialize state
	if win_type == 1 {
		// start from 0
		S0_Q16 = 0
		// approximation of sin(f)
		S1_Q16 = int32(f_Q16 + RSHIFT(length, 3))
	} else {
		// start from 1
		S0_Q16 = (int32(1) << 16)
		// approximation of cos(f)
		S1_Q16 = (int32(1) << 16) + int32(RSHIFT(c_Q16, 1)) + int32(RSHIFT(length, 4))
	}

	// Uses the recursive equation:   sin(n*f) = 2 * cos(f) * sin((n-1)*f) - sin((n-2)*f)
	// 4 samples at a time
	for k = 0; k < length; k += 4 {
		px_win[k] = int16(SMULWB(RSHIFT(S0_Q16+S1_Q16, 1), px[k]))
		px_win[k+1] = int16(SMULWB(S1_Q16, px[k+1]))
		S0_Q16 = SMULWB(S1_Q16, c_Q16) + LSHIFT(S1_Q16, 1) - S0_Q16 + 1
		S0_Q16 = min(S0_Q16, (int32(1) << 16))

		px_win[k+2] = int16(SMULWB(RSHIFT(S0_Q16+S1_Q16, 1), px[k+2]))
		px_win[k+3] = int16(SMULWB(S0_Q16, px[k+3]))
		S1_Q16 = SMULWB(S0_Q16, c_Q16) + LSHIFT(S0_Q16, 1) - S1_Q16
		S1_Q16 = min(S1_Q16, (int32(1) << 16))
	}
}
