package silk

// C: silk/LPC_analysis_filter.c

// *******************************************
// LPC analysis filter
// NB! State is kept internally and the
// filter always starts with zero state
// first d output samples are set to zero
// *******************************************

// OPT: Using celt_fir() for this function should be faster, but it may cause
// integer overflows in intermediate values (not final results), which the
// current implementation silences by casting to unsigned. Enabling
// this should be safe in pretty much all cases, even though it is not technically
// C89-compliant.
//
// C: #define USE_CELT_FIR 0
const USE_CELT_FIR = 0

// C: void silk_LPC_analysis_filter(opus_int16 *out, const opus_int16 *in, const opus_int16 *B, const opus_int32 len, const opus_int32 d, int arch)
//
// Only the `#else` (USE_CELT_FIR == 0) branch is ported.
func LPCAnalysisFilter(
	out []int16, // O    Output signal
	in []int16, // I    Input signal
	B []int16, // I    MA prediction coefficients, Q12 [order]
	len int32, // I    Signal length
	d int32, // I    Filter order
	arch int, // I    Run-time architecture
) {
	var j int
	var ix int
	var out32_Q12, out32 int32
	var in_ptr int // C: const opus_int16 *in_ptr; read at negative offsets, so kept as an index into in[]

	silk_assert(d >= 6)
	silk_assert((d & 1) == 0)
	silk_assert(d <= len)

	_ = arch
	for ix = int(d); ix < int(len); ix++ {
		in_ptr = ix - 1

		out32_Q12 = SMULBB(in[in_ptr+0], B[0])
		/* Allowing wrap around so that two wraps can cancel each other. The rare
		   cases where the result wraps around can only be triggered by invalid streams*/
		out32_Q12 = SMLABB_ovflw(out32_Q12, in[in_ptr-1], B[1])
		out32_Q12 = SMLABB_ovflw(out32_Q12, in[in_ptr-2], B[2])
		out32_Q12 = SMLABB_ovflw(out32_Q12, in[in_ptr-3], B[3])
		out32_Q12 = SMLABB_ovflw(out32_Q12, in[in_ptr-4], B[4])
		out32_Q12 = SMLABB_ovflw(out32_Q12, in[in_ptr-5], B[5])
		for j = 6; j < int(d); j += 2 {
			out32_Q12 = SMLABB_ovflw(out32_Q12, in[in_ptr-j], B[j])
			out32_Q12 = SMLABB_ovflw(out32_Q12, in[in_ptr-j-1], B[j+1])
		}

		/* Subtract prediction */
		out32_Q12 = SUB32_ovflw(LSHIFT(int32(in[in_ptr+1]), 12), out32_Q12)

		/* Scale to Q0 */
		out32 = RSHIFT_ROUND(out32_Q12, 12)

		/* Saturate output */
		out[ix] = int16(SAT16(out32))
	}

	/* Set first d output samples to zero */
	clear(out[:d])
}
