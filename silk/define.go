package silk

// C: silk/define.h

const (
	// Max number of encoder channels (1/2)
	ENCODER_NUM_CHANNELS = 2
	// Number of decoder channels (1/2)
	DECODER_NUM_CHANNELS = 2

	MAX_FRAMES_PER_PACKET = 3

	// Limits on bitrate
	MIN_TARGET_RATE_BPS = 5000
	MAX_TARGET_RATE_BPS = 80000
	TARGET_RATE_TAB_SZ  = 8

	// LBRR thresholds
	LBRR_NB_MIN_RATE_BPS = 12000
	LBRR_MB_MIN_RATE_BPS = 14000
	LBRR_WB_MIN_RATE_BPS = 16000

	// DTX settings
	NB_SPEECH_FRAMES_BEFORE_DTX = 10 // eq 200 ms
	MAX_CONSECUTIVE_DTX         = 20 // eq 400 ms

	// Maximum sampling frequency
	MAX_FS_KHZ     = 16
	MAX_API_FS_KHZ = 48

	// Signal types
	TYPE_NO_VOICE_ACTIVITY = 0
	TYPE_UNVOICED          = 1
	TYPE_VOICED            = 2

	// Conditional coding types
	CODE_INDEPENDENTLY                = 0
	CODE_INDEPENDENTLY_NO_LTP_SCALING = 1
	CODE_CONDITIONALLY                = 2

	// Settings for stereo processing
	STEREO_QUANT_TAB_SIZE    = 16
	STEREO_QUANT_SUB_STEPS   = 5
	STEREO_INTERP_LEN_MS     = 8    // must be even
	STEREO_RATIO_SMOOTH_COEF = 0.01 // smoothing coef for signal norms and stereo width

	// Range of pitch lag estimates
	PITCH_EST_MIN_LAG_MS = 2  // 2 ms -> 500 Hz
	PITCH_EST_MAX_LAG_MS = 18 // 18 ms -> 56 Hz

	// Maximum number of subframes
	MAX_NB_SUBFR = 4

	// Number of samples per frame
	LTP_MEM_LENGTH_MS    = 20
	SUB_FRAME_LENGTH_MS  = 5
	MAX_SUB_FRAME_LENGTH = (SUB_FRAME_LENGTH_MS * MAX_FS_KHZ)
	MAX_FRAME_LENGTH_MS  = (SUB_FRAME_LENGTH_MS * MAX_NB_SUBFR)
	MAX_FRAME_LENGTH     = (MAX_FRAME_LENGTH_MS * MAX_FS_KHZ)

	// Milliseconds of lookahead for pitch analysis
	LA_PITCH_MS  = 2
	LA_PITCH_MAX = (LA_PITCH_MS * MAX_FS_KHZ)

	// Order of LPC used in find pitch
	MAX_FIND_PITCH_LPC_ORDER = 16

	// Length of LPC window used in find pitch
	FIND_PITCH_LPC_WIN_MS      = (20 + (LA_PITCH_MS << 1))
	FIND_PITCH_LPC_WIN_MS_2_SF = (10 + (LA_PITCH_MS << 1))
	FIND_PITCH_LPC_WIN_MAX     = (FIND_PITCH_LPC_WIN_MS * MAX_FS_KHZ)

	// Milliseconds of lookahead for noise shape analysis
	LA_SHAPE_MS  = 5
	LA_SHAPE_MAX = (LA_SHAPE_MS * MAX_FS_KHZ)

	// Maximum length of LPC window used in noise shape analysis
	SHAPE_LPC_WIN_MAX = (15 * MAX_FS_KHZ)

	// dB level of lowest gain quantization level
	MIN_QGAIN_DB = 2
	// dB level of highest gain quantization level
	MAX_QGAIN_DB = 88
	// Number of gain quantization levels
	N_LEVELS_QGAIN = 64
	// Max increase in gain quantization index
	MAX_DELTA_GAIN_QUANT = 36
	// Max decrease in gain quantization index
	MIN_DELTA_GAIN_QUANT = -4

	// Quantization offsets (multiples of 4)
	OFFSET_VL_Q10  = 32
	OFFSET_VH_Q10  = 100
	OFFSET_UVL_Q10 = 100
	OFFSET_UVH_Q10 = 240

	QUANT_LEVEL_ADJUST_Q10 = 80

	// Maximum numbers of iterations used to stabilize an LPC vector
	MAX_LPC_STABILIZE_ITERATIONS          = 16
	MAX_PREDICTION_POWER_GAIN             = float32(1e4)
	MAX_PREDICTION_POWER_GAIN_AFTER_RESET = float32(1e2)

	MAX_LPC_ORDER = 16
	MIN_LPC_ORDER = 10

	// Find Pred Coef defines
	LTP_ORDER = 5

	// LTP quantization settings
	NB_LTP_CBKS = 3

	// Flag to use harmonic noise shaping
	USE_HARM_SHAPING = 1

	// Max LPC order of noise shaping filters
	MAX_SHAPE_LPC_ORDER = 16

	HARM_SHAPE_FIR_TAPS = 3

	// Maximum number of delayed decision states
	MAX_DEL_DEC_STATES = 4

	LTP_BUF_LENGTH = 512
	LTP_MASK       = (LTP_BUF_LENGTH - 1)

	DECISION_DELAY      = 32
	DECISION_DELAY_MASK = (DECISION_DELAY - 1)

	// Number of subframes for excitation entropy coding
	SHELL_CODEC_FRAME_LENGTH      = 16
	LOG2_SHELL_CODEC_FRAME_LENGTH = 4
	MAX_NB_SHELL_BLOCKS           = (MAX_FRAME_LENGTH / SHELL_CODEC_FRAME_LENGTH)

	// Number of rate levels, for entropy coding of excitation
	N_RATE_LEVELS = 10

	// Maximum sum of pulses per shell coding frame
	MAX_PULSES = 16

	MAX_MATRIX_SIZE = MAX_LPC_ORDER // Max of LPC Order and LTP order

	NSQ_LPC_BUF_LENGTH = DECISION_DELAY // MAX_LPC_ORDER > DECISION_DELAY is false

	// Voice activity detector
	VAD_N_BANDS = 4

	VAD_INTERNAL_SUBFRAMES_LOG2 = 2
	VAD_INTERNAL_SUBFRAMES      = (1 << VAD_INTERNAL_SUBFRAMES_LOG2)

	VAD_NOISE_LEVEL_SMOOTH_COEF_Q16 = 1024 // Must be <  4096
	VAD_NOISE_LEVELS_BIAS           = 50

	// Sigmoid settings
	VAD_NEGATIVE_OFFSET_Q5 = 128 // sigmoid is 0 at -128
	VAD_SNR_FACTOR_Q16     = 45000

	// smoothing for SNR measurement
	VAD_SNR_SMOOTH_COEF_Q18 = 4096

	// Size of the piecewise linear cosine approximation table for the LSFs
	LSF_COS_TAB_SZ_FIX = 128

	// NLSF quantizer
	NLSF_W_Q                       = 2
	NLSF_VQ_MAX_VECTORS            = 32
	NLSF_VQ_MAX_SURVIVORS          = 32
	NLSF_QUANT_MAX_AMPLITUDE       = 4
	NLSF_QUANT_MAX_AMPLITUDE_EXT   = 10
	NLSF_QUANT_LEVEL_ADJ           = 0.1
	NLSF_QUANT_DEL_DEC_STATES_LOG2 = 2
	NLSF_QUANT_DEL_DEC_STATES      = (1 << NLSF_QUANT_DEL_DEC_STATES_LOG2)

	// Transition filtering for mode switching
	TRANSITION_TIME_MS   = 5120 // 5120 = 64 * FRAME_LENGTH_MS * ( TRANSITION_INT_NUM - 1 ) = 64*(20*4)
	TRANSITION_NB        = 3    // Hardcoded in tables
	TRANSITION_NA        = 2    // Hardcoded in tables
	TRANSITION_INT_NUM   = 5    // Hardcoded in tables
	TRANSITION_FRAMES    = (TRANSITION_TIME_MS / MAX_FRAME_LENGTH_MS)
	TRANSITION_INT_STEPS = (TRANSITION_FRAMES / (TRANSITION_INT_NUM - 1))

	// BWE factors to apply after packet loss
	BWE_AFTER_LOSS_Q16 = 63570

	// Defines for CN generation
	CNG_BUF_MASK_MAX  = 255   // 2^floor(log2(MAX_FRAME_LENGTH))-1
	CNG_GAIN_SMTH_Q16 = 4634  // 0.25^(1/4)
	CNG_NLSF_SMTH_Q16 = 16348 // 0.25
)
