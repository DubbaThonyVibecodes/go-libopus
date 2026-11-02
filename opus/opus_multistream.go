package opus

// C: src/opus_multistream.c, include/opus_multistream.h

// ChannelLayout is the C struct ChannelLayout.
type ChannelLayout struct {
	nb_channels        int
	nb_streams         int
	nb_coupled_streams int
	mapping            [256]byte
}

type VorbisLayout struct {
	nb_streams         int
	nb_coupled_streams int
	mapping            [8]byte
}

// Index is nb_channel-1
var vorbis_mappings = [8]VorbisLayout{
	{1, 0, [8]byte{0}},                      // 1: mono
	{1, 1, [8]byte{0, 1}},                   // 2: stereo
	{2, 1, [8]byte{0, 2, 1}},                // 3: 1-d surround
	{2, 2, [8]byte{0, 1, 2, 3}},             // 4: quadraphonic surround
	{3, 2, [8]byte{0, 4, 1, 2, 3}},          // 5: 5-channel surround
	{4, 2, [8]byte{0, 4, 1, 2, 3, 5}},       // 6: 5.1 surround
	{4, 3, [8]byte{0, 4, 1, 2, 3, 5, 6}},    // 7: 6.1 surround
	{5, 3, [8]byte{0, 6, 1, 2, 3, 4, 5, 7}}, // 8: 7.1 surround
}

// OpusMSEncoder is the C struct OpusMSEncoder; the per-stream encoder states
// that C places after the struct are the encoders slice (coupled streams first).
type OpusMSEncoder struct {
	layout      ChannelLayout
	bitrate_bps int32
	surround    int
	lfe_stream  int
	encoders    []*OpusEncoder
}

// OpusMSDecoder is the C struct OpusMSDecoder.
type OpusMSDecoder struct {
	layout   ChannelLayout
	decoders []*OpusDecoder
}

// C: static int validate_layout(const ChannelLayout *layout)
func validate_layout(layout *ChannelLayout) int {
	var i, max_channel int

	max_channel = layout.nb_streams + layout.nb_coupled_streams
	if max_channel > 255 {
		return 0
	}
	for i = 0; i < layout.nb_channels; i++ {
		if int(layout.mapping[i]) >= max_channel && layout.mapping[i] != 255 {
			return 0
		}
	}
	return 1
}

// C: static int get_left_channel(const ChannelLayout *layout, int stream_id, int prev)
func get_left_channel(layout *ChannelLayout, stream_id int, prev int) int {
	var i int
	if prev < 0 {
		i = 0
	} else {
		i = prev + 1
	}
	for ; i < layout.nb_channels; i++ {
		if int(layout.mapping[i]) == stream_id*2 {
			return i
		}
	}
	return -1
}

// C: static int get_right_channel(const ChannelLayout *layout, int stream_id, int prev)
func get_right_channel(layout *ChannelLayout, stream_id int, prev int) int {
	var i int
	if prev < 0 {
		i = 0
	} else {
		i = prev + 1
	}
	for ; i < layout.nb_channels; i++ {
		if int(layout.mapping[i]) == stream_id*2+1 {
			return i
		}
	}
	return -1
}

// C: static int get_mono_channel(const ChannelLayout *layout, int stream_id, int prev)
func get_mono_channel(layout *ChannelLayout, stream_id int, prev int) int {
	var i int
	if prev < 0 {
		i = 0
	} else {
		i = prev + 1
	}
	for ; i < layout.nb_channels; i++ {
		if int(layout.mapping[i]) == stream_id+layout.nb_coupled_streams {
			return i
		}
	}
	return -1
}

// C: static int validate_encoder_layout(const ChannelLayout *layout)
func validate_encoder_layout(layout *ChannelLayout) int {
	var s int
	for s = 0; s < layout.nb_streams; s++ {
		if s < layout.nb_coupled_streams {
			if get_left_channel(layout, s, -1) == -1 {
				return 0
			}
			if get_right_channel(layout, s, -1) == -1 {
				return 0
			}
		} else {
			if get_mono_channel(layout, s, -1) == -1 {
				return 0
			}
		}
	}
	return 1
}

// C: opus_int32 opus_multistream_encoder_get_size(int nb_streams, int nb_coupled_streams)
func MultistreamEncoderGetSize(nb_streams int, nb_coupled_streams int) int32 {
	var coupled_size, mono_size int

	if nb_streams < 1 || nb_coupled_streams > nb_streams || nb_coupled_streams < 0 {
		return 0
	}
	coupled_size = EncoderGetSize(2)
	mono_size = EncoderGetSize(1)
	return int32(align(272) +
		nb_coupled_streams*align(coupled_size) +
		(nb_streams-nb_coupled_streams)*align(mono_size))
}

// C: opus_int32 opus_multistream_surround_encoder_get_size(int channels, int mapping_family)
func MultistreamSurroundEncoderGetSize(channels int, mapping_family int) int32 {
	var nb_streams, nb_coupled_streams int
	var size int32

	if mapping_family == 0 {
		if channels == 1 {
			nb_streams = 1
			nb_coupled_streams = 0
		} else if channels == 2 {
			nb_streams = 1
			nb_coupled_streams = 1
		} else {
			return 0
		}
	} else if mapping_family == 1 && channels <= 8 && channels >= 1 {
		nb_streams = vorbis_mappings[channels-1].nb_streams
		nb_coupled_streams = vorbis_mappings[channels-1].nb_coupled_streams
	} else if mapping_family == 255 {
		nb_streams = channels
		nb_coupled_streams = 0
	} else {
		return 0
	}
	size = MultistreamEncoderGetSize(nb_streams, nb_coupled_streams)
	return size
}

// C: static int opus_multistream_encoder_init_impl(OpusMSEncoder *st, opus_int32 Fs, int channels, int streams, int coupled_streams, const unsigned char *mapping, int application, int surround)
func (st *OpusMSEncoder) multistreamEncoderInitImpl(Fs int32, channels int, streams int, coupled_streams int, mapping []byte, application int, surround int) int {
	var i, ret int

	if (channels > 255) || (channels < 1) || (coupled_streams > streams) ||
		(coupled_streams+streams > 255) || (streams < 1) || (coupled_streams < 0) {
		return OPUS_BAD_ARG
	}

	st.layout.nb_channels = channels
	st.layout.nb_streams = streams
	st.layout.nb_coupled_streams = coupled_streams
	if surround == 0 {
		st.lfe_stream = -1
	}
	st.bitrate_bps = OPUS_AUTO
	for i = 0; i < st.layout.nb_channels; i++ {
		st.layout.mapping[i] = mapping[i]
	}
	if validate_layout(&st.layout) == 0 || validate_encoder_layout(&st.layout) == 0 {
		return OPUS_BAD_ARG
	}
	st.encoders = make([]*OpusEncoder, streams)

	for i = 0; i < st.layout.nb_coupled_streams; i++ {
		st.encoders[i] = new(OpusEncoder)
		ret = st.encoders[i].EncoderInit(Fs, 2, application)
		if ret != OPUS_OK {
			return ret
		}
	}
	for ; i < st.layout.nb_streams; i++ {
		st.encoders[i] = new(OpusEncoder)
		ret = st.encoders[i].EncoderInit(Fs, 1, application)
		if ret != OPUS_OK {
			return ret
		}
	}
	st.surround = surround
	return OPUS_OK
}

// C: int opus_multistream_encoder_init(OpusMSEncoder *st, opus_int32 Fs, int channels, int streams, int coupled_streams, const unsigned char *mapping, int application)
func (st *OpusMSEncoder) MultistreamEncoderInit(Fs int32, channels int, streams int, coupled_streams int, mapping []byte, application int) int {
	return st.multistreamEncoderInitImpl(Fs, channels, streams, coupled_streams, mapping, application, 0)
}

// C: int opus_multistream_surround_encoder_init(OpusMSEncoder *st, opus_int32 Fs, int channels, int mapping_family, int *streams, int *coupled_streams, unsigned char *mapping, int application)
func (st *OpusMSEncoder) MultistreamSurroundEncoderInit(Fs int32, channels int, mapping_family int, streams *int, coupled_streams *int, mapping []byte, application int) int {
	if (channels > 255) || (channels < 1) {
		return OPUS_BAD_ARG
	}
	st.lfe_stream = -1
	if mapping_family == 0 {
		if channels == 1 {
			*streams = 1
			*coupled_streams = 0
			mapping[0] = 0
		} else if channels == 2 {
			*streams = 1
			*coupled_streams = 1
			mapping[0] = 0
			mapping[1] = 1
		} else {
			return OPUS_UNIMPLEMENTED
		}
	} else if mapping_family == 1 && channels <= 8 && channels >= 1 {
		var i int
		*streams = vorbis_mappings[channels-1].nb_streams
		*coupled_streams = vorbis_mappings[channels-1].nb_coupled_streams
		for i = 0; i < channels; i++ {
			mapping[i] = vorbis_mappings[channels-1].mapping[i]
		}
		if channels >= 6 {
			st.lfe_stream = *streams - 1
		}
	} else if mapping_family == 255 {
		var i int
		*streams = channels
		*coupled_streams = 0
		for i = 0; i < channels; i++ {
			mapping[i] = byte(i)
		}
	} else {
		return OPUS_UNIMPLEMENTED
	}
	return st.multistreamEncoderInitImpl(Fs, channels, *streams, *coupled_streams,
		mapping, application, b2i(channels > 2 && mapping_family == 1))
}

// C: OpusMSEncoder *opus_multistream_encoder_create(opus_int32 Fs, int channels, int streams, int coupled_streams, const unsigned char *mapping, int application, int *error)
func MultistreamEncoderCreate(Fs int32, channels int, streams int, coupled_streams int, mapping []byte, application int, error *int) *OpusMSEncoder {
	var ret int
	if (channels > 255) || (channels < 1) || (coupled_streams > streams) ||
		(coupled_streams+streams > 255) || (streams < 1) || (coupled_streams < 0) {
		if error != nil {
			*error = OPUS_BAD_ARG
		}
		return nil
	}
	st := new(OpusMSEncoder)
	ret = st.MultistreamEncoderInit(Fs, channels, streams, coupled_streams, mapping, application)
	if ret != OPUS_OK {
		st = nil
	}
	if error != nil {
		*error = ret
	}
	return st
}

// C: OpusMSEncoder *opus_multistream_surround_encoder_create(opus_int32 Fs, int channels, int mapping_family, int *streams, int *coupled_streams, unsigned char *mapping, int application, int *error)
func MultistreamSurroundEncoderCreate(Fs int32, channels int, mapping_family int, streams *int, coupled_streams *int, mapping []byte, application int, error *int) *OpusMSEncoder {
	var ret int
	if (channels > 255) || (channels < 1) {
		if error != nil {
			*error = OPUS_BAD_ARG
		}
		return nil
	}
	st := new(OpusMSEncoder)
	ret = st.MultistreamSurroundEncoderInit(Fs, channels, mapping_family, streams, coupled_streams, mapping, application)
	if ret != OPUS_OK {
		st = nil
	}
	if error != nil {
		*error = ret
	}
	return st
}

// C: typedef void (*opus_copy_channel_in_func)(opus_val16 *dst, int dst_stride, const void *src, int src_stride, int src_channel, int frame_size)
type opus_copy_channel_in_func func(dst []int16, dst_stride int, src any, src_stride int, src_channel int, frame_size int)

// C: static void surround_rate_allocation(OpusMSEncoder *st, opus_int32 *rate, int frame_size)
func (st *OpusMSEncoder) surround_rate_allocation(rate []int32, frame_size int) {
	var i int
	var channel_rate int32
	var Fs int32
	var stream_offset int32
	var lfe_offset int32
	var coupled_ratio int32 // Q8
	var lfe_ratio int32     // Q8

	st.encoders[0].Ctl(OPUS_GET_SAMPLE_RATE_REQUEST, &Fs)

	// We start by giving each stream (coupled or uncoupled) the same bitrate.
	// This models the main saving of coupled channels over uncoupled.
	stream_offset = 20000
	// The LFE stream is an exception to the above and gets fewer bits.
	lfe_offset = 7000
	// Coupled streams get twice the mono rate after the first 20 kb/s.
	coupled_ratio = 512
	// Should depend on the bitrate, for now we assume LFE gets 3/8 the bits of mono
	lfe_ratio = 96

	// Compute bitrate allocation between streams
	if st.bitrate_bps == OPUS_AUTO {
		channel_rate = Fs + 60*Fs/int32(frame_size)
	} else if st.bitrate_bps == OPUS_BITRATE_MAX {
		channel_rate = 300000
	} else {
		var nb_lfe, nb_uncoupled, nb_coupled, total int32
		nb_lfe = int32(b2i(st.lfe_stream != -1))
		nb_coupled = int32(st.layout.nb_coupled_streams)
		nb_uncoupled = int32(st.layout.nb_streams) - nb_coupled - nb_lfe
		total = (nb_uncoupled << 8) + // mono
			coupled_ratio*nb_coupled + // stereo
			nb_lfe*lfe_ratio
		channel_rate = 256 * (st.bitrate_bps - lfe_offset*nb_lfe - stream_offset*(nb_coupled+nb_uncoupled)) / total
	}

	for i = 0; i < st.layout.nb_streams; i++ {
		if i < st.layout.nb_coupled_streams {
			rate[i] = stream_offset + (channel_rate * coupled_ratio >> 8)
		} else if i != st.lfe_stream {
			rate[i] = stream_offset + channel_rate
		} else {
			rate[i] = lfe_offset + (channel_rate * lfe_ratio >> 8)
		}
	}
}

// Max size in case the encoder decides to return three frames
const MS_FRAME_TMP = (3*1275 + 7)

// C: static int opus_multistream_encode_native(OpusMSEncoder *st, opus_copy_channel_in_func copy_channel_in, const void *pcm, int frame_size, unsigned char *data, opus_int32 max_data_bytes)
func (st *OpusMSEncoder) multistreamEncodeNative(copy_channel_in opus_copy_channel_in_func, pcm any, frame_size int, data []byte, max_data_bytes int32) int {
	var Fs int32
	var s int
	var tot_size int
	var tmp_data [MS_FRAME_TMP]byte
	var rp OpusRepacketizer
	var bitrates [256]int32

	st.encoders[0].Ctl(OPUS_GET_SAMPLE_RATE_REQUEST, &Fs)

	if 400*int32(frame_size) < Fs {
		return OPUS_BAD_ARG
	}
	// Validate frame_size before using it to allocate stack space.
	// This mirrors the checks in opus_encode[_float]().
	if 400*int32(frame_size) != Fs && 200*int32(frame_size) != Fs &&
		100*int32(frame_size) != Fs && 50*int32(frame_size) != Fs &&
		25*int32(frame_size) != Fs && 50*int32(frame_size) != 3*Fs {
		return OPUS_BAD_ARG
	}
	buf := make([]int16, 2*frame_size)

	if max_data_bytes < int32(4*st.layout.nb_streams-1) {
		return OPUS_BUFFER_TOO_SMALL
	}
	// Compute bitrate allocation between streams (this could be a lot better)
	st.surround_rate_allocation(bitrates[:], frame_size)

	for s = 0; s < st.layout.nb_streams; s++ {
		enc := st.encoders[s]
		enc.Ctl(OPUS_SET_BITRATE_REQUEST, bitrates[s])
		if st.surround != 0 {
			enc.Ctl(OPUS_SET_FORCE_MODE_REQUEST, int32(MODE_CELT_ONLY))
			enc.Ctl(OPUS_SET_BANDWIDTH_REQUEST, int32(OPUS_BANDWIDTH_FULLBAND))
			if s < st.layout.nb_coupled_streams {
				enc.Ctl(OPUS_SET_FORCE_CHANNELS_REQUEST, int32(2))
			}
		}
	}

	// Counting ToC
	tot_size = 0
	for s = 0; s < st.layout.nb_streams; s++ {
		var length int32
		var curr_max int32

		rp.RepacketizerInit()
		enc := st.encoders[s]
		if s < st.layout.nb_coupled_streams {
			var left, right int
			left = get_left_channel(&st.layout, s, -1)
			right = get_right_channel(&st.layout, s, -1)
			copy_channel_in(buf, 2,
				pcm, st.layout.nb_channels, left, frame_size)
			copy_channel_in(buf[1:], 2,
				pcm, st.layout.nb_channels, right, frame_size)
		} else {
			chanIdx := get_mono_channel(&st.layout, s, -1)
			copy_channel_in(buf, 1,
				pcm, st.layout.nb_channels, chanIdx, frame_size)
		}
		// number of bytes left (+Toc)
		curr_max = max_data_bytes - int32(tot_size)
		// Reserve three bytes for the last stream and four for the others
		curr_max -= int32(IMAX(0, 4*(st.layout.nb_streams-s-1)-1))
		curr_max = IMIN(curr_max, MS_FRAME_TMP)
		length = enc.Encode(buf, frame_size, tmp_data[:], curr_max)
		if length < 0 {
			return int(length)
		}
		// We need to use the repacketizer to add the self-delimiting lengths
		// while taking into account the fact that the encoder can now return
		// more than one frame at a time (e.g. 60 ms CELT-only)
		rp.RepacketizerCat(tmp_data[:], length)
		length = rp.repacketizerOutRangeImpl(0, rp.RepacketizerGetNbFrames(), data, max_data_bytes-int32(tot_size), b2i(s != st.layout.nb_streams-1))
		data = data[length:]
		tot_size += int(length)
	}
	return tot_size
}

// C: static void opus_copy_channel_in_float(opus_val16 *dst, int dst_stride, const void *src, int src_stride, int src_channel, int frame_size)
func opus_copy_channel_in_float(dst []int16, dst_stride int, src any, src_stride int, src_channel int, frame_size int) {
	float_src := src.([]float32)
	var i int
	for i = 0; i < frame_size; i++ {
		dst[i*dst_stride] = FLOAT2INT16(float_src[i*src_stride+src_channel])
	}
}

// C: static void opus_copy_channel_in_short(opus_val16 *dst, int dst_stride, const void *src, int src_stride, int src_channel, int frame_size)
func opus_copy_channel_in_short(dst []int16, dst_stride int, src any, src_stride int, src_channel int, frame_size int) {
	short_src := src.([]int16)
	var i int
	for i = 0; i < frame_size; i++ {
		dst[i*dst_stride] = short_src[i*src_stride+src_channel]
	}
}

// C: int opus_multistream_encode(OpusMSEncoder *st, const opus_val16 *pcm, int frame_size, unsigned char *data, opus_int32 max_data_bytes)
func (st *OpusMSEncoder) MultistreamEncode(pcm []int16, frame_size int, data []byte, max_data_bytes int32) int {
	return st.multistreamEncodeNative(opus_copy_channel_in_short,
		pcm, frame_size, data, max_data_bytes)
}

// C: int opus_multistream_encode_float(OpusMSEncoder *st, const float *pcm, int frame_size, unsigned char *data, opus_int32 max_data_bytes)
func (st *OpusMSEncoder) MultistreamEncodeFloat(pcm []float32, frame_size int, data []byte, max_data_bytes int32) int {
	return st.multistreamEncodeNative(opus_copy_channel_in_float,
		pcm, frame_size, data, max_data_bytes)
}

// Ctl is the C opus_multistream_encoder_ctl.
//
// C: int opus_multistream_encoder_ctl(OpusMSEncoder *st, int request, ...)
func (st *OpusMSEncoder) Ctl(request int, args ...any) int {
	var ret int = OPUS_OK

	switch request {
	case OPUS_SET_BITRATE_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok || (value < 0 && value != OPUS_AUTO && value != OPUS_BITRATE_MAX) {
			return OPUS_BAD_ARG
		}
		st.bitrate_bps = value
	case OPUS_GET_BITRATE_REQUEST:
		var s int
		value, ok := args[0].(*int32)
		if !ok {
			return OPUS_BAD_ARG
		}
		*value = 0
		for s = 0; s < st.layout.nb_streams; s++ {
			var rate int32
			enc := st.encoders[s]
			enc.Ctl(request, &rate)
			*value += rate
		}
	case OPUS_GET_LSB_DEPTH_REQUEST,
		OPUS_GET_VBR_REQUEST,
		OPUS_GET_APPLICATION_REQUEST,
		OPUS_GET_BANDWIDTH_REQUEST,
		OPUS_GET_COMPLEXITY_REQUEST,
		OPUS_GET_PACKET_LOSS_PERC_REQUEST,
		OPUS_GET_DTX_REQUEST,
		OPUS_GET_VOICE_RATIO_REQUEST,
		OPUS_GET_VBR_CONSTRAINT_REQUEST,
		OPUS_GET_SIGNAL_REQUEST,
		OPUS_GET_LOOKAHEAD_REQUEST,
		OPUS_GET_SAMPLE_RATE_REQUEST,
		OPUS_GET_INBAND_FEC_REQUEST,
		OPUS_GET_FORCE_CHANNELS_REQUEST:
		// For int32* GET params, just query the first stream
		value, ok := args[0].(*int32)
		if !ok {
			return OPUS_BAD_ARG
		}
		enc := st.encoders[0]
		ret = enc.Ctl(request, value)
	case OPUS_GET_FINAL_RANGE_REQUEST:
		var s int
		value, ok := args[0].(*uint32)
		if !ok {
			return OPUS_BAD_ARG
		}
		var tmp uint32
		*value = 0
		for s = 0; s < st.layout.nb_streams; s++ {
			enc := st.encoders[s]
			ret = enc.Ctl(request, &tmp)
			if ret != OPUS_OK {
				break
			}
			*value ^= tmp
		}
	case OPUS_SET_LSB_DEPTH_REQUEST,
		OPUS_SET_COMPLEXITY_REQUEST,
		OPUS_SET_VBR_REQUEST,
		OPUS_SET_VBR_CONSTRAINT_REQUEST,
		OPUS_SET_BANDWIDTH_REQUEST,
		OPUS_SET_SIGNAL_REQUEST,
		OPUS_SET_APPLICATION_REQUEST,
		OPUS_SET_INBAND_FEC_REQUEST,
		OPUS_SET_PACKET_LOSS_PERC_REQUEST,
		OPUS_SET_DTX_REQUEST,
		OPUS_SET_FORCE_MODE_REQUEST,
		OPUS_SET_FORCE_CHANNELS_REQUEST:
		var s int
		// This works for int32 params
		value, ok := argInt32(args, 0)
		if !ok {
			return OPUS_BAD_ARG
		}
		for s = 0; s < st.layout.nb_streams; s++ {
			enc := st.encoders[s]
			ret = enc.Ctl(request, value)
			if ret != OPUS_OK {
				break
			}
		}
	case OPUS_MULTISTREAM_GET_ENCODER_STATE_REQUEST:
		stream_id, ok := argInt32(args, 0)
		if !ok {
			return OPUS_BAD_ARG
		}
		if stream_id < 0 || int(stream_id) >= st.layout.nb_streams {
			ret = OPUS_BAD_ARG
		}
		value, ok := args[1].(**OpusEncoder)
		if !ok {
			return OPUS_BAD_ARG
		}
		if ret == OPUS_OK {
			*value = st.encoders[stream_id]
		}
	default:
		ret = OPUS_UNIMPLEMENTED
	}

	return ret
}

// C: void opus_multistream_encoder_destroy(OpusMSEncoder *st)
func (st *OpusMSEncoder) MultistreamEncoderDestroy() {}

/* DECODER */

// C: opus_int32 opus_multistream_decoder_get_size(int nb_streams, int nb_coupled_streams)
func MultistreamDecoderGetSize(nb_streams int, nb_coupled_streams int) int32 {
	var coupled_size, mono_size int

	if nb_streams < 1 || nb_coupled_streams > nb_streams || nb_coupled_streams < 0 {
		return 0
	}
	coupled_size = DecoderGetSize(2)
	mono_size = DecoderGetSize(1)
	return int32(align(268) +
		nb_coupled_streams*align(coupled_size) +
		(nb_streams-nb_coupled_streams)*align(mono_size))
}

// C: int opus_multistream_decoder_init(OpusMSDecoder *st, opus_int32 Fs, int channels, int streams, int coupled_streams, const unsigned char *mapping)
func (st *OpusMSDecoder) MultistreamDecoderInit(Fs int32, channels int, streams int, coupled_streams int, mapping []byte) int {
	var i, ret int

	if (channels > 255) || (channels < 1) || (coupled_streams > streams) ||
		(coupled_streams+streams > 255) || (streams < 1) || (coupled_streams < 0) {
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
		(coupled_streams+streams > 255) || (streams < 1) || (coupled_streams < 0) {
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

// C: typedef void (*opus_copy_channel_out_func)(void *dst, int dst_stride, int dst_channel, const opus_val16 *src, int src_stride, int frame_size)
type opus_copy_channel_out_func func(dst any, dst_stride int, dst_channel int, src []int16, src_stride int, frame_size int)

// C: static int opus_multistream_decode_native(OpusMSDecoder *st, const unsigned char *data, opus_int32 len, void *pcm, opus_copy_channel_out_func copy_channel_out, int frame_size, int decode_fec)
func (st *OpusMSDecoder) multistreamDecodeNative(data []byte, length int32, pcm any, copy_channel_out opus_copy_channel_out_func, frame_size int, decode_fec int) int {
	var Fs int32
	var s, c int
	var do_plc int = 0

	// Limit frame_size to avoid excessive stack allocations.
	st.Ctl(OPUS_GET_SAMPLE_RATE_REQUEST, &Fs)
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
	for s = 0; s < st.layout.nb_streams; s++ {
		var packet_offset, ret int

		dec := st.decoders[s]

		if do_plc == 0 && length <= 0 {
			return OPUS_INVALID_PACKET
		}
		packet_offset = 0
		ret = dec.DecodeNative(data, length, buf, frame_size, decode_fec, b2i(s != st.layout.nb_streams-1), &packet_offset)
		if data != nil {
			data = data[packet_offset:]
		}
		length -= int32(packet_offset)
		if ret > frame_size {
			return OPUS_BUFFER_TOO_SMALL
		}
		if s > 0 && ret != frame_size {
			return OPUS_INVALID_PACKET
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
					buf, 2, frame_size)
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
					buf[1:], 2, frame_size)
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
					buf, 1, frame_size)
				prev = chanIdx
			}
		}
	}
	// Handle muted channels
	for c = 0; c < st.layout.nb_channels; c++ {
		if st.layout.mapping[c] == 255 {
			copy_channel_out(pcm, st.layout.nb_channels, c,
				nil, 0, frame_size)
		}
	}
	return frame_size
}

// C: static void opus_copy_channel_out_float(void *dst, int dst_stride, int dst_channel, const opus_val16 *src, int src_stride, int frame_size)
func opus_copy_channel_out_float(dst any, dst_stride int, dst_channel int, src []int16, src_stride int, frame_size int) {
	float_dst := dst.([]float32)
	var i int
	if src != nil {
		for i = 0; i < frame_size; i++ {
			float_dst[i*dst_stride+dst_channel] = (1 / 32768.0) * float32(src[i*src_stride])
		}
	} else {
		for i = 0; i < frame_size; i++ {
			float_dst[i*dst_stride+dst_channel] = 0
		}
	}
}

// C: static void opus_copy_channel_out_short(void *dst, int dst_stride, int dst_channel, const opus_val16 *src, int src_stride, int frame_size)
func opus_copy_channel_out_short(dst any, dst_stride int, dst_channel int, src []int16, src_stride int, frame_size int) {
	short_dst := dst.([]int16)
	var i int
	if src != nil {
		for i = 0; i < frame_size; i++ {
			short_dst[i*dst_stride+dst_channel] = src[i*src_stride]
		}
	} else {
		for i = 0; i < frame_size; i++ {
			short_dst[i*dst_stride+dst_channel] = 0
		}
	}
}

// C: int opus_multistream_decode(OpusMSDecoder *st, const unsigned char *data, opus_int32 len, opus_int16 *pcm, int frame_size, int decode_fec)
func (st *OpusMSDecoder) MultistreamDecode(data []byte, length int32, pcm []int16, frame_size int, decode_fec int) int {
	return st.multistreamDecodeNative(data, length,
		pcm, opus_copy_channel_out_short, frame_size, decode_fec)
}

// C: int opus_multistream_decode_float(OpusMSDecoder *st, const unsigned char *data, opus_int32 len, float *pcm, int frame_size, int decode_fec)
func (st *OpusMSDecoder) MultistreamDecodeFloat(data []byte, length int32, pcm []float32, frame_size int, decode_fec int) int {
	return st.multistreamDecodeNative(data, length,
		pcm, opus_copy_channel_out_float, frame_size, decode_fec)
}

// Ctl is the C opus_multistream_decoder_ctl.
//
// C: int opus_multistream_decoder_ctl(OpusMSDecoder *st, int request, ...)
func (st *OpusMSDecoder) Ctl(request int, args ...any) int {
	var ret int = OPUS_OK

	switch request {
	case OPUS_GET_BANDWIDTH_REQUEST,
		OPUS_GET_SAMPLE_RATE_REQUEST:
		// For int32* GET params, just query the first stream
		value, ok := args[0].(*int32)
		if !ok {
			return OPUS_BAD_ARG
		}
		dec := st.decoders[0]
		ret = dec.Ctl(request, value)
	case OPUS_GET_FINAL_RANGE_REQUEST:
		var s int
		value, ok := args[0].(*uint32)
		if !ok {
			return OPUS_BAD_ARG
		}
		var tmp uint32
		*value = 0
		for s = 0; s < st.layout.nb_streams; s++ {
			dec := st.decoders[s]
			ret = dec.Ctl(request, &tmp)
			if ret != OPUS_OK {
				break
			}
			*value ^= tmp
		}
	case OPUS_RESET_STATE:
		var s int
		for s = 0; s < st.layout.nb_streams; s++ {
			dec := st.decoders[s]
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
			ret = OPUS_BAD_ARG
		}
		value, ok := args[1].(**OpusDecoder)
		if !ok {
			return OPUS_BAD_ARG
		}
		if ret == OPUS_OK {
			*value = st.decoders[stream_id]
		}
	case OPUS_SET_GAIN_REQUEST:
		var s int
		// This works for int32 params
		value, ok := argInt32(args, 0)
		if !ok {
			return OPUS_BAD_ARG
		}
		for s = 0; s < st.layout.nb_streams; s++ {
			dec := st.decoders[s]
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
