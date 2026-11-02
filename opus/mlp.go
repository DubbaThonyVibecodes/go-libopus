package opus

import "math"

// C: src/mlp.c, src/mlp.h

// C: #define WEIGHTS_SCALE (1.f/128)
const WEIGHTS_SCALE float32 = 1.0 / 128

const MAX_NEURONS = 32

// DenseLayer is the C struct DenseLayer.
type DenseLayer struct {
	bias          []int8
	input_weights []int8
	nb_inputs     int
	nb_neurons    int
	sigmoid       int
}

// GRULayer is the C struct GRULayer.
type GRULayer struct {
	bias              []int8
	input_weights     []int8
	recurrent_weights []int8
	nb_inputs         int
	nb_neurons        int
}

// C: static OPUS_INLINE float tansig_approx(float x)
func tansig_approx(x float32) float32 {
	var i int
	var y, dy float32
	var sign float32 = 1
	// Tests are reversed to catch NaNs
	if !(x < 8) {
		return 1
	}
	if !(x > -8) {
		return -1
	}
	if x < 0 {
		x = -x
		sign = -1
	}
	i = int(math.Floor(float64(float32(25*x) + 0.5)))
	x -= float32(0.04 * float32(i))
	y = tansig_table[i]
	dy = 1 - float32(y*y)
	y = y + float32(float32(x*dy)*(1-float32(y*x)))
	return sign * y
}

// C: static OPUS_INLINE float sigmoid_approx(float x)
func sigmoid_approx(x float32) float32 {
	return 0.5 + float32(0.5*tansig_approx(0.5*x))
}

// C: static void gemm_accum(float *out, const opus_int8 *weights, int rows, int cols, int col_stride, const float *x)
func gemm_accum(out []float32, weights []int8, rows int, cols int, col_stride int, x []float32) {
	var i, j int
	for i = 0; i < rows; i++ {
		for j = 0; j < cols; j++ {
			out[i] += float32(float32(weights[j*col_stride+i]) * x[j])
		}
	}
}

// C: void compute_dense(const DenseLayer *layer, float *output, const float *input)
func compute_dense(layer *DenseLayer, output []float32, input []float32) {
	var i int
	var N, M int
	var stride int
	M = layer.nb_inputs
	N = layer.nb_neurons
	stride = N
	for i = 0; i < N; i++ {
		output[i] = float32(layer.bias[i])
	}
	gemm_accum(output, layer.input_weights, N, M, stride, input)
	for i = 0; i < N; i++ {
		output[i] *= WEIGHTS_SCALE
	}
	if layer.sigmoid != 0 {
		for i = 0; i < N; i++ {
			output[i] = sigmoid_approx(output[i])
		}
	} else {
		for i = 0; i < N; i++ {
			output[i] = tansig_approx(output[i])
		}
	}
}

// C: void compute_gru(const GRULayer *gru, float *state, const float *input)
func compute_gru(gru *GRULayer, state []float32, input []float32) {
	var i int
	var N, M int
	var stride int
	var tmp [MAX_NEURONS]float32
	var z [MAX_NEURONS]float32
	var r [MAX_NEURONS]float32
	var h [MAX_NEURONS]float32
	M = gru.nb_inputs
	N = gru.nb_neurons
	stride = 3 * N
	// Compute update gate.
	for i = 0; i < N; i++ {
		z[i] = float32(gru.bias[i])
	}
	gemm_accum(z[:], gru.input_weights, N, M, stride, input)
	gemm_accum(z[:], gru.recurrent_weights, N, N, stride, state)
	for i = 0; i < N; i++ {
		z[i] = sigmoid_approx(WEIGHTS_SCALE * z[i])
	}

	// Compute reset gate.
	for i = 0; i < N; i++ {
		r[i] = float32(gru.bias[N+i])
	}
	gemm_accum(r[:], gru.input_weights[N:], N, M, stride, input)
	gemm_accum(r[:], gru.recurrent_weights[N:], N, N, stride, state)
	for i = 0; i < N; i++ {
		r[i] = sigmoid_approx(WEIGHTS_SCALE * r[i])
	}

	// Compute output.
	for i = 0; i < N; i++ {
		h[i] = float32(gru.bias[2*N+i])
	}
	for i = 0; i < N; i++ {
		tmp[i] = state[i] * r[i]
	}
	gemm_accum(h[:], gru.input_weights[2*N:], N, M, stride, input)
	gemm_accum(h[:], gru.recurrent_weights[2*N:], N, N, stride, tmp[:])
	for i = 0; i < N; i++ {
		h[i] = float32(z[i]*state[i]) + float32((1-z[i])*tansig_approx(WEIGHTS_SCALE*h[i]))
	}
	for i = 0; i < N; i++ {
		state[i] = h[i]
	}
}
