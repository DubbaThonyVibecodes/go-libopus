package silk

// C: silk/resampler_structs.h

const (
	SILK_RESAMPLER_MAX_FIR_ORDER = 36
	SILK_RESAMPLER_MAX_IIR_ORDER = 6
)

// ResamplerStateStruct is the C silk_resampler_state_struct. The C sFIR
// union is split into its two views; a state only ever uses one of them.
type ResamplerStateStruct struct {
	sIIR               [SILK_RESAMPLER_MAX_IIR_ORDER]int32 // this must be the first element of this struct
	sFIR_i32           [SILK_RESAMPLER_MAX_FIR_ORDER]int32
	sFIR_i16           [SILK_RESAMPLER_MAX_FIR_ORDER]int16
	delayBuf           [48]int16
	resampler_function int
	batchSize          int
	invRatio_Q16       int32
	FIR_Order          int
	FIR_Fracs          int
	Fs_in_kHz          int
	Fs_out_kHz         int
	inputDelay         int
	Coefs              []int16
}
