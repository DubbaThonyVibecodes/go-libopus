package silk

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// C: silk/code_signs.c

/*#define silk_enc_map(a)                ((a) > 0 ? 1 : 0)*/
/*#define silk_dec_map(a)                ((a) > 0 ? 1 : -1)*/
/* shifting avoids if-statement */

// C: #define silk_enc_map(a) ( silk_RSHIFT( (a), 15 ) + 1 )
func encMap[T integer](a T) int { return int(RSHIFT(int(a), 15)) + 1 }

// C: #define silk_dec_map(a) ( silk_LSHIFT( (a),  1 ) - 1 )
func decMap[T integer](a T) int { return int(LSHIFT(a, 1)) - 1 }

/* Encodes signs of excitation */
//
// C: void silk_encode_signs(ec_enc *psRangeEnc, const opus_int8 pulses[], opus_int length,
// const opus_int signalType, const opus_int quantOffsetType, const opus_int sum_pulses[ MAX_NB_SHELL_BLOCKS ])
func EncodeSigns(psRangeEnc *celt.EcEnc, pulses []int8, length int, signalType int, quantOffsetType int, sum_pulses []int) {
	var i, j, p int
	var icdf [2]uint8
	var q_ptr []int8
	var icdf_ptr []uint8

	icdf[1] = 0
	q_ptr = pulses
	i = int(SMULBB(7, ADD_LSHIFT(quantOffsetType, signalType, 1)))
	icdf_ptr = sign_iCDF[i:]
	length = RSHIFT(length+SHELL_CODEC_FRAME_LENGTH/2, LOG2_SHELL_CODEC_FRAME_LENGTH)
	for i = 0; i < length; i++ {
		p = sum_pulses[i]
		if p > 0 {
			icdf[0] = icdf_ptr[min(p&0x1F, 6)]
			for j = 0; j < SHELL_CODEC_FRAME_LENGTH; j++ {
				if q_ptr[j] != 0 {
					psRangeEnc.EncIcdf(encMap(q_ptr[j]), icdf[:], 8)
				}
			}
		}
		q_ptr = q_ptr[SHELL_CODEC_FRAME_LENGTH:]
	}
}

/* Decodes signs of excitation */
//
// C: void silk_decode_signs(ec_dec *psRangeDec, opus_int16 pulses[], opus_int length,
// const opus_int signalType, const opus_int quantOffsetType, const opus_int sum_pulses[ MAX_NB_SHELL_BLOCKS ])
func DecodeSigns(psRangeDec *celt.EcDec, pulses []int16, length int, signalType int, quantOffsetType int, sum_pulses []int) {
	var i, j, p int
	var icdf [2]uint8
	var q_ptr []int16
	var icdf_ptr []uint8

	icdf[1] = 0
	q_ptr = pulses
	i = int(SMULBB(7, ADD_LSHIFT(quantOffsetType, signalType, 1)))
	icdf_ptr = sign_iCDF[i:]
	length = RSHIFT(length+SHELL_CODEC_FRAME_LENGTH/2, LOG2_SHELL_CODEC_FRAME_LENGTH)
	for i = 0; i < length; i++ {
		p = sum_pulses[i]
		if p > 0 {
			icdf[0] = icdf_ptr[min(p&0x1F, 6)]
			for j = 0; j < SHELL_CODEC_FRAME_LENGTH; j++ {
				if q_ptr[j] > 0 {
					/* attach sign */
					/* implementation with shift, subtraction, multiplication */
					q_ptr[j] *= int16(decMap(psRangeDec.DecIcdf(icdf[:], 8)))
				}
			}
		}
		q_ptr = q_ptr[SHELL_CODEC_FRAME_LENGTH:]
	}
}
