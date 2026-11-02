package silk

// C: silk/NLSF_decode.c

// Predictive dequantizer for NLSF residuals
//
// C: static inline void silk_NLSF_residual_dequant(opus_int16 x_Q10[], const opus_int8 indices[], const opus_uint8 pred_coef_Q8[], const opus_int quant_step_size_Q16, const opus_int16 order)
func nlsfResidualDequant( // O    Returns RD value in Q30
	x_Q10 []int16, // O    Output [ order ]
	indices []int8, // I    Quantization indices [ order ]
	pred_coef_Q8 []uint8, // I    Backward predictor coefs [ order ]
	quant_step_size_Q16 int, // I    Quantization step size
	order int16, // I    Number of input values
) {
	var i, out_Q10, pred_Q10 int

	out_Q10 = 0
	for i = int(order) - 1; i >= 0; i-- {
		pred_Q10 = int(RSHIFT(SMULBB(out_Q10, int16(pred_coef_Q8[i])), 8))
		out_Q10 = int(LSHIFT(indices[i], 10))
		if out_Q10 > 0 {
			out_Q10 = int(SUB16(out_Q10, SILK_FIX_CONST(NLSF_QUANT_LEVEL_ADJ, 10)))
		} else if out_Q10 < 0 {
			out_Q10 = int(ADD16(out_Q10, SILK_FIX_CONST(NLSF_QUANT_LEVEL_ADJ, 10)))
		}
		out_Q10 = int(SMLAWB(pred_Q10, int32(out_Q10), quant_step_size_Q16))
		x_Q10[i] = int16(out_Q10)
	}
}

// ***********************
// NLSF vector decoder
// ***********************
//
// C: void silk_NLSF_decode(opus_int16 *pNLSF_Q15, opus_int8 *NLSFIndices, const silk_NLSF_CB_struct *psNLSF_CB)
func NLSFDecode(
	pNLSF_Q15 []int16, // O    Quantized NLSF vector [ LPC_ORDER ]
	NLSFIndices []int8, // I    Codebook path vector [ LPC_ORDER + 1 ]
	psNLSF_CB *NLSFCBStruct, // I    Codebook object
) {
	var i int
	var pred_Q8 [MAX_LPC_ORDER]uint8
	var ec_ix [MAX_LPC_ORDER]int16
	var res_Q10 [MAX_LPC_ORDER]int16
	var W_tmp_QW [MAX_LPC_ORDER]int16
	var W_tmp_Q9, NLSF_Q15_tmp int32
	var pCB_element []uint8

	// Decode first stage
	pCB_element = psNLSF_CB.CB1_NLSF_Q8[int(NLSFIndices[0])*int(psNLSF_CB.order):]
	for i = 0; i < int(psNLSF_CB.order); i++ {
		pNLSF_Q15[i] = int16(LSHIFT(int16(pCB_element[i]), 7))
	}

	// Unpack entropy table indices and predictor for current CB1 index
	NLSFUnpack(ec_ix[:], pred_Q8[:], psNLSF_CB, int(NLSFIndices[0]))

	// Predictive residual dequantizer
	nlsfResidualDequant(res_Q10[:], NLSFIndices[1:], pred_Q8[:], int(psNLSF_CB.quantStepSize_Q16), psNLSF_CB.order)

	// Weights from codebook vector
	NLSFVQWeightsLaroia(W_tmp_QW[:], pNLSF_Q15, int(psNLSF_CB.order))

	// Apply inverse square-rooted weights and add to output
	for i = 0; i < int(psNLSF_CB.order); i++ {
		W_tmp_Q9 = SQRT_APPROX(LSHIFT(int32(W_tmp_QW[i]), 18-NLSF_W_Q))
		NLSF_Q15_tmp = ADD32(pNLSF_Q15[i], DIV32_16(LSHIFT(int32(res_Q10[i]), 14), W_tmp_Q9))
		pNLSF_Q15[i] = int16(LIMIT(NLSF_Q15_tmp, 0, 32767))
	}

	// NLSF stabilization
	NLSFStabilize(pNLSF_Q15, psNLSF_CB.deltaMin_Q15, int(psNLSF_CB.order))
}
