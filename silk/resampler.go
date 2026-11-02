package silk

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// C: silk/resampler.c

/*
 * Matrix of resampling methods used:
 *                                 Fs_out (kHz)
 *                        8      12     16     24     48
 *
 *               8        C      UF     U      UF     UF
 *              12        AF     C      UF     U      UF
 * Fs_in (kHz)  16        D      AF     C      UF     UF
 *              24        AF     D      AF     C      U
 *              48        AF     AF     AF     D      C
 *
 * C   -> Copy (no resampling)
 * D   -> Allpass-based 2x downsampling
 * U   -> Allpass-based 2x upsampling
 * UF  -> Allpass-based 2x upsampling followed by FIR interpolation
 * AF  -> AR2 filter followed by FIR interpolation
 */

// Tables with delay compensation values to equalize total delay for different modes
var delay_matrix_enc = [6][3]int8{
	/* in  \ out  8  12  16 */
	/*  8 */ {6, 0, 3},
	/* 12 */ {0, 7, 3},
	/* 16 */ {0, 1, 10},
	/* 24 */ {0, 2, 6},
	/* 48 */ {18, 10, 12},
	/* 96 */ {0, 0, 44},
}

var delay_matrix_dec = [3][6]int8{
	/* in  \ out  8  12  16  24  48  96*/
	/*  8 */ {4, 0, 2, 0, 0, 0},
	/* 12 */ {0, 9, 4, 7, 4, 4},
	/* 16 */ {0, 3, 12, 7, 7, 7},
}

// Simple way to make [8000, 12000, 16000, 24000, 48000] to [0, 1, 2, 3, 4]
//
// C: #define rateID(R) IMIN(5, ( ( ( ((R)>>12) - ((R)>16000) ) >> ((R)>24000) ) - 1 ))
func rateID(R int32) int32 {
	return celt.IMIN(5, (((R>>12)-int32(b2i(R > 16000)))>>b2i(R > 24000))-1)
}

// C: #define USE_silk_resampler_* — selects the private resampler function
const (
	USE_silk_resampler_copy                   = 0
	USE_silk_resampler_private_up2_HQ_wrapper = 1
	USE_silk_resampler_private_IIR_FIR        = 2
	USE_silk_resampler_private_down_FIR       = 3
)

// Initialize/reset the resampler state for a given pair of input/output sampling rates
//
// C: opus_int silk_resampler_init(
//
//	silk_resampler_state_struct *S,                 /* I/O  Resampler state                                             */
//	opus_int32                  Fs_Hz_in,           /* I    Input sampling rate (Hz)                                    */
//	opus_int32                  Fs_Hz_out,          /* I    Output sampling rate (Hz)                                   */
//	opus_int                    forEnc              /* I    If 1: encoder; if 0: decoder                                */
//
// )
func (S *ResamplerStateStruct) ResamplerInit(Fs_Hz_in int32, Fs_Hz_out int32, forEnc int) int {
	var up2x int

	/* Clear state */
	*S = ResamplerStateStruct{}

	/* Input checking */
	if forEnc != 0 {
		if (Fs_Hz_in != 8000 && Fs_Hz_in != 12000 && Fs_Hz_in != 16000 && Fs_Hz_in != 24000 && Fs_Hz_in != 48000) ||
			(Fs_Hz_out != 8000 && Fs_Hz_out != 12000 && Fs_Hz_out != 16000) {
			silk_assert(false)
			return -1
		}
		S.inputDelay = int(delay_matrix_enc[rateID(Fs_Hz_in)][rateID(Fs_Hz_out)])
	} else {
		if (Fs_Hz_in != 8000 && Fs_Hz_in != 12000 && Fs_Hz_in != 16000) ||
			(Fs_Hz_out != 8000 && Fs_Hz_out != 12000 && Fs_Hz_out != 16000 && Fs_Hz_out != 24000 && Fs_Hz_out != 48000) {
			silk_assert(false)
			return -1
		}
		S.inputDelay = int(delay_matrix_dec[rateID(Fs_Hz_in)][rateID(Fs_Hz_out)])
	}

	S.Fs_in_kHz = int(DIV32_16(Fs_Hz_in, 1000))
	S.Fs_out_kHz = int(DIV32_16(Fs_Hz_out, 1000))

	/* Number of samples processed per batch */
	S.batchSize = S.Fs_in_kHz * RESAMPLER_MAX_BATCH_SIZE_MS

	/* Find resampler with the right sampling ratio */
	up2x = 0
	if Fs_Hz_out > Fs_Hz_in {
		/* Upsample */
		if Fs_Hz_out == MUL(Fs_Hz_in, 2) { /* Fs_out : Fs_in = 2 : 1 */
			/* Special case: directly use 2x upsampler */
			S.resampler_function = USE_silk_resampler_private_up2_HQ_wrapper
		} else {
			/* Default resampler */
			S.resampler_function = USE_silk_resampler_private_IIR_FIR
			up2x = 1
		}
	} else if Fs_Hz_out < Fs_Hz_in {
		/* Downsample */
		S.resampler_function = USE_silk_resampler_private_down_FIR
		if MUL(Fs_Hz_out, 4) == MUL(Fs_Hz_in, 3) { /* Fs_out : Fs_in = 3 : 4 */
			S.FIR_Fracs = 3
			S.FIR_Order = RESAMPLER_DOWN_ORDER_FIR0
			S.Coefs = Resampler_3_4_COEFS[:]
		} else if MUL(Fs_Hz_out, 3) == MUL(Fs_Hz_in, 2) { /* Fs_out : Fs_in = 2 : 3 */
			S.FIR_Fracs = 2
			S.FIR_Order = RESAMPLER_DOWN_ORDER_FIR0
			S.Coefs = Resampler_2_3_COEFS[:]
		} else if MUL(Fs_Hz_out, 2) == Fs_Hz_in { /* Fs_out : Fs_in = 1 : 2 */
			S.FIR_Fracs = 1
			S.FIR_Order = RESAMPLER_DOWN_ORDER_FIR1
			S.Coefs = Resampler_1_2_COEFS[:]
		} else if MUL(Fs_Hz_out, 3) == Fs_Hz_in { /* Fs_out : Fs_in = 1 : 3 */
			S.FIR_Fracs = 1
			S.FIR_Order = RESAMPLER_DOWN_ORDER_FIR2
			S.Coefs = Resampler_1_3_COEFS[:]
		} else if MUL(Fs_Hz_out, 4) == Fs_Hz_in { /* Fs_out : Fs_in = 1 : 4 */
			S.FIR_Fracs = 1
			S.FIR_Order = RESAMPLER_DOWN_ORDER_FIR2
			S.Coefs = Resampler_1_4_COEFS[:]
		} else if MUL(Fs_Hz_out, 6) == Fs_Hz_in { /* Fs_out : Fs_in = 1 : 6 */
			S.FIR_Fracs = 1
			S.FIR_Order = RESAMPLER_DOWN_ORDER_FIR2
			S.Coefs = Resampler_1_6_COEFS[:]
		} else {
			/* None available */
			silk_assert(false)
			return -1
		}
	} else {
		/* Input and output sampling rates are equal: copy */
		S.resampler_function = USE_silk_resampler_copy
	}

	/* Ratio of input/output samples */
	S.invRatio_Q16 = LSHIFT32(DIV32(LSHIFT32(Fs_Hz_in, 14+up2x), Fs_Hz_out), 2)
	/* Make sure the ratio is rounded up */
	for SMULWW(S.invRatio_Q16, Fs_Hz_out) < LSHIFT32(Fs_Hz_in, up2x) {
		S.invRatio_Q16++
	}

	return 0
}

// Resampler: convert from one sampling rate to another
// Input and output sampling rate are at most 48000 Hz
//
// C: opus_int silk_resampler(
//
//	silk_resampler_state_struct *S,                 /* I/O  Resampler state                                             */
//	opus_int16                  out[],              /* O    Output signal                                               */
//	const opus_int16            in[],               /* I    Input signal                                                */
//	opus_int32                  inLen               /* I    Number of input samples                                     */
//
// )
func (S *ResamplerStateStruct) Resampler(out []int16, in []int16, inLen int32) int {
	var nSamples int

	/* Need at least 1 ms of input data */
	silk_assert(inLen >= int32(S.Fs_in_kHz))
	/* Delay can't exceed the 1 ms of buffering */
	silk_assert(S.inputDelay <= S.Fs_in_kHz)

	nSamples = S.Fs_in_kHz - S.inputDelay

	/* Copy to delay buffer */
	copy(S.delayBuf[S.inputDelay:S.inputDelay+nSamples], in[:nSamples])

	switch S.resampler_function {
	case USE_silk_resampler_private_up2_HQ_wrapper:
		S.ResamplerPrivateUp2HQWrapper(out, S.delayBuf[:], int32(S.Fs_in_kHz))
		S.ResamplerPrivateUp2HQWrapper(out[S.Fs_out_kHz:], in[nSamples:], inLen-int32(S.Fs_in_kHz))
	case USE_silk_resampler_private_IIR_FIR:
		S.ResamplerPrivateIIRFIR(out, S.delayBuf[:], int32(S.Fs_in_kHz))
		S.ResamplerPrivateIIRFIR(out[S.Fs_out_kHz:], in[nSamples:], inLen-int32(S.Fs_in_kHz))
	case USE_silk_resampler_private_down_FIR:
		S.ResamplerPrivateDownFIR(out, S.delayBuf[:], int32(S.Fs_in_kHz))
		S.ResamplerPrivateDownFIR(out[S.Fs_out_kHz:], in[nSamples:], inLen-int32(S.Fs_in_kHz))
	default:
		copy(out[:S.Fs_in_kHz], S.delayBuf[:S.Fs_in_kHz])
		copy(out[S.Fs_out_kHz:S.Fs_out_kHz+int(inLen)-S.Fs_in_kHz], in[nSamples:nSamples+int(inLen)-S.Fs_in_kHz])
	}

	/* Copy to delay buffer */
	copy(S.delayBuf[:S.inputDelay], in[int(inLen)-S.inputDelay:int(inLen)])

	return 0
}
