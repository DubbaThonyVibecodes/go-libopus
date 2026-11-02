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
//	const opus_val16 *window, int overlap, int shift, int stride, int arch)
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
			f[yp] = MULT16_32_Q15(window[wp2], in[xp1+N2]) + MULT16_32_Q15(window[wp1], in[xp2])
			yp++
			f[yp] = MULT16_32_Q15(window[wp1], in[xp1]) - MULT16_32_Q15(window[wp2], in[xp2-N2])
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
			f[yp] = -MULT16_32_Q15(window[wp1], in[xp1-N2]) + MULT16_32_Q15(window[wp2], in[xp2])
			yp++
			f[yp] = MULT16_32_Q15(window[wp2], in[xp1]) + MULT16_32_Q15(window[wp1], in[xp2+N2])
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
			yc.R = yr
			yc.I = yi
			yc.R = PSHR32(MULT16_32_Q16(scale, yc.R), scale_shift)
			yc.I = PSHR32(MULT16_32_Q16(scale, yc.I), scale_shift)
			f2[st.bitrev[i]] = yc
		}
	}

	// N/4 complex FFT, does not downscale anymore
	FftImpl(st, f2)

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
			yr = S_MUL(f2[fp].I, t[N4+i]) - S_MUL(f2[fp].R, t[i])
			yi = S_MUL(f2[fp].R, t[N4+i]) + S_MUL(f2[fp].I, t[i])
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
//	const opus_val16 * OPUS_RESTRICT window, int overlap, int shift, int stride, int arch)
//
// C: #define clt_mdct_backward(_l, _in, _out, _window, _overlap, _shift, _stride, _arch) clt_mdct_backward_c(...)
func MdctBackward(l *mdct_lookup, in []int32, out []int32, window []int16, overlap, shift, stride int, arch int) {
	var i int
	var N, N2, N4 int
	var trig []int16
	_ = arch

	N = l.n
	trig = l.trig
	for i = 0; i < shift; i++ {
		N >>= 1
		trig = trig[N:]
	}
	N2 = N >> 1
	N4 = N >> 2

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
			rev = int(bitrev[i])
			yr = ADD32_ovflw(S_MUL(in[xp2], t[i]), S_MUL(in[xp1], t[N4+i]))
			yi = SUB32_ovflw(S_MUL(in[xp1], t[i]), S_MUL(in[xp2], t[N4+i]))
			// We swap real and imag because we use an FFT instead of an IFFT.
			yp[2*rev+1] = yr
			yp[2*rev] = yi
			// Storing the pre-rotation directly in the bitrev order.
			xp1 += 2 * stride
			xp2 -= 2 * stride
		}
	}

	FftImpl(l.kfft[shift], cpxView(out[(overlap>>1):(overlap>>1)+N2]))

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
			yr = ADD32_ovflw(S_MUL(re, t0), S_MUL(im, t1))
			yi = SUB32_ovflw(S_MUL(re, t1), S_MUL(im, t0))
			// We swap real and imag because we're using an FFT instead of an IFFT.
			re = out[yp1+1]
			im = out[yp1+0]
			out[yp0+0] = yr
			out[yp1+1] = yi

			t0 = t[(N4 - i - 1)]
			t1 = t[(N2 - i - 1)]
			// We'd scale up by 2 here, but instead it's done when mixing the windows
			yr = ADD32_ovflw(S_MUL(re, t0), S_MUL(im, t1))
			yi = SUB32_ovflw(S_MUL(re, t1), S_MUL(im, t0))
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
			out[yp1] = SUB32_ovflw(MULT16_32_Q15(window[wp2], x2), MULT16_32_Q15(window[wp1], x1))
			yp1++
			out[xp1] = ADD32_ovflw(MULT16_32_Q15(window[wp1], x2), MULT16_32_Q15(window[wp2], x1))
			xp1--
			wp1++
			wp2--
		}
	}
}
