package opus

// C: src/mlp.c, src/mlp.h

// C: #define WEIGHTS_SCALE (1.f/128)
const WEIGHTS_SCALE float32 = 1.0 / 128

const MAX_NEURONS = 32

// AnalysisDenseLayer is the C struct AnalysisDenseLayer.
type AnalysisDenseLayer struct {
	bias          []int8
	input_weights []int8
	nb_inputs     int
	nb_neurons    int
	sigmoid       int
}

// AnalysisGRULayer is the C struct AnalysisGRULayer.
type AnalysisGRULayer struct {
	bias              []int8
	input_weights     []int8
	recurrent_weights []int8
	nb_inputs         int
	nb_neurons        int
}

// C: #define fmadd(a, b, c) ((a)*(b)+(c))
func fmadd(a, b, c float32) float32 { return float32(a*b) + c }

// C: static OPUS_INLINE float tansig_approx(float x)
func tansig_approx(x float32) float32 {
	const N0 float32 = 952.52801514
	const N1 float32 = 96.39235687
	const N2 float32 = 0.60863042
	const D0 float32 = 952.72399902
	const D1 float32 = 413.36801147
	const D2 float32 = 11.88600922
	var X2, num, den float32
	X2 = x * x
	num = fmadd(fmadd(N2, X2, N1), X2, N0)
	den = fmadd(fmadd(D2, X2, D1), X2, D0)
	num = num * x / den
	// MAX32/MIN32 on floats (plain conditionals in the fixed-point arch.h)
	return maxf(-1.0, minf(1.0, num))
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

// C: void analysis_compute_dense(const AnalysisDenseLayer *layer, float *output, const float *input)
func analysis_compute_dense(layer *AnalysisDenseLayer, output []float32, input []float32) {
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

// C: void analysis_compute_gru(const AnalysisGRULayer *gru, float *state, const float *input)
func analysis_compute_gru(gru *AnalysisGRULayer, state []float32, input []float32) {
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
