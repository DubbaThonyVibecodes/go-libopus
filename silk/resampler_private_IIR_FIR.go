package silk

// C: silk/resampler_private_IIR_FIR.c

// C: static inline opus_int16 *silk_resampler_private_IIR_FIR_INTERPOL(
//
//	opus_int16  *out,
//	opus_int16  *buf,
//	opus_int32  max_index_Q16,
//	opus_int32  index_increment_Q16
//
// )
//
// Returns the advanced output pointer (the C returns `out` after the
// `*out++` stores).
func resamplerPrivateIIRFIRINTERPOL(out []int16, buf []int16, max_index_Q16 int32, index_increment_Q16 int32) []int16 {
	var index_Q16, res_Q15 int32
	var buf_ptr []int16
	var table_index int32

	/* Interpolate upsampled signal and store in output array */
	for index_Q16 = 0; index_Q16 < max_index_Q16; index_Q16 += index_increment_Q16 {
		table_index = SMULWB(index_Q16&0xFFFF, 12)
		buf_ptr = buf[index_Q16>>16:]

		res_Q15 = SMULBB(buf_ptr[0], resampler_frac_FIR_12[table_index][0])
		res_Q15 = SMLABB(res_Q15, buf_ptr[1], resampler_frac_FIR_12[table_index][1])
		res_Q15 = SMLABB(res_Q15, buf_ptr[2], resampler_frac_FIR_12[table_index][2])
		res_Q15 = SMLABB(res_Q15, buf_ptr[3], resampler_frac_FIR_12[table_index][3])
		res_Q15 = SMLABB(res_Q15, buf_ptr[4], resampler_frac_FIR_12[11-table_index][3])
		res_Q15 = SMLABB(res_Q15, buf_ptr[5], resampler_frac_FIR_12[11-table_index][2])
		res_Q15 = SMLABB(res_Q15, buf_ptr[6], resampler_frac_FIR_12[11-table_index][1])
		res_Q15 = SMLABB(res_Q15, buf_ptr[7], resampler_frac_FIR_12[11-table_index][0])
		out[0] = int16(SAT16(RSHIFT_ROUND(res_Q15, 15)))
		out = out[1:]
	}
	return out
}

// Upsample using a combination of allpass-based 2x upsampling and FIR interpolation
//
// C: void silk_resampler_private_IIR_FIR(
//
//	void                            *SS,            /* I/O  Resampler state             */
//	opus_int16                      out[],          /* O    Output signal               */
//	const opus_int16                in[],           /* I    Input signal                */
//	opus_int32                      inLen           /* I    Number of input samples     */
//
// )
func (SS *ResamplerStateStruct) ResamplerPrivateIIRFIR(out []int16, in []int16, inLen int32) {
	S := SS
	var nSamplesIn int32
	var max_index_Q16, index_increment_Q16 int32

	buf := make([]int16, 2*S.batchSize+RESAMPLER_ORDER_FIR_12)

	/* Copy buffered samples to start of buffer */
	copy(buf[:RESAMPLER_ORDER_FIR_12], S.sFIR_i16[:RESAMPLER_ORDER_FIR_12])

	/* Iterate over blocks of frameSizeIn input samples */
	index_increment_Q16 = S.invRatio_Q16
	for {
		nSamplesIn = min(inLen, int32(S.batchSize))

		/* Upsample 2x */
		ResamplerPrivateUp2HQ(S.sIIR[:], buf[RESAMPLER_ORDER_FIR_12:], in, nSamplesIn)

		max_index_Q16 = LSHIFT32(nSamplesIn, 16+1) /* + 1 because 2x upsampling */
		out = resamplerPrivateIIRFIRINTERPOL(out, buf, max_index_Q16, index_increment_Q16)
		in = in[nSamplesIn:]
		inLen -= nSamplesIn

		if inLen > 0 {
			/* More iterations to do; copy last part of filtered signal to beginning of buffer */
			copy(buf[:RESAMPLER_ORDER_FIR_12], buf[nSamplesIn<<1:int(nSamplesIn<<1)+RESAMPLER_ORDER_FIR_12])
		} else {
			break
		}
	}

	/* Copy last part of filtered signal to the state for the next call */
	copy(S.sFIR_i16[:RESAMPLER_ORDER_FIR_12], buf[nSamplesIn<<1:int(nSamplesIn<<1)+RESAMPLER_ORDER_FIR_12])
}
