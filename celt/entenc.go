package celt

// C: celt/entenc.h, celt/entenc.c — range encoder.
// See entdec.go and the references there for implementation details.

// C: static int ec_write_byte(ec_enc *_this,unsigned _value)
func (e *EcEnc) writeByte(value uint32) int {
	if e.offs+e.end_offs >= e.storage {
		return -1
	}
	e.buf[e.offs] = byte(value)
	e.offs++
	return 0
}

// C: static int ec_write_byte_at_end(ec_enc *_this,unsigned _value)
func (e *EcEnc) writeByteAtEnd(value uint32) int {
	if e.offs+e.end_offs >= e.storage {
		return -1
	}
	e.end_offs++
	e.buf[e.storage-e.end_offs] = byte(value)
	return 0
}

// Outputs a symbol, with a carry bit.
// If there is a potential to propagate a carry over several symbols, they are
// buffered until it can be determined whether or not an actual carry will
// occur. If the counter for the buffered symbols overflows, then the stream
// becomes undecodable.
//
// C: static void ec_enc_carry_out(ec_enc *_this,int _c)
func (e *EcEnc) encCarryOut(c int) {
	if uint32(c) != EC_SYM_MAX {
		var carry int
		carry = c >> EC_SYM_BITS
		// Don't output a byte on the first write.
		// This compare should be taken care of by branch-prediction thereafter.
		if e.rem >= 0 {
			e.error |= e.writeByte(uint32(e.rem + carry))
		}
		if e.ext > 0 {
			var sym uint32
			sym = (EC_SYM_MAX + uint32(carry)) & EC_SYM_MAX
			for {
				e.error |= e.writeByte(sym)
				e.ext--
				if e.ext <= 0 {
					break
				}
			}
		}
		e.rem = c & int(EC_SYM_MAX)
	} else {
		e.ext++
	}
}

// C: static void ec_enc_normalize(ec_enc *_this)
func (e *EcEnc) encNormalize() {
	// If the range is too small, output some bits and rescale it.
	for e.rng <= EC_CODE_BOT {
		e.encCarryOut(int(e.val >> EC_CODE_SHIFT))
		// Move the next-to-high-order symbol into the high-order position.
		e.val = (e.val << EC_SYM_BITS) & (EC_CODE_TOP - 1)
		e.rng <<= EC_SYM_BITS
		e.nbits_total += EC_SYM_BITS
	}
}

// C: void ec_enc_init(ec_enc *_this,unsigned char *_buf,opus_uint32 _size)
func (e *EcEnc) EncInit(buf []byte, size uint32) {
	e.buf = buf
	e.end_offs = 0
	e.end_window = 0
	e.nend_bits = 0
	e.nbits_total = EC_CODE_BITS + 1
	e.offs = 0
	e.rng = EC_CODE_TOP
	e.rem = -1
	e.val = 0
	e.ext = 0
	e.storage = size
	e.error = 0
}

// C: void ec_encode(ec_enc *_this,unsigned _fl,unsigned _fh,unsigned _ft)
func (e *EcEnc) Encode(fl, fh, ft uint32) {
	var r uint32
	r = Udiv(e.rng, ft)
	if fl > 0 {
		e.val += e.rng - IMUL32(r, ft-fl)
		e.rng = IMUL32(r, fh-fl)
	} else {
		e.rng -= IMUL32(r, ft-fh)
	}
	e.encNormalize()
}

// C: void ec_encode_bin(ec_enc *_this,unsigned _fl,unsigned _fh,unsigned _bits)
func (e *EcEnc) EncodeBin(fl, fh uint32, bits uint) {
	var r uint32
	r = e.rng >> bits
	if fl > 0 {
		e.val += e.rng - IMUL32(r, (uint32(1)<<bits)-fl)
		e.rng = IMUL32(r, fh-fl)
	} else {
		e.rng -= IMUL32(r, (uint32(1)<<bits)-fh)
	}
	e.encNormalize()
}

// The probability of having a "one" is 1/(1<<_logp).
//
// C: void ec_enc_bit_logp(ec_enc *_this,int _val,unsigned _logp)
func (e *EcEnc) EncBitLogp(val int, logp uint) {
	var r, s, l uint32
	r = e.rng
	l = e.val
	s = r >> logp
	r -= s
	if val != 0 {
		e.val = l + r
	}
	if val != 0 {
		e.rng = s
	} else {
		e.rng = r
	}
	e.encNormalize()
}

// C: void ec_enc_icdf(ec_enc *_this,int _s,const unsigned char *_icdf,unsigned _ftb)
func (e *EcEnc) EncIcdf(s int, icdf []uint8, ftb uint) {
	var r uint32
	r = e.rng >> ftb
	if s > 0 {
		e.val += e.rng - IMUL32(r, uint32(icdf[s-1]))
		e.rng = IMUL32(r, uint32(icdf[s-1])-uint32(icdf[s]))
	} else {
		e.rng -= IMUL32(r, uint32(icdf[s]))
	}
	e.encNormalize()
}

// EncIcdf16 encodes a symbol given an "inverse" CDF table with 16-bit entries.
//
// C: void ec_enc_icdf16(ec_enc *_this,int _s,const opus_uint16 *_icdf,unsigned _ftb)
func (e *EcEnc) EncIcdf16(s int, icdf []uint16, ftb uint) {
	var r uint32
	r = e.rng >> ftb
	if s > 0 {
		e.val += e.rng - IMUL32(r, uint32(icdf[s-1]))
		e.rng = IMUL32(r, uint32(icdf[s-1])-uint32(icdf[s]))
	} else {
		e.rng -= IMUL32(r, uint32(icdf[s]))
	}
	e.encNormalize()
}

// C: void ec_enc_uint(ec_enc *_this,opus_uint32 _fl,opus_uint32 _ft)
func (e *EcEnc) EncUint(fl, ft uint32) {
	var ft2, fl2 uint32
	var ftb int
	// In order to optimize EC_ILOG(), it is best to have a value where the
	// value of _ft is a power of two.
	celt_assert(ft > 1)
	ft--
	ftb = EC_ILOG(ft)
	if ftb > EC_UINT_BITS {
		ftb -= EC_UINT_BITS
		ft2 = (ft >> uint(ftb)) + 1
		fl2 = fl >> uint(ftb)
		e.Encode(fl2, fl2+1, ft2)
		e.EncBits(fl&((uint32(1)<<uint(ftb))-1), uint(ftb))
	} else {
		e.Encode(fl, fl+1, ft+1)
	}
}

// C: void ec_enc_bits(ec_enc *_this,opus_uint32 _fl,unsigned _bits)
func (e *EcEnc) EncBits(fl uint32, bits uint) {
	var window EcWindow
	var used int
	window = e.end_window
	used = e.nend_bits
	celt_assert(bits > 0)
	if used+int(bits) > EC_WINDOW_SIZE {
		for {
			e.error |= e.writeByteAtEnd(uint32(window) & EC_SYM_MAX)
			window >>= EC_SYM_BITS
			used -= EC_SYM_BITS
			if used < EC_SYM_BITS {
				break
			}
		}
	}
	window |= EcWindow(fl) << uint(used)
	used += int(bits)
	e.end_window = window
	e.nend_bits = used
	e.nbits_total += int(bits)
}

// C: void ec_enc_patch_initial_bits(ec_enc *_this,unsigned _val,unsigned _nbits)
func (e *EcEnc) EncPatchInitialBits(val uint32, nbits uint) {
	var shift int
	var mask uint32
	celt_assert(nbits <= EC_SYM_BITS)
	shift = EC_SYM_BITS - int(nbits)
	mask = ((1 << nbits) - 1) << uint(shift)
	if e.offs > 0 {
		// The first byte has been finalized.
		e.buf[0] = byte((uint32(e.buf[0]) &^ mask) | val<<uint(shift))
	} else if e.rem >= 0 {
		// The first byte is still awaiting carry propagation.
		e.rem = int((uint32(e.rem) &^ mask) | val<<uint(shift))
	} else if e.rng <= (EC_CODE_TOP >> nbits) {
		// The renormalization loop has never been run.
		e.val = (e.val &^ (mask << EC_CODE_SHIFT)) | val<<uint(EC_CODE_SHIFT+shift)
	} else {
		// The encoder hasn't even encoded _nbits of data yet.
		e.error = -1
	}
}

// C: void ec_enc_shrink(ec_enc *_this,opus_uint32 _size)
func (e *EcEnc) EncShrink(size uint32) {
	celt_assert(e.offs+e.end_offs <= size)
	copy(e.buf[size-e.end_offs:size], e.buf[e.storage-e.end_offs:e.storage])
	e.storage = size
}

// C: void ec_enc_done(ec_enc *_this)
func (e *EcEnc) EncDone() {
	var window EcWindow
	var used int
	var msk, end uint32
	var l int
	// We output the minimum number of bits that ensures that the symbols encoded
	// thus far will be decoded correctly regardless of the bits that follow.
	l = EC_CODE_BITS - EC_ILOG(e.rng)
	msk = (EC_CODE_TOP - 1) >> uint(l)
	end = (e.val + msk) &^ msk
	if (end | msk) >= e.val+e.rng {
		l++
		msk >>= 1
		end = (e.val + msk) &^ msk
	}
	for l > 0 {
		e.encCarryOut(int(end >> EC_CODE_SHIFT))
		end = (end << EC_SYM_BITS) & (EC_CODE_TOP - 1)
		l -= EC_SYM_BITS
	}
	// If we have a buffered byte flush it into the output buffer.
	if e.rem >= 0 || e.ext > 0 {
		e.encCarryOut(0)
	}
	// If we have buffered extra bits, flush them as well.
	window = e.end_window
	used = e.nend_bits
	for used >= EC_SYM_BITS {
		e.error |= e.writeByteAtEnd(uint32(window) & EC_SYM_MAX)
		window >>= EC_SYM_BITS
		used -= EC_SYM_BITS
	}
	// Clear any excess space and add any remaining extra bits to the last byte.
	if e.error == 0 {
		if e.buf != nil {
			clear(e.buf[e.offs : e.storage-e.end_offs])
		}
		if used > 0 {
			// If there's no range coder data at all, give up.
			if e.end_offs >= e.storage {
				e.error = -1
			} else {
				l = -l
				// If we've busted, don't add too many extra bits to the last byte; it
				// would corrupt the range coder data, and that's more important.
				if e.offs+e.end_offs >= e.storage && l < used {
					window &= (1 << uint(l)) - 1
					e.error = -1
				}
				e.buf[e.storage-e.end_offs-1] |= byte(window)
			}
		}
	}
}
