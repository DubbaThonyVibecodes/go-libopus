package silk

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// C: silk/shell_coder.c

/* shell coder; pulse-subframe length is hardcoded */

// C: static inline void combine_pulses(opus_int *out, const opus_int *in, const opus_int len)
func combinePulses(
	out []int, /* O    combined pulses vector [len] */
	in []int, /* I    input vector       [2 * len] */
	len int, /* I    number of OUTPUT samples     */
) {
	var k int
	for k = 0; k < len; k++ {
		out[k] = in[2*k] + in[2*k+1]
	}
}

// C: static inline void encode_split(ec_enc *psRangeEnc, const opus_int p_child1, const opus_int p,
// const opus_uint8 *shell_table)
func encodeSplit(
	psRangeEnc *celt.EcEnc, /* I/O  compressor data structure                   */
	p_child1 int, /* I    pulse amplitude of first child subframe     */
	p int, /* I    pulse amplitude of current subframe         */
	shell_table []uint8, /* I    table of shell cdfs                         */
) {
	if p > 0 {
		psRangeEnc.EncIcdf(p_child1, shell_table[shell_code_table_offsets[p]:], 8)
	}
}

// C: static inline void decode_split(opus_int *p_child1, opus_int *p_child2, ec_dec *psRangeDec,
// const opus_int p, const opus_uint8 *shell_table)
func decodeSplit(
	p_child1 *int, /* O    pulse amplitude of first child subframe     */
	p_child2 *int, /* O    pulse amplitude of second child subframe    */
	psRangeDec *celt.EcDec, /* I/O  Compressor data structure                   */
	p int, /* I    pulse amplitude of current subframe         */
	shell_table []uint8, /* I    table of shell cdfs                         */
) {
	if p > 0 {
		*p_child1 = psRangeDec.DecIcdf(shell_table[shell_code_table_offsets[p]:], 8)
		*p_child2 = p - *p_child1
	} else {
		*p_child1 = 0
		*p_child2 = 0
	}
}

/* Shell encoder, operates on one shell code frame of 16 pulses */
//
// C: void silk_shell_encoder(ec_enc *psRangeEnc, const opus_int *pulses0)
func ShellEncoder(psRangeEnc *celt.EcEnc, pulses0 []int) {
	var pulses1 [8]int
	var pulses2 [4]int
	var pulses3 [2]int
	var pulses4 [1]int

	/* this function operates on one shell code frame of 16 pulses */
	silk_assert(SHELL_CODEC_FRAME_LENGTH == 16)

	/* tree representation per pulse-subframe */
	combinePulses(pulses1[:], pulses0, 8)
	combinePulses(pulses2[:], pulses1[:], 4)
	combinePulses(pulses3[:], pulses2[:], 2)
	combinePulses(pulses4[:], pulses3[:], 1)

	encodeSplit(psRangeEnc, pulses3[0], pulses4[0], shell_code_table3[:])

	encodeSplit(psRangeEnc, pulses2[0], pulses3[0], shell_code_table2[:])

	encodeSplit(psRangeEnc, pulses1[0], pulses2[0], shell_code_table1[:])
	encodeSplit(psRangeEnc, pulses0[0], pulses1[0], shell_code_table0[:])
	encodeSplit(psRangeEnc, pulses0[2], pulses1[1], shell_code_table0[:])

	encodeSplit(psRangeEnc, pulses1[2], pulses2[1], shell_code_table1[:])
	encodeSplit(psRangeEnc, pulses0[4], pulses1[2], shell_code_table0[:])
	encodeSplit(psRangeEnc, pulses0[6], pulses1[3], shell_code_table0[:])

	encodeSplit(psRangeEnc, pulses2[2], pulses3[1], shell_code_table2[:])

	encodeSplit(psRangeEnc, pulses1[4], pulses2[2], shell_code_table1[:])
	encodeSplit(psRangeEnc, pulses0[8], pulses1[4], shell_code_table0[:])
	encodeSplit(psRangeEnc, pulses0[10], pulses1[5], shell_code_table0[:])

	encodeSplit(psRangeEnc, pulses1[6], pulses2[3], shell_code_table1[:])
	encodeSplit(psRangeEnc, pulses0[12], pulses1[6], shell_code_table0[:])
	encodeSplit(psRangeEnc, pulses0[14], pulses1[7], shell_code_table0[:])
}

/* Shell decoder, operates on one shell code frame of 16 pulses */
//
// C: void silk_shell_decoder(opus_int *pulses0, ec_dec *psRangeDec, const opus_int pulses4)
func ShellDecoder(pulses0 []int, psRangeDec *celt.EcDec, pulses4 int) {
	var pulses3 [2]int
	var pulses2 [4]int
	var pulses1 [8]int

	/* this function operates on one shell code frame of 16 pulses */
	silk_assert(SHELL_CODEC_FRAME_LENGTH == 16)

	decodeSplit(&pulses3[0], &pulses3[1], psRangeDec, pulses4, shell_code_table3[:])

	decodeSplit(&pulses2[0], &pulses2[1], psRangeDec, pulses3[0], shell_code_table2[:])

	decodeSplit(&pulses1[0], &pulses1[1], psRangeDec, pulses2[0], shell_code_table1[:])
	decodeSplit(&pulses0[0], &pulses0[1], psRangeDec, pulses1[0], shell_code_table0[:])
	decodeSplit(&pulses0[2], &pulses0[3], psRangeDec, pulses1[1], shell_code_table0[:])

	decodeSplit(&pulses1[2], &pulses1[3], psRangeDec, pulses2[1], shell_code_table1[:])
	decodeSplit(&pulses0[4], &pulses0[5], psRangeDec, pulses1[2], shell_code_table0[:])
	decodeSplit(&pulses0[6], &pulses0[7], psRangeDec, pulses1[3], shell_code_table0[:])

	decodeSplit(&pulses2[2], &pulses2[3], psRangeDec, pulses3[1], shell_code_table2[:])

	decodeSplit(&pulses1[4], &pulses1[5], psRangeDec, pulses2[2], shell_code_table1[:])
	decodeSplit(&pulses0[8], &pulses0[9], psRangeDec, pulses1[4], shell_code_table0[:])
	decodeSplit(&pulses0[10], &pulses0[11], psRangeDec, pulses1[5], shell_code_table0[:])

	decodeSplit(&pulses1[6], &pulses1[7], psRangeDec, pulses2[3], shell_code_table1[:])
	decodeSplit(&pulses0[12], &pulses0[13], psRangeDec, pulses1[6], shell_code_table0[:])
	decodeSplit(&pulses0[14], &pulses0[15], psRangeDec, pulses1[7], shell_code_table0[:])
}
