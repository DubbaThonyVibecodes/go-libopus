package opus

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// C: src/mapping_matrix.c, src/mapping_matrix.h

// MappingMatrix is the C struct MappingMatrix. The matrix cell data that C
// stores after the struct (col-wise ordering) is the data slice.
type MappingMatrix struct {
	rows int // number of channels outputted from matrix.
	cols int // number of channels inputted to matrix.
	gain int // in dB. S7.8-format.
	// Matrix cell data goes here using col-wise ordering.
	data []int16
}

// C: #define MATRIX_INDEX(nb_rows, row, col) (nb_rows * col + row)
func MATRIX_INDEX(nb_rows, row, col int) int { return nb_rows*col + row }

// MappingMatrixGetSize returns the C allocation size (for API parity).
//
// C: opus_int32 mapping_matrix_get_size(int rows, int cols)
func MappingMatrixGetSize(rows int, cols int) int32 {
	var size int32

	// Mapping Matrix must only support up to 255 channels in or out.
	// Additionally, the total cell count must be <= 65004 octets in order
	// for the matrix to be stored in an OGG header.
	if rows > 255 || cols > 255 {
		return 0
	}
	size = int32(rows) * int32(cols) * 2 // sizeof(opus_int16)
	if size > 65004 {
		return 0
	}

	return int32(align(12) + align(int(size))) // sizeof(MappingMatrix) == 12
}

// C: opus_int16 *mapping_matrix_get_data(const MappingMatrix *matrix)
func (matrix *MappingMatrix) MappingMatrixGetData() []int16 {
	return matrix.data
}

// C: void mapping_matrix_init(MappingMatrix * const matrix, int rows, int cols, int gain, const opus_int16 *data, opus_int32 data_size)
func (matrix *MappingMatrix) MappingMatrixInit(rows int, cols int, gain int, data []int16, data_size int32) {
	var i int
	var ptr []int16

	celt.Assert(align(int(data_size)) == align(rows*cols*2))

	matrix.rows = rows
	matrix.cols = cols
	matrix.gain = gain
	matrix.data = make([]int16, rows*cols)
	ptr = matrix.MappingMatrixGetData()
	for i = 0; i < rows*cols; i++ {
		ptr[i] = data[i]
	}
}

// C: void mapping_matrix_multiply_channel_in_float(const MappingMatrix *matrix, const float *input, int input_rows, opus_val16 *output, int output_row, int output_rows, int frame_size)
func (matrix *MappingMatrix) MappingMatrixMultiplyChannelInFloat(
	input []float32,
	input_rows int,
	output []int16,
	output_row int,
	output_rows int,
	frame_size int) {
	// Matrix data is ordered col-wise.
	var matrix_data []int16
	var i, col int

	celt.Assert(input_rows <= matrix.cols && output_rows <= matrix.rows)

	matrix_data = matrix.MappingMatrixGetData()

	for i = 0; i < frame_size; i++ {
		var tmp float32 = 0
		for col = 0; col < input_rows; col++ {
			tmp += float32(float32(matrix_data[MATRIX_INDEX(matrix.rows, output_row, col)]) *
				input[MATRIX_INDEX(input_rows, col, i)])
		}
		// FIXED_POINT
		output[output_rows*i] = FLOAT2INT16((1 / 32768.0) * tmp)
	}
}

// C: void mapping_matrix_multiply_channel_out_float(const MappingMatrix *matrix, const opus_val16 *input, int input_row, int input_rows, float *output, int output_rows, int frame_size)
func (matrix *MappingMatrix) MappingMatrixMultiplyChannelOutFloat(
	input []int16,
	input_row int,
	input_rows int,
	output []float32,
	output_rows int,
	frame_size int) {
	// Matrix data is ordered col-wise.
	var matrix_data []int16
	var i, row int
	var input_sample float32

	celt.Assert(input_rows <= matrix.cols && output_rows <= matrix.rows)

	matrix_data = matrix.MappingMatrixGetData()

	for i = 0; i < frame_size; i++ {
		// FIXED_POINT
		input_sample = (1 / 32768.0) * float32(input[input_rows*i])
		for row = 0; row < output_rows; row++ {
			var tmp float32 = float32(float32((1/32768.0)*float32(matrix_data[MATRIX_INDEX(matrix.rows, row, input_row)])) *
				input_sample)
			output[MATRIX_INDEX(output_rows, row, i)] += tmp
		}
	}
}

// C: void mapping_matrix_multiply_channel_in_short(const MappingMatrix *matrix, const opus_int16 *input, int input_rows, opus_val16 *output, int output_row, int output_rows, int frame_size)
func (matrix *MappingMatrix) MappingMatrixMultiplyChannelInShort(
	input []int16,
	input_rows int,
	output []int16,
	output_row int,
	output_rows int,
	frame_size int) {
	// Matrix data is ordered col-wise.
	var matrix_data []int16
	var i, col int

	celt.Assert(input_rows <= matrix.cols && output_rows <= matrix.rows)

	matrix_data = matrix.MappingMatrixGetData()

	for i = 0; i < frame_size; i++ {
		var tmp int32 = 0
		for col = 0; col < input_rows; col++ {
			// FIXED_POINT
			tmp +=
				(int32(matrix_data[MATRIX_INDEX(matrix.rows, output_row, col)]) *
					int32(input[MATRIX_INDEX(input_rows, col, i)])) >> 8
		}
		// FIXED_POINT
		output[output_rows*i] = int16((tmp + 64) >> 7)
	}
}

// C: void mapping_matrix_multiply_channel_out_short(const MappingMatrix *matrix, const opus_val16 *input, int input_row, int input_rows, opus_int16 *output, int output_rows, int frame_size)
func (matrix *MappingMatrix) MappingMatrixMultiplyChannelOutShort(
	input []int16,
	input_row int,
	input_rows int,
	output []int16,
	output_rows int,
	frame_size int) {
	// Matrix data is ordered col-wise.
	var matrix_data []int16
	var i, row int
	var input_sample int32

	celt.Assert(input_rows <= matrix.cols && output_rows <= matrix.rows)

	matrix_data = matrix.MappingMatrixGetData()

	for i = 0; i < frame_size; i++ {
		// FIXED_POINT
		input_sample = int32(input[input_rows*i])
		for row = 0; row < output_rows; row++ {
			var tmp int32 = int32(matrix_data[MATRIX_INDEX(matrix.rows, row, input_row)]) *
				input_sample
			output[MATRIX_INDEX(output_rows, row, i)] = int16(int32(output[MATRIX_INDEX(output_rows, row, i)]) + ((tmp + 16384) >> 15))
		}
	}
}

// Pre-computed mixing and demixing matrices for 1st to 3rd-order ambisonics.
//   foa: first-order ambisonics
//   soa: second-order ambisonics
//   toa: third-order ambisonics

// C: const MappingMatrix mapping_matrix_foa_mixing = { 6, 6, 0 };
var MappingMatrixFoaMixing = MappingMatrix{6, 6, 0, nil}

// C: const opus_int16 mapping_matrix_foa_mixing_data[36]
var MappingMatrixFoaMixingData = [36]int16{
	16384, 0, -16384, 23170, 0, 0, 16384, 23170,
	16384, 0, 0, 0, 16384, 0, -16384, -23170,
	0, 0, 16384, -23170, 16384, 0, 0, 0,
	0, 0, 0, 0, 32767, 0, 0, 0,
	0, 0, 0, 32767,
}

// C: const MappingMatrix mapping_matrix_soa_mixing = { 11, 11, 0 };
var MappingMatrixSoaMixing = MappingMatrix{11, 11, 0, nil}

// C: const opus_int16 mapping_matrix_soa_mixing_data[121]
var MappingMatrixSoaMixingData = [121]int16{
	10923, 7723, 13377, -13377, 11585, 9459, 7723, -16384,
	-6689, 0, 0, 10923, 7723, 13377, 13377, -11585,
	9459, 7723, 16384, -6689, 0, 0, 10923, -15447,
	13377, 0, 0, -18919, 7723, 0, 13377, 0,
	0, 10923, 7723, -13377, -13377, 11585, -9459, 7723,
	16384, -6689, 0, 0, 10923, -7723, 0, 13377,
	-16384, 0, -15447, 0, 9459, 0, 0, 10923,
	-7723, 0, -13377, 16384, 0, -15447, 0, 9459,
	0, 0, 10923, 15447, 0, 0, 0, 0,
	-15447, 0, -18919, 0, 0, 10923, 7723, -13377,
	13377, -11585, -9459, 7723, -16384, -6689, 0, 0,
	10923, -15447, -13377, 0, 0, 18919, 7723, 0,
	13377, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 32767, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	32767,
}

// C: const MappingMatrix mapping_matrix_toa_mixing = { 18, 18, 0 };
var MappingMatrixToaMixing = MappingMatrix{18, 18, 0, nil}

// C: const opus_int16 mapping_matrix_toa_mixing_data[324]
var MappingMatrixToaMixingData = [324]int16{
	8208, 0, -881, 14369, 0, 0, -8192, -4163,
	13218, 0, 0, 0, 11095, -8836, -6218, 14833,
	0, 0, 8208, -10161, 881, 10161, -13218, -2944,
	-8192, 2944, 0, -10488, -6218, 6248, -11095, -6248,
	0, -10488, 0, 0, 8208, 10161, 881, -10161,
	-13218, 2944, -8192, -2944, 0, 10488, -6218, -6248,
	-11095, 6248, 0, 10488, 0, 0, 8176, 5566,
	-11552, 5566, 9681, -11205, 8192, -11205, 0, 4920,
	-15158, 9756, -3334, 9756, 0, -4920, 0, 0,
	8176, 7871, 11552, 0, 0, 15846, 8192, 0,
	-9681, -6958, 0, 13797, 3334, 0, -15158, 0,
	0, 0, 8176, 0, 11552, 7871, 0, 0,
	8192, 15846, 9681, 0, 0, 0, 3334, 13797,
	15158, 6958, 0, 0, 8176, 5566, -11552, -5566,
	-9681, -11205, 8192, 11205, 0, 4920, 15158, 9756,
	-3334, -9756, 0, 4920, 0, 0, 8208, 14369,
	-881, 0, 0, -4163, -8192, 0, -13218, -14833,
	0, -8836, 11095, 0, 6218, 0, 0, 0,
	8208, 10161, 881, 10161, 13218, 2944, -8192, 2944,
	0, 10488, 6218, -6248, -11095, -6248, 0, -10488,
	0, 0, 8208, -14369, -881, 0, 0, 4163,
	-8192, 0, -13218, 14833, 0, 8836, 11095, 0,
	6218, 0, 0, 0, 8208, 0, -881, -14369,
	0, 0, -8192, 4163, 13218, 0, 0, 0,
	11095, 8836, -6218, -14833, 0, 0, 8176, -5566,
	-11552, 5566, -9681, 11205, 8192, -11205, 0, -4920,
	15158, -9756, -3334, 9756, 0, -4920, 0, 0,
	8176, 0, 11552, -7871, 0, 0, 8192, -15846,
	9681, 0, 0, 0, 3334, -13797, 15158, -6958,
	0, 0, 8176, -7871, 11552, 0, 0, -15846,
	8192, 0, -9681, 6958, 0, -13797, 3334, 0,
	-15158, 0, 0, 0, 8176, -5566, -11552, -5566,
	9681, 11205, 8192, 11205, 0, -4920, -15158, -9756,
	-3334, -9756, 0, 4920, 0, 0, 8208, -10161,
	881, -10161, 13218, -2944, -8192, -2944, 0, -10488,
	6218, 6248, -11095, 6248, 0, 10488, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	32767, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 32767,
}

// C: const MappingMatrix mapping_matrix_foa_demixing = { 6, 6, 0 };
var MappingMatrixFoaDemixing = MappingMatrix{6, 6, 0, nil}

// C: const opus_int16 mapping_matrix_foa_demixing_data[36]
var MappingMatrixFoaDemixingData = [36]int16{
	16384, 16384, 16384, 16384, 0, 0, 0, 23170,
	0, -23170, 0, 0, -16384, 16384, -16384, 16384,
	0, 0, 23170, 0, -23170, 0, 0, 0,
	0, 0, 0, 0, 32767, 0, 0, 0,
	0, 0, 0, 32767,
}

// C: const MappingMatrix mapping_matrix_soa_demixing = { 11, 11, 3050 };
var MappingMatrixSoaDemixing = MappingMatrix{11, 11, 3050, nil}

// C: const opus_int16 mapping_matrix_soa_demixing_data[121]
var MappingMatrixSoaDemixingData = [121]int16{
	2771, 2771, 2771, 2771, 2771, 2771, 2771, 2771,
	2771, 0, 0, 10033, 10033, -20066, 10033, 14189,
	14189, -28378, 10033, -20066, 0, 0, 3393, 3393,
	3393, -3393, 0, 0, 0, -3393, -3393, 0,
	0, -17378, 17378, 0, -17378, -24576, 24576, 0,
	17378, 0, 0, 0, -14189, 14189, 0, -14189,
	-28378, 28378, 0, 14189, 0, 0, 0, 2399,
	2399, -4799, -2399, 0, 0, 0, -2399, 4799,
	0, 0, 1959, 1959, 1959, 1959, -3918, -3918,
	-3918, 1959, 1959, 0, 0, -4156, 4156, 0,
	4156, 0, 0, 0, -4156, 0, 0, 0,
	8192, 8192, -16384, 8192, 16384, 16384, -32768, 8192,
	-16384, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 8312, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	8312,
}

// C: const MappingMatrix mapping_matrix_toa_demixing = { 18, 18, 0 };
var MappingMatrixToaDemixing = MappingMatrix{18, 18, 0, nil}

// C: const opus_int16 mapping_matrix_toa_demixing_data[324]
var MappingMatrixToaDemixingData = [324]int16{
	8192, 8192, 8192, 8192, 8192, 8192, 8192, 8192,
	8192, 8192, 8192, 8192, 8192, 8192, 8192, 8192,
	0, 0, 0, -9779, 9779, 6263, 8857, 0,
	6263, 13829, 9779, -13829, 0, -6263, 0, -8857,
	-6263, -9779, 0, 0, -3413, 3413, 3413, -11359,
	11359, 11359, -11359, -3413, 3413, -3413, -3413, -11359,
	11359, 11359, -11359, 3413, 0, 0, 13829, 9779,
	-9779, 6263, 0, 8857, -6263, 0, 9779, 0,
	-13829, 6263, -8857, 0, -6263, -9779, 0, 0,
	0, -15617, -15617, 6406, 0, 0, -6406, 0,
	15617, 0, 0, -6406, 0, 0, 6406, 15617,
	0, 0, 0, -5003, 5003, -10664, 15081, 0,
	-10664, -7075, 5003, 7075, 0, 10664, 0, -15081,
	10664, -5003, 0, 0, -8176, -8176, -8176, 8208,
	8208, 8208, 8208, -8176, -8176, -8176, -8176, 8208,
	8208, 8208, 8208, -8176, 0, 0, -7075, 5003,
	-5003, -10664, 0, 15081, 10664, 0, 5003, 0,
	7075, -10664, -15081, 0, 10664, -5003, 0, 0,
	15617, 0, 0, 0, -6406, 6406, 0, -15617,
	0, -15617, 15617, 0, 6406, -6406, 0, 0,
	0, 0, 0, -11393, 11393, 2993, -4233, 0,
	2993, -16112, 11393, 16112, 0, -2993, 0, 4233,
	-2993, -11393, 0, 0, 0, -9974, -9974, -13617,
	0, 0, 13617, 0, 9974, 0, 0, 13617,
	0, 0, -13617, 9974, 0, 0, 0, 5579,
	-5579, 10185, 14403, 0, 10185, -7890, -5579, 7890,
	0, -10185, 0, -14403, -10185, 5579, 0, 0,
	11826, -11826, -11826, -901, 901, 901, -901, 11826,
	-11826, 11826, 11826, -901, 901, 901, -901, -11826,
	0, 0, -7890, -5579, 5579, 10185, 0, 14403,
	-10185, 0, -5579, 0, 7890, 10185, -14403, 0,
	-10185, 5579, 0, 0, -9974, 0, 0, 0,
	-13617, 13617, 0, 9974, 0, 9974, -9974, 0,
	13617, -13617, 0, 0, 0, 0, 16112, -11393,
	11393, -2993, 0, 4233, 2993, 0, -11393, 0,
	-16112, -2993, -4233, 0, 2993, 11393, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	32767, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 32767,
}
