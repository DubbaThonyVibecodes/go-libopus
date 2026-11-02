package silk

// C: silk/pitch_est_defines.h — definitions for pitch estimator
// (the tables are in pitch_est_tables.go).

const (
	PE_MAX_FS_KHZ = 16 // Maximum sampling frequency used

	PE_MAX_NB_SUBFR    = 4
	PE_SUBFR_LENGTH_MS = 5 // 5 ms

	PE_LTP_MEM_LENGTH_MS = (4 * PE_SUBFR_LENGTH_MS)

	PE_MAX_FRAME_LENGTH_MS   = (PE_LTP_MEM_LENGTH_MS + PE_MAX_NB_SUBFR*PE_SUBFR_LENGTH_MS)
	PE_MAX_FRAME_LENGTH      = (PE_MAX_FRAME_LENGTH_MS * PE_MAX_FS_KHZ)
	PE_MAX_FRAME_LENGTH_ST_1 = (PE_MAX_FRAME_LENGTH >> 2)
	PE_MAX_FRAME_LENGTH_ST_2 = (PE_MAX_FRAME_LENGTH >> 1)

	PE_MAX_LAG_MS = 18 // 18 ms -> 56 Hz
	PE_MIN_LAG_MS = 2  // 2 ms -> 500 Hz
	PE_MAX_LAG    = (PE_MAX_LAG_MS * PE_MAX_FS_KHZ)
	PE_MIN_LAG    = (PE_MIN_LAG_MS * PE_MAX_FS_KHZ)

	PE_D_SRCH_LENGTH = 24

	PE_NB_STAGE3_LAGS = 5

	PE_NB_CBKS_STAGE2     = 3
	PE_NB_CBKS_STAGE2_EXT = 11

	PE_NB_CBKS_STAGE3_MAX = 34
	PE_NB_CBKS_STAGE3_MID = 24
	PE_NB_CBKS_STAGE3_MIN = 16

	PE_NB_CBKS_STAGE3_10MS = 12
	PE_NB_CBKS_STAGE2_10MS = 3

	PE_SHORTLAG_BIAS    float32 = 0.2 // for logarithmic weighting
	PE_PREVLAG_BIAS     float32 = 0.2 // for logarithmic weighting
	PE_FLATCONTOUR_BIAS float32 = 0.05

	SILK_PE_MIN_COMPLEX = 0
	SILK_PE_MID_COMPLEX = 1
	SILK_PE_MAX_COMPLEX = 2
)
