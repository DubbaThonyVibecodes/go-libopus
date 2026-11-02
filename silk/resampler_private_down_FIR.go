package silk

// C: silk/resampler_private_down_FIR.c

// C: static OPUS_INLINE opus_int16 *silk_resampler_private_down_FIR_INTERPOL(
//
//	opus_int16          *out,
//	opus_int32          *buf,
//	const opus_int16    *FIR_Coefs,
//	opus_int            FIR_Order,
//	opus_int            FIR_Fracs,
//	opus_int32          max_index_Q16,
//	opus_int32          index_increment_Q16
//
// )
//
// Returns the advanced output pointer (the C returns `out` after the
// `*out++` stores).
func resamplerPrivateDownFIRINTERPOL(out []int16, buf []int32, FIR_Coefs []int16, FIR_Order int, FIR_Fracs int, max_index_Q16 int32, index_increment_Q16 int32) []int16 {
	var index_Q16, res_Q6 int32
	var buf_ptr []int32
	var interpol_ind int32
	var interpol_ptr []int16

	switch FIR_Order {
	case RESAMPLER_DOWN_ORDER_FIR0:
		for index_Q16 = 0; index_Q16 < max_index_Q16; index_Q16 += index_increment_Q16 {
			/* Integer part gives pointer to buffered input */
			buf_ptr = buf[RSHIFT(index_Q16, 16):]

			/* Fractional part gives interpolation coefficients */
			interpol_ind = SMULWB(index_Q16&0xFFFF, FIR_Fracs)

			/* Inner product */
			interpol_ptr = FIR_Coefs[RESAMPLER_DOWN_ORDER_FIR0/2*interpol_ind:]
			res_Q6 = SMULWB(buf_ptr[0], interpol_ptr[0])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[1], interpol_ptr[1])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[2], interpol_ptr[2])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[3], interpol_ptr[3])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[4], interpol_ptr[4])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[5], interpol_ptr[5])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[6], interpol_ptr[6])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[7], interpol_ptr[7])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[8], interpol_ptr[8])
			interpol_ptr = FIR_Coefs[RESAMPLER_DOWN_ORDER_FIR0/2*(int32(FIR_Fracs)-1-interpol_ind):]
			res_Q6 = SMLAWB(res_Q6, buf_ptr[17], interpol_ptr[0])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[16], interpol_ptr[1])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[15], interpol_ptr[2])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[14], interpol_ptr[3])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[13], interpol_ptr[4])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[12], interpol_ptr[5])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[11], interpol_ptr[6])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[10], interpol_ptr[7])
			res_Q6 = SMLAWB(res_Q6, buf_ptr[9], interpol_ptr[8])

			/* Scale down, saturate and store in output array */
			out[0] = int16(SAT16(RSHIFT_ROUND(res_Q6, 6)))
			out = out[1:]
		}
	case RESAMPLER_DOWN_ORDER_FIR1:
		for index_Q16 = 0; index_Q16 < max_index_Q16; index_Q16 += index_increment_Q16 {
			/* Integer part gives pointer to buffered input */
			buf_ptr = buf[RSHIFT(index_Q16, 16):]

			/* Inner product */
			res_Q6 = SMULWB(ADD32(buf_ptr[0], buf_ptr[23]), FIR_Coefs[0])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[1], buf_ptr[22]), FIR_Coefs[1])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[2], buf_ptr[21]), FIR_Coefs[2])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[3], buf_ptr[20]), FIR_Coefs[3])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[4], buf_ptr[19]), FIR_Coefs[4])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[5], buf_ptr[18]), FIR_Coefs[5])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[6], buf_ptr[17]), FIR_Coefs[6])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[7], buf_ptr[16]), FIR_Coefs[7])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[8], buf_ptr[15]), FIR_Coefs[8])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[9], buf_ptr[14]), FIR_Coefs[9])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[10], buf_ptr[13]), FIR_Coefs[10])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[11], buf_ptr[12]), FIR_Coefs[11])

			/* Scale down, saturate and store in output array */
			out[0] = int16(SAT16(RSHIFT_ROUND(res_Q6, 6)))
			out = out[1:]
		}
	case RESAMPLER_DOWN_ORDER_FIR2:
		for index_Q16 = 0; index_Q16 < max_index_Q16; index_Q16 += index_increment_Q16 {
			/* Integer part gives pointer to buffered input */
			buf_ptr = buf[RSHIFT(index_Q16, 16):]

			/* Inner product */
			res_Q6 = SMULWB(ADD32(buf_ptr[0], buf_ptr[35]), FIR_Coefs[0])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[1], buf_ptr[34]), FIR_Coefs[1])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[2], buf_ptr[33]), FIR_Coefs[2])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[3], buf_ptr[32]), FIR_Coefs[3])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[4], buf_ptr[31]), FIR_Coefs[4])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[5], buf_ptr[30]), FIR_Coefs[5])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[6], buf_ptr[29]), FIR_Coefs[6])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[7], buf_ptr[28]), FIR_Coefs[7])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[8], buf_ptr[27]), FIR_Coefs[8])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[9], buf_ptr[26]), FIR_Coefs[9])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[10], buf_ptr[25]), FIR_Coefs[10])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[11], buf_ptr[24]), FIR_Coefs[11])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[12], buf_ptr[23]), FIR_Coefs[12])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[13], buf_ptr[22]), FIR_Coefs[13])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[14], buf_ptr[21]), FIR_Coefs[14])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[15], buf_ptr[20]), FIR_Coefs[15])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[16], buf_ptr[19]), FIR_Coefs[16])
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr[17], buf_ptr[18]), FIR_Coefs[17])

			/* Scale down, saturate and store in output array */
			out[0] = int16(SAT16(RSHIFT_ROUND(res_Q6, 6)))
			out = out[1:]
		}
	default:
		silk_assert(false)
	}
	return out
}

// Resample with a 2nd order AR filter followed by FIR interpolation
//
// C: void silk_resampler_private_down_FIR(
//
//	void                            *SS,            /* I/O  Resampler state             */
//	opus_int16                      out[],          /* O    Output signal               */
//	const opus_int16                in[],           /* I    Input signal                */
//	opus_int32                      inLen           /* I    Number of input samples     */
//
// )
func (SS *ResamplerStateStruct) ResamplerPrivateDownFIR(out []int16, in []int16, inLen int32) {
	S := SS
	var nSamplesIn int32
	var max_index_Q16, index_increment_Q16 int32
	var buf []int32
	var FIR_Coefs []int16

	buf = make([]int32, S.batchSize+S.FIR_Order)

	/* Copy buffered samples to start of buffer */
	copy(buf[:S.FIR_Order], S.sFIR_i32[:S.FIR_Order])

	FIR_Coefs = S.Coefs[2:]

	/* Iterate over blocks of frameSizeIn input samples */
	index_increment_Q16 = S.invRatio_Q16
	for {
		nSamplesIn = min(inLen, int32(S.batchSize))

		/* Second-order AR filter (output in Q8) */
		ResamplerPrivateAR2(S.sIIR[:], buf[S.FIR_Order:], in, S.Coefs, nSamplesIn)

		max_index_Q16 = LSHIFT32(nSamplesIn, 16)

		/* Interpolate filtered signal */
		out = resamplerPrivateDownFIRINTERPOL(out, buf, FIR_Coefs, S.FIR_Order,
			S.FIR_Fracs, max_index_Q16, index_increment_Q16)

		in = in[nSamplesIn:]
		inLen -= nSamplesIn

		if inLen > 1 {
			/* More iterations to do; copy last part of filtered signal to beginning of buffer */
			copy(buf[:S.FIR_Order], buf[nSamplesIn:int(nSamplesIn)+S.FIR_Order])
		} else {
			break
		}
	}

	/* Copy last part of filtered signal to the state for the next call */
	copy(S.sFIR_i32[:S.FIR_Order], buf[nSamplesIn:int(nSamplesIn)+S.FIR_Order])
}
