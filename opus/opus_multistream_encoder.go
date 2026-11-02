package opus

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// C: src/opus_multistream_encoder.c, include/opus_multistream.h

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

// C: typedef void (*opus_copy_channel_in_func)(opus_val16 *dst, int dst_stride, const void *src, int src_stride, int src_channel, int frame_size)
type opus_copy_channel_in_func func(dst []int16, dst_stride int, src any, src_stride int, src_channel int, frame_size int)

// C: typedef enum { MAPPING_TYPE_NONE, MAPPING_TYPE_SURROUND } MappingType
type MappingType int

const (
	MAPPING_TYPE_NONE MappingType = iota
	MAPPING_TYPE_SURROUND
)

// OpusMSEncoder is the C struct OpusMSEncoder; the per-stream encoder states
// that C places after the struct are the encoders slice (coupled streams
// first), followed by window_mem[channels*120] and preemph_mem[channels].
type OpusMSEncoder struct {
	layout            ChannelLayout
	arch              int
	lfe_stream        int
	application       int
	variable_duration int
	mapping_type      MappingType
	bitrate_bps       int32
	subframe_mem      [3]float32
	// Encoder states go here
	encoders []*OpusEncoder
	// then opus_val32 window_mem[channels*120];
	window_mem []int32
	// then opus_val32 preemph_mem[channels];
	preemph_mem []int32
}

// C: static opus_val32 *ms_get_preemph_mem(OpusMSEncoder *st)
func (st *OpusMSEncoder) ms_get_preemph_mem() []int32 {
	return st.preemph_mem
}

// C: static opus_val32 *ms_get_window_mem(OpusMSEncoder *st)
func (st *OpusMSEncoder) ms_get_window_mem() []int32 {
	return st.window_mem
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

// C: static void channel_pos(int channels, int pos[8])
func channel_pos(channels int, pos []int) {
	// Position in the mix: 0 don't mix, 1: left, 2: center, 3:right
	if channels == 4 {
		pos[0] = 1
		pos[1] = 3
		pos[2] = 1
		pos[3] = 3
	} else if channels == 3 || channels == 5 || channels == 6 {
		pos[0] = 1
		pos[1] = 2
		pos[2] = 3
		pos[3] = 1
		pos[4] = 3
		pos[5] = 0
	} else if channels == 7 {
		pos[0] = 1
		pos[1] = 2
		pos[2] = 3
		pos[3] = 1
		pos[4] = 3
		pos[5] = 2
		pos[6] = 0
	} else if channels == 8 {
		pos[0] = 1
		pos[1] = 2
		pos[2] = 3
		pos[3] = 1
		pos[4] = 3
		pos[5] = 1
		pos[6] = 3
		pos[7] = 0
	}
}

var logSum_diff_table = [17]int16{
	celt.QCONST16(0.5000000, celt.DB_SHIFT), celt.QCONST16(0.2924813, celt.DB_SHIFT), celt.QCONST16(0.1609640, celt.DB_SHIFT), celt.QCONST16(0.0849625, celt.DB_SHIFT),
	celt.QCONST16(0.0437314, celt.DB_SHIFT), celt.QCONST16(0.0221971, celt.DB_SHIFT), celt.QCONST16(0.0111839, celt.DB_SHIFT), celt.QCONST16(0.0056136, celt.DB_SHIFT),
	celt.QCONST16(0.0028123, celt.DB_SHIFT),
}

// Computes a rough approximation of log2(2^a + 2^b)
//
// C: static opus_val16 logSum(opus_val16 a, opus_val16 b)
func logSum(a int16, b int16) int16 {
	var max int16
	var diff int32
	var frac int16
	diff_table := &logSum_diff_table
	var low int
	if a > b {
		max = a
		diff = celt.SUB32(celt.EXTEND32(a), celt.EXTEND32(b))
	} else {
		max = b
		diff = celt.SUB32(celt.EXTEND32(b), celt.EXTEND32(a))
	}
	if !(diff < int32(celt.QCONST16(8.0, celt.DB_SHIFT))) { // inverted to catch NaNs
		return max
	}
	low = int(celt.SHR32(diff, celt.DB_SHIFT-1))
	frac = celt.SHL16(diff-int32(celt.SHL16(low, celt.DB_SHIFT-1)), 16-celt.DB_SHIFT)
	return int16(int32(max) + int32(diff_table[low]) + celt.MULT16_16_Q15(frac, celt.SUB16(diff_table[low+1], diff_table[low])))
}

// C: void surround_analysis(const CELTMode *celt_mode, const void *pcm, opus_val16 *bandLogE, opus_val32 *mem, opus_val32 *preemph_mem,
//
//	int len, int overlap, int channels, int rate, opus_copy_channel_in_func copy_channel_in, int arch)
func SurroundAnalysis(celt_mode *celt.CELTMode, pcm any, bandLogE []int16, mem []int32, preemph_mem []int32,
	length int, overlap int, channels int, rate int, copy_channel_in opus_copy_channel_in_func, arch int) {
	var c int
	var i int
	var LM int
	var pos [8]int
	var upsample int
	var frame_size int
	var channel_offset int16
	var bandE [21]int32
	var maskLogE [3][21]int16

	upsample = celt.ResamplingFactor(int32(rate))
	frame_size = length * upsample

	// LM = log2(frame_size / 120)
	for LM = 0; LM < celt_mode.MaxLM(); LM++ {
		if celt_mode.ShortMdctSize()<<uint(LM) == frame_size {
			break
		}
	}

	in := make([]int32, frame_size+overlap)
	x := make([]int16, length)
	freq := make([]int32, frame_size)

	channel_pos(channels, pos[:])

	for c = 0; c < 3; c++ {
		for i = 0; i < 21; i++ {
			maskLogE[c][i] = -celt.QCONST16(28.0, celt.DB_SHIFT)
		}
	}

	for c = 0; c < channels; c++ {
		copy(in[:overlap], mem[c*overlap:])
		copy_channel_in(x, 1, pcm, channels, c, length)
		preemph := celt_mode.Preemph()
		celt.Preemphasis(x, in[overlap:], frame_size, 1, upsample, preemph[:], &preemph_mem[c], 0)
		celt.MdctForward(celt_mode.Mdct(), in, freq, celt_mode.Window(),
			overlap, celt_mode.MaxLM()-LM, 1, arch)
		if upsample != 1 {
			var bound int = length
			for i = 0; i < bound; i++ {
				freq[i] *= int32(upsample)
			}
			for ; i < frame_size; i++ {
				freq[i] = 0
			}
		}

		celt.ComputeBandEnergies(celt_mode, freq, bandE[:], 21, 1, LM)
		celt.Amp2Log2(celt_mode, 21, 21, bandE[:], bandLogE[21*c:], 1)
		// Apply spreading function with -6 dB/band going up and -12 dB/band going down.
		for i = 1; i < 21; i++ {
			bandLogE[21*c+i] = int16(celt.MAX16(int32(bandLogE[21*c+i]), int32(bandLogE[21*c+i-1])-int32(celt.QCONST16(1.0, celt.DB_SHIFT))))
		}
		for i = 19; i >= 0; i-- {
			bandLogE[21*c+i] = int16(celt.MAX16(int32(bandLogE[21*c+i]), int32(bandLogE[21*c+i+1])-int32(celt.QCONST16(2.0, celt.DB_SHIFT))))
		}
		if pos[c] == 1 {
			for i = 0; i < 21; i++ {
				maskLogE[0][i] = logSum(maskLogE[0][i], bandLogE[21*c+i])
			}
		} else if pos[c] == 3 {
			for i = 0; i < 21; i++ {
				maskLogE[2][i] = logSum(maskLogE[2][i], bandLogE[21*c+i])
			}
		} else if pos[c] == 2 {
			for i = 0; i < 21; i++ {
				maskLogE[0][i] = logSum(maskLogE[0][i], int16(int32(bandLogE[21*c+i])-int32(celt.QCONST16(.5, celt.DB_SHIFT))))
				maskLogE[2][i] = logSum(maskLogE[2][i], int16(int32(bandLogE[21*c+i])-int32(celt.QCONST16(.5, celt.DB_SHIFT))))
			}
		}
		copy(mem[c*overlap:][:overlap], in[frame_size:])
	}
	for i = 0; i < 21; i++ {
		maskLogE[1][i] = celt.MIN32(maskLogE[0][i], maskLogE[2][i])
	}
	channel_offset = int16(celt.HALF16(celt.Log2(celt.QCONST32(2.0, 14) / int32(channels-1))))
	for c = 0; c < 3; c++ {
		for i = 0; i < 21; i++ {
			maskLogE[c][i] += channel_offset
		}
	}
	for c = 0; c < channels; c++ {
		var mask []int16
		if pos[c] != 0 {
			mask = maskLogE[pos[c]-1][:]
			for i = 0; i < 21; i++ {
				bandLogE[21*c+i] = bandLogE[21*c+i] - mask[i]
			}
		} else {
			for i = 0; i < 21; i++ {
				bandLogE[21*c+i] = 0
			}
		}
	}
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
	if channels > 2 {
		size += int32(channels * (120*4 + 4))
	}
	return size
}

// C: static int opus_multistream_encoder_init_impl(OpusMSEncoder *st, opus_int32 Fs, int channels, int streams, int coupled_streams, const unsigned char *mapping, int application, MappingType mapping_type)
func (st *OpusMSEncoder) multistreamEncoderInitImpl(Fs int32, channels int, streams int, coupled_streams int, mapping []byte, application int, mapping_type MappingType) int {
	var i, ret int

	if (channels > 255) || (channels < 1) || (coupled_streams > streams) ||
		(streams < 1) || (coupled_streams < 0) || (streams > 255-coupled_streams) {
		return OPUS_BAD_ARG
	}

	st.arch = celt.SelectArch()
	st.layout.nb_channels = channels
	st.layout.nb_streams = streams
	st.layout.nb_coupled_streams = coupled_streams
	st.subframe_mem[0] = 0
	st.subframe_mem[1] = 0
	st.subframe_mem[2] = 0
	if mapping_type != MAPPING_TYPE_SURROUND {
		st.lfe_stream = -1
	}
	st.bitrate_bps = OPUS_AUTO
	st.application = application
	st.variable_duration = OPUS_FRAMESIZE_ARG
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
		if i == st.lfe_stream {
			st.encoders[i].Ctl(celt.OPUS_SET_LFE_REQUEST, int32(1))
		}
	}
	for ; i < st.layout.nb_streams; i++ {
		st.encoders[i] = new(OpusEncoder)
		ret = st.encoders[i].EncoderInit(Fs, 1, application)
		if i == st.lfe_stream {
			st.encoders[i].Ctl(celt.OPUS_SET_LFE_REQUEST, int32(1))
		}
		if ret != OPUS_OK {
			return ret
		}
	}
	if mapping_type == MAPPING_TYPE_SURROUND {
		st.preemph_mem = make([]int32, channels)
		st.window_mem = make([]int32, channels*120)
	}
	st.mapping_type = mapping_type
	return OPUS_OK
}

// C: int opus_multistream_encoder_init(OpusMSEncoder *st, opus_int32 Fs, int channels, int streams, int coupled_streams, const unsigned char *mapping, int application)
func (st *OpusMSEncoder) MultistreamEncoderInit(Fs int32, channels int, streams int, coupled_streams int, mapping []byte, application int) int {
	return st.multistreamEncoderInitImpl(Fs, channels, streams,
		coupled_streams, mapping,
		application, MAPPING_TYPE_NONE)
}

// C: int opus_multistream_surround_encoder_init(OpusMSEncoder *st, opus_int32 Fs, int channels, int mapping_family, int *streams, int *coupled_streams, unsigned char *mapping, int application)
func (st *OpusMSEncoder) MultistreamSurroundEncoderInit(Fs int32, channels int, mapping_family int, streams *int, coupled_streams *int, mapping []byte, application int) int {
	var mapping_type MappingType

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
	if channels > 2 && mapping_family == 1 {
		mapping_type = MAPPING_TYPE_SURROUND
	} else {
		mapping_type = MAPPING_TYPE_NONE
	}
	return st.multistreamEncoderInitImpl(Fs, channels, *streams,
		*coupled_streams, mapping,
		application, mapping_type)
}

// C: OpusMSEncoder *opus_multistream_encoder_create(opus_int32 Fs, int channels, int streams, int coupled_streams, const unsigned char *mapping, int application, int *error)
func MultistreamEncoderCreate(Fs int32, channels int, streams int, coupled_streams int, mapping []byte, application int, error *int) *OpusMSEncoder {
	var ret int
	if (channels > 255) || (channels < 1) || (coupled_streams > streams) ||
		(streams < 1) || (coupled_streams < 0) || (streams > 255-coupled_streams) {
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
	var size int32
	if (channels > 255) || (channels < 1) {
		if error != nil {
			*error = OPUS_BAD_ARG
		}
		return nil
	}
	size = MultistreamSurroundEncoderGetSize(channels, mapping_family)
	if size == 0 {
		if error != nil {
			*error = OPUS_UNIMPLEMENTED
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

// C: static void surround_rate_allocation(OpusMSEncoder *st, opus_int32 *rate, int frame_size, opus_int32 Fs)
func (st *OpusMSEncoder) surround_rate_allocation(rate []int32, frame_size int, Fs int32) {
	var i int
	var channel_rate int32
	var stream_offset int
	var lfe_offset int
	var coupled_ratio int // Q8
	var lfe_ratio int     // Q8

	if st.bitrate_bps > int32(st.layout.nb_channels*40000) {
		stream_offset = 20000
	} else {
		stream_offset = int(st.bitrate_bps) / st.layout.nb_channels / 2
	}
	stream_offset += 60 * (int(Fs)/frame_size - 50)

	// We start by giving each stream (coupled or uncoupled) the same bitrate.
	// This models the main saving of coupled channels over uncoupled.
	// The LFE stream is an exception to the above and gets fewer bits.
	lfe_offset = 3500 + 60*(int(Fs)/frame_size-50)
	// Coupled streams get twice the mono rate after the first 20 kb/s.
	coupled_ratio = 512
	// Should depend on the bitrate, for now we assume LFE gets 1/8 the bits of mono
	lfe_ratio = 32

	// Compute bitrate allocation between streams
	if st.bitrate_bps == OPUS_AUTO {
		channel_rate = Fs + 60*Fs/int32(frame_size)
	} else if st.bitrate_bps == OPUS_BITRATE_MAX {
		channel_rate = 300000
	} else {
		var nb_lfe int
		var nb_uncoupled int
		var nb_coupled int
		var total int
		nb_lfe = b2i(st.lfe_stream != -1)
		nb_coupled = st.layout.nb_coupled_streams
		nb_uncoupled = st.layout.nb_streams - nb_coupled - nb_lfe
		total = (nb_uncoupled << 8) + // mono
			coupled_ratio*nb_coupled + // stereo
			nb_lfe*lfe_ratio
		channel_rate = 256 * (st.bitrate_bps - int32(lfe_offset*nb_lfe) - int32(stream_offset*(nb_coupled+nb_uncoupled))) / int32(total)
	}

	for i = 0; i < st.layout.nb_streams; i++ {
		if i < st.layout.nb_coupled_streams {
			rate[i] = int32(stream_offset) + (channel_rate * int32(coupled_ratio) >> 8)
		} else if i != st.lfe_stream {
			rate[i] = int32(stream_offset) + channel_rate
		} else {
			rate[i] = int32(lfe_offset) + (channel_rate * int32(lfe_ratio) >> 8)
		}
	}
}

// C: static opus_int32 rate_allocation(OpusMSEncoder *st, opus_int32 *rate, int frame_size)
func (st *OpusMSEncoder) rate_allocation(rate []int32, frame_size int) int32 {
	var i int
	var rate_sum int32 = 0
	var Fs int32
	st.encoders[0].Ctl(OPUS_GET_SAMPLE_RATE_REQUEST, &Fs)
	{
		st.surround_rate_allocation(rate, frame_size, Fs)
	}

	for i = 0; i < st.layout.nb_streams; i++ {
		rate[i] = IMAX(rate[i], 500)
		rate_sum += rate[i]
	}
	return rate_sum
}

// Max size in case the encoder decides to return three frames
const MS_FRAME_TMP = (3*1275 + 7)

// C: static int opus_multistream_encode_native(OpusMSEncoder *st, opus_copy_channel_in_func copy_channel_in, const void *pcm, int analysis_frame_size, unsigned char *data, opus_int32 max_data_bytes, int lsb_depth, downmix_func downmix, int float_api)
func (st *OpusMSEncoder) multistreamEncodeNative(copy_channel_in opus_copy_channel_in_func, pcm any, analysis_frame_size int, data []byte, max_data_bytes int32, lsb_depth int, downmix downmix_func, float_api int) int {
	var Fs int32
	var s int
	var tot_size int
	var tmp_data [MS_FRAME_TMP]byte
	var rp OpusRepacketizer
	var vbr int32
	var celt_mode *celt.CELTMode
	var bitrates [256]int32
	var bandLogE [42]int16
	var mem []int32 = nil
	var preemph_mem []int32 = nil
	var frame_size int
	var rate_sum int32
	var smallest_packet int32

	if st.mapping_type == MAPPING_TYPE_SURROUND {
		preemph_mem = st.ms_get_preemph_mem()
		mem = st.ms_get_window_mem()
	}
	st.encoders[0].Ctl(OPUS_GET_SAMPLE_RATE_REQUEST, &Fs)
	st.encoders[0].Ctl(OPUS_GET_VBR_REQUEST, &vbr)
	st.encoders[0].Ctl(celt.CELT_GET_MODE_REQUEST, &celt_mode)

	{
		var delay_compensation int32
		var channels int

		channels = st.layout.nb_streams + st.layout.nb_coupled_streams
		st.encoders[0].Ctl(OPUS_GET_LOOKAHEAD_REQUEST, &delay_compensation)
		delay_compensation -= Fs / 400
		frame_size = int(ComputeFrameSize(pcm, analysis_frame_size,
			st.variable_duration, channels, Fs, int(st.bitrate_bps),
			int(delay_compensation), downmix, st.subframe_mem[:]))
	}

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

	// Smallest packet the encoder can produce.
	smallest_packet = int32(st.layout.nb_streams*2 - 1)
	if max_data_bytes < smallest_packet {
		return OPUS_BUFFER_TOO_SMALL
	}
	buf := make([]int16, 2*frame_size)

	bandSMR := make([]int16, 21*st.layout.nb_channels)
	if st.mapping_type == MAPPING_TYPE_SURROUND {
		SurroundAnalysis(celt_mode, pcm, bandSMR, mem, preemph_mem, frame_size, 120, st.layout.nb_channels, int(Fs), copy_channel_in, st.arch)
	}

	// Compute bitrate allocation between streams (this could be a lot better)
	rate_sum = st.rate_allocation(bitrates[:], frame_size)

	if vbr == 0 {
		if st.bitrate_bps == OPUS_AUTO {
			max_data_bytes = IMIN(max_data_bytes, 3*rate_sum/(3*8*Fs/int32(frame_size)))
		} else if st.bitrate_bps != OPUS_BITRATE_MAX {
			max_data_bytes = IMIN(max_data_bytes, IMAX(smallest_packet,
				3*st.bitrate_bps/(3*8*Fs/int32(frame_size))))
		}
	}
	for s = 0; s < st.layout.nb_streams; s++ {
		enc := st.encoders[s]
		enc.Ctl(OPUS_SET_BITRATE_REQUEST, bitrates[s])
		if st.mapping_type == MAPPING_TYPE_SURROUND {
			var equiv_rate int32
			equiv_rate = st.bitrate_bps
			if int32(frame_size*50) < Fs {
				equiv_rate -= 60 * (Fs/int32(frame_size) - 50) * int32(st.layout.nb_channels)
			}
			if equiv_rate > int32(10000*st.layout.nb_channels) {
				enc.Ctl(OPUS_SET_BANDWIDTH_REQUEST, int32(OPUS_BANDWIDTH_FULLBAND))
			} else if equiv_rate > int32(7000*st.layout.nb_channels) {
				enc.Ctl(OPUS_SET_BANDWIDTH_REQUEST, int32(OPUS_BANDWIDTH_SUPERWIDEBAND))
			} else if equiv_rate > int32(5000*st.layout.nb_channels) {
				enc.Ctl(OPUS_SET_BANDWIDTH_REQUEST, int32(OPUS_BANDWIDTH_WIDEBAND))
			} else {
				enc.Ctl(OPUS_SET_BANDWIDTH_REQUEST, int32(OPUS_BANDWIDTH_NARROWBAND))
			}
			if s < st.layout.nb_coupled_streams {
				// To preserve the spatial image, force stereo CELT on coupled streams
				enc.Ctl(OPUS_SET_FORCE_MODE_REQUEST, int32(MODE_CELT_ONLY))
				enc.Ctl(OPUS_SET_FORCE_CHANNELS_REQUEST, int32(2))
			}
		}
	}

	// Counting ToC
	tot_size = 0
	for s = 0; s < st.layout.nb_streams; s++ {
		var length int32
		var curr_max int32
		var c1, c2 int
		var ret int

		rp.RepacketizerInit()
		enc := st.encoders[s]
		if s < st.layout.nb_coupled_streams {
			var i int
			var left, right int
			left = get_left_channel(&st.layout, s, -1)
			right = get_right_channel(&st.layout, s, -1)
			copy_channel_in(buf, 2,
				pcm, st.layout.nb_channels, left, frame_size)
			copy_channel_in(buf[1:], 2,
				pcm, st.layout.nb_channels, right, frame_size)
			if st.mapping_type == MAPPING_TYPE_SURROUND {
				for i = 0; i < 21; i++ {
					bandLogE[i] = bandSMR[21*left+i]
					bandLogE[21+i] = bandSMR[21*right+i]
				}
			}
			c1 = left
			c2 = right
		} else {
			var i int
			chanIdx := get_mono_channel(&st.layout, s, -1)
			copy_channel_in(buf, 1,
				pcm, st.layout.nb_channels, chanIdx, frame_size)
			if st.mapping_type == MAPPING_TYPE_SURROUND {
				for i = 0; i < 21; i++ {
					bandLogE[i] = bandSMR[21*chanIdx+i]
				}
			}
			c1 = chanIdx
			c2 = -1
		}
		if st.mapping_type == MAPPING_TYPE_SURROUND {
			enc.Ctl(celt.OPUS_SET_ENERGY_MASK_REQUEST, bandLogE[:])
		}

		// number of bytes left (+Toc)
		curr_max = max_data_bytes - int32(tot_size)
		// Reserve one byte for the last stream and two for the others
		curr_max -= int32(IMAX(0, 2*(st.layout.nb_streams-s-1)-1))
		curr_max = IMIN(curr_max, MS_FRAME_TMP)
		// Repacketizer will add one or two bytes for self-delimited frames
		if s != st.layout.nb_streams-1 {
			if curr_max > 253 {
				curr_max -= 2
			} else {
				curr_max -= 1
			}
		}
		if vbr == 0 && s == st.layout.nb_streams-1 {
			enc.Ctl(OPUS_SET_BITRATE_REQUEST, curr_max*(8*Fs/int32(frame_size)))
		}
		length = enc.EncodeNative(buf, frame_size, tmp_data[:], curr_max, lsb_depth,
			pcm, int32(analysis_frame_size), c1, c2, st.layout.nb_channels, downmix, float_api)
		if length < 0 {
			return int(length)
		}
		// We need to use the repacketizer to add the self-delimiting lengths
		// while taking into account the fact that the encoder can now return
		// more than one frame at a time (e.g. 60 ms CELT-only)
		ret = rp.RepacketizerCat(tmp_data[:], length)
		// If the opus_repacketizer_cat() fails, then something's seriously wrong
		// with the encoder.
		if ret != OPUS_OK {
			return OPUS_INTERNAL_ERROR
		}
		length = rp.repacketizerOutRangeImpl(0, rp.RepacketizerGetNbFrames(),
			data, max_data_bytes-int32(tot_size), b2i(s != st.layout.nb_streams-1), b2i(vbr == 0 && s == st.layout.nb_streams-1))
		data = data[length:]
		tot_size += int(length)
	}
	// printf("\n");
	return tot_size
}

// C: static void opus_copy_channel_in_float(opus_val16 *dst, int dst_stride, const void *src, int src_stride, int src_channel, int frame_size)
func opus_copy_channel_in_float(dst []int16, dst_stride int, src any, src_stride int, src_channel int, frame_size int) {
	float_src := src.([]float32)
	var i int32
	for i = 0; i < int32(frame_size); i++ {
		dst[i*int32(dst_stride)] = FLOAT2INT16(float_src[i*int32(src_stride)+int32(src_channel)])
	}
}

// C: static void opus_copy_channel_in_short(opus_val16 *dst, int dst_stride, const void *src, int src_stride, int src_channel, int frame_size)
func opus_copy_channel_in_short(dst []int16, dst_stride int, src any, src_stride int, src_channel int, frame_size int) {
	short_src := src.([]int16)
	var i int32
	for i = 0; i < int32(frame_size); i++ {
		dst[i*int32(dst_stride)] = short_src[i*int32(src_stride)+int32(src_channel)]
	}
}

// C: int opus_multistream_encode(OpusMSEncoder *st, const opus_val16 *pcm, int frame_size, unsigned char *data, opus_int32 max_data_bytes)   (FIXED_POINT)
func (st *OpusMSEncoder) MultistreamEncode(pcm []int16, frame_size int, data []byte, max_data_bytes int32) int {
	return st.multistreamEncodeNative(opus_copy_channel_in_short,
		pcm, frame_size, data, max_data_bytes, 16, DownmixInt, 0)
}

// C: int opus_multistream_encode_float(OpusMSEncoder *st, const float *pcm, int frame_size, unsigned char *data, opus_int32 max_data_bytes)   (FIXED_POINT)
func (st *OpusMSEncoder) MultistreamEncodeFloat(pcm []float32, frame_size int, data []byte, max_data_bytes int32) int {
	return st.multistreamEncodeNative(opus_copy_channel_in_float,
		pcm, frame_size, data, max_data_bytes, 16, DownmixFloat, 1)
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
		if !ok || value == nil {
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
		OPUS_GET_FORCE_CHANNELS_REQUEST,
		OPUS_GET_PREDICTION_DISABLED_REQUEST:
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
		if !ok || value == nil {
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
		OPUS_SET_MAX_BANDWIDTH_REQUEST,
		OPUS_SET_BANDWIDTH_REQUEST,
		OPUS_SET_SIGNAL_REQUEST,
		OPUS_SET_APPLICATION_REQUEST,
		OPUS_SET_INBAND_FEC_REQUEST,
		OPUS_SET_PACKET_LOSS_PERC_REQUEST,
		OPUS_SET_DTX_REQUEST,
		OPUS_SET_FORCE_MODE_REQUEST,
		OPUS_SET_FORCE_CHANNELS_REQUEST,
		OPUS_SET_PREDICTION_DISABLED_REQUEST:
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
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		if ret == OPUS_OK {
			*value = st.encoders[stream_id]
		}
	case OPUS_SET_EXPERT_FRAME_DURATION_REQUEST:
		value, ok := argInt32(args, 0)
		if !ok {
			return OPUS_BAD_ARG
		}
		st.variable_duration = int(value)
	case OPUS_GET_EXPERT_FRAME_DURATION_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG
		}
		*value = int32(st.variable_duration)
	case OPUS_RESET_STATE:
		var s int
		st.subframe_mem[0] = 0
		st.subframe_mem[1] = 0
		st.subframe_mem[2] = 0
		if st.mapping_type == MAPPING_TYPE_SURROUND {
			clear(st.ms_get_preemph_mem()[:st.layout.nb_channels])
			clear(st.ms_get_window_mem()[:st.layout.nb_channels*120])
		}
		for s = 0; s < st.layout.nb_streams; s++ {
			enc := st.encoders[s]
			ret = enc.Ctl(OPUS_RESET_STATE)
			if ret != OPUS_OK {
				break
			}
		}
	default:
		ret = OPUS_UNIMPLEMENTED
	}

	return ret
}

// C: void opus_multistream_encoder_destroy(OpusMSEncoder *st)
func (st *OpusMSEncoder) MultistreamEncoderDestroy() {}
