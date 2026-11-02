package silk

// C: silk/structs.h

// NsqState is the C silk_nsq_state: noise shaping quantization state.
type NsqState struct {
	xq               [2 * MAX_FRAME_LENGTH]int16 // Buffer for quantized output signal
	sLTP_shp_Q14     [2 * MAX_FRAME_LENGTH]int32
	sLPC_Q14         [MAX_SUB_FRAME_LENGTH + NSQ_LPC_BUF_LENGTH]int32
	sAR2_Q14         [MAX_SHAPE_LPC_ORDER]int32
	sLF_AR_shp_Q14   int32
	sDiff_shp_Q14    int32
	lagPrev          int
	sLTP_buf_idx     int
	sLTP_shp_buf_idx int
	rand_seed        int32
	prev_gain_Q16    int32
	rewhite_flag     int
}

// VADState is the C silk_VAD_state.
type VADState struct {
	AnaState        [2]int32           // Analysis filterbank state: 0-8 kHz
	AnaState1       [2]int32           // Analysis filterbank state: 0-4 kHz
	AnaState2       [2]int32           // Analysis filterbank state: 0-2 kHz
	XnrgSubfr       [VAD_N_BANDS]int32 // Subframe energies
	NrgRatioSmth_Q8 [VAD_N_BANDS]int32 // Smoothed energy level in each band
	HPstate         int16              // State of differentiator in the lowest band
	NL              [VAD_N_BANDS]int32 // Noise energy level in each band
	inv_NL          [VAD_N_BANDS]int32 // Inverse noise energy level in each band
	NoiseLevelBias  [VAD_N_BANDS]int32 // Noise level estimator bias/offset
	counter         int32              // Frame counter used in the initial phase
}

// LPState is the C silk_LP_state: variable cut-off low-pass filter state.
type LPState struct {
	In_LP_State         [2]int32 // Low pass filter state
	transition_frame_no int32    // Counter which is mapped to a cut-off frequency
	mode                int      // Operating mode, <0: switch down, >0: switch up; 0: do nothing
	saved_fs_kHz        int32    // If non-zero, holds the last sampling rate before a bandwidth switching reset.
}

// NLSFCBStruct is the C silk_NLSF_CB_struct: structure containing NLSF codebook.
type NLSFCBStruct struct {
	nVectors            int16
	order               int16
	quantStepSize_Q16   int16
	invQuantStepSize_Q6 int16
	CB1_NLSF_Q8         []uint8
	CB1_Wght_Q9         []int16
	CB1_iCDF            []uint8
	pred_Q8             []uint8
	ec_sel              []uint8
	ec_iCDF             []uint8
	ec_Rates_Q5         []uint8
	deltaMin_Q15        []int16
}

// StereoEncState is the C stereo_enc_state.
type StereoEncState struct {
	pred_prev_Q13   [2]int16
	sMid            [2]int16
	sSide           [2]int16
	mid_side_amp_Q0 [4]int32
	smth_width_Q14  int16
	width_prev_Q14  int16
	silent_side_len int16
	predIx          [MAX_FRAMES_PER_PACKET][2][3]int8
	mid_only_flags  [MAX_FRAMES_PER_PACKET]int8
}

// StereoDecState is the C stereo_dec_state.
type StereoDecState struct {
	pred_prev_Q13 [2]int16
	sMid          [2]int16
	sSide         [2]int16
}

// SideInfoIndices is the C SideInfoIndices.
type SideInfoIndices struct {
	GainsIndices      [MAX_NB_SUBFR]int8
	LTPIndex          [MAX_NB_SUBFR]int8
	NLSFIndices       [MAX_LPC_ORDER + 1]int8
	lagIndex          int16
	contourIndex      int8
	signalType        int8
	quantOffsetType   int8
	NLSFInterpCoef_Q2 int8
	PERIndex          int8
	LTP_scaleIndex    int8
	Seed              int8
}

// EncoderState is the C silk_encoder_state (common part, shared with the
// floating-point code upstream).
type EncoderState struct {
	In_HP_State                   [2]int32             // High pass filter state
	variable_HP_smth1_Q15         int32                // State of first smoother
	variable_HP_smth2_Q15         int32                // State of second smoother
	sLP                           LPState              // Low pass filter state
	sVAD                          VADState             // Voice activity detector state
	sNSQ                          NsqState             // Noise Shape Quantizer State
	prev_NLSFq_Q15                [MAX_LPC_ORDER]int16 // Previously quantized NLSF vector
	speech_activity_Q8            int                  // Speech activity
	allow_bandwidth_switch        int                  // Flag indicating that switching of internal bandwidth is allowed
	LBRRprevLastGainIndex         int8
	prevSignalType                int8
	prevLag                       int
	pitch_LPC_win_length          int
	max_pitch_lag                 int   // Highest possible pitch lag (samples)
	API_fs_Hz                     int32 // API sampling frequency (Hz)
	prev_API_fs_Hz                int32 // Previous API sampling frequency (Hz)
	maxInternal_fs_Hz             int   // Maximum internal sampling frequency (Hz)
	minInternal_fs_Hz             int   // Minimum internal sampling frequency (Hz)
	desiredInternal_fs_Hz         int   // Soft request for internal sampling frequency (Hz)
	fs_kHz                        int   // Internal sampling frequency (kHz)
	nb_subfr                      int   // Number of 5 ms subframes in a frame
	frame_length                  int   // Frame length (samples)
	subfr_length                  int   // Subframe length (samples)
	ltp_mem_length                int   // Length of LTP memory
	la_pitch                      int   // Look-ahead for pitch analysis (samples)
	la_shape                      int   // Look-ahead for noise shape analysis (samples)
	shapeWinLength                int   // Window length for noise shape analysis (samples)
	TargetRate_bps                int32 // Target bitrate (bps)
	PacketSize_ms                 int   // Number of milliseconds to put in each packet
	PacketLoss_perc               int   // Packet loss rate measured by farend
	frameCounter                  int32
	Complexity                    int           // Complexity setting
	nStatesDelayedDecision        int           // Number of states in delayed decision quantization
	useInterpolatedNLSFs          int           // Flag for using NLSF interpolation
	shapingLPCOrder               int           // Filter order for noise shaping filters
	predictLPCOrder               int           // Filter order for prediction filters
	pitchEstimationComplexity     int           // Complexity level for pitch estimator
	pitchEstimationLPCOrder       int           // Whitening filter order for pitch estimator
	pitchEstimationThreshold_Q16  int32         // Threshold for pitch estimator
	sum_log_gain_Q7               int32         // Cumulative max prediction gain
	NLSF_MSVQ_Survivors           int           // Number of survivors in NLSF MSVQ
	first_frame_after_reset       int           // Flag for deactivating NLSF interpolation, pitch prediction
	controlled_since_last_payload int           // Flag for ensuring codec_control only runs once per packet
	warping_Q16                   int           // Warping parameter for warped noise shaping
	useCBR                        int           // Flag to enable constant bitrate
	prefillFlag                   int           // Flag to indicate that only buffers are prefilled, no coding
	pitch_lag_low_bits_iCDF       []uint8       // Pointer to iCDF table for low bits of pitch lag index
	pitch_contour_iCDF            []uint8       // Pointer to iCDF table for pitch contour index
	psNLSF_CB                     *NLSFCBStruct // Pointer to NLSF codebook
	input_quality_bands_Q15       [VAD_N_BANDS]int
	input_tilt_Q15                int
	SNR_dB_Q7                     int // Quality setting

	VAD_flags  [MAX_FRAMES_PER_PACKET]int8
	LBRR_flag  int8
	LBRR_flags [MAX_FRAMES_PER_PACKET]int

	indices SideInfoIndices
	pulses  [MAX_FRAME_LENGTH]int8

	arch int

	// Input/output buffering
	inputBuf         [MAX_FRAME_LENGTH + 2]int16 // Buffer containing input signal
	inputBufIx       int
	nFramesPerPacket int
	nFramesEncoded   int // Number of frames analyzed in current packet

	nChannelsAPI      int
	nChannelsInternal int
	channelNb         int

	// Parameters For LTP scaling Control
	frames_since_onset int

	// Specifically for entropy coding
	ec_prevSignalType int
	ec_prevLagIndex   int16

	resampler_state ResamplerStateStruct

	// DTX
	useDTX          int // Flag to enable DTX
	inDTX           int // Flag to signal DTX period
	noSpeechCounter int // Counts concecutive nonactive frames, used by DTX

	// Inband Low Bitrate Redundancy (LBRR) data
	useInBandFEC       int // Saves the API setting for query
	LBRR_enabled       int // Depends on useInBandFRC, bitrate and packet loss rate
	LBRR_GainIncreases int // Gains increment for coding LBRR frames
	indices_LBRR       [MAX_FRAMES_PER_PACKET]SideInfoIndices
	pulses_LBRR        [MAX_FRAMES_PER_PACKET][MAX_FRAME_LENGTH]int8
}

// PLCStruct is the C silk_PLC_struct: struct for Packet Loss Concealment.
type PLCStruct struct {
	pitchL_Q8         int32            // Pitch lag to use for voiced concealment
	LTPCoef_Q14       [LTP_ORDER]int16 // LTP coefficients to use for voiced concealment
	prevLPC_Q12       [MAX_LPC_ORDER]int16
	last_frame_lost   int   // Was previous frame lost
	rand_seed         int32 // Seed for unvoiced signal generation
	randScale_Q14     int16 // Scaling of unvoiced random signal
	conc_energy       int32
	conc_energy_shift int
	prevLTP_scale_Q14 int16
	prevGain_Q16      [2]int32
	fs_kHz            int
	nb_subfr          int
	subfr_length      int
	enable_deep_plc   int
}

// CNGStruct is the C silk_CNG_struct: struct for CNG.
type CNGStruct struct {
	CNG_exc_buf_Q14   [MAX_FRAME_LENGTH]int32
	CNG_smth_NLSF_Q15 [MAX_LPC_ORDER]int16
	CNG_synth_state   [MAX_LPC_ORDER]int32
	CNG_smth_Gain_Q16 int32
	rand_seed         int32
	fs_kHz            int
}

// DecoderState is the C silk_decoder_state.
type DecoderState struct {
	// C: #define SILK_DECODER_STATE_RESET_START prev_gain_Q16
	// (ENABLE_OSCE is off, so this is the first field and ResetDecoder clears the whole struct.)
	prev_gain_Q16           int32
	exc_Q14                 [MAX_FRAME_LENGTH]int32
	sLPC_Q14_buf            [MAX_LPC_ORDER]int32
	outBuf                  [MAX_FRAME_LENGTH + 2*MAX_SUB_FRAME_LENGTH]int16 // Buffer for output signal
	lagPrev                 int                                              // Previous Lag
	LastGainIndex           int8                                             // Previous gain index
	fs_kHz                  int                                              // Sampling frequency in kHz
	fs_API_hz               int32                                            // API sample frequency (Hz)
	nb_subfr                int                                              // Number of 5 ms subframes in a frame
	frame_length            int                                              // Frame length (samples)
	subfr_length            int                                              // Subframe length (samples)
	ltp_mem_length          int                                              // Length of LTP memory
	LPC_order               int                                              // LPC order
	prevNLSF_Q15            [MAX_LPC_ORDER]int16                             // Used to interpolate LSFs
	first_frame_after_reset int                                              // Flag for deactivating NLSF interpolation
	pitch_lag_low_bits_iCDF []uint8                                          // Pointer to iCDF table for low bits of pitch lag index
	pitch_contour_iCDF      []uint8                                          // Pointer to iCDF table for pitch contour index

	// For buffering payload in case of more frames per packet
	nFramesDecoded   int
	nFramesPerPacket int

	// Specifically for entropy coding
	ec_prevSignalType int
	ec_prevLagIndex   int16

	VAD_flags  [MAX_FRAMES_PER_PACKET]int
	LBRR_flag  int
	LBRR_flags [MAX_FRAMES_PER_PACKET]int

	resampler_state ResamplerStateStruct

	psNLSF_CB *NLSFCBStruct // Pointer to NLSF codebook

	// Quantization indices
	indices SideInfoIndices

	// CNG state
	sCNG CNGStruct

	// Stuff used for PLC
	lossCnt        int
	prevSignalType int
	arch           int

	sPLC PLCStruct
}

// DecoderControl is the C silk_decoder_control.
type DecoderControl struct {
	// Prediction and coding parameters
	pitchL    [MAX_NB_SUBFR]int
	Gains_Q16 [MAX_NB_SUBFR]int32
	// Holds interpolated and final coefficients, 4-byte aligned
	PredCoef_Q12  [2][MAX_LPC_ORDER]int16
	LTPCoef_Q14   [LTP_ORDER * MAX_NB_SUBFR]int16
	LTP_scale_Q14 int
}

// Decoder is the C silk_decoder (defined in dec_API.c): decoder super struct.
type Decoder struct {
	channel_state           [DECODER_NUM_CHANNELS]DecoderState
	sStereo                 StereoDecState
	nChannelsAPI            int
	nChannelsInternal       int
	prev_decode_only_middle int
}
