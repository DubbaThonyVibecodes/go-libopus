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
// kiss_fft_cpx, which has no padding).

type mdct_lookup struct {
	n        int
	maxshift int
	kfft     [4]*kiss_fft_state
	trig     []int16
}

// cpxView aliases a scalar buffer as complex pairs, like the C cast
// (kiss_fft_cpx *)f.
func cpxView(f []int32) []kiss_fft_cpx {
	return unsafe.Slice((*kiss_fft_cpx)(unsafe.Pointer(&f[0])), len(f)/2)
}

// MdctForward computes a forward MDCT and scales by 4/N; trashes the input array.
//
// C: void clt_mdct_forward(const mdct_lookup *l, kiss_fft_scalar *in, kiss_fft_scalar * OPUS_RESTRICT out,
//
//	const opus_val16 *window, int overlap, int shift, int stride)
func MdctForward(l *mdct_lookup, in, out []int32, window []int16, overlap, shift, stride int) {
	var i int
	var N, N2, N4 int
	var sine int16
	N = l.n
	N >>= uint(shift)
	N2 = N >> 1
	N4 = N >> 2
	f := make([]int32, N2)
	// sin(x) ~= x here
	sine = int16(TRIG_UPSCALE * (int32(QCONST16(0.7853981, 15)) + int32(N2)) / int32(N))

	// Consider the input to be composed of four blocks: [a, b, c, d]
	// Window, shuffle, fold
	{
		// Temp pointers to make it really clear to the compiler what we're doing
		xp1 := overlap >> 1
		xp2 := N2 - 1 + (overlap >> 1)
		yp := 0
		wp1 := overlap >> 1
		wp2 := (overlap >> 1) - 1
		for i = 0; i < (overlap >> 2); i++ {
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
		for ; i < N4-(overlap>>2); i++ {
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
		t := l.trig
		for i = 0; i < N4; i++ {
			var re, im, yr, yi int32
			re = f[yp]
			im = f[yp+1]
			yr = -S_MUL(re, t[i<<uint(shift)]) - S_MUL(im, t[(N4-i)<<uint(shift)])
			yi = -S_MUL(im, t[i<<uint(shift)]) + S_MUL(re, t[(N4-i)<<uint(shift)])
			// works because the cos is nearly one
			f[yp] = yr + S_MUL(yi, sine)
			yp++
			f[yp] = yi - S_MUL(yr, sine)
			yp++
		}
	}

	// N/4 complex FFT, down-scales by 4/N
	Fft(l.kfft[shift], cpxView(f), cpxView(in[:N2]))

	// Post-rotate
	{
		// Temp pointers to make it really clear to the compiler what we're doing
		fp := 0
		yp1 := 0
		yp2 := stride * (N2 - 1)
		t := l.trig
		for i = 0; i < N4; i++ {
			var yr, yi int32
			yr = S_MUL(in[fp+1], t[(N4-i)<<uint(shift)]) + S_MUL(in[fp], t[i<<uint(shift)])
			yi = S_MUL(in[fp], t[(N4-i)<<uint(shift)]) - S_MUL(in[fp+1], t[i<<uint(shift)])
			// works because the cos is nearly one
			out[yp1] = yr - S_MUL(yi, sine)
			out[yp2] = yi + S_MUL(yr, sine)
			fp += 2
			yp1 += 2 * stride
			yp2 -= 2 * stride
		}
	}
}

// MdctBackward computes a backward MDCT (no scaling) and performs weighted
// overlap-add (scales implicitly by 1/2). outOff is the index in out the C
// `out` pointer refers to; the C code writes from out-(N2-overlap)/2 onwards,
// so callers pass a slice that includes that headroom.
//
// C: void clt_mdct_backward(const mdct_lookup *l, kiss_fft_scalar *in, kiss_fft_scalar * OPUS_RESTRICT out,
//
//	const opus_val16 * OPUS_RESTRICT window, int overlap, int shift, int stride)
func MdctBackward(l *mdct_lookup, in []int32, out []int32, outOff int, window []int16, overlap, shift, stride int) {
	var i int
	var N, N2, N4 int
	var sine int16
	N = l.n
	N >>= uint(shift)
	N2 = N >> 1
	N4 = N >> 2
	f := make([]int32, N2)
	f2 := make([]int32, N2)
	// sin(x) ~= x here
	sine = int16(TRIG_UPSCALE * (int32(QCONST16(0.7853981, 15)) + int32(N2)) / int32(N))

	// Pre-rotate
	{
		// Temp pointers to make it really clear to the compiler what we're doing
		xp1 := 0
		xp2 := stride * (N2 - 1)
		yp := 0
		t := l.trig
		for i = 0; i < N4; i++ {
			var yr, yi int32
			yr = -S_MUL(in[xp2], t[i<<uint(shift)]) + S_MUL(in[xp1], t[(N4-i)<<uint(shift)])
			yi = -S_MUL(in[xp2], t[(N4-i)<<uint(shift)]) - S_MUL(in[xp1], t[i<<uint(shift)])
			// works because the cos is nearly one
			f2[yp] = yr - S_MUL(yi, sine)
			yp++
			f2[yp] = yi + S_MUL(yr, sine)
			yp++
			xp1 += 2 * stride
			xp2 -= 2 * stride
		}
	}

	// Inverse N/4 complex FFT. This one should *not* downscale even in fixed-point
	Ifft(l.kfft[shift], cpxView(f2), cpxView(f))

	// Post-rotate
	{
		fp := 0
		t := l.trig

		for i = 0; i < N4; i++ {
			var re, im, yr, yi int32
			re = f[fp]
			im = f[fp+1]
			// We'd scale up by 2 here, but instead it's done when mixing the windows
			yr = S_MUL(re, t[i<<uint(shift)]) - S_MUL(im, t[(N4-i)<<uint(shift)])
			yi = S_MUL(im, t[i<<uint(shift)]) + S_MUL(re, t[(N4-i)<<uint(shift)])
			// works because the cos is nearly one
			f[fp] = yr - S_MUL(yi, sine)
			fp++
			f[fp] = yi + S_MUL(yr, sine)
			fp++
		}
	}
	// De-shuffle the components for the middle of the window only
	{
		fp1 := 0
		fp2 := N2 - 1
		yp := 0
		for i = 0; i < N4; i++ {
			f2[yp] = -f[fp1]
			yp++
			f2[yp] = f[fp2]
			yp++
			fp1 += 2
			fp2 -= 2
		}
	}
	o := outOff - ((N2 - overlap) >> 1)
	// Mirror on both sides for TDAC
	{
		fp1 := N4 - 1
		xp1 := o + N2 - 1
		yp1 := o + N4 - overlap/2
		wp1 := 0
		wp2 := overlap - 1
		for i = 0; i < N4-overlap/2; i++ {
			out[xp1] = f2[fp1]
			xp1--
			fp1--
		}
		for ; i < N4; i++ {
			var x1 int32
			x1 = f2[fp1]
			fp1--
			out[yp1] += -MULT16_32_Q15(window[wp1], x1)
			yp1++
			out[xp1] += MULT16_32_Q15(window[wp2], x1)
			xp1--
			wp1++
			wp2--
		}
	}
	{
		fp2 := N4
		xp2 := o + N2
		yp2 := o + N - 1 - (N4 - overlap/2)
		wp1 := 0
		wp2 := overlap - 1
		for i = 0; i < N4-overlap/2; i++ {
			out[xp2] = f2[fp2]
			xp2++
			fp2++
		}
		for ; i < N4; i++ {
			var x2 int32
			x2 = f2[fp2]
			fp2++
			out[yp2] = MULT16_32_Q15(window[wp1], x2)
			yp2--
			out[xp2] = MULT16_32_Q15(window[wp2], x2)
			xp2++
			wp1++
			wp2--
		}
	}
}
