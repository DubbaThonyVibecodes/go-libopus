package silk

// C: silk/NLSF_encode.c

// ***********************
// NLSF vector encoder
// ***********************
//
// C: opus_int32 silk_NLSF_encode(opus_int8 *NLSFIndices, opus_int16 *pNLSF_Q15, const silk_NLSF_CB_struct *psNLSF_CB, const opus_int16 *pW_QW, const opus_int NLSF_mu_Q20, const opus_int nSurvivors, const opus_int signalType)
func NLSFEncode( // O    Returns RD value in Q25
	NLSFIndices []int8, // I    Codebook path vector [ LPC_ORDER + 1 ]
	pNLSF_Q15 []int16, // I/O  Quantized NLSF vector [ LPC_ORDER ]
	psNLSF_CB *NLSFCBStruct, // I    Codebook object
	pW_QW []int16, // I    NLSF weight vector [ LPC_ORDER ]
	NLSF_mu_Q20 int, // I    Rate weight for the RD optimization
	nSurvivors int, // I    Max survivors after first stage
	signalType int, // I    Signal type: 0/1/2
) int32 {
	var i, s, ind1, prob_Q8, bits_q7 int
	var bestIndex [1]int // C: opus_int bestIndex, passed as the idx[] vector of length 1
	var W_tmp_Q9 int32
	var err_Q26 [NLSF_VQ_MAX_VECTORS]int32
	var RD_Q25 [NLSF_VQ_MAX_SURVIVORS]int32
	var tempIndices1 [NLSF_VQ_MAX_SURVIVORS]int
	var tempIndices2 [NLSF_VQ_MAX_SURVIVORS * MAX_LPC_ORDER]int8
	var res_Q15 [MAX_LPC_ORDER]int16
	var res_Q10 [MAX_LPC_ORDER]int16
	var NLSF_tmp_Q15 [MAX_LPC_ORDER]int16
	var W_tmp_QW [MAX_LPC_ORDER]int16
	var W_adj_Q5 [MAX_LPC_ORDER]int16
	var pred_Q8 [MAX_LPC_ORDER]uint8
	var ec_ix [MAX_LPC_ORDER]int16
	var pCB_element, iCDF_ptr []uint8

	silk_assert(nSurvivors <= NLSF_VQ_MAX_SURVIVORS)
	silk_assert(signalType >= 0 && signalType <= 2)
	silk_assert(NLSF_mu_Q20 <= 32767 && NLSF_mu_Q20 >= 0)

	// NLSF stabilization
	NLSFStabilize(pNLSF_Q15, psNLSF_CB.deltaMin_Q15, int(psNLSF_CB.order))

	// First stage: VQ
	NLSFVQ(err_Q26[:], pNLSF_Q15, psNLSF_CB.CB1_NLSF_Q8, int(psNLSF_CB.nVectors), int(psNLSF_CB.order))

	// Sort the quantization errors
	InsertionSortIncreasing(err_Q26[:], tempIndices1[:], int(psNLSF_CB.nVectors), nSurvivors)

	// Loop over survivors
	for s = 0; s < nSurvivors; s++ {
		ind1 = tempIndices1[s]

		// Residual after first stage
		pCB_element = psNLSF_CB.CB1_NLSF_Q8[ind1*int(psNLSF_CB.order):]
		for i = 0; i < int(psNLSF_CB.order); i++ {
			NLSF_tmp_Q15[i] = LSHIFT16(int16(pCB_element[i]), 7)
			res_Q15[i] = pNLSF_Q15[i] - NLSF_tmp_Q15[i]
		}

		// Weights from codebook vector
		NLSFVQWeightsLaroia(W_tmp_QW[:], NLSF_tmp_Q15[:], int(psNLSF_CB.order))

		// Apply square-rooted weights
		for i = 0; i < int(psNLSF_CB.order); i++ {
			W_tmp_Q9 = SQRT_APPROX(LSHIFT(int32(W_tmp_QW[i]), 18-NLSF_W_Q))
			res_Q10[i] = int16(RSHIFT(SMULBB(res_Q15[i], W_tmp_Q9), 14))
		}

		// Modify input weights accordingly
		for i = 0; i < int(psNLSF_CB.order); i++ {
			W_adj_Q5[i] = int16(DIV32_16(LSHIFT(int32(pW_QW[i]), 5), W_tmp_QW[i]))
		}

		// Unpack entropy table indices and predictor for current CB1 index
		NLSFUnpack(ec_ix[:], pred_Q8[:], psNLSF_CB, ind1)

		// Trellis quantizer
		RD_Q25[s] = NLSFDelDecQuant(tempIndices2[s*MAX_LPC_ORDER:], res_Q10[:], W_adj_Q5[:], pred_Q8[:], ec_ix[:],
			psNLSF_CB.ec_Rates_Q5, int(psNLSF_CB.quantStepSize_Q16), psNLSF_CB.invQuantStepSize_Q6, int32(NLSF_mu_Q20), psNLSF_CB.order)

		// Add rate for first stage
		iCDF_ptr = psNLSF_CB.CB1_iCDF[(signalType>>1)*int(psNLSF_CB.nVectors):]
		if ind1 == 0 {
			prob_Q8 = 256 - int(iCDF_ptr[ind1])
		} else {
			prob_Q8 = int(iCDF_ptr[ind1-1]) - int(iCDF_ptr[ind1])
		}
		bits_q7 = (8 << 7) - int(Lin2log(int32(prob_Q8)))
		RD_Q25[s] = SMLABB(RD_Q25[s], bits_q7, RSHIFT(NLSF_mu_Q20, 2))
	}

	// Find the lowest rate-distortion error
	InsertionSortIncreasing(RD_Q25[:], bestIndex[:], nSurvivors, 1)

	NLSFIndices[0] = int8(tempIndices1[bestIndex[0]])
	copy(NLSFIndices[1:1+int(psNLSF_CB.order)], tempIndices2[bestIndex[0]*MAX_LPC_ORDER:][:int(psNLSF_CB.order)])

	// Decode
	NLSFDecode(pNLSF_Q15, NLSFIndices, psNLSF_CB)

	return RD_Q25[0]
}
