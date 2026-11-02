package opus

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// C: src/opus_projection_encoder.c, include/opus_projection.h

// OpusProjectionEncoder is the C struct OpusProjectionEncoder. The mixing
// matrix, demixing matrix and multistream encoder state that C places after
// the struct in the same allocation are contained fields.
type OpusProjectionEncoder struct {
	mixing_matrix_size_in_bytes   int32
	demixing_matrix_size_in_bytes int32
	// Encoder states go here
	mixing_matrix   MappingMatrix
	demixing_matrix MappingMatrix
	ms_encoder      OpusMSEncoder
}

// C: static void opus_projection_copy_channel_in_float(opus_val16 *dst, int dst_stride, const void *src, int src_stride, int src_channel, int frame_size, void *user_data)
func opus_projection_copy_channel_in_float(
	dst []int16,
	dst_stride int,
	src any,
	src_stride int,
	src_channel int,
	frame_size int,
	user_data any) {
	user_data.(*MappingMatrix).MappingMatrixMultiplyChannelInFloat(
		src.([]float32), src_stride, dst, src_channel, dst_stride, frame_size)
}

// C: static void opus_projection_copy_channel_in_short(opus_val16 *dst, int dst_stride, const void *src, int src_stride, int src_channel, int frame_size, void *user_data)
func opus_projection_copy_channel_in_short(
	dst []int16,
	dst_stride int,
	src any,
	src_stride int,
	src_channel int,
	frame_size int,
	user_data any) {
	user_data.(*MappingMatrix).MappingMatrixMultiplyChannelInShort(
		src.([]int16), src_stride, dst, src_channel, dst_stride, frame_size)
}

// C: static int get_order_plus_one_from_channels(int channels, int *order_plus_one)
func get_order_plus_one_from_channels(channels int, order_plus_one *int) int {
	var order_plus_one_ int
	var acn_channels int
	var nondiegetic_channels int

	// Allowed numbers of channels:
	// (1 + n)^2 + 2j, for n = 0...14 and j = 0 or 1.
	if channels < 1 || channels > 227 {
		return OPUS_BAD_ARG
	}

	order_plus_one_ = int(celt.Isqrt32(uint32(channels)))
	acn_channels = order_plus_one_ * order_plus_one_
	nondiegetic_channels = channels - acn_channels
	if nondiegetic_channels != 0 && nondiegetic_channels != 2 {
		return OPUS_BAD_ARG
	}

	if order_plus_one != nil {
		*order_plus_one = order_plus_one_
	}
	return OPUS_OK
}

// C: static int get_streams_from_channels(int channels, int mapping_family, int *streams, int *coupled_streams, int *order_plus_one)
func get_streams_from_channels(channels int, mapping_family int,
	streams *int, coupled_streams *int,
	order_plus_one *int) int {
	if mapping_family == 3 {
		if get_order_plus_one_from_channels(channels, order_plus_one) != OPUS_OK {
			return OPUS_BAD_ARG
		}
		if streams != nil {
			*streams = (channels + 1) / 2
		}
		if coupled_streams != nil {
			*coupled_streams = channels / 2
		}
		return OPUS_OK
	}
	return OPUS_BAD_ARG
}

// C: static MappingMatrix *get_mixing_matrix(OpusProjectionEncoder *st)
func (st *OpusProjectionEncoder) get_mixing_matrix() *MappingMatrix {
	return &st.mixing_matrix
}

// C: static MappingMatrix *get_enc_demixing_matrix(OpusProjectionEncoder *st)
func (st *OpusProjectionEncoder) get_enc_demixing_matrix() *MappingMatrix {
	return &st.demixing_matrix
}

// C: static OpusMSEncoder *get_multistream_encoder(OpusProjectionEncoder *st)
func (st *OpusProjectionEncoder) get_multistream_encoder() *OpusMSEncoder {
	return &st.ms_encoder
}

// ProjectionAmbisonicsEncoderGetSize returns the C allocation size (for API
// parity).
//
// C: opus_int32 opus_projection_ambisonics_encoder_get_size(int channels, int mapping_family)
func ProjectionAmbisonicsEncoderGetSize(channels int, mapping_family int) int32 {
	var nb_streams int
	var nb_coupled_streams int
	var order_plus_one int
	var mixing_matrix_rows, mixing_matrix_cols int
	var demixing_matrix_rows, demixing_matrix_cols int
	var mixing_matrix_size, demixing_matrix_size int32
	var encoder_size int32
	var ret int

	ret = get_streams_from_channels(channels, mapping_family, &nb_streams,
		&nb_coupled_streams, &order_plus_one)
	if ret != OPUS_OK {
		return 0
	}

	if order_plus_one == 2 {
		mixing_matrix_rows = MappingMatrixFoaMixing.rows
		mixing_matrix_cols = MappingMatrixFoaMixing.cols
		demixing_matrix_rows = MappingMatrixFoaDemixing.rows
		demixing_matrix_cols = MappingMatrixFoaDemixing.cols
	} else if order_plus_one == 3 {
		mixing_matrix_rows = MappingMatrixSoaMixing.rows
		mixing_matrix_cols = MappingMatrixSoaMixing.cols
		demixing_matrix_rows = MappingMatrixSoaDemixing.rows
		demixing_matrix_cols = MappingMatrixSoaDemixing.cols
	} else if order_plus_one == 4 {
		mixing_matrix_rows = MappingMatrixToaMixing.rows
		mixing_matrix_cols = MappingMatrixToaMixing.cols
		demixing_matrix_rows = MappingMatrixToaDemixing.rows
		demixing_matrix_cols = MappingMatrixToaDemixing.cols
	} else {
		return 0
	}

	mixing_matrix_size =
		MappingMatrixGetSize(mixing_matrix_rows, mixing_matrix_cols)
	if mixing_matrix_size == 0 {
		return 0
	}

	demixing_matrix_size =
		MappingMatrixGetSize(demixing_matrix_rows, demixing_matrix_cols)
	if demixing_matrix_size == 0 {
		return 0
	}

	encoder_size =
		MultistreamEncoderGetSize(nb_streams, nb_coupled_streams)
	if encoder_size == 0 {
		return 0
	}

	return int32(align(8)) + // sizeof(OpusProjectionEncoder) == 8
		mixing_matrix_size + demixing_matrix_size + encoder_size
}

// C: int opus_projection_ambisonics_encoder_init(OpusProjectionEncoder *st, opus_int32 Fs, int channels, int mapping_family, int *streams, int *coupled_streams, int application)
func (st *OpusProjectionEncoder) ProjectionAmbisonicsEncoderInit(Fs int32,
	channels int, mapping_family int,
	streams *int, coupled_streams *int,
	application int) int {
	var mixing_matrix *MappingMatrix
	var demixing_matrix *MappingMatrix
	var ms_encoder *OpusMSEncoder
	var i int
	var ret int
	var order_plus_one int
	var mapping [255]byte

	if streams == nil || coupled_streams == nil {
		return OPUS_BAD_ARG
	}

	if get_streams_from_channels(channels, mapping_family, streams,
		coupled_streams, &order_plus_one) != OPUS_OK {
		return OPUS_BAD_ARG
	}

	if mapping_family == 3 {
		// Assign mixing matrix based on available pre-computed matrices.
		mixing_matrix = st.get_mixing_matrix()
		if order_plus_one == 2 {
			mixing_matrix.MappingMatrixInit(MappingMatrixFoaMixing.rows,
				MappingMatrixFoaMixing.cols, MappingMatrixFoaMixing.gain,
				MappingMatrixFoaMixingData[:],
				int32(len(MappingMatrixFoaMixingData)*2))
		} else if order_plus_one == 3 {
			mixing_matrix.MappingMatrixInit(MappingMatrixSoaMixing.rows,
				MappingMatrixSoaMixing.cols, MappingMatrixSoaMixing.gain,
				MappingMatrixSoaMixingData[:],
				int32(len(MappingMatrixSoaMixingData)*2))
		} else if order_plus_one == 4 {
			mixing_matrix.MappingMatrixInit(MappingMatrixToaMixing.rows,
				MappingMatrixToaMixing.cols, MappingMatrixToaMixing.gain,
				MappingMatrixToaMixingData[:],
				int32(len(MappingMatrixToaMixingData)*2))
		} else {
			return OPUS_BAD_ARG
		}

		st.mixing_matrix_size_in_bytes = MappingMatrixGetSize(
			mixing_matrix.rows, mixing_matrix.cols)
		if st.mixing_matrix_size_in_bytes == 0 {
			return OPUS_BAD_ARG
		}

		// Assign demixing matrix based on available pre-computed matrices.
		demixing_matrix = st.get_enc_demixing_matrix()
		if order_plus_one == 2 {
			demixing_matrix.MappingMatrixInit(MappingMatrixFoaDemixing.rows,
				MappingMatrixFoaDemixing.cols, MappingMatrixFoaDemixing.gain,
				MappingMatrixFoaDemixingData[:],
				int32(len(MappingMatrixFoaDemixingData)*2))
		} else if order_plus_one == 3 {
			demixing_matrix.MappingMatrixInit(MappingMatrixSoaDemixing.rows,
				MappingMatrixSoaDemixing.cols, MappingMatrixSoaDemixing.gain,
				MappingMatrixSoaDemixingData[:],
				int32(len(MappingMatrixSoaDemixingData)*2))
		} else if order_plus_one == 4 {
			demixing_matrix.MappingMatrixInit(MappingMatrixToaDemixing.rows,
				MappingMatrixToaDemixing.cols, MappingMatrixToaDemixing.gain,
				MappingMatrixToaDemixingData[:],
				int32(len(MappingMatrixToaDemixingData)*2))
		} else {
			return OPUS_BAD_ARG
		}

		st.demixing_matrix_size_in_bytes = MappingMatrixGetSize(
			demixing_matrix.rows, demixing_matrix.cols)
		if st.demixing_matrix_size_in_bytes == 0 {
			return OPUS_BAD_ARG
		}
	} else {
		return OPUS_UNIMPLEMENTED
	}

	// Ensure matrices are large enough for desired coding scheme.
	if *streams+*coupled_streams > mixing_matrix.rows ||
		channels > mixing_matrix.cols ||
		channels > demixing_matrix.rows ||
		*streams+*coupled_streams > demixing_matrix.cols {
		return OPUS_BAD_ARG
	}

	// Set trivial mapping so each input channel pairs with a matrix column.
	for i = 0; i < channels; i++ {
		mapping[i] = byte(i)
	}

	// Initialize multistream encoder with provided settings.
	ms_encoder = st.get_multistream_encoder()
	ret = ms_encoder.MultistreamEncoderInit(Fs, channels, *streams,
		*coupled_streams, mapping[:], application)
	return ret
}

// C: OpusProjectionEncoder *opus_projection_ambisonics_encoder_create(opus_int32 Fs, int channels, int mapping_family, int *streams, int *coupled_streams, int application, int *error)
func ProjectionAmbisonicsEncoderCreate(
	Fs int32, channels int, mapping_family int, streams *int,
	coupled_streams *int, application int, error *int) *OpusProjectionEncoder {
	var size int
	var ret int
	var st *OpusProjectionEncoder

	// Allocate space for the projection encoder.
	size = int(ProjectionAmbisonicsEncoderGetSize(channels, mapping_family))
	if size == 0 {
		if error != nil {
			*error = OPUS_ALLOC_FAIL
		}
		return nil
	}
	st = new(OpusProjectionEncoder)

	// Initialize projection encoder with provided settings.
	ret = st.ProjectionAmbisonicsEncoderInit(Fs, channels,
		mapping_family, streams, coupled_streams, application)
	if ret != OPUS_OK {
		st = nil
	}
	if error != nil {
		*error = ret
	}
	return st
}

// C: int opus_projection_encode(OpusProjectionEncoder *st, const opus_int16 *pcm, int frame_size, unsigned char *data, opus_int32 max_data_bytes)
func (st *OpusProjectionEncoder) ProjectionEncode(pcm []int16,
	frame_size int, data []byte,
	max_data_bytes int32) int {
	return st.get_multistream_encoder().multistreamEncodeNative(
		opus_projection_copy_channel_in_short, pcm, frame_size, data,
		max_data_bytes, 16, DownmixInt, 0, st.get_mixing_matrix())
}

// C: int opus_projection_encode_float(OpusProjectionEncoder *st, const float *pcm, int frame_size, unsigned char *data, opus_int32 max_data_bytes)   (FIXED_POINT)
func (st *OpusProjectionEncoder) ProjectionEncodeFloat(pcm []float32,
	frame_size int, data []byte,
	max_data_bytes int32) int {
	return st.get_multistream_encoder().multistreamEncodeNative(
		opus_projection_copy_channel_in_float, pcm, frame_size, data,
		max_data_bytes, 16, DownmixFloat, 1, st.get_mixing_matrix())
}

// ProjectionEncoderDestroy frees the state (no-op in Go; kept for API parity).
//
// C: void opus_projection_encoder_destroy(OpusProjectionEncoder *st)
func (st *OpusProjectionEncoder) ProjectionEncoderDestroy() {}

// Ctl is the C opus_projection_encoder_ctl. The OPUS_PROJECTION_GET_* GAIN
// and SIZE requests take an *int32; OPUS_PROJECTION_GET_DEMIXING_MATRIX
// takes a []byte and its size (int32). Everything else is passed on to the
// multistream encoder.
//
// C: int opus_projection_encoder_ctl(OpusProjectionEncoder *st, int request, ...)
func (st *OpusProjectionEncoder) Ctl(request int, args ...any) int {
	var demixing_matrix *MappingMatrix
	var ms_encoder *OpusMSEncoder
	var ret int = OPUS_OK

	ms_encoder = st.get_multistream_encoder()
	demixing_matrix = st.get_enc_demixing_matrix()

	switch request {
	case OPUS_PROJECTION_GET_DEMIXING_MATRIX_SIZE_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG // goto bad_arg
		}
		*value =
			int32(ms_encoder.layout.nb_channels * (ms_encoder.layout.nb_streams +
				ms_encoder.layout.nb_coupled_streams) * 2) // sizeof(opus_int16)
	case OPUS_PROJECTION_GET_DEMIXING_MATRIX_GAIN_REQUEST:
		value, ok := args[0].(*int32)
		if !ok || value == nil {
			return OPUS_BAD_ARG // goto bad_arg
		}
		*value = int32(demixing_matrix.gain)
	case OPUS_PROJECTION_GET_DEMIXING_MATRIX_REQUEST:
		var i, j, k, l int
		var nb_input_streams int
		var nb_output_streams int
		var external_char []byte
		var internal_short []int16
		var external_size int32
		var internal_size int32

		// (I/O is in relation to the decoder's perspective).
		nb_input_streams = ms_encoder.layout.nb_streams +
			ms_encoder.layout.nb_coupled_streams
		nb_output_streams = ms_encoder.layout.nb_channels

		external_char, ok := args[0].([]byte)
		if !ok {
			return OPUS_BAD_ARG // goto bad_arg
		}
		external_size, ok = argInt32(args, 1)
		if !ok {
			return OPUS_BAD_ARG // goto bad_arg
		}
		if external_char == nil {
			return OPUS_BAD_ARG // goto bad_arg
		}
		internal_short = demixing_matrix.MappingMatrixGetData()
		internal_size = int32(nb_input_streams * nb_output_streams * 2) // sizeof(opus_int16)
		if external_size != internal_size {
			return OPUS_BAD_ARG // goto bad_arg
		}

		// Copy demixing matrix subset to output destination.
		l = 0
		for i = 0; i < nb_input_streams; i++ {
			for j = 0; j < nb_output_streams; j++ {
				k = demixing_matrix.rows*i + j
				external_char[2*l] = byte(internal_short[k])
				external_char[2*l+1] = byte(internal_short[k] >> 8)
				l++
			}
		}
	default:
		ret = ms_encoder.Ctl(request, args...)
	}
	return ret
}
