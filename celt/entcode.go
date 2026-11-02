package celt

import "math/bits"

// C: celt/entcode.h, celt/entcode.c, celt/ecintrin.h, celt/mfrngcod.h

// EcWindow is the C ec_window type.
type EcWindow = uint32

const (
	EC_WINDOW_SIZE = 32
	EC_UINT_BITS   = 8
	// BITRES is the number of fractional bits of precision in ec_tell_frac: 3 => 1/8th bits.
	BITRES = 3
)

// mfrngcod.h
const (
	EC_SYM_BITS   = 8
	EC_CODE_BITS  = 32
	EC_SYM_MAX    = uint32(1<<EC_SYM_BITS) - 1
	EC_CODE_SHIFT = EC_CODE_BITS - EC_SYM_BITS - 1
	EC_CODE_TOP   = uint32(1) << (EC_CODE_BITS - 1)
	EC_CODE_BOT   = EC_CODE_TOP >> EC_SYM_BITS
	EC_CODE_EXTRA = (EC_CODE_BITS-2)%EC_SYM_BITS + 1
)

// C: EC_MINI(_a,_b) ((_a)+(((_b)-(_a))&-((_b)<(_a))))
func EC_MINI(a, b uint32) uint32 {
	if b < a {
		return b
	}
	return a
}

// C: EC_ILOG(_x) — position of the highest set bit plus one; EC_ILOG(0)==0.
func EC_ILOG(x uint32) int { return bits.Len32(x) }

// C: int ec_ilog(opus_uint32 _v)
func Ilog(v uint32) int { return bits.Len32(v) }

// EcCtx is the C struct ec_ctx; the same structure is used for both the
// encoder (EcEnc) and the decoder (EcDec), so common functions like Tell
// can be used on either one.
type EcCtx struct {
	buf        []byte
	storage    uint32
	end_offs   uint32
	end_window EcWindow
	nend_bits  int
	// The total number of whole bits read/written. This does not include
	// partial bits currently in the range coder.
	nbits_total int
	offs        uint32
	rng         uint32
	// In the decoder: the difference between the top of the current range and
	// the input value, minus one. In the encoder: the low end of the current range.
	val uint32
	// In the decoder: the saved normalization factor from ec_decode().
	// In the encoder: the number of outstanding carry propagating symbols.
	ext   uint32
	rem   int
	error int
}

type EcEnc = EcCtx
type EcDec = EcCtx

// C: static inline opus_uint32 ec_range_bytes(ec_ctx *_this)
func (e *EcCtx) RangeBytes() uint32 { return e.offs }

// C: static inline unsigned char *ec_get_buffer(ec_ctx *_this)
func (e *EcCtx) GetBuffer() []byte { return e.buf }

// C: static inline int ec_get_error(ec_ctx *_this)
func (e *EcCtx) GetError() int { return e.error }

// Rng returns the current range (the C code reads _this->rng directly, e.g.
// for OPUS_GET_FINAL_RANGE).
func (e *EcCtx) Rng() uint32 { return e.rng }

// Tell returns the number of bits "used" by the encoded or decoded symbols so far.
// This same number can be computed in either the encoder or the decoder, and is
// suitable for making coding decisions. This will always be slightly larger
// than the exact value (e.g., all rounding error is in the positive direction).
//
// C: static inline int ec_tell(ec_ctx *_this)
func (e *EcCtx) Tell() int { return e.nbits_total - EC_ILOG(e.rng) }

// TellFrac returns the number of bits scaled by 2**BITRES.
// This is a faster version of ec_tell_frac() that takes advantage
// of the low (1/8 bit) resolution to use just a linear function
// followed by a lookup to determine the exact transition thresholds.
//
// C: opus_uint32 ec_tell_frac(ec_ctx *_this)
func (e *EcCtx) TellFrac() uint32 {
	var correction = [8]uint32{35733, 38967, 42495, 46340,
		50535, 55109, 60097, 65535}
	var nbits uint32
	var r uint32
	var l int
	var b uint32
	nbits = uint32(e.nbits_total) << BITRES
	l = EC_ILOG(e.rng)
	r = e.rng >> uint(l-16)
	b = (r >> 12) - 8
	if r > correction[b] {
		b++
	}
	l = (l << 3) + int(b)
	return nbits - uint32(l)
}

// Tested exhaustively for all n and for 1<=d<=256
//
// C: static OPUS_INLINE opus_uint32 celt_udiv(opus_uint32 n, opus_uint32 d)
func Udiv(n, d uint32) uint32 {
	celt_sig_assert(d > 0)
	return n / d
}

// C: static OPUS_INLINE opus_int32 celt_sudiv(opus_int32 n, opus_int32 d)
func Sudiv(n, d int32) int32 {
	celt_sig_assert(d > 0)
	return n / d
}

// Storage returns the buffer size (the C code reads _this->storage directly,
// e.g. the Opus decoder for redundancy handling).
func (e *EcCtx) Storage() uint32 { return e.storage }

// SetStorage sets the buffer size (C: dec.storage -= redundancy_bytes).
func (e *EcCtx) SetStorage(v uint32) { e.storage = v }

// NbitsTotal returns nbits_total (the C code adjusts it directly).
func (e *EcCtx) NbitsTotal() int { return e.nbits_total }

// SetNbitsTotal sets nbits_total.
func (e *EcCtx) SetNbitsTotal(v int) { e.nbits_total = v }
