package silk

// C: silk/LPC_analysis_filter.c

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// *******************************************
// LPC analysis filter
// NB! State is kept internally and the
// filter always starts with zero state
// first d output samples are set to zero
// *******************************************
//
// C: void silk_LPC_analysis_filter(opus_int16 *out, const opus_int16 *in, const opus_int16 *B, const opus_int32 len, const opus_int32 d, int arch)
func LPCAnalysisFilter(
	out []int16, // O    Output signal
	in []int16, // I    Input signal
	B []int16, // I    MA prediction coefficients, Q12 [order]
	len int32, // I    Signal length
	d int32, // I    Filter order
	arch int, // I    Run-time architecture
) {
	var j int
	var mem [SILK_MAX_ORDER_LPC]int16
	var num [SILK_MAX_ORDER_LPC]int16

	silk_assert(d >= 6)
	silk_assert((d & 1) == 0)
	silk_assert(d <= len)

	silk_assert(d <= SILK_MAX_ORDER_LPC)
	for j = 0; j < int(d); j++ {
		num[j] = -B[j]
	}
	for j = 0; j < int(d); j++ {
		mem[j] = in[int(d)-j-1]
	}
	celt.Fir(in[d:], num[:], out[d:], int(len-d), int(d), mem[:], arch)
	for j = 0; j < int(d); j++ {
		out[j] = 0
	}
}
