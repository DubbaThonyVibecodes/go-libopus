package silk

// C: silk/process_NLSFs.c

// Limit, stabilize, convert and quantize NLSFs
//
// C: void silk_process_NLSFs(silk_encoder_state *psEncC, opus_int16 PredCoef_Q12[ 2 ][ MAX_LPC_ORDER ], opus_int16 pNLSF_Q15[ MAX_LPC_ORDER ], const opus_int16 prev_NLSFq_Q15[ MAX_LPC_ORDER ])
func (psEncC *EncoderState) ProcessNLSFs(
	PredCoef_Q12 *[2][MAX_LPC_ORDER]int16, // O    Prediction coefficients
	pNLSF_Q15 []int16, // I/O  Normalized LSFs (quant out) (0 - (2^15-1))
	prev_NLSFq_Q15 []int16, // I    Previous Normalized LSFs (0 - (2^15-1))
) {
	var i, doInterpolate int
	var NLSF_mu_Q20 int
	var i_sqr_Q15 int32
	var pNLSF0_temp_Q15 [MAX_LPC_ORDER]int16
	var pNLSFW_QW [MAX_LPC_ORDER]int16
	var pNLSFW0_temp_QW [MAX_LPC_ORDER]int16

	silk_assert(psEncC.speech_activity_Q8 >= 0)
	silk_assert(psEncC.speech_activity_Q8 <= int(SILK_FIX_CONST(1.0, 8)))
	silk_assert(psEncC.useInterpolatedNLSFs == 1 || psEncC.indices.NLSFInterpCoef_Q2 == (1<<2))

	// ***********************
	// Calculate mu values
	// ***********************
	// NLSF_mu  = 0.003 - 0.0015 * psEnc->speech_activity;
	NLSF_mu_Q20 = int(SMLAWB(SILK_FIX_CONST(0.003, 20), SILK_FIX_CONST(-0.001, 28), psEncC.speech_activity_Q8))
	if psEncC.nb_subfr == 2 {
		// Multiply by 1.5 for 10 ms packets
		NLSF_mu_Q20 = int(ADD_RSHIFT(NLSF_mu_Q20, NLSF_mu_Q20, 1))
	}

	silk_assert(NLSF_mu_Q20 > 0)
	silk_assert(NLSF_mu_Q20 <= int(SILK_FIX_CONST(0.005, 20)))

	// Calculate NLSF weights
	NLSFVQWeightsLaroia(pNLSFW_QW[:], pNLSF_Q15, psEncC.predictLPCOrder)

	// Update NLSF weights for interpolated NLSFs
	doInterpolate = b2i((psEncC.useInterpolatedNLSFs == 1) && (psEncC.indices.NLSFInterpCoef_Q2 < 4))
	if doInterpolate != 0 {
		// Calculate the interpolated NLSF vector for the first half
		Interpolate(pNLSF0_temp_Q15[:], prev_NLSFq_Q15, pNLSF_Q15,
			int(psEncC.indices.NLSFInterpCoef_Q2), psEncC.predictLPCOrder)

		// Calculate first half NLSF weights for the interpolated NLSFs
		NLSFVQWeightsLaroia(pNLSFW0_temp_QW[:], pNLSF0_temp_Q15[:], psEncC.predictLPCOrder)

		// Update NLSF weights with contribution from first half
		i_sqr_Q15 = LSHIFT(SMULBB(psEncC.indices.NLSFInterpCoef_Q2, psEncC.indices.NLSFInterpCoef_Q2), 11)
		for i = 0; i < psEncC.predictLPCOrder; i++ {
			pNLSFW_QW[i] = int16(SMLAWB(RSHIFT(pNLSFW_QW[i], 1), int32(pNLSFW0_temp_QW[i]), i_sqr_Q15))
			silk_assert(pNLSFW_QW[i] >= 1)
		}
	}

	NLSFEncode(psEncC.indices.NLSFIndices[:], pNLSF_Q15, psEncC.psNLSF_CB, pNLSFW_QW[:],
		NLSF_mu_Q20, psEncC.NLSF_MSVQ_Survivors, int(psEncC.indices.signalType))

	// Convert quantized NLSFs back to LPC coefficients
	NLSF2A(PredCoef_Q12[1][:], pNLSF_Q15, psEncC.predictLPCOrder)

	if doInterpolate != 0 {
		// Calculate the interpolated, quantized LSF vector for the first half
		Interpolate(pNLSF0_temp_Q15[:], prev_NLSFq_Q15, pNLSF_Q15,
			int(psEncC.indices.NLSFInterpCoef_Q2), psEncC.predictLPCOrder)

		// Convert back to LPC coefficients
		NLSF2A(PredCoef_Q12[0][:], pNLSF0_temp_Q15[:], psEncC.predictLPCOrder)

	} else {
		// Copy LPC coefficients for first half from second half
		copy(PredCoef_Q12[0][:psEncC.predictLPCOrder], PredCoef_Q12[1][:psEncC.predictLPCOrder])
	}
}
