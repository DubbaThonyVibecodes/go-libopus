package celt

// C: celt/modes.h, celt/modes.c — mode definition. Only the static
// (non-CUSTOM_MODES) path is ported: opus_custom_mode_create() looks the
// request up in static_mode_list.

const MAX_PERIOD = 1024

// C: PulseCache
type PulseCache struct {
	size  int
	index []int16
	bits  []uint8
	caps  []uint8
}

// CELTMode is the C struct OpusCustomMode (CELTMode).
type CELTMode struct {
	Fs      int32
	overlap int

	nbEBands  int
	effEBands int
	preemph   [4]int16
	eBands    []int16 // Definition for each "pseudo-critical band"

	maxLM         int
	nbShortMdcts  int
	shortMdctSize int

	nbAllocVectors int     // Number of lines in the matrix below
	allocVectors   []uint8 // Number of bits in each band for several rates
	logN           []int16

	window []int16
	mdct   mdct_lookup
	cache  PulseCache
}

// Accessors for other packages (the C code reads the fields directly).
func (m *CELTMode) Overlap() int       { return m.overlap }
func (m *CELTMode) NbEBands() int      { return m.nbEBands }
func (m *CELTMode) ShortMdctSize() int { return m.shortMdctSize }
func (m *CELTMode) EBands() []int16    { return m.eBands }

// C: #define OVERLAP(mode) ((mode)->overlap)
func OVERLAP(mode *CELTMode) int { return mode.overlap }

// C: #define FRAMESIZE(mode) ((mode)->mdctSize)   (unused upstream)

var eband5ms = [22]int16{
	// 0  200 400 600 800  1k 1.2 1.4 1.6  2k 2.4 2.8 3.2  4k 4.8 5.6 6.8  8k 9.6 12k 15.6
	0, 1, 2, 3, 4, 5, 6, 7, 8, 10, 12, 14, 16, 20, 24, 28, 34, 40, 48, 60, 78, 100,
}

// Alternate tuning (partially derived from Vorbis)
const BITALLOC_SIZE = 11

// Bit allocation table in units of 1/32 bit/sample (0.1875 dB SNR)
var band_allocation = [BITALLOC_SIZE * 21]uint8{
	// 0  200 400 600 800  1k 1.2 1.4 1.6  2k 2.4 2.8 3.2  4k 4.8 5.6 6.8  8k 9.6 12k 15.6
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	90, 80, 75, 69, 63, 56, 49, 40, 34, 29, 20, 18, 10, 0, 0, 0, 0, 0, 0, 0, 0,
	110, 100, 90, 84, 78, 71, 65, 58, 51, 45, 39, 32, 26, 20, 12, 0, 0, 0, 0, 0, 0,
	118, 110, 103, 93, 86, 80, 75, 70, 65, 59, 53, 47, 40, 31, 23, 15, 4, 0, 0, 0, 0,
	126, 119, 112, 104, 95, 89, 83, 78, 72, 66, 60, 54, 47, 39, 32, 25, 17, 12, 1, 0, 0,
	134, 127, 120, 114, 103, 97, 91, 85, 78, 72, 66, 60, 54, 47, 41, 35, 29, 23, 16, 10, 1,
	144, 137, 130, 124, 113, 107, 101, 95, 88, 82, 76, 70, 64, 57, 51, 45, 39, 33, 26, 15, 1,
	152, 145, 138, 132, 123, 117, 111, 105, 98, 92, 86, 80, 74, 67, 61, 55, 49, 43, 36, 20, 1,
	162, 155, 148, 142, 133, 127, 121, 115, 108, 102, 96, 90, 84, 77, 71, 65, 59, 53, 46, 30, 1,
	172, 165, 158, 152, 143, 137, 131, 125, 118, 112, 106, 100, 94, 87, 81, 75, 69, 63, 56, 45, 20,
	200, 200, 200, 200, 200, 200, 200, 200, 198, 193, 188, 183, 178, 173, 168, 163, 158, 153, 148, 129, 104,
}

// CustomModeCreate returns the static mode matching Fs and frame_size, or
// nil with *error = OPUS_BAD_ARG (custom modes are not compiled in).
//
// C: CELTMode *opus_custom_mode_create(opus_int32 Fs, int frame_size, int *error)
func CustomModeCreate(Fs int32, frame_size int, error *int) *CELTMode {
	var i int
	for i = 0; i < TOTAL_MODES; i++ {
		var j int
		for j = 0; j < 4; j++ {
			if Fs == static_mode_list[i].Fs &&
				(frame_size<<uint(j)) == static_mode_list[i].shortMdctSize*static_mode_list[i].nbShortMdcts {
				if error != nil {
					*error = OPUS_OK
				}
				return static_mode_list[i]
			}
		}
	}
	if error != nil {
		*error = OPUS_BAD_ARG
	}
	return nil
}

// Window returns the MDCT window (C: mode->window).
func (m *CELTMode) Window() []int16 { return m.window }

// Fs returns the sampling rate (C: mode->Fs).
func (m *CELTMode) SampleRate() int32 { return m.Fs }

// Preemph returns the pre-emphasis coefficients (C: mode->preemph).
func (m *CELTMode) Preemph() [4]int16 { return m.preemph }
