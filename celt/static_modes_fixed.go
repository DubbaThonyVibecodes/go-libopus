package celt

// C: celt/static_modes_fixed.h — the FFT states and the mode definition
// (the numeric tables are in static_modes_fixed_tables.go).

var fft_state48000_960_0 = kiss_fft_state{
	nfft:     480,
	shift:    -1,
	factors:  [2 * MAXFACTORS]int16{4, 120, 4, 30, 2, 15, 3, 5, 5, 1, 0, 0, 0, 0, 0, 0},
	bitrev:   fft_bitrev480[:],
	twiddles: fft_twiddles48000_960[:],
}

var fft_state48000_960_1 = kiss_fft_state{
	nfft:     240,
	shift:    1,
	factors:  [2 * MAXFACTORS]int16{4, 60, 4, 15, 3, 5, 5, 1, 0, 0, 0, 0, 0, 0, 0, 0},
	bitrev:   fft_bitrev240[:],
	twiddles: fft_twiddles48000_960[:],
}

var fft_state48000_960_2 = kiss_fft_state{
	nfft:     120,
	shift:    2,
	factors:  [2 * MAXFACTORS]int16{4, 30, 2, 15, 3, 5, 5, 1, 0, 0, 0, 0, 0, 0, 0, 0},
	bitrev:   fft_bitrev120[:],
	twiddles: fft_twiddles48000_960[:],
}

var fft_state48000_960_3 = kiss_fft_state{
	nfft:     60,
	shift:    3,
	factors:  [2 * MAXFACTORS]int16{4, 15, 3, 5, 5, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	bitrev:   fft_bitrev60[:],
	twiddles: fft_twiddles48000_960[:],
}

var mode48000_960_120 = CELTMode{
	Fs:             48000,
	overlap:        120,
	nbEBands:       21,
	effEBands:      21,
	preemph:        [4]int16{27853, 0, 4096, 8192},
	eBands:         eband5ms[:],
	maxLM:          3,
	nbShortMdcts:   8,
	shortMdctSize:  120,
	nbAllocVectors: 11,
	allocVectors:   band_allocation[:],
	logN:           logN400[:],
	window:         window120[:],
	mdct: mdct_lookup{
		n:        1920,
		maxshift: 3,
		kfft:     [4]*kiss_fft_state{&fft_state48000_960_0, &fft_state48000_960_1, &fft_state48000_960_2, &fft_state48000_960_3},
		trig:     mdct_twiddles960[:],
	},
	cache: PulseCache{size: 392, index: cache_index50[:], bits: cache_bits50[:], caps: cache_caps50[:]},
}

// List of all the available modes
const TOTAL_MODES = 1

var static_mode_list = [TOTAL_MODES]*CELTMode{
	&mode48000_960_120,
}
