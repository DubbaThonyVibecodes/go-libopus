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
//
// C: opus_uint32 ec_tell_frac(ec_ctx *_this)
func (e *EcCtx) TellFrac() uint32 {
	var nbits uint32
	var r uint32
	var l int
	var i int
	// To handle the non-integral number of bits still left in the encoder/decoder
	// state, we compute the worst-case number of bits of val that must be
	// encoded to ensure that the value is inside the range for any possible
	// subsequent bits. (See upstream for the full discussion.)
	nbits = uint32(e.nbits_total) << BITRES
	l = EC_ILOG(e.rng)
	r = e.rng >> uint(l-16)
	for i = BITRES; i > 0; i-- {
		var b int
		r = r * r >> 15
		b = int(r >> 16)
		l = l<<1 | b
		r >>= uint(b)
	}
	return nbits - uint32(l)
}
