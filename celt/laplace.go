package celt

// C: celt/laplace.h, celt/laplace.c

// The minimum probability of an energy delta (out of 32768).
const LAPLACE_LOG_MINP = 0
const LAPLACE_MINP = 1 << LAPLACE_LOG_MINP

// The minimum number of guaranteed representable energy deltas (in one direction).
const LAPLACE_NMIN = 16

// When called, decay is positive and at most 11456.
//
// C: static unsigned ec_laplace_get_freq1(unsigned fs0, int decay)
func laplaceGetFreq1(fs0 uint32, decay int) uint32 {
	var ft uint32
	ft = 32768 - LAPLACE_MINP*(2*LAPLACE_NMIN) - fs0
	return uint32(int32(ft) * int32(16384-decay) >> 15)
}

// LaplaceEncode encodes a value that is assumed to be the realisation of a
// Laplace-distributed random process.
//
//	value: Value to encode (clamped on return)
//	fs:    Probability of 0, multiplied by 32768
//	decay: Probability of the value +/- 1, multiplied by 16384
//
// C: void ec_laplace_encode(ec_enc *enc, int *value, unsigned fs, int decay)
func (enc *EcEnc) LaplaceEncode(value *int, fs uint32, decay int) {
	var fl uint32
	val := *value
	fl = 0
	if val != 0 {
		var s, i int
		if val < 0 {
			s = -1
		}
		val = (val + s) ^ s
		fl = fs
		fs = laplaceGetFreq1(fs, decay)
		// Search the decaying part of the PDF.
		for i = 1; fs > 0 && i < val; i++ {
			fs *= 2
			fl += fs + 2*LAPLACE_MINP
			fs = uint32(int32(fs) * int32(decay) >> 15)
		}
		// Everything beyond that has probability LAPLACE_MINP.
		if fs == 0 {
			var di, ndi_max int
			ndi_max = int(32768-fl+LAPLACE_MINP-1) >> LAPLACE_LOG_MINP
			ndi_max = (ndi_max - s) >> 1
			di = IMIN(val-i, ndi_max-1)
			fl += uint32((2*di + 1 + s) * LAPLACE_MINP)
			fs = IMIN(LAPLACE_MINP, 32768-fl)
			*value = (i + di + s) ^ s
		} else {
			fs += LAPLACE_MINP
			fl += fs & ^uint32(s)
		}
		celt_assert(fl+fs <= 32768)
		celt_assert(fs > 0)
	}
	enc.EncodeBin(fl, fl+fs, 15)
}

// LaplaceDecode decodes a value that is assumed to be the realisation of a
// Laplace-distributed random process.
//
// C: int ec_laplace_decode(ec_dec *dec, unsigned fs, int decay)
func (dec *EcDec) LaplaceDecode(fs uint32, decay int) int {
	var val int
	var fl, fm uint32
	fm = dec.DecodeBin(15)
	fl = 0
	if fm >= fs {
		val++
		fl = fs
		fs = laplaceGetFreq1(fs, decay) + LAPLACE_MINP
		// Search the decaying part of the PDF.
		for fs > LAPLACE_MINP && fm >= fl+2*fs {
			fs *= 2
			fl += fs
			fs = uint32(int32(fs-2*LAPLACE_MINP) * int32(decay) >> 15)
			fs += LAPLACE_MINP
			val++
		}
		// Everything beyond that has probability LAPLACE_MINP.
		if fs <= LAPLACE_MINP {
			var di int
			di = int(fm-fl) >> (LAPLACE_LOG_MINP + 1)
			val += di
			fl += uint32(2 * di * LAPLACE_MINP)
		}
		if fm < fl+fs {
			val = -val
		} else {
			fl += fs
		}
	}
	celt_assert(fl < 32768)
	celt_assert(fs > 0)
	celt_assert(fl <= fm)
	celt_assert(fm < IMIN(fl+fs, 32768))
	dec.DecUpdate(fl, IMIN(fl+fs, 32768), 32768)
	return val
}
