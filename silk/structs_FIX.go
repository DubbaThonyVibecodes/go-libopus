package silk

// C: silk/fixed/structs_FIX.h

// ShapeStateFIX is the C silk_shape_state_FIX: noise shaping analysis state.
type ShapeStateFIX struct {
	LastGainIndex          int8
	HarmBoost_smth_Q16     int32
	HarmShapeGain_smth_Q16 int32
	Tilt_smth_Q16          int32
}

// EncoderStateFIX is the C silk_encoder_state_FIX.
type EncoderStateFIX struct {
	sCmn   EncoderState  // Common struct, shared with floating-point code
	sShape ShapeStateFIX // Shape state

	// Buffer for find pitch and noise shape analysis
	x_buf       [2*MAX_FRAME_LENGTH + LA_SHAPE_MAX]int16 // Buffer for find pitch and noise shape analysis
	LTPCorr_Q15 int                                      // Normalized correlation from pitch lag estimator
	resNrgSmth  int32
}

// EncoderControlFIX is the C silk_encoder_control_FIX.
type EncoderControlFIX struct {
	// Prediction and coding parameters
	Gains_Q16     [MAX_NB_SUBFR]int32
	PredCoef_Q12  [2][MAX_LPC_ORDER]int16
	LTPCoef_Q14   [LTP_ORDER * MAX_NB_SUBFR]int16
	LTP_scale_Q14 int
	pitchL        [MAX_NB_SUBFR]int

	// Noise shaping parameters
	// Testing
	AR_Q13             [MAX_NB_SUBFR * MAX_SHAPE_LPC_ORDER]int16
	LF_shp_Q14         [MAX_NB_SUBFR]int32 // Packs two int16 coefficients per int32 value
	Tilt_Q14           [MAX_NB_SUBFR]int
	HarmShapeGain_Q14  [MAX_NB_SUBFR]int
	Lambda_Q10         int
	input_quality_Q14  int
	coding_quality_Q14 int

	// measures
	predGain_Q16     int32
	LTPredCodGain_Q7 int
	ResNrg           [MAX_NB_SUBFR]int32 // Residual energy per subframe
	ResNrgQ          [MAX_NB_SUBFR]int   // Q domain for the residual energy > 0

	// Parameters for CBR mode
	GainsUnq_Q16      [MAX_NB_SUBFR]int32
	lastGainIndexPrev int8
}

// Encoder is the C silk_encoder: encoder super struct.
type Encoder struct {
	state_Fxx                 [ENCODER_NUM_CHANNELS]EncoderStateFIX
	sStereo                   StereoEncState
	nBitsUsedLBRR             int32
	nBitsExceeded             int32
	nChannelsAPI              int
	nChannelsInternal         int
	nPrevChannelsInternal     int
	timeSinceSwitchAllowed_ms int
	allowBandwidthSwitch      int
	prev_decode_only_middle   int
}

// VariableHPSmth1Q15 returns state_Fxx[0].sCmn.variable_HP_smth1_Q15 (read
// directly by the Opus encoder in C).
func (e *Encoder) VariableHPSmth1Q15() int32 { return e.state_Fxx[0].sCmn.variable_HP_smth1_Q15 }

// NoSpeechCounter returns state_Fxx[n].sCmn.noSpeechCounter (read by
// opus_encoder_ctl(OPUS_GET_IN_DTX) in src/opus_encoder.c).
func (e *Encoder) NoSpeechCounter(n int) int { return e.state_Fxx[n].sCmn.noSpeechCounter }

// PrevDecodeOnlyMiddle returns prev_decode_only_middle (read by
// opus_encoder_ctl(OPUS_GET_IN_DTX) in src/opus_encoder.c since 1.4).
func (e *Encoder) PrevDecodeOnlyMiddle() int { return e.prev_decode_only_middle }
