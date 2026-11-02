package silk

// C: silk/resampler_rom.h — filter coefficients for IIR/FIR polyphase
// resampling (the extern tables are in resampler_rom_tables.go).

const (
	RESAMPLER_DOWN_ORDER_FIR0 = 18
	RESAMPLER_DOWN_ORDER_FIR1 = 24
	RESAMPLER_DOWN_ORDER_FIR2 = 36
	RESAMPLER_ORDER_FIR_12    = 8
)

// Tables for 2x downsampler
const resampler_down2_0 int16 = 9872
const resampler_down2_1 int16 = 39809 - 65536

// Tables for 2x upsampler, high quality
var resampler_up2_hq_0 = [3]int16{1746, 14986, 39083 - 65536}
var resampler_up2_hq_1 = [3]int16{6854, 25769, 55542 - 65536}
