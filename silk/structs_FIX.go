package silk

// C: silk/fixed/structs_FIX.h

// ShapeStateFIX is the C silk_shape_state_FIX: noise shaping analysis state.
type ShapeStateFIX struct {
	LastGainIndex          int8
	HarmBoost_smth_Q16     int32
	HarmShapeGain_smth_Q16 int32
	Tilt_smth_Q16          int32
}

// PrefilterStateFIX is the C silk_prefilter_state_FIX.
type PrefilterStateFIX struct {
	sLTP_shp         [LTP_BUF_LENGTH]int16
	sAR_shp          [MAX_SHAPE_LPC_ORDER + 1]int32
	sLTP_shp_buf_idx int
	sLF_AR_shp_Q12   int32
	sLF_MA_shp_Q12   int32
	sHarmHP_Q2       int32
	rand_seed        int32
	lagPrev          int
}

// EncoderStateFIX is the C silk_encoder_state_FIX.
type EncoderStateFIX struct {
	sCmn     EncoderState      // Common struct, shared with floating-point code
	sShape   ShapeStateFIX     // Shape state
	sPrefilt PrefilterStateFIX // Prefilter State

	// Buffer for find pitch and noise shape analysis
	x_buf       [2*MAX_FRAME_LENGTH + LA_SHAPE_MAX]int16 // Buffer for find pitch and noise shape analysis
	LTPCorr_Q15 int                                      // Normalized correlation from pitch lag estimator
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
	AR1_Q13            [MAX_NB_SUBFR * MAX_SHAPE_LPC_ORDER]int16
	AR2_Q13            [MAX_NB_SUBFR * MAX_SHAPE_LPC_ORDER]int16
	LF_shp_Q14         [MAX_NB_SUBFR]int32 // Packs two int16 coefficients per int32 value
	GainsPre_Q14       [MAX_NB_SUBFR]int
	HarmBoost_Q14      [MAX_NB_SUBFR]int
	Tilt_Q14           [MAX_NB_SUBFR]int
	HarmShapeGain_Q14  [MAX_NB_SUBFR]int
	Lambda_Q10         int
	input_quality_Q14  int
	coding_quality_Q14 int

	// measures
	sparseness_Q8    int
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
