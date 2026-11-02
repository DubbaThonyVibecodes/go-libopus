package opus

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// C: src/opus_multistream_decoder.c, include/opus_multistream.h

// OpusMSDecoder is the C struct OpusMSDecoder (src/opus_private.h); the
// per-stream decoder states that C places after the struct are the decoders
// slice (coupled streams first).
type OpusMSDecoder struct {
	layout ChannelLayout
	// Decoder states go here
	decoders []*OpusDecoder
}

/* DECODER */

// C: static void validate_ms_decoder(OpusMSDecoder *st)   (ENABLE_HARDENING)
func validate_ms_decoder(st *OpusMSDecoder) {
	validate_layout(&st.layout)
}

// C: #define VALIDATE_MS_DECODER(st) validate_ms_decoder(st)
func VALIDATE_MS_DECODER(st *OpusMSDecoder) { validate_ms_decoder(st) }

// C: opus_int32 opus_multistream_decoder_get_size(int nb_streams, int nb_coupled_streams)
func MultistreamDecoderGetSize(nb_streams int, nb_coupled_streams int) int32 {
	var coupled_size, mono_size int

	if nb_streams < 1 || nb_coupled_streams > nb_streams || nb_coupled_streams < 0 {
		return 0
	}
	coupled_size = DecoderGetSize(2)
	mono_size = DecoderGetSize(1)
	return int32(align(268) + // sizeof(OpusMSDecoder) == 268
		nb_coupled_streams*align(coupled_size) +
		(nb_streams-nb_coupled_streams)*align(mono_size))
}

// C: int opus_multistream_decoder_init(OpusMSDecoder *st, opus_int32 Fs, int channels, int streams, int coupled_streams, const unsigned char *mapping)
func (st *OpusMSDecoder) MultistreamDecoderInit(Fs int32, channels int, streams int, coupled_streams int, mapping []byte) int {
	var i, ret int

	if (channels > 255) || (channels < 1) || (coupled_streams > streams) ||
		(streams < 1) || (coupled_streams < 0) || (streams > 255-coupled_streams) {
		return OPUS_BAD_ARG
	}

	st.layout.nb_channels = channels
	st.layout.nb_streams = streams
	st.layout.nb_coupled_streams = coupled_streams

	for i = 0; i < st.layout.nb_channels; i++ {
		st.layout.mapping[i] = mapping[i]
	}
	if validate_layout(&st.layout) == 0 {
		return OPUS_BAD_ARG
	}

	st.decoders = make([]*OpusDecoder, streams)

	for i = 0; i < st.layout.nb_coupled_streams; i++ {
		st.decoders[i] = new(OpusDecoder)
		ret = st.decoders[i].DecoderInit(Fs, 2)
		if ret != OPUS_OK {
			return ret
		}
	}
	for ; i < st.layout.nb_streams; i++ {
		st.decoders[i] = new(OpusDecoder)
		ret = st.decoders[i].DecoderInit(Fs, 1)
		if ret != OPUS_OK {
			return ret
		}
	}
	return OPUS_OK
}

// C: OpusMSDecoder *opus_multistream_decoder_create(opus_int32 Fs, int channels, int streams, int coupled_streams, const unsigned char *mapping, int *error)
func MultistreamDecoderCreate(Fs int32, channels int, streams int, coupled_streams int, mapping []byte, error *int) *OpusMSDecoder {
	var ret int
	if (channels > 255) || (channels < 1) || (coupled_streams > streams) ||
		(streams < 1) || (coupled_streams < 0) || (streams > 255-coupled_streams) {
		if error != nil {
			*error = OPUS_BAD_ARG
		}
		return nil
	}
	st := new(OpusMSDecoder)
	ret = st.MultistreamDecoderInit(Fs, channels, streams, coupled_streams, mapping)
	if error != nil {
		*error = ret
	}
	if ret != OPUS_OK {
		st = nil
	}
	return st
}

// C: typedef void (*opus_copy_channel_out_func)(void *dst, int dst_stride, int dst_channel, const opus_val16 *src, int src_stride, int frame_size, void *user_data)   (src/opus_private.h)
type opus_copy_channel_out_func func(dst any, dst_stride int, dst_channel int, src []int16, src_stride int, frame_size int, user_data any)

// C: static int opus_multistream_packet_validate(const unsigned char *data, opus_int32 len, int nb_streams, opus_int32 Fs)
func opus_multistream_packet_validate(data []byte, length int32, nb_streams int, Fs int32) int {
	var s int
	var count int
	var toc byte
	var size [48]int16
	var samples int = 0
	var packet_offset int32

	for s = 0; s < nb_streams; s++ {
		var tmp_samples int
		if length <= 0 {
			return OPUS_INVALID_PACKET
		}
		count = opus_packet_parse_impl(data, length, b2i(s != nb_streams-1), &toc, nil,
			size[:], nil, &packet_offset, nil, nil)
		if count < 0 {
			return count
		}
		tmp_samples = PacketGetNbSamples(data, packet_offset, Fs)
		if s != 0 && samples != tmp_samples {
			return OPUS_INVALID_PACKET
		}
		samples = tmp_samples
		data = data[packet_offset:]
		length -= packet_offset
	}
	return samples
}

// C: int opus_multistream_decode_native(OpusMSDecoder *st, const unsigned char *data, opus_int32 len, void *pcm, opus_copy_channel_out_func copy_channel_out, int frame_size, int decode_fec, int soft_clip, void *user_data)
func (st *OpusMSDecoder) MultistreamDecodeNative(data []byte, length int32, pcm any, copy_channel_out opus_copy_channel_out_func, frame_size int, decode_fec int, soft_clip int, user_data any) int {
	var Fs int32
	var s, c int
	var do_plc int = 0

	VALIDATE_MS_DECODER(st)
	if frame_size <= 0 {
		return OPUS_BAD_ARG
	}
	// Limit frame_size to avoid excessive stack allocations.
	celt.MustSucceed(st.Ctl(OPUS_GET_SAMPLE_RATE_REQUEST, &Fs))
	frame_size = IMIN(frame_size, int(Fs/25*3))
	buf := make([]int16, 2*frame_size)

	if length == 0 {
		do_plc = 1
	}
	if length < 0 {
		return OPUS_BAD_ARG
	}
	if do_plc == 0 && length < int32(2*st.layout.nb_streams-1) {
		return OPUS_INVALID_PACKET
	}
	if do_plc == 0 {
		ret := opus_multistream_packet_validate(data, length, st.layout.nb_streams, Fs)
		if ret < 0 {
			return ret
		} else if ret > frame_size {
			return OPUS_BUFFER_TOO_SMALL
		}
	}
	for s = 0; s < st.layout.nb_streams; s++ {
		var dec *OpusDecoder
		var packet_offset int32
		var ret int

		dec = st.decoders[s]

		if do_plc == 0 && length <= 0 {
			return OPUS_INTERNAL_ERROR
		}
		packet_offset = 0
		ret = dec.DecodeNative(data, length, buf, frame_size, decode_fec, b2i(s != st.layout.nb_streams-1), &packet_offset, soft_clip, nil, 0)
		if do_plc == 0 {
			data = data[packet_offset:]
			length -= packet_offset
		}
		if ret <= 0 {
			return ret
		}
		frame_size = ret
		if s < st.layout.nb_coupled_streams {
			var chanIdx, prev int
			prev = -1
			// Copy "left" audio to the channel(s) where it belongs
			for {
				chanIdx = get_left_channel(&st.layout, s, prev)
				if chanIdx == -1 {
					break
				}
				copy_channel_out(pcm, st.layout.nb_channels, chanIdx,
					buf, 2, frame_size, user_data)
				prev = chanIdx
			}
			prev = -1
			// Copy "right" audio to the channel(s) where it belongs
			for {
				chanIdx = get_right_channel(&st.layout, s, prev)
				if chanIdx == -1 {
					break
				}
				copy_channel_out(pcm, st.layout.nb_channels, chanIdx,
					buf[1:], 2, frame_size, user_data)
				prev = chanIdx
			}
		} else {
			var chanIdx, prev int
			prev = -1
			// Copy audio to the channel(s) where it belongs
			for {
				chanIdx = get_mono_channel(&st.layout, s, prev)
				if chanIdx == -1 {
					break
				}
				copy_channel_out(pcm, st.layout.nb_channels, chanIdx,
					buf, 1, frame_size, user_data)
				prev = chanIdx
			}
		}
	}
	// Handle muted channels
	for c = 0; c < st.layout.nb_channels; c++ {
		if st.layout.mapping[c] == 255 {
			copy_channel_out(pcm, st.layout.nb_channels, c,
				nil, 0, frame_size, user_data)
		}
	}
	return frame_size
}

// C: static void opus_copy_channel_out_float(void *dst, int dst_stride, int dst_channel, const opus_val16 *src, int src_stride, int frame_size, void *user_data)
func opus_copy_channel_out_float(dst any, dst_stride int, dst_channel int, src []int16, src_stride int, frame_size int, user_data any) {
	float_dst := dst.([]float32)
	var i int32
	_ = user_data
	if src != nil {
		for i = 0; i < int32(frame_size); i++ {
			float_dst[int(i)*dst_stride+dst_channel] = (1 / 32768.0) * float32(src[int(i)*src_stride])
		}
	} else {
		for i = 0; i < int32(frame_size); i++ {
			float_dst[int(i)*dst_stride+dst_channel] = 0
		}
	}
}

// C: static void opus_copy_channel_out_short(void *dst, int dst_stride, int dst_channel, const opus_val16 *src, int src_stride, int frame_size, void *user_data)
func opus_copy_channel_out_short(dst any, dst_stride int, dst_channel int, src []int16, src_stride int, frame_size int, user_data any) {
	short_dst := dst.([]int16)
	var i int32
	_ = user_data
	if src != nil {
		for i = 0; i < int32(frame_size); i++ {
			short_dst[int(i)*dst_stride+dst_channel] = src[int(i)*src_stride]
		}
	} else {
		for i = 0; i < int32(frame_size); i++ {
			short_dst[int(i)*dst_stride+dst_channel] = 0
		}
	}
}

// C: int opus_multistream_decode(OpusMSDecoder *st, const unsigned char *data, opus_int32 len, opus_int16 *pcm, int frame_size, int decode_fec)   (FIXED_POINT)
func (st *OpusMSDecoder) MultistreamDecode(data []byte, length int32, pcm []int16, frame_size int, decode_fec int) int {
	return st.MultistreamDecodeNative(data, length,
		pcm, opus_copy_channel_out_short, frame_size, decode_fec, 0, nil)
}

// C: int opus_multistream_decode_float(OpusMSDecoder *st, const unsigned char *data, opus_int32 len, float *pcm, int frame_size, int decode_fec)   (FIXED_POINT)
func (st *OpusMSDecoder) MultistreamDecodeFloat(data []byte, length int32, pcm []float32, frame_size int, decode_fec int) int {
	return st.MultistreamDecodeNative(data, length,
		pcm, opus_copy_channel_out_float, frame_size, decode_fec, 0, nil)
}

// Ctl is the C opus_multistream_decoder_ctl_va_list; the variadic Go form
// is also the va_list form, so opus_multistream_decoder_ctl (which only
// wraps va_start/va_end around it) is the same function.
//
// C: int opus_multistream_decoder_ctl_va_list(OpusMSDecoder *st, int request, va_list ap)
// C: int opus_multistream_decoder_ctl(OpusMSDecoder *st, int request, ...)
func (st *OpusMSDecoder) Ctl(request int, args ...any) int {
	var ret int = OPUS_OK

	switch request {
	case OPUS_GET_BANDWIDTH_REQUEST,
		OPUS_GET_SAMPLE_RATE_REQUEST,
		OPUS_GET_GAIN_REQUEST,
		OPUS_GET_LAST_PACKET_DURATION_REQUEST,
		OPUS_GET_PHASE_INVERSION_DISABLED_REQUEST:
		var dec *OpusDecoder
		// For int32* GET params, just query the first stream
		value, ok := args[0].(*int32)
		if !ok {
			return OPUS_BAD_ARG
		}
		dec = st.decoders[0]
		ret = dec.Ctl(request, value)
	case OPUS_GET_FINAL_RANGE_REQUEST:
		var s int
		value, ok := args[0].(*uint32)
		var tmp uint32
		if !ok || value == nil {
			return OPUS_BAD_ARG // goto bad_arg
		}
		*value = 0
		for s = 0; s < st.layout.nb_streams; s++ {
			var dec *OpusDecoder
			dec = st.decoders[s]
			ret = dec.Ctl(request, &tmp)
			if ret != OPUS_OK {
				break
			}
			*value ^= tmp
		}
	case OPUS_RESET_STATE:
		var s int
		for s = 0; s < st.layout.nb_streams; s++ {
			var dec *OpusDecoder

			dec = st.decoders[s]
			ret = dec.Ctl(OPUS_RESET_STATE)
			if ret != OPUS_OK {
				break
			}
		}
	case OPUS_MULTISTREAM_GET_DECODER_STATE_REQUEST:
		stream_id, ok := argInt32(args, 0)
		if !ok {
			return OPUS_BAD_ARG
		}
		if stream_id < 0 || int(stream_id) >= st.layout.nb_streams {
			return OPUS_BAD_ARG // goto bad_arg
		}
		value, ok := args[1].(**OpusDecoder)
		if !ok || value == nil {
			return OPUS_BAD_ARG // goto bad_arg
		}
		*value = st.decoders[stream_id]
	case OPUS_SET_GAIN_REQUEST,
		OPUS_SET_PHASE_INVERSION_DISABLED_REQUEST:
		var s int
		// This works for int32 params
		value, ok := argInt32(args, 0)
		if !ok {
			return OPUS_BAD_ARG
		}
		for s = 0; s < st.layout.nb_streams; s++ {
			var dec *OpusDecoder

			dec = st.decoders[s]
			ret = dec.Ctl(request, value)
			if ret != OPUS_OK {
				break
			}
		}
	default:
		ret = OPUS_UNIMPLEMENTED
	}

	return ret
}

// C: void opus_multistream_decoder_destroy(OpusMSDecoder *st)
func (st *OpusMSDecoder) MultistreamDecoderDestroy() {}
