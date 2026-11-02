package silk

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// C: silk/decode_indices.c

/* Decode side-information parameters from payload */
//
// C: void silk_decode_indices(silk_decoder_state *psDec, ec_dec *psRangeDec, opus_int FrameIndex,
// opus_int decode_LBRR, opus_int condCoding)
func (psDec *DecoderState) DecodeIndices(psRangeDec *celt.EcDec, FrameIndex int, decode_LBRR int, condCoding int) {
	var i, k, Ix int
	var decode_absolute_lagIndex, delta_lagIndex int
	var ec_ix [MAX_LPC_ORDER]int16
	var pred_Q8 [MAX_LPC_ORDER]uint8

	/*******************************************/
	/* Decode signal type and quantizer offset */
	/*******************************************/
	if decode_LBRR != 0 || psDec.VAD_flags[FrameIndex] != 0 {
		Ix = psRangeDec.DecIcdf(type_offset_VAD_iCDF[:], 8) + 2
	} else {
		Ix = psRangeDec.DecIcdf(type_offset_no_VAD_iCDF[:], 8)
	}
	psDec.indices.signalType = int8(RSHIFT(Ix, 1))
	psDec.indices.quantOffsetType = int8(Ix & 1)

	/****************/
	/* Decode gains */
	/****************/
	/* First subframe */
	if condCoding == CODE_CONDITIONALLY {
		/* Conditional coding */
		psDec.indices.GainsIndices[0] = int8(psRangeDec.DecIcdf(delta_gain_iCDF[:], 8))
	} else {
		/* Independent coding, in two stages: MSB bits followed by 3 LSBs */
		psDec.indices.GainsIndices[0] = int8(LSHIFT(psRangeDec.DecIcdf(gain_iCDF[psDec.indices.signalType][:], 8), 3))
		psDec.indices.GainsIndices[0] += int8(psRangeDec.DecIcdf(uniform8_iCDF[:], 8))
	}

	/* Remaining subframes */
	for i = 1; i < psDec.nb_subfr; i++ {
		psDec.indices.GainsIndices[i] = int8(psRangeDec.DecIcdf(delta_gain_iCDF[:], 8))
	}

	/**********************/
	/* Decode LSF Indices */
	/**********************/
	psDec.indices.NLSFIndices[0] = int8(psRangeDec.DecIcdf(psDec.psNLSF_CB.CB1_iCDF[(int(psDec.indices.signalType)>>1)*int(psDec.psNLSF_CB.nVectors):], 8))
	NLSFUnpack(ec_ix[:], pred_Q8[:], psDec.psNLSF_CB, int(psDec.indices.NLSFIndices[0]))
	silk_assert(int(psDec.psNLSF_CB.order) == psDec.LPC_order)
	for i = 0; i < int(psDec.psNLSF_CB.order); i++ {
		Ix = psRangeDec.DecIcdf(psDec.psNLSF_CB.ec_iCDF[ec_ix[i]:], 8)
		if Ix == 0 {
			Ix -= psRangeDec.DecIcdf(NLSF_EXT_iCDF[:], 8)
		} else if Ix == 2*NLSF_QUANT_MAX_AMPLITUDE {
			Ix += psRangeDec.DecIcdf(NLSF_EXT_iCDF[:], 8)
		}
		psDec.indices.NLSFIndices[i+1] = int8(Ix - NLSF_QUANT_MAX_AMPLITUDE)
	}

	/* Decode LSF interpolation factor */
	if psDec.nb_subfr == MAX_NB_SUBFR {
		psDec.indices.NLSFInterpCoef_Q2 = int8(psRangeDec.DecIcdf(NLSF_interpolation_factor_iCDF[:], 8))
	} else {
		psDec.indices.NLSFInterpCoef_Q2 = 4
	}

	if psDec.indices.signalType == TYPE_VOICED {
		/*********************/
		/* Decode pitch lags */
		/*********************/
		/* Get lag index */
		decode_absolute_lagIndex = 1
		if condCoding == CODE_CONDITIONALLY && psDec.ec_prevSignalType == TYPE_VOICED {
			/* Decode Delta index */
			delta_lagIndex = int(int16(psRangeDec.DecIcdf(pitch_delta_iCDF[:], 8)))
			if delta_lagIndex > 0 {
				delta_lagIndex = delta_lagIndex - 9
				psDec.indices.lagIndex = int16(int(psDec.ec_prevLagIndex) + delta_lagIndex)
				decode_absolute_lagIndex = 0
			}
		}
		if decode_absolute_lagIndex != 0 {
			/* Absolute decoding */
			psDec.indices.lagIndex = int16(psRangeDec.DecIcdf(pitch_lag_iCDF[:], 8)) * int16(RSHIFT(psDec.fs_kHz, 1))
			psDec.indices.lagIndex += int16(psRangeDec.DecIcdf(psDec.pitch_lag_low_bits_iCDF, 8))
		}
		psDec.ec_prevLagIndex = psDec.indices.lagIndex

		/* Get countour index */
		psDec.indices.contourIndex = int8(psRangeDec.DecIcdf(psDec.pitch_contour_iCDF, 8))

		/********************/
		/* Decode LTP gains */
		/********************/
		/* Decode PERIndex value */
		psDec.indices.PERIndex = int8(psRangeDec.DecIcdf(LTP_per_index_iCDF[:], 8))

		for k = 0; k < psDec.nb_subfr; k++ {
			psDec.indices.LTPIndex[k] = int8(psRangeDec.DecIcdf(LTP_gain_iCDF_ptrs[psDec.indices.PERIndex], 8))
		}

		/**********************/
		/* Decode LTP scaling */
		/**********************/
		if condCoding == CODE_INDEPENDENTLY {
			psDec.indices.LTP_scaleIndex = int8(psRangeDec.DecIcdf(LTPscale_iCDF[:], 8))
		} else {
			psDec.indices.LTP_scaleIndex = 0
		}
	}
	psDec.ec_prevSignalType = int(psDec.indices.signalType)

	/***************/
	/* Decode seed */
	/***************/
	psDec.indices.Seed = int8(psRangeDec.DecIcdf(uniform4_iCDF[:], 8))
}
