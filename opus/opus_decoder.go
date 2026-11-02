package opus

import (
	"github.com/DubbaThonyVibecodes/go-libopus/celt"
	"github.com/DubbaThonyVibecodes/go-libopus/silk"
)

// C: src/opus_decoder.c

// ALLOC_NONE is the size used for stack buffers that are never read
// (celt/stack_alloc.h).
const ALLOC_NONE = 1

// OpusDecoder is the C struct OpusDecoder. The SILK and CELT states that C
// places after the struct in the same allocation are embedded fields.
type OpusDecoder struct {
	channels    int
	Fs          int32 // Sampling rate (at the API level)
	DecControl  silk.DecControlStruct
	decode_gain int
	arch        int

	// Everything beyond this point gets cleared on a reset (OPUS_DECODER_RESET_START)
	stream_channels int

	bandwidth            int
	mode                 int
	prev_mode            int
	frame_size           int
	prev_redundancy      int
	last_packet_duration int
	// softclip_mem [2]opus_val16 exists only in the float build (#ifndef FIXED_POINT).

	rangeFinal uint32

	silk_dec silk.Decoder
	celt_dec celt.CELTDecoder
}

// DecoderGetSize returns the C allocation size (for API parity).
//
// C: int opus_decoder_get_size(int channels)
func DecoderGetSize(channels int) int {
	var silkDecSizeBytes, celtDecSizeBytes int
	var ret int
	if channels < 1 || channels > 2 {
		return 0
	}
	ret = silk.GetDecoderSize(&silkDecSizeBytes)
	if ret != 0 {
		return 0
	}
	silkDecSizeBytes = align(silkDecSizeBytes)
	celtDecSizeBytes = celt.DecoderGetSize(channels)
	return align(80) + silkDecSizeBytes + celtDecSizeBytes // sizeof(OpusDecoder) == 80 in the fixed-point build on x86-64
}

// C: int opus_decoder_init(OpusDecoder *st, opus_int32 Fs, int channels)
func (st *OpusDecoder) DecoderInit(Fs int32, channels int) int {
	var ret int

	if (Fs != 48000 && Fs != 24000 && Fs != 16000 && Fs != 12000 && Fs != 8000) ||
		(channels != 1 && channels != 2) {
		return OPUS_BAD_ARG
	}

	*st = OpusDecoder{}
	st.channels = channels
	st.stream_channels = channels

	st.Fs = Fs
	st.DecControl.API_sampleRate = st.Fs
	st.DecControl.NChannelsAPI = int32(st.channels)

	// Reset decoder
	ret = st.silk_dec.InitDecoder()
	if ret != 0 {
		return OPUS_INTERNAL_ERROR
	}

	// Initialize CELT decoder
	ret = st.celt_dec.DecoderInit(Fs, channels)
	if ret != OPUS_OK {
		return OPUS_INTERNAL_ERROR
	}

	st.celt_dec.Ctl(celt.CELT_SET_SIGNALLING_REQUEST, int32(0))

	st.prev_mode = 0
	st.frame_size = int(Fs / 400)
	st.arch = celt.SelectArch()
	return OPUS_OK
}

// C: OpusDecoder *opus_decoder_create(opus_int32 Fs, int channels, int *error)
func DecoderCreate(Fs int32, channels int, error *int) *OpusDecoder {
	var ret int
	if (Fs != 48000 && Fs != 24000 && Fs != 16000 && Fs != 12000 && Fs != 8000) ||
		(channels != 1 && channels != 2) {
		if error != nil {
			*error = OPUS_BAD_ARG
		}
		return nil
	}
	st := new(OpusDecoder)
	ret = st.DecoderInit(Fs, channels)
	if error != nil {
		*error = ret
	}
	if ret != OPUS_OK {
		st = nil
	}
	return st
}

// C: static void smooth_fade(const opus_val16 *in1, const opus_val16 *in2, opus_val16 *out, int overlap, int channels, const opus_val16 *window, opus_int32 Fs)
func smooth_fade(in1, in2 []int16, out []int16, overlap, channels int, window []int16, Fs int32) {
	var i, c int
	inc := int(48000 / Fs)
	for c = 0; c < channels; c++ {
		for i = 0; i < overlap; i++ {
			var w int16 = int16(celt.MULT16_16_Q15(window[i*inc], window[i*inc]))
			out[i*channels+c] = int16(celt.SHR32(celt.MAC16_16(celt.MULT16_16(w, in2[i*channels+c]),
				int32(celt.Q15ONE)-int32(w), in1[i*channels+c]), 15))
		}
	}
}

// C: static int opus_packet_get_mode(const unsigned char *data)
func opus_packet_get_mode(data []byte) int {
	var mode int
	if data[0]&0x80 != 0 {
		mode = MODE_CELT_ONLY
	} else if (data[0] & 0x60) == 0x60 {
		mode = MODE_HYBRID
	} else {
		mode = MODE_SILK_ONLY
	}
	return mode
}

// C: static int opus_decode_frame(OpusDecoder *st, const unsigned char *data, opus_int32 len, opus_val16 *pcm, int frame_size, int decode_fec)
func (st *OpusDecoder) decodeFrame(data []byte, length int32, pcm []int16, frame_size int, decode_fec int) int {
	silk_dec := &st.silk_dec
	celt_dec := &st.celt_dec
	var i int
	var silk_ret, celt_ret int = 0, 0
	var dec celt.EcDec
	var silk_frame_size int32
	var pcm_silk_size int
	var pcm_silk []int16
	var pcm_transition_silk_size int
	var pcm_transition_silk []int16
	var pcm_transition_celt_size int
	var pcm_transition_celt []int16
	var pcm_transition []int16 = nil
	var redundant_audio_size int
	var redundant_audio []int16

	var audiosize int
	var mode int
	var transition int = 0
	var start_band int
	var redundancy int = 0
	var redundancy_bytes int = 0
	var celt_to_silk int = 0
	var c int
	var F2_5, F5, F10, F20 int
	var window []int16
	var redundant_rng uint32 = 0
	var celt_accum int

	F20 = int(st.Fs / 50)
	F10 = F20 >> 1
	F5 = F10 >> 1
	F2_5 = F5 >> 1
	if frame_size < F2_5 {
		return OPUS_BUFFER_TOO_SMALL
	}
	// Limit frame_size to avoid excessive stack allocations.
	frame_size = IMIN(frame_size, int(st.Fs/25*3))
	// Payloads of 1 (2 including ToC) or 0 trigger the PLC/DTX
	if length <= 1 {
		data = nil
		// In that case, don't conceal more than what the ToC says
		frame_size = IMIN(frame_size, st.frame_size)
	}
	if data != nil {
		audiosize = st.frame_size
		mode = st.mode
		dec.DecInit(data, uint32(length))
	} else {
		audiosize = frame_size
		mode = st.prev_mode

		if mode == 0 {
			// If we haven't got any packet yet, all we can do is return zeros
			for i = 0; i < audiosize*st.channels; i++ {
				pcm[i] = 0
			}
			return audiosize
		}

		// Avoids trying to run the PLC on sizes other than 2.5 (CELT), 5 (CELT),
		// 10, or 20 (e.g. 12.5 or 30 ms).
		if audiosize > F20 {
			for {
				ret := st.decodeFrame(nil, 0, pcm, IMIN(audiosize, F20), 0)
				if ret < 0 {
					return ret
				}
				pcm = pcm[ret*st.channels:]
				audiosize -= ret
				if audiosize <= 0 {
					break
				}
			}
			return frame_size
		} else if audiosize < F20 {
			if audiosize > F10 {
				audiosize = F10
			} else if mode != MODE_SILK_ONLY && audiosize > F5 && audiosize < F10 {
				audiosize = F5
			}
		}
	}

	// In fixed-point, we can tell CELT to do the accumulation on top of the
	// SILK PCM buffer. This saves some stack space.
	celt_accum = b2i((mode != MODE_CELT_ONLY) && (frame_size >= F10))

	pcm_transition_silk_size = ALLOC_NONE
	pcm_transition_celt_size = ALLOC_NONE
	if data != nil && st.prev_mode > 0 && ((mode == MODE_CELT_ONLY && st.prev_mode != MODE_CELT_ONLY && st.prev_redundancy == 0) ||
		(mode != MODE_CELT_ONLY && st.prev_mode == MODE_CELT_ONLY)) {
		transition = 1
		// Decide where to allocate the stack memory for pcm_transition
		if mode == MODE_CELT_ONLY {
			pcm_transition_celt_size = F5 * st.channels
		} else {
			pcm_transition_silk_size = F5 * st.channels
		}
	}
	pcm_transition_celt = make([]int16, pcm_transition_celt_size)
	if transition != 0 && mode == MODE_CELT_ONLY {
		pcm_transition = pcm_transition_celt
		st.decodeFrame(nil, 0, pcm_transition, IMIN(F5, audiosize), 0)
	}
	if audiosize > frame_size {
		// fprintf(stderr, "PCM buffer too small: %d vs %d (mode = %d)\n", audiosize, frame_size, mode);
		return OPUS_BAD_ARG
	} else {
		frame_size = audiosize
	}

	// Don't allocate any memory when in CELT-only mode
	if mode != MODE_CELT_ONLY && celt_accum == 0 {
		pcm_silk_size = IMAX(F10, frame_size) * st.channels
	} else {
		pcm_silk_size = ALLOC_NONE
	}
	pcm_silk = make([]int16, pcm_silk_size)

	// SILK processing
	if mode != MODE_CELT_ONLY {
		var lost_flag, decoded_samples int
		var pcm_ptr []int16
		if celt_accum != 0 {
			pcm_ptr = pcm
		} else {
			pcm_ptr = pcm_silk
		}

		if st.prev_mode == MODE_CELT_ONLY {
			silk_dec.InitDecoder()
		}

		// The SILK PLC cannot produce frames of less than 10 ms
		st.DecControl.PayloadSize_ms = IMAX(10, 1000*audiosize/int(st.Fs))

		if data != nil {
			st.DecControl.NChannelsInternal = int32(st.stream_channels)
			if mode == MODE_SILK_ONLY {
				if st.bandwidth == OPUS_BANDWIDTH_NARROWBAND {
					st.DecControl.InternalSampleRate = 8000
				} else if st.bandwidth == OPUS_BANDWIDTH_MEDIUMBAND {
					st.DecControl.InternalSampleRate = 12000
				} else if st.bandwidth == OPUS_BANDWIDTH_WIDEBAND {
					st.DecControl.InternalSampleRate = 16000
				} else {
					st.DecControl.InternalSampleRate = 16000
					celt.Assert(false)
				}
			} else {
				// Hybrid mode
				st.DecControl.InternalSampleRate = 16000
			}
		}

		if data == nil {
			lost_flag = 1
		} else {
			lost_flag = 2 * decode_fec
		}
		decoded_samples = 0
		for {
			// Call SILK decoder
			first_frame := b2i(decoded_samples == 0)
			silk_ret = silk_dec.Decode(&st.DecControl,
				lost_flag, first_frame, &dec, pcm_ptr, &silk_frame_size, st.arch)
			if silk_ret != 0 {
				if lost_flag != 0 {
					// PLC failure should not be fatal
					silk_frame_size = int32(frame_size)
					for i = 0; i < frame_size*st.channels; i++ {
						pcm_ptr[i] = 0
					}
				} else {
					return OPUS_INTERNAL_ERROR
				}
			}
			pcm_ptr = pcm_ptr[int(silk_frame_size)*st.channels:]
			decoded_samples += int(silk_frame_size)
			if decoded_samples >= frame_size {
				break
			}
		}
	}

	start_band = 0
	if decode_fec == 0 && mode != MODE_CELT_ONLY && data != nil &&
		dec.Tell()+17+20*b2i(st.mode == MODE_HYBRID) <= 8*int(length) {
		// Check if we have a redundant 0-8 kHz band
		if mode == MODE_HYBRID {
			redundancy = dec.DecBitLogp(12)
		} else {
			redundancy = 1
		}
		if redundancy != 0 {
			celt_to_silk = dec.DecBitLogp(1)
			// redundancy_bytes will be at least two, in the non-hybrid
			// case due to the ec_tell() check above
			if mode == MODE_HYBRID {
				redundancy_bytes = int(dec.DecUint(256)) + 2
			} else {
				redundancy_bytes = int(length) - ((dec.Tell() + 7) >> 3)
			}
			length -= int32(redundancy_bytes)
			// This is a sanity check. It should never happen for a valid
			// packet, so the exact behaviour is not normative.
			if int(length)*8 < dec.Tell() {
				length = 0
				redundancy_bytes = 0
				redundancy = 0
			}
			// Shrink decoder because of raw bits
			dec.SetStorage(dec.Storage() - uint32(redundancy_bytes))
		}
	}
	if mode != MODE_CELT_ONLY {
		start_band = 17
	}

	{
		endband := 21

		switch st.bandwidth {
		case OPUS_BANDWIDTH_NARROWBAND:
			endband = 13
		case OPUS_BANDWIDTH_MEDIUMBAND, OPUS_BANDWIDTH_WIDEBAND:
			endband = 17
		case OPUS_BANDWIDTH_SUPERWIDEBAND:
			endband = 19
		case OPUS_BANDWIDTH_FULLBAND:
			endband = 21
		}
		celt_dec.Ctl(celt.CELT_SET_END_BAND_REQUEST, int32(endband))
		celt_dec.Ctl(celt.CELT_SET_CHANNELS_REQUEST, int32(st.stream_channels))
	}

	if redundancy != 0 {
		transition = 0
		pcm_transition_silk_size = ALLOC_NONE
	}

	pcm_transition_silk = make([]int16, pcm_transition_silk_size)

	if transition != 0 && mode != MODE_CELT_ONLY {
		pcm_transition = pcm_transition_silk
		st.decodeFrame(nil, 0, pcm_transition, IMIN(F5, audiosize), 0)
	}

	// Only allocation memory for redundancy if/when needed
	if redundancy != 0 {
		redundant_audio_size = F5 * st.channels
	} else {
		redundant_audio_size = ALLOC_NONE
	}
	redundant_audio = make([]int16, redundant_audio_size)

	// 5 ms redundant frame for CELT->SILK
	if redundancy != 0 && celt_to_silk != 0 {
		celt_dec.Ctl(celt.CELT_SET_START_BAND_REQUEST, int32(0))
		celt_dec.DecodeWithEc(data[length:], redundancy_bytes,
			redundant_audio, F5, nil, 0)
		celt_dec.Ctl(celt.OPUS_GET_FINAL_RANGE_REQUEST, &redundant_rng)
	}

	// MUST be after PLC
	celt_dec.Ctl(celt.CELT_SET_START_BAND_REQUEST, int32(start_band))

	if mode != MODE_SILK_ONLY {
		celt_frame_size := IMIN(F20, frame_size)
		// Make sure to discard any previous CELT state
		if mode != st.prev_mode && st.prev_mode > 0 && st.prev_redundancy == 0 {
			celt_dec.Ctl(celt.OPUS_RESET_STATE)
		}
		// Decode CELT
		var cdata []byte
		if decode_fec == 0 {
			cdata = data
		}
		celt_ret = celt_dec.DecodeWithEc(cdata,
			int(length), pcm, celt_frame_size, &dec, celt_accum)
	} else {
		silence := []byte{0xFF, 0xFF}
		if celt_accum == 0 {
			for i = 0; i < frame_size*st.channels; i++ {
				pcm[i] = 0
			}
		}
		// For hybrid -> SILK transitions, we let the CELT MDCT
		// do a fade-out by decoding a silence frame
		if st.prev_mode == MODE_HYBRID && !(redundancy != 0 && celt_to_silk != 0 && st.prev_redundancy != 0) {
			celt_dec.Ctl(celt.CELT_SET_START_BAND_REQUEST, int32(0))
			celt_dec.DecodeWithEc(silence, 2, pcm, F2_5, nil, celt_accum)
		}
	}

	if mode != MODE_CELT_ONLY && celt_accum == 0 {
		for i = 0; i < frame_size*st.channels; i++ {
			pcm[i] = celt.SAT16(celt.ADD32(pcm[i], pcm_silk[i]))
		}
	}

	{
		var celt_mode *celt.CELTMode
		celt_dec.Ctl(celt.CELT_GET_MODE_REQUEST, &celt_mode)
		window = celt_mode.Window()
	}

	// 5 ms redundant frame for SILK->CELT
	if redundancy != 0 && celt_to_silk == 0 {
		celt_dec.Ctl(celt.OPUS_RESET_STATE)
		celt_dec.Ctl(celt.CELT_SET_START_BAND_REQUEST, int32(0))

		celt_dec.DecodeWithEc(data[length:], redundancy_bytes, redundant_audio, F5, nil, 0)
		celt_dec.Ctl(celt.OPUS_GET_FINAL_RANGE_REQUEST, &redundant_rng)
		smooth_fade(pcm[st.channels*(frame_size-F2_5):], redundant_audio[st.channels*F2_5:],
			pcm[st.channels*(frame_size-F2_5):], F2_5, st.channels, window, st.Fs)
	}
	if redundancy != 0 && celt_to_silk != 0 {
		for c = 0; c < st.channels; c++ {
			for i = 0; i < F2_5; i++ {
				pcm[st.channels*i+c] = redundant_audio[st.channels*i+c]
			}
		}
		smooth_fade(redundant_audio[st.channels*F2_5:], pcm[st.channels*F2_5:],
			pcm[st.channels*F2_5:], F2_5, st.channels, window, st.Fs)
	}
	if transition != 0 {
		if audiosize >= F5 {
			for i = 0; i < st.channels*F2_5; i++ {
				pcm[i] = pcm_transition[i]
			}
			smooth_fade(pcm_transition[st.channels*F2_5:], pcm[st.channels*F2_5:],
				pcm[st.channels*F2_5:], F2_5,
				st.channels, window, st.Fs)
		} else {
			// Not enough time to do a clean transition, but we do it anyway
			// This will not preserve amplitude perfectly and may introduce
			// a bit of temporal aliasing, but it shouldn't be too bad and
			// that's pretty much the best we can do. In any case, generating this
			// transition it pretty silly in the first place
			smooth_fade(pcm_transition, pcm,
				pcm, F2_5,
				st.channels, window, st.Fs)
		}
	}

	if st.decode_gain != 0 {
		var gain int32
		gain = celt.Exp2(int16(celt.MULT16_16_P15(celt.QCONST16(6.48814081e-4, 25), st.decode_gain)))
		for i = 0; i < frame_size*st.channels; i++ {
			var x int32
			x = celt.MULT16_32_P16(pcm[i], gain)
			pcm[i] = int16(celt.SATURATE(x, 32767))
		}
	}

	if length <= 1 {
		st.rangeFinal = 0
	} else {
		st.rangeFinal = dec.Rng() ^ redundant_rng
	}

	st.prev_mode = mode
	st.prev_redundancy = b2i(redundancy != 0 && celt_to_silk == 0)

	// OPUS_CHECK_ARRAY/OPUS_PRINT_INT are no-ops in this build.

	if celt_ret < 0 {
		return celt_ret
	}
	return audiosize
}

// C: int opus_decode_native(OpusDecoder *st, const unsigned char *data,
//
//	opus_int32 len, opus_val16 *pcm, int frame_size, int decode_fec,
//	int self_delimited, opus_int32 *packet_offset, int soft_clip)
//
// soft_clip is only used in the float build (#ifndef FIXED_POINT); it is
// kept for signature parity.
func (st *OpusDecoder) DecodeNative(data []byte,
	length int32, pcm []int16, frame_size int, decode_fec int,
	self_delimited int, packet_offset *int32, soft_clip int) int {
	var i, nb_samples int
	var count, offset int
	var toc byte
	var packet_frame_size, packet_bandwidth, packet_mode, packet_stream_channels int
	// 48 x 2.5 ms = 120 ms
	var size [48]int16
	if decode_fec < 0 || decode_fec > 1 {
		return OPUS_BAD_ARG
	}
	// For FEC/PLC, frame_size has to be to have a multiple of 2.5 ms
	if (decode_fec != 0 || length == 0 || data == nil) && frame_size%int(st.Fs/400) != 0 {
		return OPUS_BAD_ARG
	}
	if length == 0 || data == nil {
		var pcm_count int = 0
		for {
			var ret int
			ret = st.decodeFrame(nil, 0, pcm[pcm_count*st.channels:], frame_size-pcm_count, 0)
			if ret < 0 {
				return ret
			}
			pcm_count += ret
			if pcm_count >= frame_size {
				break
			}
		}
		celt.Assert(pcm_count == frame_size)
		st.last_packet_duration = pcm_count
		return pcm_count
	} else if length < 0 {
		return OPUS_BAD_ARG
	}

	packet_mode = opus_packet_get_mode(data)
	packet_bandwidth = PacketGetBandwidth(data)
	packet_frame_size = PacketGetSamplesPerFrame(data, st.Fs)
	packet_stream_channels = PacketGetNbChannels(data)

	count = opus_packet_parse_impl(data, length, self_delimited, &toc, nil,
		size[:], &offset, packet_offset)
	if count < 0 {
		return count
	}

	data = data[offset:]

	if decode_fec != 0 {
		var duration_copy int
		var ret int
		// If no FEC can be present, run the PLC (recursive call)
		if frame_size < packet_frame_size || packet_mode == MODE_CELT_ONLY || st.mode == MODE_CELT_ONLY {
			return st.DecodeNative(nil, 0, pcm, frame_size, 0, 0, nil, soft_clip)
		}
		// Otherwise, run the PLC on everything except the size for which we might have FEC
		duration_copy = st.last_packet_duration
		if frame_size-packet_frame_size != 0 {
			ret = st.DecodeNative(nil, 0, pcm, frame_size-packet_frame_size, 0, 0, nil, soft_clip)
			if ret < 0 {
				st.last_packet_duration = duration_copy
				return ret
			}
			celt.Assert(ret == frame_size-packet_frame_size)
		}
		// Complete with FEC
		st.mode = packet_mode
		st.bandwidth = packet_bandwidth
		st.frame_size = packet_frame_size
		st.stream_channels = packet_stream_channels
		ret = st.decodeFrame(data, int32(size[0]), pcm[st.channels*(frame_size-packet_frame_size):],
			packet_frame_size, 1)
		if ret < 0 {
			return ret
		} else {
			st.last_packet_duration = frame_size
			return frame_size
		}
	}

	if count*packet_frame_size > frame_size {
		return OPUS_BUFFER_TOO_SMALL
	}

	// Update the state as the last step to avoid updating it on an invalid packet
	st.mode = packet_mode
	st.bandwidth = packet_bandwidth
	st.frame_size = packet_frame_size
	st.stream_channels = packet_stream_channels

	nb_samples = 0
	for i = 0; i < count; i++ {
		var ret int
		ret = st.decodeFrame(data, int32(size[i]), pcm[nb_samples*st.channels:], frame_size-nb_samples, 0)
		if ret < 0 {
			return ret
		}
		celt.Assert(ret == packet_frame_size)
		data = data[size[i]:]
		nb_samples += ret
	}
	st.last_packet_duration = nb_samples
	// #ifndef FIXED_POINT: opus_pcm_soft_clip(pcm, nb_samples, st->channels, st->softclip_mem)
	// is only compiled in the float build.
	return nb_samples
}

// Decode decodes an Opus packet. data==nil or len==0 triggers packet loss
// concealment. Returns the number of decoded samples per channel.
//
// C: int opus_decode(OpusDecoder *st, const unsigned char *data, opus_int32 len, opus_val16 *pcm, int frame_size, int decode_fec)
func (st *OpusDecoder) Decode(data []byte,
	length int32, pcm []int16, frame_size int, decode_fec int) int {
	if frame_size <= 0 {
		return OPUS_BAD_ARG
	}
	return st.DecodeNative(data, length, pcm, frame_size, decode_fec, 0, nil, 0)
}

// DecodeFloat is the float output variant (fixed-point build: converts).
//
// C: int opus_decode_float(OpusDecoder *st, const unsigned char *data, opus_int32 len, float *pcm, int frame_size, int decode_fec)
func (st *OpusDecoder) DecodeFloat(data []byte,
	length int32, pcm []float32, frame_size int, decode_fec int) int {
	var ret, i int
	var nb_samples int

	if frame_size <= 0 {
		return OPUS_BAD_ARG
	}
	if data != nil && length > 0 && decode_fec == 0 {
		nb_samples = st.DecoderGetNbSamples(data, length)
		if nb_samples > 0 {
			frame_size = IMIN(frame_size, nb_samples)
		} else {
			return OPUS_INVALID_PACKET
		}
	}
	out := make([]int16, frame_size*st.channels)

	ret = st.DecodeNative(data, length, out, frame_size, decode_fec, 0, nil, 0)
	if ret > 0 {
		for i = 0; i < ret*st.channels; i++ {
			pcm[i] = (1.0 / 32768.0) * float32(out[i])
		}
	}
	return ret
}

// Ctl is the C opus_decoder_ctl. Value arguments are int32 (int accepted),
// output arguments are *int32 or *uint32.
//
// C: int opus_decoder_ctl(OpusDecoder *st, int request, ...)
func (st *OpusDecoder) Ctl(request int, args ...any) int {
	var ret int = OPUS_OK
	silk_dec := &st.silk_dec
	celt_dec := &st.celt_dec

	switch request {
	case OPUS_GET_BANDWIDTH_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG // goto bad_arg
		}
		*value = int32(st.bandwidth)
	case OPUS_GET_FINAL_RANGE_REQUEST:
		value, ok := args[0].(*uint32)
		if !ok || value == nil {
			return OPUS_BAD_ARG // goto bad_arg
		}
		*value = st.rangeFinal
	case OPUS_RESET_STATE:
		// OPUS_CLEAR from OPUS_DECODER_RESET_START (stream_channels) to the end
		// of the C struct (the embedded codec states are reset below).
		st.stream_channels = 0
		st.bandwidth = 0
		st.mode = 0
		st.prev_mode = 0
		st.frame_size = 0
		st.prev_redundancy = 0
		st.last_packet_duration = 0
		st.rangeFinal = 0

		celt_dec.Ctl(celt.OPUS_RESET_STATE)
		silk_dec.InitDecoder()
		st.stream_channels = st.channels
		st.frame_size = int(st.Fs / 400)
	case OPUS_GET_SAMPLE_RATE_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG // goto bad_arg
		}
		*value = st.Fs
	case OPUS_GET_PITCH_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG // goto bad_arg
		}
		if st.prev_mode == MODE_CELT_ONLY {
			celt_dec.Ctl(celt.OPUS_GET_PITCH_REQUEST, value)
		} else {
			*value = int32(st.DecControl.PrevPitchLag)
		}
	case OPUS_GET_GAIN_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG // goto bad_arg
		}
		*value = int32(st.decode_gain)
	case OPUS_SET_GAIN_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || value < -32768 || value > 32767 {
			return OPUS_BAD_ARG // goto bad_arg
		}
		st.decode_gain = int(value)
	case OPUS_GET_LAST_PACKET_DURATION_REQUEST:
		switch value := args[0].(type) {
		case *uint32:
			if value == nil {
				return OPUS_BAD_ARG // goto bad_arg
			}
			*value = uint32(st.last_packet_duration)
		case *int32:
			if value == nil {
				return OPUS_BAD_ARG // goto bad_arg
			}
			*value = int32(st.last_packet_duration)
		default:
			return OPUS_BAD_ARG
		}
	default:
		// fprintf(stderr, "unknown opus_decoder_ctl() request: %d", request);
		ret = OPUS_UNIMPLEMENTED
	}

	return ret
}

// argInt32 extracts an opus_int32 value argument from a Ctl call.
func argInt32(args []any, i int) (int32, bool) {
	if i >= len(args) {
		return 0, false
	}
	switch v := args[i].(type) {
	case int32:
		return v, true
	case int:
		return int32(v), true
	case int64:
		return int32(v), true
	case uint32:
		return int32(v), true
	}
	return 0, false
}

// DecoderDestroy frees the state (no-op in Go; kept for API parity).
//
// C: void opus_decoder_destroy(OpusDecoder *st)
func (st *OpusDecoder) DecoderDestroy() {}

// C: int opus_packet_get_bandwidth(const unsigned char *data)
func PacketGetBandwidth(data []byte) int {
	var bandwidth int
	if data[0]&0x80 != 0 {
		bandwidth = OPUS_BANDWIDTH_MEDIUMBAND + int((data[0]>>5)&0x3)
		if bandwidth == OPUS_BANDWIDTH_MEDIUMBAND {
			bandwidth = OPUS_BANDWIDTH_NARROWBAND
		}
	} else if (data[0] & 0x60) == 0x60 {
		if data[0]&0x10 != 0 {
			bandwidth = OPUS_BANDWIDTH_FULLBAND
		} else {
			bandwidth = OPUS_BANDWIDTH_SUPERWIDEBAND
		}
	} else {
		bandwidth = OPUS_BANDWIDTH_NARROWBAND + int((data[0]>>5)&0x3)
	}
	return bandwidth
}

// C: int opus_packet_get_nb_channels(const unsigned char *data)
func PacketGetNbChannels(data []byte) int {
	if data[0]&0x4 != 0 {
		return 2
	}
	return 1
}

// C: int opus_packet_get_nb_frames(const unsigned char packet[], opus_int32 len)
func PacketGetNbFrames(packet []byte, length int32) int {
	var count int
	if length < 1 {
		return OPUS_BAD_ARG
	}
	count = int(packet[0] & 0x3)
	if count == 0 {
		return 1
	} else if count != 3 {
		return 2
	} else if length < 2 {
		return OPUS_INVALID_PACKET
	}
	return int(packet[1] & 0x3F)
}

// C: int opus_packet_get_nb_samples(const unsigned char packet[], opus_int32 len, opus_int32 Fs)
func PacketGetNbSamples(packet []byte, length int32, Fs int32) int {
	var samples int
	count := PacketGetNbFrames(packet, length)

	if count < 0 {
		return count
	}

	samples = count * PacketGetSamplesPerFrame(packet, Fs)
	// Can't have more than 120 ms
	if int32(samples)*25 > Fs*3 {
		return OPUS_INVALID_PACKET
	}
	return samples
}

// C: int opus_decoder_get_nb_samples(const OpusDecoder *dec, const unsigned char packet[], opus_int32 len)
func (dec *OpusDecoder) DecoderGetNbSamples(packet []byte, length int32) int {
	return PacketGetNbSamples(packet, length, dec.Fs)
}
