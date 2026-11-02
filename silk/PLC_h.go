package silk

// C: silk/PLC.h — constants; the functions are in PLC.go.

const (
	BWE_COEF                     = 0.99
	V_PITCH_GAIN_START_MIN_Q14   = 11469 // 0.7 in Q14
	V_PITCH_GAIN_START_MAX_Q14   = 15565 // 0.95 in Q14
	MAX_PITCH_LAG_MS             = 18
	RAND_BUF_SIZE                = 128
	RAND_BUF_MASK                = (RAND_BUF_SIZE - 1)
	LOG2_INV_LPC_GAIN_HIGH_THRES = 3   // 2^3 = 8 dB LPC gain
	LOG2_INV_LPC_GAIN_LOW_THRES  = 8   // 2^8 = 24 dB LPC gain
	PITCH_DRIFT_FAC_Q16          = 655 // 0.01 in Q16
)
