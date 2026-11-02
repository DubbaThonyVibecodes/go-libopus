package celt

// C: celt/entdec.h, celt/entdec.c — range decoder.
//
// This is an entropy decoder based upon Martin (1979), which is itself a
// rediscovery of the FIFO arithmetic code introduced by Pasco (1976).
// End of stream is handled by writing out the smallest number of bits that
// ensures that the stream will be correctly decoded regardless of the value of
// any subsequent bits. Tell() can be used to determine how many bits were
// needed to decode all the symbols thus far; other data can be packed in the
// remaining bits of the input buffer.

// C: static int ec_read_byte(ec_dec *_this)
func (d *EcDec) readByte() int {
	if d.offs < d.storage {
		b := d.buf[d.offs]
		d.offs++
		return int(b)
	}
	return 0
}

// C: static int ec_read_byte_from_end(ec_dec *_this)
func (d *EcDec) readByteFromEnd() int {
	if d.end_offs < d.storage {
		d.end_offs++
		return int(d.buf[d.storage-d.end_offs])
	}
	return 0
}

// Normalizes the contents of val and rng so that rng lies entirely in the
// high-order symbol.
//
// C: static void ec_dec_normalize(ec_dec *_this)
func (d *EcDec) decNormalize() {
	// If the range is too small, rescale it and input some bits.
	for d.rng <= EC_CODE_BOT {
		var sym int
		d.nbits_total += EC_SYM_BITS
		d.rng <<= EC_SYM_BITS
		// Use up the remaining bits from our last symbol.
		sym = d.rem
		// Read the next value from the input.
		d.rem = d.readByte()
		// Take the rest of the bits we need from this new symbol.
		sym = (sym<<EC_SYM_BITS | d.rem) >> (EC_SYM_BITS - EC_CODE_EXTRA)
		// And subtract them from val, capped to be less than EC_CODE_TOP.
		d.val = ((d.val << EC_SYM_BITS) + (EC_SYM_MAX &^ uint32(sym))) & (EC_CODE_TOP - 1)
	}
}

// C: void ec_dec_init(ec_dec *_this,unsigned char *_buf,opus_uint32 _storage)
func (d *EcDec) DecInit(buf []byte, storage uint32) {
	d.buf = buf
	d.storage = storage
	d.end_offs = 0
	d.end_window = 0
	d.nend_bits = 0
	// This is the offset from which ec_tell() will subtract partial bits.
	// The final value after the ec_dec_normalize() call will be the same as in
	// the encoder, but we have to compensate for the bits that are added there.
	d.nbits_total = EC_CODE_BITS + 1 - ((EC_CODE_BITS-EC_CODE_EXTRA)/EC_SYM_BITS)*EC_SYM_BITS
	d.offs = 0
	d.rng = 1 << EC_CODE_EXTRA
	d.rem = d.readByte()
	d.val = d.rng - 1 - uint32(d.rem>>(EC_SYM_BITS-EC_CODE_EXTRA))
	d.error = 0
	// Normalize the interval.
	d.decNormalize()
}

// C: unsigned ec_decode(ec_dec *_this,unsigned _ft)
func (d *EcDec) Decode(ft uint32) uint32 {
	var s uint32
	d.ext = Udiv(d.rng, ft)
	s = d.val / d.ext
	return ft - EC_MINI(s+1, ft)
}

// C: unsigned ec_decode_bin(ec_dec *_this,unsigned _bits)
func (d *EcDec) DecodeBin(bits uint) uint32 {
	var s uint32
	d.ext = d.rng >> bits
	s = d.val / d.ext
	return (uint32(1) << bits) - EC_MINI(s+1, uint32(1)<<bits)
}

// C: void ec_dec_update(ec_dec *_this,unsigned _fl,unsigned _fh,unsigned _ft)
func (d *EcDec) DecUpdate(fl, fh, ft uint32) {
	var s uint32
	s = IMUL32(d.ext, ft-fh)
	d.val -= s
	if fl > 0 {
		d.rng = IMUL32(d.ext, fh-fl)
	} else {
		d.rng = d.rng - s
	}
	d.decNormalize()
}

// The probability of having a "one" is 1/(1<<_logp).
//
// C: int ec_dec_bit_logp(ec_dec *_this,unsigned _logp)
func (d *EcDec) DecBitLogp(logp uint) int {
	var r, dd, s uint32
	var ret int
	r = d.rng
	dd = d.val
	s = r >> logp
	if dd < s {
		ret = 1
	}
	if ret == 0 {
		d.val = dd - s
	}
	if ret != 0 {
		d.rng = s
	} else {
		d.rng = r - s
	}
	d.decNormalize()
	return ret
}

// C: int ec_dec_icdf(ec_dec *_this,const unsigned char *_icdf,unsigned _ftb)
func (d *EcDec) DecIcdf(icdf []uint8, ftb uint) int {
	var r, dd, s, t uint32
	var ret int
	s = d.rng
	dd = d.val
	r = s >> ftb
	ret = -1
	for {
		t = s
		ret++
		s = IMUL32(r, uint32(icdf[ret]))
		if dd >= s {
			break
		}
	}
	d.val = dd - s
	d.rng = t - s
	d.decNormalize()
	return ret
}

// DecIcdf16 decodes a symbol given an "inverse" CDF table with 16-bit entries.
// No call to DecUpdate() is necessary after this call.
//
// C: int ec_dec_icdf16(ec_dec *_this,const opus_uint16 *_icdf,unsigned _ftb)
func (d *EcDec) DecIcdf16(icdf []uint16, ftb uint) int {
	var r, dd, s, t uint32
	var ret int
	s = d.rng
	dd = d.val
	r = s >> ftb
	ret = -1
	for {
		t = s
		ret++
		s = IMUL32(r, uint32(icdf[ret]))
		if dd >= s {
			break
		}
	}
	d.val = dd - s
	d.rng = t - s
	d.decNormalize()
	return ret
}

// C: opus_uint32 ec_dec_uint(ec_dec *_this,opus_uint32 _ft)
func (d *EcDec) DecUint(ft uint32) uint32 {
	var ft2, s uint32
	var ftb int
	// In order to optimize EC_ILOG(), it is best to have a value where the
	// value of _ft is a power of two.
	celt_assert(ft > 1)
	ft--
	ftb = EC_ILOG(ft)
	if ftb > EC_UINT_BITS {
		var t uint32
		ftb -= EC_UINT_BITS
		ft2 = (ft >> uint(ftb)) + 1
		s = d.Decode(ft2)
		d.DecUpdate(s, s+1, ft2)
		t = s<<uint(ftb) | d.DecBits(uint(ftb))
		if t <= ft {
			return t
		}
		d.error = 1
		return ft
	}
	ft++
	s = d.Decode(ft)
	d.DecUpdate(s, s+1, ft)
	return s
}

// C: opus_uint32 ec_dec_bits(ec_dec *_this,unsigned _bits)
func (d *EcDec) DecBits(bits uint) uint32 {
	var window EcWindow
	var available int
	var ret uint32
	window = d.end_window
	available = d.nend_bits
	if uint(available) < bits {
		for {
			window |= EcWindow(d.readByteFromEnd()) << uint(available)
			available += EC_SYM_BITS
			if available > EC_WINDOW_SIZE-EC_SYM_BITS {
				break
			}
		}
	}
	ret = uint32(window) & ((uint32(1) << bits) - 1)
	window >>= bits
	available -= int(bits)
	d.end_window = window
	d.nend_bits = available
	d.nbits_total += int(bits)
	return ret
}
