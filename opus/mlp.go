package opus

import "math"

// C: src/mlp.c, src/mlp.h

// MLP is the C struct MLP.
type MLP struct {
	layers  int
	topo    []int
	weights []float32
}

const MAX_NEURONS = 100

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

// C: void mlp_process(const MLP *m, const float *in, float *out)
func mlp_process(m *MLP, in []float32, out []float32) {
	var j int
	var hidden [MAX_NEURONS]float32
	W := m.weights
	w := 0 // index of the C `W` pointer
	// Copy to tmp_in
	for j = 0; j < m.topo[1]; j++ {
		var k int
		var sum float32 = W[w]
		w++
		for k = 0; k < m.topo[0]; k++ {
			sum = sum + float32(in[k]*W[w])
			w++
		}
		hidden[j] = tansig_approx(sum)
	}
	for j = 0; j < m.topo[2]; j++ {
		var k int
		var sum float32 = W[w]
		w++
		for k = 0; k < m.topo[1]; k++ {
			sum = sum + float32(hidden[k]*W[w])
			w++
		}
		out[j] = tansig_approx(sum)
	}
}
