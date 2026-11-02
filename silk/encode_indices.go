package silk

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// C: silk/encode_indices.c

// Encode side-information parameters to payload
//
// C: void silk_encode_indices(silk_encoder_state *psEncC, ec_enc *psRangeEnc, opus_int FrameIndex, opus_int encode_LBRR, opus_int condCoding)
func (psEncC *EncoderState) EncodeIndices(psRangeEnc *celt.EcEnc, FrameIndex int, encode_LBRR int, condCoding int) {
	var i, k, typeOffset int
	var encode_absolute_lagIndex, delta_lagIndex int
	var ec_ix [MAX_LPC_ORDER]int16
	var pred_Q8 [MAX_LPC_ORDER]uint8
	var psIndices *SideInfoIndices

	if encode_LBRR != 0 {
		psIndices = &psEncC.indices_LBRR[FrameIndex]
	} else {
		psIndices = &psEncC.indices
	}

	/*******************************************/
	/* Encode signal type and quantizer offset */
	/*******************************************/
	typeOffset = 2*int(psIndices.signalType) + int(psIndices.quantOffsetType)
	silk_assert(typeOffset >= 0 && typeOffset < 6)
	silk_assert(encode_LBRR == 0 || typeOffset >= 2)
	if encode_LBRR != 0 || typeOffset >= 2 {
		psRangeEnc.EncIcdf(typeOffset-2, type_offset_VAD_iCDF[:], 8)
	} else {
		psRangeEnc.EncIcdf(typeOffset, type_offset_no_VAD_iCDF[:], 8)
	}

	/****************/
	/* Encode gains */
	/****************/
	/* first subframe */
	if condCoding == CODE_CONDITIONALLY {
		/* conditional coding */
		silk_assert(psIndices.GainsIndices[0] >= 0 && psIndices.GainsIndices[0] < MAX_DELTA_GAIN_QUANT-MIN_DELTA_GAIN_QUANT+1)
		psRangeEnc.EncIcdf(int(psIndices.GainsIndices[0]), delta_gain_iCDF[:], 8)
	} else {
		/* independent coding, in two stages: MSB bits followed by 3 LSBs */
		silk_assert(psIndices.GainsIndices[0] >= 0 && psIndices.GainsIndices[0] < N_LEVELS_QGAIN)
		psRangeEnc.EncIcdf(int(RSHIFT(psIndices.GainsIndices[0], 3)), gain_iCDF[psIndices.signalType][:], 8)
		psRangeEnc.EncIcdf(int(psIndices.GainsIndices[0]&7), uniform8_iCDF[:], 8)
	}

	/* remaining subframes */
	for i = 1; i < psEncC.nb_subfr; i++ {
		silk_assert(psIndices.GainsIndices[i] >= 0 && psIndices.GainsIndices[i] < MAX_DELTA_GAIN_QUANT-MIN_DELTA_GAIN_QUANT+1)
		psRangeEnc.EncIcdf(int(psIndices.GainsIndices[i]), delta_gain_iCDF[:], 8)
	}

	/****************/
	/* Encode NLSFs */
	/****************/
	psRangeEnc.EncIcdf(int(psIndices.NLSFIndices[0]), psEncC.psNLSF_CB.CB1_iCDF[int(psIndices.signalType>>1)*int(psEncC.psNLSF_CB.nVectors):], 8)
	NLSFUnpack(ec_ix[:], pred_Q8[:], psEncC.psNLSF_CB, int(psIndices.NLSFIndices[0]))
	silk_assert(int(psEncC.psNLSF_CB.order) == psEncC.predictLPCOrder)
	for i = 0; i < int(psEncC.psNLSF_CB.order); i++ {
		if psIndices.NLSFIndices[i+1] >= NLSF_QUANT_MAX_AMPLITUDE {
			psRangeEnc.EncIcdf(2*NLSF_QUANT_MAX_AMPLITUDE, psEncC.psNLSF_CB.ec_iCDF[ec_ix[i]:], 8)
			psRangeEnc.EncIcdf(int(psIndices.NLSFIndices[i+1])-NLSF_QUANT_MAX_AMPLITUDE, NLSF_EXT_iCDF[:], 8)
		} else if psIndices.NLSFIndices[i+1] <= -NLSF_QUANT_MAX_AMPLITUDE {
			psRangeEnc.EncIcdf(0, psEncC.psNLSF_CB.ec_iCDF[ec_ix[i]:], 8)
			psRangeEnc.EncIcdf(-int(psIndices.NLSFIndices[i+1])-NLSF_QUANT_MAX_AMPLITUDE, NLSF_EXT_iCDF[:], 8)
		} else {
			psRangeEnc.EncIcdf(int(psIndices.NLSFIndices[i+1])+NLSF_QUANT_MAX_AMPLITUDE, psEncC.psNLSF_CB.ec_iCDF[ec_ix[i]:], 8)
		}
	}

	/* Encode NLSF interpolation factor */
	if psEncC.nb_subfr == MAX_NB_SUBFR {
		silk_assert(psIndices.NLSFInterpCoef_Q2 >= 0 && psIndices.NLSFInterpCoef_Q2 < 5)
		psRangeEnc.EncIcdf(int(psIndices.NLSFInterpCoef_Q2), NLSF_interpolation_factor_iCDF[:], 8)
	}

	if psIndices.signalType == TYPE_VOICED {
		/*********************/
		/* Encode pitch lags */
		/*********************/
		/* lag index */
		encode_absolute_lagIndex = 1
		if condCoding == CODE_CONDITIONALLY && psEncC.ec_prevSignalType == TYPE_VOICED {
			/* Delta Encoding */
			delta_lagIndex = int(psIndices.lagIndex) - int(psEncC.ec_prevLagIndex)
			if delta_lagIndex < -8 || delta_lagIndex > 11 {
				delta_lagIndex = 0
			} else {
				delta_lagIndex = delta_lagIndex + 9
				encode_absolute_lagIndex = 0 /* Only use delta */
			}
			silk_assert(delta_lagIndex >= 0 && delta_lagIndex < 21)
			psRangeEnc.EncIcdf(delta_lagIndex, pitch_delta_iCDF[:], 8)
		}
		if encode_absolute_lagIndex != 0 {
			/* Absolute encoding */
			var pitch_high_bits, pitch_low_bits int32
			pitch_high_bits = DIV32_16(psIndices.lagIndex, RSHIFT(psEncC.fs_kHz, 1))
			pitch_low_bits = int32(psIndices.lagIndex) - SMULBB(pitch_high_bits, RSHIFT(psEncC.fs_kHz, 1))
			silk_assert(int(pitch_low_bits) < psEncC.fs_kHz/2)
			silk_assert(pitch_high_bits < 32)
			psRangeEnc.EncIcdf(int(pitch_high_bits), pitch_lag_iCDF[:], 8)
			psRangeEnc.EncIcdf(int(pitch_low_bits), psEncC.pitch_lag_low_bits_iCDF, 8)
		}
		psEncC.ec_prevLagIndex = psIndices.lagIndex

		/* Contour index */
		silk_assert(psIndices.contourIndex >= 0)
		silk_assert((psIndices.contourIndex < 34 && psEncC.fs_kHz > 8 && psEncC.nb_subfr == 4) ||
			(psIndices.contourIndex < 11 && psEncC.fs_kHz == 8 && psEncC.nb_subfr == 4) ||
			(psIndices.contourIndex < 12 && psEncC.fs_kHz > 8 && psEncC.nb_subfr == 2) ||
			(psIndices.contourIndex < 3 && psEncC.fs_kHz == 8 && psEncC.nb_subfr == 2))
		psRangeEnc.EncIcdf(int(psIndices.contourIndex), psEncC.pitch_contour_iCDF, 8)

		/********************/
		/* Encode LTP gains */
		/********************/
		/* PERIndex value */
		silk_assert(psIndices.PERIndex >= 0 && psIndices.PERIndex < 3)
		psRangeEnc.EncIcdf(int(psIndices.PERIndex), LTP_per_index_iCDF[:], 8)

		/* Codebook Indices */
		for k = 0; k < psEncC.nb_subfr; k++ {
			silk_assert(psIndices.LTPIndex[k] >= 0 && int(psIndices.LTPIndex[k]) < (8<<psIndices.PERIndex))
			psRangeEnc.EncIcdf(int(psIndices.LTPIndex[k]), LTP_gain_iCDF_ptrs[psIndices.PERIndex], 8)
		}

		/**********************/
		/* Encode LTP scaling */
		/**********************/
		if condCoding == CODE_INDEPENDENTLY {
			silk_assert(psIndices.LTP_scaleIndex >= 0 && psIndices.LTP_scaleIndex < 3)
			psRangeEnc.EncIcdf(int(psIndices.LTP_scaleIndex), LTPscale_iCDF[:], 8)
		}
		silk_assert(condCoding == 0 || psIndices.LTP_scaleIndex == 0)
	}

	psEncC.ec_prevSignalType = int(psIndices.signalType)

	/***************/
	/* Encode seed */
	/***************/
	silk_assert(psIndices.Seed >= 0 && psIndices.Seed < 4)
	psRangeEnc.EncIcdf(int(psIndices.Seed), uniform4_iCDF[:], 8)
}
