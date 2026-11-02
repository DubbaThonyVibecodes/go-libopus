package silk

// C: silk/resampler_private.h — constants; the functions live in the
// resampler_private_*.go files.

const (
	// Number of input samples to process in the inner loop
	RESAMPLER_MAX_BATCH_SIZE_MS = 10
	RESAMPLER_MAX_FS_KHZ        = 48
	RESAMPLER_MAX_BATCH_SIZE_IN = (RESAMPLER_MAX_BATCH_SIZE_MS * RESAMPLER_MAX_FS_KHZ)
)
