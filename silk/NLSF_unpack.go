package silk

// C: silk/NLSF_unpack.c

// Unpack predictor values and indices for entropy coding tables
//
// C: void silk_NLSF_unpack(opus_int16 ec_ix[], opus_uint8 pred_Q8[], const silk_NLSF_CB_struct *psNLSF_CB, const opus_int CB1_index)
func NLSFUnpack(
	ec_ix []int16, // O    Indices to entropy tables [ LPC_ORDER ]
	pred_Q8 []uint8, // O    LSF predictor [ LPC_ORDER ]
	psNLSF_CB *NLSFCBStruct, // I    Codebook object
	CB1_index int, // I    Index of vector in first LSF codebook
) {
	var i int
	var entry uint8
	var ec_sel_ptr []uint8

	ec_sel_ptr = psNLSF_CB.ec_sel[CB1_index*int(psNLSF_CB.order)/2:]
	for i = 0; i < int(psNLSF_CB.order); i += 2 {
		entry = ec_sel_ptr[0]
		ec_sel_ptr = ec_sel_ptr[1:]
		ec_ix[i] = int16(SMULBB(RSHIFT(entry, 1)&7, 2*NLSF_QUANT_MAX_AMPLITUDE+1))
		pred_Q8[i] = psNLSF_CB.pred_Q8[i+int(entry&1)*(int(psNLSF_CB.order)-1)]
		ec_ix[i+1] = int16(SMULBB(RSHIFT(entry, 5)&7, 2*NLSF_QUANT_MAX_AMPLITUDE+1))
		pred_Q8[i+1] = psNLSF_CB.pred_Q8[i+int(RSHIFT(entry, 4)&1)*(int(psNLSF_CB.order)-1)+1]
	}
}
