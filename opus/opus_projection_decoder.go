package opus

// C: src/opus_projection_decoder.c, include/opus_projection.h

// OpusProjectionDecoder is the C struct OpusProjectionDecoder. The demixing
// matrix and multistream decoder state that C places after the struct in the
// same allocation are contained fields.
type OpusProjectionDecoder struct {
	demixing_matrix_size_in_bytes int32
	// Encoder states go here
	demixing_matrix MappingMatrix
	ms_decoder      OpusMSDecoder
}

// C: static void opus_projection_copy_channel_out_float(void *dst, int dst_stride, int dst_channel, const opus_val16 *src, int src_stride, int frame_size, void *user_data)
func opus_projection_copy_channel_out_float(
	dst any,
	dst_stride int,
	dst_channel int,
	src []int16,
	src_stride int,
	frame_size int,
	user_data any) {
	var float_dst []float32
	var matrix *MappingMatrix
	float_dst = dst.([]float32)
	matrix = user_data.(*MappingMatrix)

	if dst_channel == 0 {
		clear(float_dst[:frame_size*dst_stride])
	}

	if src != nil {
		matrix.MappingMatrixMultiplyChannelOutFloat(src, dst_channel,
			src_stride, float_dst, dst_stride, frame_size)
	}
}

// C: static void opus_projection_copy_channel_out_short(void *dst, int dst_stride, int dst_channel, const opus_val16 *src, int src_stride, int frame_size, void *user_data)
func opus_projection_copy_channel_out_short(
	dst any,
	dst_stride int,
	dst_channel int,
	src []int16,
	src_stride int,
	frame_size int,
	user_data any) {
	var short_dst []int16
	var matrix *MappingMatrix
	short_dst = dst.([]int16)
	matrix = user_data.(*MappingMatrix)
	if dst_channel == 0 {
		clear(short_dst[:frame_size*dst_stride])
	}

	if src != nil {
		matrix.MappingMatrixMultiplyChannelOutShort(src, dst_channel,
			src_stride, short_dst, dst_stride, frame_size)
	}
}

// C: static MappingMatrix *get_dec_demixing_matrix(OpusProjectionDecoder *st)
func (st *OpusProjectionDecoder) get_dec_demixing_matrix() *MappingMatrix {
	return &st.demixing_matrix
}

// C: static OpusMSDecoder *get_multistream_decoder(OpusProjectionDecoder *st)
func (st *OpusProjectionDecoder) get_multistream_decoder() *OpusMSDecoder {
	return &st.ms_decoder
}

// ProjectionDecoderGetSize returns the C allocation size (for API parity).
//
// C: opus_int32 opus_projection_decoder_get_size(int channels, int streams, int coupled_streams)
func ProjectionDecoderGetSize(channels int, streams int,
	coupled_streams int) int32 {
	var matrix_size int32
	var decoder_size int32

	matrix_size =
		MappingMatrixGetSize(streams+coupled_streams, channels)
	if matrix_size == 0 {
		return 0
	}

	decoder_size = MultistreamDecoderGetSize(streams, coupled_streams)
	if decoder_size == 0 {
		return 0
	}

	return int32(align(4)) + matrix_size + decoder_size // sizeof(OpusProjectionDecoder) == 4
}

// C: int opus_projection_decoder_init(OpusProjectionDecoder *st, opus_int32 Fs, int channels, int streams, int coupled_streams, unsigned char *demixing_matrix, opus_int32 demixing_matrix_size)
func (st *OpusProjectionDecoder) ProjectionDecoderInit(Fs int32,
	channels int, streams int, coupled_streams int,
	demixing_matrix []byte, demixing_matrix_size int32) int {
	var nb_input_streams int
	var expected_matrix_size int32
	var i, ret int
	var mapping [255]byte
	var buf []int16

	// Verify supplied matrix size.
	nb_input_streams = streams + coupled_streams
	expected_matrix_size = int32(nb_input_streams * channels * 2) // sizeof(opus_int16)
	if expected_matrix_size != demixing_matrix_size {
		return OPUS_BAD_ARG
	}

	// Convert demixing matrix input into internal format.
	buf = make([]int16, nb_input_streams*channels)
	for i = 0; i < nb_input_streams*channels; i++ {
		s := int(demixing_matrix[2*i+1])<<8 | int(demixing_matrix[2*i])
		s = ((s & 0xFFFF) ^ 0x8000) - 0x8000
		buf[i] = int16(s)
	}

	// Assign demixing matrix.
	st.demixing_matrix_size_in_bytes =
		MappingMatrixGetSize(channels, nb_input_streams)
	if st.demixing_matrix_size_in_bytes == 0 {
		return OPUS_BAD_ARG
	}

	st.get_dec_demixing_matrix().MappingMatrixInit(channels, nb_input_streams, 0,
		buf, demixing_matrix_size)

	// Set trivial mapping so each input channel pairs with a matrix column.
	for i = 0; i < channels; i++ {
		mapping[i] = byte(i)
	}

	ret = st.get_multistream_decoder().MultistreamDecoderInit(
		Fs, channels, streams, coupled_streams, mapping[:])
	return ret
}

// C: OpusProjectionDecoder *opus_projection_decoder_create(opus_int32 Fs, int channels, int streams, int coupled_streams, unsigned char *demixing_matrix, opus_int32 demixing_matrix_size, int *error)
func ProjectionDecoderCreate(
	Fs int32, channels int, streams int, coupled_streams int,
	demixing_matrix []byte, demixing_matrix_size int32, error *int) *OpusProjectionDecoder {
	var size int
	var ret int
	var st *OpusProjectionDecoder

	// Allocate space for the projection decoder.
	size = int(ProjectionDecoderGetSize(channels, streams, coupled_streams))
	if size == 0 {
		if error != nil {
			*error = OPUS_ALLOC_FAIL
		}
		return nil
	}
	st = new(OpusProjectionDecoder)

	// Initialize projection decoder with provided settings.
	ret = st.ProjectionDecoderInit(Fs, channels, streams, coupled_streams,
		demixing_matrix, demixing_matrix_size)
	if ret != OPUS_OK {
		st = nil
	}
	if error != nil {
		*error = ret
	}
	return st
}

// C: int opus_projection_decode(OpusProjectionDecoder *st, const unsigned char *data, opus_int32 len, opus_int16 *pcm, int frame_size, int decode_fec)   (FIXED_POINT)
func (st *OpusProjectionDecoder) ProjectionDecode(data []byte,
	length int32, pcm []int16, frame_size int,
	decode_fec int) int {
	return st.get_multistream_decoder().MultistreamDecodeNative(data, length,
		pcm, opus_projection_copy_channel_out_short, frame_size, decode_fec, 0,
		st.get_dec_demixing_matrix())
}

// C: int opus_projection_decode_float(OpusProjectionDecoder *st, const unsigned char *data, opus_int32 len, float *pcm, int frame_size, int decode_fec)
func (st *OpusProjectionDecoder) ProjectionDecodeFloat(data []byte,
	length int32, pcm []float32, frame_size int, decode_fec int) int {
	return st.get_multistream_decoder().MultistreamDecodeNative(data, length,
		pcm, opus_projection_copy_channel_out_float, frame_size, decode_fec, 0,
		st.get_dec_demixing_matrix())
}

// Ctl is the C opus_projection_decoder_ctl: every request is passed on to
// the multistream decoder (opus_multistream_decoder_ctl_va_list).
//
// C: int opus_projection_decoder_ctl(OpusProjectionDecoder *st, int request, ...)
func (st *OpusProjectionDecoder) Ctl(request int, args ...any) int {
	var ret int = OPUS_OK

	ret = st.get_multistream_decoder().Ctl(request, args...)
	return ret
}

// ProjectionDecoderDestroy frees the state (no-op in Go; kept for API parity).
//
// C: void opus_projection_decoder_destroy(OpusProjectionDecoder *st)
func (st *OpusProjectionDecoder) ProjectionDecoderDestroy() {}
