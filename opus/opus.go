package opus

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// C: src/opus.c

// PcmSoftClipImpl applies soft-clipping to bring a float signal within the
// [-1,1] range. This is genuine C float code (DISABLE_FLOAT_API not
// defined); every product that feeds an addition is converted to float32
// to prevent FMA fusion.
//
// C: void opus_pcm_soft_clip_impl(float *_x, int N, int C, float *declip_mem, int arch)
func PcmSoftClipImpl(_x []float32, N int, C int, declip_mem []float32, arch int) {
	var c int
	var i int
	var x []float32
	var all_within_neg1pos1 int

	if C < 1 || N < 1 || _x == nil || declip_mem == nil {
		return
	}

	// Clamp everything within the range [-2, +2] which is the domain of the soft
	// clipping non-linearity. Outside the defined range the derivative will be zero,
	// therefore there is no discontinuity introduced here. The implementation
	// might provide a hint if all input samples are within the [-1, +1] range.
	//
	// `opus_limit2_checkwithin1()`:
	//    - Clamps all samples within the valid range [-2, +2].
	//    - Generic C implementation:
	//       * Does not attempt early detection whether samples are within hinted range.
	//       * Always returns 0.
	//    - Architecture specific implementation:
	//       * Uses SIMD instructions to efficiently detect if all samples are
	//         within the hinted range [-1, +1].
	//       * Returns 1 if no samples exceed the hinted range, 0 otherwise.
	//
	// `all_within_neg1pos1`:
	//    - Optimization hint to skip per-sample out-of-bound checks.
	//      If true, the check can be skipped.
	all_within_neg1pos1 = celt.Limit2Checkwithin1(_x, N*C, arch)

	for c = 0; c < C; c++ {
		var a float32
		var x0 float32
		var curr int

		x = _x[c:]
		a = declip_mem[c]
		// Continue applying the non-linearity from the previous frame to avoid
		// any discontinuity.
		for i = 0; i < N; i++ {
			if x[i*C]*a >= 0 {
				break
			}
			x[i*C] = x[i*C] + float32(float32(a*x[i*C])*x[i*C])
		}

		curr = 0
		x0 = x[0]
		for {
			var start, end int
			var maxval float32
			var special int = 0
			var peak_pos int
			// Detection for early exit can be skipped if hinted by `all_within_neg1pos1`
			if all_within_neg1pos1 != 0 {
				i = N
			} else {
				for i = curr; i < N; i++ {
					if x[i*C] > 1 || x[i*C] < -1 {
						break
					}
				}
			}
			if i == N {
				a = 0
				break
			}
			peak_pos = i
			start = i
			end = i
			maxval = abs32(x[i*C])
			// Look for first zero crossing before clipping
			for start > 0 && x[i*C]*x[(start-1)*C] >= 0 {
				start--
			}
			// Look for first zero crossing after clipping
			for end < N && x[i*C]*x[end*C] >= 0 {
				// Look for other peaks until the next zero-crossing.
				if abs32(x[end*C]) > maxval {
					maxval = abs32(x[end*C])
					peak_pos = end
				}
				end++
			}
			// Detect the special case where we clip before the first zero crossing
			special = b2i(start == 0 && x[i*C]*x[0] >= 0)

			// Compute a such that maxval + a*maxval^2 = 1
			a = (maxval - 1) / (maxval * maxval)
			// Slightly boost "a" by 2^-22. This is just enough to ensure -ffast-math
			// does not cause output values larger than +/-1, but small enough not
			// to matter even for 24-bit output.
			a = a + float32(a*2.4e-7)
			if x[i*C] > 0 {
				a = -a
			}
			// Apply soft clipping
			for i = start; i < end; i++ {
				x[i*C] = x[i*C] + float32(float32(a*x[i*C])*x[i*C])
			}

			if special != 0 && peak_pos >= 2 {
				// Add a linear ramp from the first sample to the signal peak.
				// This avoids a discontinuity at the beginning of the frame.
				var delta float32
				var offset float32 = x0 - x[0]
				delta = offset / float32(peak_pos)
				for i = curr; i < peak_pos; i++ {
					offset -= delta
					x[i*C] += offset
					x[i*C] = max(-1.0, min(1.0, x[i*C]))
				}
			}
			curr = end
			if curr == N {
				break
			}
		}
		declip_mem[c] = a
	}
}

// PcmSoftClip applies soft-clipping to bring a float signal within the
// [-1,1] range.
//
// C: void opus_pcm_soft_clip(float *_x, int N, int C, float *declip_mem)
func PcmSoftClip(_x []float32, N int, C int, declip_mem []float32) {
	PcmSoftClipImpl(_x, N, C, declip_mem, 0)
}

// abs32 is the C ABS16() macro applied to a float (float build of arch.h).
func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// C: int encode_size(int size, unsigned char *data)
func encode_size(size int, data []byte) int {
	if size < 252 {
		data[0] = byte(size)
		return 1
	}
	data[0] = byte(252 + (size & 0x3))
	data[1] = byte((size - int(data[0])) >> 2)
	return 2
}

// C: static int parse_size(const unsigned char *data, opus_int32 len, opus_int16 *size)
func parse_size(data []byte, length int32, size *int16) int {
	if length < 1 {
		*size = -1
		return -1
	} else if data[0] < 252 {
		*size = int16(data[0])
		return 1
	} else if length < 2 {
		*size = -1
		return -1
	}
	*size = int16(4*int(data[1]) + int(data[0]))
	return 2
}

// C: int opus_packet_get_samples_per_frame(const unsigned char *data, opus_int32 Fs)
func PacketGetSamplesPerFrame(data []byte, Fs int32) int {
	var audiosize int
	if data[0]&0x80 != 0 {
		audiosize = int((data[0] >> 3) & 0x3)
		audiosize = int((Fs << uint(audiosize)) / 400)
	} else if (data[0] & 0x60) == 0x60 {
		if data[0]&0x08 != 0 {
			audiosize = int(Fs / 50)
		} else {
			audiosize = int(Fs / 100)
		}
	} else {
		audiosize = int((data[0] >> 3) & 0x3)
		if audiosize == 3 {
			audiosize = int(Fs * 60 / 1000)
		} else {
			audiosize = int((Fs << uint(audiosize)) / 100)
		}
	}
	return audiosize
}

// C: int opus_packet_parse_impl(const unsigned char *data, opus_int32 len,
//
//	int self_delimited, unsigned char *out_toc,
//	const unsigned char *frames[48], opus_int16 size[48],
//	int *payload_offset, opus_int32 *packet_offset,
//	const unsigned char **padding, opus_int32 *padding_len)
//
// frames may be nil; when given, frames[i] is set to the slice of data
// starting at frame i (like the C pointer). padding, when given, is set to
// the slice of data starting at the padding section (padding_len bytes).
func opus_packet_parse_impl(data []byte, length int32,
	self_delimited int, out_toc *byte,
	frames [][]byte, size []int16,
	payload_offset *int, packet_offset *int32,
	padding *[]byte, padding_len *int32) int {
	var i, bytes int
	var count int
	var cbr int
	var ch, toc byte
	var framesize int
	var last_size int32
	var pad int32 = 0
	data0 := data
	pos := 0 // index of the C `data` pointer in data0

	// Make sure we return NULL/0 on error.
	if padding != nil {
		*padding = nil
		*padding_len = 0
	}

	if size == nil || length < 0 {
		return OPUS_BAD_ARG
	}
	if length == 0 {
		return OPUS_INVALID_PACKET
	}

	framesize = PacketGetSamplesPerFrame(data, 48000)

	cbr = 0
	toc = data[pos]
	pos++
	length--
	last_size = length
	switch toc & 0x3 {
	// One frame
	case 0:
		count = 1
	// Two CBR frames
	case 1:
		count = 2
		cbr = 1
		if self_delimited == 0 {
			if length&0x1 != 0 {
				return OPUS_INVALID_PACKET
			}
			last_size = length / 2
			// If last_size doesn't fit in size[0], we'll catch it later
			size[0] = int16(last_size)
		}
	// Two VBR frames
	case 2:
		count = 2
		bytes = parse_size(data[pos:], length, &size[0])
		length -= int32(bytes)
		if size[0] < 0 || int32(size[0]) > length {
			return OPUS_INVALID_PACKET
		}
		pos += bytes
		last_size = length - int32(size[0])
	// Multiple CBR/VBR frames (from 0 to 120 ms)
	default: // case 3:
		if length < 1 {
			return OPUS_INVALID_PACKET
		}
		// Number of frames encoded in bits 0 to 5
		ch = data[pos]
		pos++
		count = int(ch & 0x3F)
		if count <= 0 || framesize*count > 5760 {
			return OPUS_INVALID_PACKET
		}
		length--
		// Padding flag is bit 6
		if ch&0x40 != 0 {
			var p int
			for {
				var tmp int
				if length <= 0 {
					return OPUS_INVALID_PACKET
				}
				p = int(data[pos])
				pos++
				length--
				if p == 255 {
					tmp = 254
				} else {
					tmp = p
				}
				length -= int32(tmp)
				pad += int32(tmp)
				if p != 255 {
					break
				}
			}
		}
		if length < 0 {
			return OPUS_INVALID_PACKET
		}
		// VBR flag is bit 7
		cbr = b2i(ch&0x80 == 0)
		if cbr == 0 {
			// VBR case
			last_size = length
			for i = 0; i < count-1; i++ {
				bytes = parse_size(data[pos:], length, &size[i])
				length -= int32(bytes)
				if size[i] < 0 || int32(size[i]) > length {
					return OPUS_INVALID_PACKET
				}
				pos += bytes
				last_size -= int32(bytes) + int32(size[i])
			}
			if last_size < 0 {
				return OPUS_INVALID_PACKET
			}
		} else if self_delimited == 0 {
			// CBR case
			last_size = length / int32(count)
			if last_size*int32(count) != length {
				return OPUS_INVALID_PACKET
			}
			for i = 0; i < count-1; i++ {
				size[i] = int16(last_size)
			}
		}
	}
	// Self-delimited framing has an extra size for the last frame.
	if self_delimited != 0 {
		bytes = parse_size(data[pos:], length, &size[count-1])
		length -= int32(bytes)
		if size[count-1] < 0 || int32(size[count-1]) > length {
			return OPUS_INVALID_PACKET
		}
		pos += bytes
		// For CBR packets, apply the size to all the frames.
		if cbr != 0 {
			if int32(size[count-1])*int32(count) > length {
				return OPUS_INVALID_PACKET
			}
			for i = 0; i < count-1; i++ {
				size[i] = size[count-1]
			}
		} else if int32(bytes)+int32(size[count-1]) > last_size {
			return OPUS_INVALID_PACKET
		}
	} else {
		// Because it's not encoded explicitly, it's possible the size of the
		// last packet (or all the packets, for the CBR case) is larger than
		// 1275. Reject them here.
		if last_size > 1275 {
			return OPUS_INVALID_PACKET
		}
		size[count-1] = int16(last_size)
	}

	if payload_offset != nil {
		*payload_offset = pos
	}

	for i = 0; i < count; i++ {
		if frames != nil {
			frames[i] = data0[pos:]
		}
		pos += int(size[i])
	}

	if padding != nil {
		*padding = data0[pos:]
		*padding_len = pad
	}
	if packet_offset != nil {
		*packet_offset = pad + int32(pos)
	}

	if out_toc != nil {
		*out_toc = toc
	}

	return count
}

// PacketParse parses an opus packet into one or more frames.
//
// C: int opus_packet_parse(const unsigned char *data, opus_int32 len, unsigned char *out_toc, const unsigned char *frames[48], opus_int16 size[48], int *payload_offset)
func PacketParse(data []byte, length int32,
	out_toc *byte, frames [][]byte,
	size []int16, payload_offset *int) int {
	return opus_packet_parse_impl(data, length, 0, out_toc,
		frames, size, payload_offset, nil, nil, nil)
}

// C: static OPUS_INLINE int align(int i)   (opus_private.h)
//
// alignment is offsetof(struct foo, u) with
// struct foo {char c; union { void* p; opus_int32 i; opus_val32 v; } u;},
// i.e. 8 on a 64-bit platform.
func align(i int) int {
	const alignment = 8
	// Optimizing compilers should optimize div and multiply into and
	// for all sensible alignment values.
	return ((i + alignment - 1) / alignment) * alignment
}

// IMIN/IMAX etc. used by the C src/ files come from arch.h via celt.
func IMIN[T celtInteger](a, b T) T { return celt.IMIN(a, b) }
func IMAX[T celtInteger](a, b T) T { return celt.IMAX(a, b) }

type celtInteger interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

// C: const char *opus_strerror(int error)
func Strerror(error int) string { return celt.Strerror(error) }

// C: const char *opus_get_version_string(void)
func GetVersionString() string { return celt.GetVersionString() }

// b2i converts a C boolean expression to int.
func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
