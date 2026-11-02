package celt

import "unsafe"

// C: celt/mdct.h, celt/mdct.c
//
// This is a simple MDCT implementation that uses a N/4 complex FFT to do
// most of the work. The algorithm is similar to (and inspired from) Fabrice
// Bellard's MDCT implementation in FFMPEG, but has differences in signs,
// ordering and scaling in many places.
//
// The C code reinterprets scalar buffers as kiss_fft_cpx arrays with casts;
// cpxView does the same without copying (two consecutive int32 == one
// KissFftCpx, which has no padding).
//
// clt_mdct_init / clt_mdct_clear are CUSTOM_MODES only and not ported.

type mdct_lookup struct {
	n        int
	maxshift int
	kfft     [4]*kiss_fft_state
	trig     []int16
}

// cpxView aliases a scalar buffer as complex pairs, like the C cast
// (kiss_fft_cpx *)f.
func cpxView(f []int32) []KissFftCpx {
	return unsafe.Slice((*KissFftCpx)(unsafe.Pointer(&f[0])), len(f)/2)
}

// MdctForward computes a forward MDCT and scales by 4/N; trashes the input array.
//
// C: void clt_mdct_forward_c(const mdct_lookup *l, kiss_fft_scalar *in, kiss_fft_scalar * OPUS_RESTRICT out,
//
//	const celt_coef *window, int overlap, int shift, int stride, int arch)
//
// C: #define clt_mdct_forward(_l, _in, _out, _window, _overlap, _shift, _stride, _arch) clt_mdct_forward_c(...)
func MdctForward(l *mdct_lookup, in, out []int32, window []int16, overlap, shift, stride int, arch int) {
	var i int
	var N, N2, N4 int
	var st *kiss_fft_state = l.kfft[shift]
	var trig []int16
	var scale int16
	// Allows us to scale with MULT16_32_Q16(), which is faster than
	// MULT16_32_Q15() on ARM.
	var scale_shift int = st.scale_shift - 1
	var headroom int
	_ = arch
	scale = st.scale

	N = l.n
	trig = l.trig
	for i = 0; i < shift; i++ {
		N >>= 1
		trig = trig[N:]
	}
	N2 = N >> 1
	N4 = N >> 2

	f := make([]int32, N2)
	f2 := make([]KissFftCpx, N4)

	// Consider the input to be composed of four blocks: [a, b, c, d]
	// Window, shuffle, fold
	{
		// Temp pointers to make it really clear to the compiler what we're doing
		xp1 := overlap >> 1
		xp2 := N2 - 1 + (overlap >> 1)
		yp := 0
		wp1 := overlap >> 1
		wp2 := (overlap >> 1) - 1
		for i = 0; i < ((overlap + 3) >> 2); i++ {
			// Real part arranged as -d-cR, Imag part arranged as -b+aR
			f[yp] = S_MUL(in[xp1+N2], window[wp2]) + S_MUL(in[xp2], window[wp1])
			yp++
			f[yp] = S_MUL(in[xp1], window[wp1]) - S_MUL(in[xp2-N2], window[wp2])
			yp++
			xp1 += 2
			xp2 -= 2
			wp1 += 2
			wp2 -= 2
		}
		wp1 = 0
		wp2 = overlap - 1
		for ; i < N4-((overlap+3)>>2); i++ {
			// Real part arranged as a-bR, Imag part arranged as -c-dR
			f[yp] = in[xp2]
			yp++
			f[yp] = in[xp1]
			yp++
			xp1 += 2
			xp2 -= 2
		}
		for ; i < N4; i++ {
			// Real part arranged as a-bR, Imag part arranged as -c-dR
			f[yp] = -S_MUL(in[xp1-N2], window[wp1]) + S_MUL(in[xp2], window[wp2])
			yp++
			f[yp] = S_MUL(in[xp1], window[wp2]) + S_MUL(in[xp2+N2], window[wp1])
			yp++
			xp1 += 2
			xp2 -= 2
			wp1 += 2
			wp2 -= 2
		}
	}
	// Pre-rotation
	{
		yp := 0
		t := trig
		var maxval int32 = 1
		for i = 0; i < N4; i++ {
			var yc KissFftCpx
			var t0, t1 int16
			var re, im, yr, yi int32
			t0 = t[i]
			t1 = t[N4+i]
			re = f[yp]
			yp++
			im = f[yp]
			yp++
			yr = S_MUL(re, t0) - S_MUL(im, t1)
			yi = S_MUL(im, t0) + S_MUL(re, t1)
			// For QEXT, it's best to scale before the FFT, but otherwise it's best to scale after.
			// For floating-point it doesn't matter.
			yc.R = S_MUL2(yr, scale)
			yc.I = S_MUL2(yi, scale)
			maxval = MAX32(maxval, MAX32(ABS32(yc.R), ABS32(yc.I)))
			f2[st.bitrev[i]] = yc
		}
		headroom = IMAX(0, IMIN(scale_shift, 28-int(Ilog2(maxval))))
	}

	// N/4 complex FFT, does not downscale anymore
	FftImpl(st, f2, scale_shift-headroom)

	// Post-rotate
	{
		// Temp pointers to make it really clear to the compiler what we're doing
		fp := 0
		yp1 := 0
		yp2 := stride * (N2 - 1)
		t := trig
		// Temp pointers to make it really clear to the compiler what we're doing
		for i = 0; i < N4; i++ {
			var yr, yi int32
			var t0, t1 int16
			t0 = t[i]
			t1 = t[N4+i]
			yr = PSHR32(S_MUL(f2[fp].I, t1)-S_MUL(f2[fp].R, t0), headroom)
			yi = PSHR32(S_MUL(f2[fp].R, t1)+S_MUL(f2[fp].I, t0), headroom)
			out[yp1] = yr
			out[yp2] = yi
			fp++
			yp1 += 2 * stride
			yp2 -= 2 * stride
		}
	}
}

// MdctBackward computes a backward MDCT (no scaling) and performs weighted
// overlap-add (scales implicitly by 1/2).
//
// C: void clt_mdct_backward_c(const mdct_lookup *l, kiss_fft_scalar *in, kiss_fft_scalar * OPUS_RESTRICT out,
//
//	const celt_coef * OPUS_RESTRICT window, int overlap, int shift, int stride, int arch)
//
// C: #define clt_mdct_backward(_l, _in, _out, _window, _overlap, _shift, _stride, _arch) clt_mdct_backward_c(...)
func MdctBackward(l *mdct_lookup, in []int32, out []int32, window []int16, overlap, shift, stride int, arch int) {
	var i int
	var N, N2, N4 int
	var trig []int16
	var pre_shift, post_shift, fft_shift int
	_ = arch

	N = l.n
	trig = l.trig
	for i = 0; i < shift; i++ {
		N >>= 1
		trig = trig[N:]
	}
	N2 = N >> 1
	N4 = N >> 2

	{
		var sumval int32 = int32(N2)
		var maxval int32 = 0
		for i = 0; i < N2; i++ {
			maxval = MAX32(maxval, ABS32(in[i*stride]))
			sumval = ADD32_ovflw(sumval, ABS32(SHR32(in[i*stride], 11)))
		}
		pre_shift = IMAX(0, 29-int(Zlog2(1+maxval)))
		// Worst-case where all the energy goes to a single sample.
		post_shift = IMAX(0, 19-int(Ilog2(ABS32(sumval))))
		post_shift = IMIN(post_shift, pre_shift)
		fft_shift = pre_shift - post_shift
	}
	// Pre-rotate
	{
		// Temp pointers to make it really clear to the compiler what we're doing
		xp1 := 0
		xp2 := stride * (N2 - 1)
		yp := out[(overlap >> 1):]
		t := trig
		bitrev := l.kfft[shift].bitrev
		for i = 0; i < N4; i++ {
			var rev int
			var yr, yi int32
			var x1, x2 int32
			rev = int(bitrev[i])
			x1 = SHL32_ovflw(in[xp1], pre_shift)
			x2 = SHL32_ovflw(in[xp2], pre_shift)
			yr = ADD32_ovflw(S_MUL(x2, t[i]), S_MUL(x1, t[N4+i]))
			yi = SUB32_ovflw(S_MUL(x1, t[i]), S_MUL(x2, t[N4+i]))
			// We swap real and imag because we use an FFT instead of an IFFT.
			yp[2*rev+1] = yr
			yp[2*rev] = yi
			// Storing the pre-rotation directly in the bitrev order.
			xp1 += 2 * stride
			xp2 -= 2 * stride
		}
	}

	FftImpl(l.kfft[shift], cpxView(out[(overlap>>1):(overlap>>1)+N2]), fft_shift)

	// Post-rotate and de-shuffle from both ends of the buffer at once to make
	// it in-place.
	{
		yp0 := (overlap >> 1)
		yp1 := (overlap >> 1) + N2 - 2
		t := trig
		// Loop to (N4+1)>>1 to handle odd N4. When N4 is odd, the
		// middle pair will be computed twice.
		for i = 0; i < (N4+1)>>1; i++ {
			var re, im, yr, yi int32
			var t0, t1 int16
			// We swap real and imag because we're using an FFT instead of an IFFT.
			re = out[yp0+1]
			im = out[yp0+0]
			t0 = t[i]
			t1 = t[N4+i]
			// We'd scale up by 2 here, but instead it's done when mixing the windows
			yr = PSHR32_ovflw(ADD32_ovflw(S_MUL(re, t0), S_MUL(im, t1)), post_shift)
			yi = PSHR32_ovflw(SUB32_ovflw(S_MUL(re, t1), S_MUL(im, t0)), post_shift)
			// We swap real and imag because we're using an FFT instead of an IFFT.
			re = out[yp1+1]
			im = out[yp1+0]
			out[yp0+0] = yr
			out[yp1+1] = yi

			t0 = t[(N4 - i - 1)]
			t1 = t[(N2 - i - 1)]
			// We'd scale up by 2 here, but instead it's done when mixing the windows
			yr = PSHR32_ovflw(ADD32_ovflw(S_MUL(re, t0), S_MUL(im, t1)), post_shift)
			yi = PSHR32_ovflw(SUB32_ovflw(S_MUL(re, t1), S_MUL(im, t0)), post_shift)
			out[yp1+0] = yr
			out[yp0+1] = yi
			yp0 += 2
			yp1 -= 2
		}
	}

	// Mirror on both sides for TDAC
	{
		xp1 := overlap - 1
		yp1 := 0
		wp1 := 0
		wp2 := overlap - 1

		for i = 0; i < overlap/2; i++ {
			var x1, x2 int32
			x1 = out[xp1]
			x2 = out[yp1]
			out[yp1] = SUB32_ovflw(S_MUL(x2, window[wp2]), S_MUL(x1, window[wp1]))
			yp1++
			out[xp1] = ADD32_ovflw(S_MUL(x2, window[wp1]), S_MUL(x1, window[wp2]))
			xp1--
			wp1++
			wp2--
		}
	}
}
