package celt

// C: celt/cwrs.h, celt/cwrs.c — pulse vector (PVQ) enumeration.
// The CUSTOM_MODES-only helpers (log2_frac, get_required_bits) are not ported
// since the reference build has no custom modes; the pulse caches come from
// static_modes_fixed.go.

const mask32 = 0xFFFFFFFF

// INV_TABLE[i] holds the multiplicative inverse of (2*i+1) mod 2**32.
var inv_table = [53]uint32{
	0x00000001, 0xAAAAAAAB, 0xCCCCCCCD, 0xB6DB6DB7,
	0x38E38E39, 0xBA2E8BA3, 0xC4EC4EC5, 0xEEEEEEEF,
	0xF0F0F0F1, 0x286BCA1B, 0x3CF3CF3D, 0xE9BD37A7,
	0xC28F5C29, 0x684BDA13, 0x4F72C235, 0xBDEF7BDF,
	0x3E0F83E1, 0x8AF8AF8B, 0x914C1BAD, 0x96F96F97,
	0xC18F9C19, 0x2FA0BE83, 0xA4FA4FA5, 0x677D46CF,
	0x1A1F58D1, 0xFAFAFAFB, 0x8C13521D, 0x586FB587,
	0xB823EE09, 0xA08AD8F3, 0xC10C9715, 0xBEFBEFBF,
	0xC0FC0FC1, 0x07A44C6B, 0xA33F128D, 0xE327A977,
	0xC7E3F1F9, 0x962FC963, 0x3F2B3885, 0x613716AF,
	0x781948B1, 0x2B2E43DB, 0xFCFCFCFD, 0x6FD0EB67,
	0xFA3F47E9, 0xD2FD2FD3, 0x3F4FD3F5, 0xD4E25B9F,
	0x5F02A3A1, 0xBF5A814B, 0x7C32B16D, 0xD3431B57,
	0xD8FD8FD9,
}

// Computes (_a*_b-_c)/(2*_d+1) when the quotient is known to be exact.
//
// C: static inline opus_uint32 imusdiv32odd(opus_uint32 _a,opus_uint32 _b,opus_uint32 _c,int _d)
func imusdiv32odd(a, b, c uint32, d int) uint32 {
	celt_assert(d <= 52)
	return (a*b - c) * inv_table[d] & mask32
}

// Computes (_a*_b-_c)/_d when the quotient is known to be exact.
//
// C: static inline opus_uint32 imusdiv32even(opus_uint32 _a,opus_uint32 _b,opus_uint32 _c,int _d)
func imusdiv32even(a, b, c uint32, d int) uint32 {
	var inv uint32
	var mask, shift, one int
	celt_assert(d > 0)
	celt_assert(d <= 54)
	shift = EC_ILOG(uint32(d ^ (d - 1)))
	inv = inv_table[(d-1)>>uint(shift)]
	shift--
	one = 1 << uint(shift)
	mask = one - 1
	return (a*(b>>uint(shift)) - (c >> uint(shift)) +
		((a*(b&uint32(mask)) + uint32(one) - (c & uint32(mask))) >> uint(shift)) - 1) * inv & mask32
}

// Compute U(2,_k). Note that this may be called with _k=32768 (maxK[2]+1).
//
// C: static inline unsigned ucwrs2(unsigned _k)
func ucwrs2(k uint32) uint32 {
	celt_assert(k > 0)
	return k + (k - 1)
}

// Compute V(2,_k).
//
// C: static inline opus_uint32 ncwrs2(int _k)
func ncwrs2(k int) uint32 {
	celt_assert(k > 0)
	return 4 * uint32(k)
}

// Compute U(3,_k). Note that this may be called with _k=32768 (maxK[3]+1).
//
// C: static inline opus_uint32 ucwrs3(unsigned _k)
func ucwrs3(k uint32) uint32 {
	celt_assert(k > 0)
	return (2*k-2)*k + 1
}

// Compute V(3,_k).
//
// C: static inline opus_uint32 ncwrs3(int _k)
func ncwrs3(k int) uint32 {
	celt_assert(k > 0)
	return 2 * (2*uint32(k)*uint32(k) + 1)
}

// Compute U(4,_k).
//
// C: static inline opus_uint32 ucwrs4(int _k)
func ucwrs4(k int) uint32 {
	celt_assert(k > 0)
	return imusdiv32odd(uint32(2*k), uint32(2*k-3)*uint32(k)+4, 3, 1)
}

// Compute V(4,_k).
//
// C: static inline opus_uint32 ncwrs4(int _k)
func ncwrs4(k int) uint32 {
	celt_assert(k > 0)
	return ((uint32(k)*uint32(k) + 2) * uint32(k)) / 3 << 3
}

// Computes the next row/column of any recurrence that obeys the relation
// u[i][j]=u[i-1][j]+u[i][j-1]+u[i-1][j-1]. _ui0 is the base case for the new row/column.
//
// C: static inline void unext(opus_uint32 *_ui,unsigned _len,opus_uint32 _ui0)
func unext(ui []uint32, n uint32, ui0 uint32) {
	var ui1 uint32
	var j uint32
	// This do-while will overrun the array if we don't have storage for at least 2 values.
	j = 1
	for {
		ui1 = UADD32(UADD32(ui[j], ui[j-1]), ui0)
		ui[j-1] = ui0
		ui0 = ui1
		j++
		if j >= n {
			break
		}
	}
	ui[j-1] = ui0
}

// Computes the previous row/column of any recurrence that obeys the relation
// u[i-1][j]=u[i][j]-u[i][j-1]-u[i-1][j-1]. _ui0 is the base case for the new row/column.
//
// C: static inline void uprev(opus_uint32 *_ui,unsigned _n,opus_uint32 _ui0)
func uprev(ui []uint32, n uint32, ui0 uint32) {
	var ui1 uint32
	var j uint32
	j = 1
	for {
		ui1 = USUB32(USUB32(ui[j], ui[j-1]), ui0)
		ui[j-1] = ui0
		ui0 = ui1
		j++
		if j >= n {
			break
		}
	}
	ui[j-1] = ui0
}

// Compute V(_n,_k), as well as U(_n,0..._k+1).
// _u: On exit, _u[i] contains U(_n,i) for i in [0..._k+1].
//
// C: static opus_uint32 ncwrs_urow(unsigned _n,unsigned _k,opus_uint32 *_u)
func ncwrs_urow(n, k uint32, u []uint32) uint32 {
	var um2 uint32
	var length, kk uint32
	length = k + 2
	// We require storage at least 3 values (e.g., _k>0).
	celt_assert(length >= 3)
	u[0] = 0
	um2 = 1
	u[1] = um2
	// _k>52 doesn't work in the false branch due to the limits of INV_TABLE,
	// but _k isn't tested here because k<=52 for n=7
	if n <= 6 {
		// If _n==0, _u[0] should be 1 and the rest should be 0.
		// If _n==1, _u[i] should be 1 for i>1.
		celt_assert(n >= 2)
		// If _k==0, the following do-while loop will overflow the buffer.
		celt_assert(k > 0)
		kk = 2
		for {
			u[kk] = (kk << 1) - 1
			kk++
			if kk >= length {
				break
			}
		}
		for kk = 2; kk < n; kk++ {
			unext(u[1:], k+1, 1)
		}
	} else {
		var um1, n2m1 uint32
		n2m1 = (n << 1) - 1
		um1 = n2m1
		u[2] = um1
		for kk = 3; kk < length; kk++ {
			// U(N,K) = ((2*N-1)*U(N,K-1)-U(N,K-2))/(K-1) + U(N,K-2)
			um2 = imusdiv32even(n2m1, um1, um2, int(kk-1)) + um2
			u[kk] = um2
			kk++
			if kk >= length {
				break
			}
			um1 = imusdiv32odd(n2m1, um2, um1, int(kk-1)>>1) + um1
			u[kk] = um1
		}
	}
	return u[k] + u[k+1]
}

// Returns the _i'th combination of _k elements (at most 32767) chosen from a
// set of size 1 with associated sign bits. _y: Returns the vector of pulses.
//
// C: static inline void cwrsi1(int _k,opus_uint32 _i,int *_y)
func cwrsi1(k int, i uint32, y []int) {
	var s int
	s = -int(i)
	y[0] = (k + s) ^ s
}

// C: static inline void cwrsi2(int _k,opus_uint32 _i,int *_y)
func cwrsi2(k int, i uint32, y []int) {
	var p uint32
	var s, yj int
	p = ucwrs2(uint32(k) + 1)
	if i >= p {
		s = -1
	}
	i -= p & uint32(s)
	yj = k
	k = int((i + 1) >> 1)
	if k != 0 {
		p = ucwrs2(uint32(k))
	} else {
		p = 0
	}
	i -= p
	yj -= k
	y[0] = (yj + s) ^ s
	cwrsi1(k, i, y[1:])
}

// C: static void cwrsi3(int _k,opus_uint32 _i,int *_y)
func cwrsi3(k int, i uint32, y []int) {
	var p uint32
	var s, yj int
	p = ucwrs3(uint32(k) + 1)
	if i >= p {
		s = -1
	}
	i -= p & uint32(s)
	yj = k
	// Finds the maximum _k such that ucwrs3(_k)<=_i (tested for all
	// _i<2147418113=U(3,32768)).
	if i > 0 {
		k = int((Isqrt32(2*i-1) + 1) >> 1)
	} else {
		k = 0
	}
	if k != 0 {
		p = ucwrs3(uint32(k))
	} else {
		p = 0
	}
	i -= p
	yj -= k
	y[0] = (yj + s) ^ s
	cwrsi2(k, i, y[1:])
}

// C: static void cwrsi4(int _k,opus_uint32 _i,int *_y)
func cwrsi4(k int, i uint32, y []int) {
	var p uint32
	var s, yj, kl, kr int
	p = ucwrs4(k + 1)
	if i >= p {
		s = -1
	}
	i -= p & uint32(s)
	yj = k
	// We could solve a cubic for k here, but the form of the direct solution does
	// not lend itself well to exact integer arithmetic.
	// Instead we do a binary search on U(4,K).
	kl = 0
	kr = k
	for {
		k = (kl + kr) >> 1
		if k != 0 {
			p = ucwrs4(k)
		} else {
			p = 0
		}
		if p < i {
			if k >= kr {
				break
			}
			kl = k + 1
		} else if p > i {
			kr = k - 1
		} else {
			break
		}
	}
	i -= p
	yj -= k
	y[0] = (yj + s) ^ s
	cwrsi3(k, i, y[1:])
}

// Returns the _i'th combination of _k elements chosen from a set of size _n
// with associated sign bits.
// _y: Returns the vector of pulses.
// _u: Must contain entries [0..._k+1] of row _n of U() on input.
// Its contents will be destructively modified.
//
// C: static void cwrsi(int _n,int _k,opus_uint32 _i,int *_y,opus_uint32 *_u)
func cwrsi(n, k int, i uint32, y []int, u []uint32) {
	var j int
	celt_assert(n > 0)
	j = 0
	for {
		var p uint32
		var s, yj int
		p = u[k+1]
		if i >= p {
			s = -1
		} else {
			s = 0
		}
		i -= p & uint32(s)
		yj = k
		p = u[k]
		for p > i {
			k--
			p = u[k]
		}
		i -= p
		yj -= k
		y[j] = (yj + s) ^ s
		uprev(u, uint32(k+2), 0)
		j++
		if j >= n {
			break
		}
	}
}

// Returns the index of the given combination of K elements chosen from a set
// of size 1 with associated sign bits.
// _y: The vector of pulses, whose sum of absolute values is K. _k: Returns K.
//
// C: static inline opus_uint32 icwrs1(const int *_y,int *_k)
func icwrs1(y []int, k *int) uint32 {
	*k = ABS(y[0])
	if y[0] < 0 {
		return 1
	}
	return 0
}

// C: static inline opus_uint32 icwrs2(const int *_y,int *_k)
func icwrs2(y []int, k *int) uint32 {
	var i uint32
	var kk int
	i = icwrs1(y[1:], &kk)
	if kk != 0 {
		i += ucwrs2(uint32(kk))
	}
	kk += ABS(y[0])
	if y[0] < 0 {
		i += ucwrs2(uint32(kk) + 1)
	}
	*k = kk
	return i
}

// C: static inline opus_uint32 icwrs3(const int *_y,int *_k)
func icwrs3(y []int, k *int) uint32 {
	var i uint32
	var kk int
	i = icwrs2(y[1:], &kk)
	if kk != 0 {
		i += ucwrs3(uint32(kk))
	}
	kk += ABS(y[0])
	if y[0] < 0 {
		i += ucwrs3(uint32(kk) + 1)
	}
	*k = kk
	return i
}

// C: static inline opus_uint32 icwrs4(const int *_y,int *_k)
func icwrs4(y []int, k *int) uint32 {
	var i uint32
	var kk int
	i = icwrs3(y[1:], &kk)
	if kk != 0 {
		i += ucwrs4(kk)
	}
	kk += ABS(y[0])
	if y[0] < 0 {
		i += ucwrs4(kk + 1)
	}
	*k = kk
	return i
}

// Returns the index of the given combination of K elements chosen from a set
// of size _n with associated sign bits.
// _y:  The vector of pulses, whose sum of absolute values must be _k.
// _nc: Returns V(_n,_k).
//
// C: static inline opus_uint32 icwrs(int _n,int _k,opus_uint32 *_nc,const int *_y,opus_uint32 *_u)
func icwrs(n, k int, nc *uint32, y []int, u []uint32) uint32 {
	var i uint32
	var j, kk int
	// We can't unroll the first two iterations of the loop unless _n>=2.
	celt_assert(n >= 2)
	u[0] = 0
	for kk = 1; kk <= k+1; kk++ {
		u[kk] = uint32(kk<<1) - 1
	}
	i = icwrs1(y[n-1:], &kk)
	j = n - 2
	i += u[kk]
	kk += ABS(y[j])
	if y[j] < 0 {
		i += u[kk+1]
	}
	for j > 0 {
		j--
		unext(u, uint32(k+2), 0)
		i += u[kk]
		kk += ABS(y[j])
		if y[j] < 0 {
			i += u[kk+1]
		}
	}
	*nc = u[kk] + u[kk+1]
	return i
}

// C: void encode_pulses(const int *_y,int _n,int _k,ec_enc *_enc)
func EncodePulses(y []int, n, k int, enc *EcEnc) {
	var i uint32
	celt_assert(k > 0)
	switch n {
	case 2:
		i = icwrs2(y, &k)
		enc.EncUint(i, ncwrs2(k))
	case 3:
		i = icwrs3(y, &k)
		enc.EncUint(i, ncwrs3(k))
	case 4:
		i = icwrs4(y, &k)
		enc.EncUint(i, ncwrs4(k))
	default:
		var nc uint32
		u := make([]uint32, k+2)
		i = icwrs(n, k, &nc, y, u)
		enc.EncUint(i, nc)
	}
}

// C: void decode_pulses(int *_y,int _n,int _k,ec_dec *_dec)
func DecodePulses(y []int, n, k int, dec *EcDec) {
	celt_assert(k > 0)
	switch n {
	case 2:
		cwrsi2(k, dec.DecUint(ncwrs2(k)), y)
	case 3:
		cwrsi3(k, dec.DecUint(ncwrs3(k)), y)
	case 4:
		cwrsi4(k, dec.DecUint(ncwrs4(k)), y)
	default:
		u := make([]uint32, k+2)
		cwrsi(n, k, dec.DecUint(ncwrs_urow(uint32(n), uint32(k), u)), y, u)
	}
}
