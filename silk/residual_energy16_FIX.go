package silk

// C: silk/fixed/residual_energy16_FIX.c

// Residual energy: nrg = wxx - 2 * wXx * c + c' * wXX * c
//
// C: opus_int32 silk_residual_energy16_covar_FIX(const opus_int16 *c, const opus_int32 *wXX, const opus_int32 *wXx, opus_int32 wxx, opus_int D, opus_int cQ)
func ResidualEnergy16CovarFIX(
	c []int16, // I    Prediction vector
	wXX []int32, // I    Correlation matrix
	wXx []int32, // I    Correlation vector
	wxx int32, // I    Signal energy
	D int, // I    Dimension
	cQ int, // I    Q value for c vector 0 - 15
) int32 {
	var i, j, lshifts, Qxtra int
	var c_max, w_max, tmp, tmp2, nrg int32
	var cn [MAX_MATRIX_SIZE]int
	var pRow []int32

	// Safety checks
	silk_assert(D >= 0)
	silk_assert(D <= 16)
	silk_assert(cQ > 0)
	silk_assert(cQ < 16)

	lshifts = 16 - cQ
	Qxtra = lshifts

	c_max = 0
	for i = 0; i < D; i++ {
		c_max = max_32(c_max, abs(int32(c[i])))
	}
	Qxtra = min_int(Qxtra, int(CLZ32(c_max))-17)

	w_max = max_32(wXX[0], wXX[D*D-1])
	Qxtra = min_int(Qxtra, int(CLZ32(MUL(D, RSHIFT(SMULWB(w_max, c_max), 4))))-5)
	Qxtra = max_int(Qxtra, 0)
	for i = 0; i < D; i++ {
		cn[i] = int(LSHIFT(int(c[i]), Qxtra))
		silk_assert(abs(cn[i]) <= (int(int16_MAX) + 1)) // Check that silk_SMLAWB can be used
	}
	lshifts -= Qxtra

	// Compute wxx - 2 * wXx * c
	tmp = 0
	for i = 0; i < D; i++ {
		tmp = SMLAWB(tmp, wXx[i], cn[i])
	}
	nrg = RSHIFT(wxx, 1+lshifts) - tmp // Q: -lshifts - 1

	// Add c' * wXX * c, assuming wXX is symmetric
	tmp2 = 0
	for i = 0; i < D; i++ {
		tmp = 0
		pRow = wXX[i*D:]
		for j = i + 1; j < D; j++ {
			tmp = SMLAWB(tmp, pRow[j], cn[j])
		}
		tmp = SMLAWB(tmp, RSHIFT(pRow[i], 1), cn[i])
		tmp2 = SMLAWB(tmp2, tmp, cn[i])
	}
	nrg = ADD_LSHIFT32(nrg, tmp2, lshifts) // Q: -lshifts - 1

	// Keep one bit free always, because we add them for LSF interpolation
	if nrg < 1 {
		nrg = 1
	} else if nrg > RSHIFT(int32_MAX, lshifts+2) {
		nrg = int32_MAX >> 1
	} else {
		nrg = LSHIFT(nrg, lshifts+1) // Q0
	}
	return nrg

}
