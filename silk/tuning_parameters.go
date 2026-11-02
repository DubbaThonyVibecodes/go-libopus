package silk

// C: silk/tuning_parameters.h — constants with an `f` suffix in C are typed
// float32 here so that SILK_FIX_CONST reproduces the single-precision product.

const (
	// Decay time for bitreservoir
	BITRESERVOIR_DECAY_TIME_MS = 500

	// Pitch estimator

	// Level of noise floor for whitening filter LPC analysis in pitch analysis
	FIND_PITCH_WHITE_NOISE_FRACTION float32 = 1e-3
	// Bandwidth expansion for whitening filter in pitch analysis
	FIND_PITCH_BANDWIDTH_EXPANSION float32 = 0.99

	// Linear prediction

	// LPC analysis defines: regularization and bandwidth expansion
	FIND_LPC_COND_FAC float32 = 1e-5

	// LTP analysis defines
	FIND_LTP_COND_FAC float32 = 1e-5
	LTP_DAMPING       float32 = 0.05
	LTP_SMOOTHING     float32 = 0.1

	// LTP quantization settings
	MU_LTP_QUANT_NB float32 = 0.03
	MU_LTP_QUANT_MB float32 = 0.025
	MU_LTP_QUANT_WB float32 = 0.02

	// Max cumulative LTP gain
	MAX_SUM_LOG_GAIN_DB float32 = 250.0

	// High pass filtering

	// Smoothing parameters for low end of pitch frequency range estimation
	VARIABLE_HP_SMTH_COEF1     float32 = 0.1
	VARIABLE_HP_SMTH_COEF2     float32 = 0.015
	VARIABLE_HP_MAX_DELTA_FREQ float32 = 0.4

	// Min and max cut-off frequency values (-3 dB points)
	VARIABLE_HP_MIN_CUTOFF_HZ = 60
	VARIABLE_HP_MAX_CUTOFF_HZ = 100

	// Various

	// VAD threshold
	SPEECH_ACTIVITY_DTX_THRES float32 = 0.05

	// Speech Activity LBRR enable threshold
	LBRR_SPEECH_ACTIVITY_THRES float32 = 0.3

	// Perceptual parameters

	// reduction in coding SNR during low speech activity
	BG_SNR_DECR_dB float32 = 2.0

	// factor for reducing quantization noise during voiced speech
	HARM_SNR_INCR_dB float32 = 2.0

	// factor for reducing quantization noise for unvoiced sparse signals
	SPARSE_SNR_INCR_dB float32 = 2.0

	// threshold for sparseness measure above which to use lower quantization offset during unvoiced
	SPARSENESS_THRESHOLD_QNT_OFFSET float32 = 0.75

	// warping control
	WARPING_MULTIPLIER float32 = 0.015

	// fraction added to first autocorrelation value
	SHAPE_WHITE_NOISE_FRACTION float32 = 5e-5

	// noise shaping filter chirp factor
	BANDWIDTH_EXPANSION float32 = 0.95

	// difference between chirp factors for analysis and synthesis noise shaping filters at low bitrates
	LOW_RATE_BANDWIDTH_EXPANSION_DELTA float32 = 0.01

	// extra harmonic boosting (signal shaping) at low bitrates
	LOW_RATE_HARMONIC_BOOST float32 = 0.1

	// extra harmonic boosting (signal shaping) for noisy input signals
	LOW_INPUT_QUALITY_HARMONIC_BOOST float32 = 0.1

	// harmonic noise shaping
	HARMONIC_SHAPING float32 = 0.3

	// extra harmonic noise shaping for high bitrates or noisy input
	HIGH_RATE_OR_LOW_QUALITY_HARMONIC_SHAPING float32 = 0.2

	// parameter for shaping noise towards higher frequencies
	HP_NOISE_COEF float32 = 0.25

	// parameter for shaping noise even more towards higher frequencies during voiced speech
	HARM_HP_NOISE_COEF float32 = 0.35

	// parameter for applying a high-pass tilt to the input signal
	INPUT_TILT float32 = 0.05

	// parameter for extra high-pass tilt to the input signal at high rates
	HIGH_RATE_INPUT_TILT float32 = 0.1

	// parameter for reducing noise at the very low frequencies
	LOW_FREQ_SHAPING float32 = 4.0

	// less reduction of noise at the very low frequencies for signals with low SNR at low frequencies
	LOW_QUALITY_LOW_FREQ_SHAPING_DECR float32 = 0.5

	// subframe smoothing coefficient for HarmBoost, HarmShapeGain, Tilt (lower -> more smoothing)
	SUBFR_SMTH_COEF float32 = 0.4

	// parameters defining the R/D tradeoff in the residual quantizer
	LAMBDA_OFFSET            float32 = 1.2
	LAMBDA_SPEECH_ACT        float32 = -0.2
	LAMBDA_DELAYED_DECISIONS float32 = -0.05
	LAMBDA_INPUT_QUALITY     float32 = -0.1
	LAMBDA_CODING_QUALITY    float32 = -0.2
	LAMBDA_QUANT_OFFSET      float32 = 0.8

	// Compensation in bitrate calculations for 10 ms modes
	REDUCE_BITRATE_10_MS_BPS = 2200

	// Maximum time before allowing a bandwidth transition
	MAX_BANDWIDTH_SWITCH_DELAY_MS = 5000
)
