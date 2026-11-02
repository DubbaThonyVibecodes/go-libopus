package silk

// C: silk/A2NLSF.c

// Conversion between prediction filter coefficients and NLSFs
// Requires the order to be an even number
// A piecewise linear approximation maps LSF <-> cos(LSF)
// Therefore the result is not accurate NLSFs, but the two
// functions are accurate inverses of each other

// Number of binary divisions, when not in low complexity mode
const BIN_DIV_STEPS_A2NLSF_FIX = 3 // must be no higher than 16 - log2( LSF_COS_TAB_SZ_FIX )
const MAX_ITERATIONS_A2NLSF_FIX = 16

// Helper function for A2NLSF(..)
// Transforms polynomials from cos(n*f) to cos(f)^n
//
// C: static OPUS_INLINE void silk_A2NLSF_trans_poly(opus_int32 *p, const opus_int dd)
func a2NLSFTransPoly(
	p []int32, // I/O    Polynomial
	dd int, // I      Polynomial order (= filter order / 2 )
) {
	var k, n int

	for k = 2; k <= dd; k++ {
		for n = dd; n > k; n-- {
			p[n-2] -= p[n]
		}
		p[k-2] -= LSHIFT(p[k], 1)
	}
}

// Helper function for A2NLSF(..)
// Polynomial evaluation
//
// C: static OPUS_INLINE opus_int32 silk_A2NLSF_eval_poly(opus_int32 *p, const opus_int32 x, const opus_int dd)
func a2NLSFEvalPoly( // return the polynomial evaluation, in Q16
	p []int32, // I    Polynomial, Q16
	x int32, // I    Evaluation point, Q12
	dd int, // I    Order
) int32 {
	var n int
	var x_Q16, y32 int32

	y32 = p[dd] // Q16
	x_Q16 = LSHIFT(x, 4)

	if 8 == dd { // opus_likely
		y32 = SMLAWW(p[7], y32, x_Q16)
		y32 = SMLAWW(p[6], y32, x_Q16)
		y32 = SMLAWW(p[5], y32, x_Q16)
		y32 = SMLAWW(p[4], y32, x_Q16)
		y32 = SMLAWW(p[3], y32, x_Q16)
		y32 = SMLAWW(p[2], y32, x_Q16)
		y32 = SMLAWW(p[1], y32, x_Q16)
		y32 = SMLAWW(p[0], y32, x_Q16)
	} else {
		for n = dd - 1; n >= 0; n-- {
			y32 = SMLAWW(p[n], y32, x_Q16) // Q16
		}
	}
	return y32
}

// C: static OPUS_INLINE void silk_A2NLSF_init(const opus_int32 *a_Q16, opus_int32 *P, opus_int32 *Q, const opus_int dd)
func a2NLSFInit(
	a_Q16 []int32,
	P []int32,
	Q []int32,
	dd int,
) {
	var k int

	// Convert filter coefs to even and odd polynomials
	P[dd] = LSHIFT(1, 16)
	Q[dd] = LSHIFT(1, 16)
	for k = 0; k < dd; k++ {
		P[k] = -a_Q16[dd-k-1] - a_Q16[dd+k] // Q16
		Q[k] = -a_Q16[dd-k-1] + a_Q16[dd+k] // Q16
	}

	// Divide out zeros as we have that for even filter orders,
	// z =  1 is always a root in Q, and
	// z = -1 is always a root in P
	for k = dd; k > 0; k-- {
		P[k-1] -= P[k]
		Q[k-1] += Q[k]
	}

	// Transform polynomials from cos(n*f) to cos(f)^n
	a2NLSFTransPoly(P, dd)
	a2NLSFTransPoly(Q, dd)
}

// Compute Normalized Line Spectral Frequencies (NLSFs) from whitening filter coefficients
// If not all roots are found, the a_Q16 coefficients are bandwidth expanded until convergence.
//
// C: void silk_A2NLSF(opus_int16 *NLSF, opus_int32 *a_Q16, const opus_int d)
func A2NLSF(
	NLSF []int16, // O    Normalized Line Spectral Frequencies in Q15 (0..2^15-1) [d]
	a_Q16 []int32, // I/O  Monic whitening filter coefficients in Q16 [d]
	d int, // I    Filter order (must be even)
) {
	var i, k, m, dd, root_ix, ffrac int
	var xlo, xhi, xmid int32
	var ylo, yhi, ymid, thr int32
	var nom, den int32
	var P [SILK_MAX_ORDER_LPC/2 + 1]int32
	var Q [SILK_MAX_ORDER_LPC/2 + 1]int32
	var PQ [2][]int32
	var p []int32

	// Store pointers to array
	PQ[0] = P[:]
	PQ[1] = Q[:]

	dd = RSHIFT(d, 1)

	a2NLSFInit(a_Q16, P[:], Q[:], dd)

	// Find roots, alternating between P and Q
	p = P[:] // Pointer to polynomial

	xlo = int32(LSFCosTab_FIX_Q12[0]) // Q12
	ylo = a2NLSFEvalPoly(p, xlo, dd)

	if ylo < 0 {
		// Set the first NLSF to zero and move on to the next
		NLSF[0] = 0
		p = Q[:] // Pointer to polynomial
		ylo = a2NLSFEvalPoly(p, xlo, dd)
		root_ix = 1 // Index of current root
	} else {
		root_ix = 0 // Index of current root
	}
	k = 1 // Loop counter
	i = 0 // Counter for bandwidth expansions applied
	thr = 0
	for {
		// Evaluate polynomial
		xhi = int32(LSFCosTab_FIX_Q12[k]) // Q12
		yhi = a2NLSFEvalPoly(p, xhi, dd)

		// Detect zero crossing
		if (ylo <= 0 && yhi >= thr) || (ylo >= 0 && yhi <= -thr) {
			if yhi == 0 {
				// If the root lies exactly at the end of the current
				// interval, look for the next root in the next interval
				thr = 1
			} else {
				thr = 0
			}
			// Binary division
			ffrac = -256
			for m = 0; m < BIN_DIV_STEPS_A2NLSF_FIX; m++ {
				// Evaluate polynomial
				xmid = RSHIFT_ROUND(xlo+xhi, 1)
				ymid = a2NLSFEvalPoly(p, xmid, dd)

				// Detect zero crossing
				if (ylo <= 0 && ymid >= 0) || (ylo >= 0 && ymid <= 0) {
					// Reduce frequency
					xhi = xmid
					yhi = ymid
				} else {
					// Increase frequency
					xlo = xmid
					ylo = ymid
					ffrac = int(ADD_RSHIFT(ffrac, 128, m))
				}
			}

			// Interpolate
			if abs(ylo) < 65536 {
				// Avoid dividing by zero
				den = ylo - yhi
				nom = LSHIFT(ylo, 8-BIN_DIV_STEPS_A2NLSF_FIX) + RSHIFT(den, 1)
				if den != 0 {
					ffrac += int(DIV32(nom, den))
				}
			} else {
				// No risk of dividing by zero because abs(ylo - yhi) >= abs(ylo) >= 65536
				ffrac += int(DIV32(ylo, RSHIFT(ylo-yhi, 8-BIN_DIV_STEPS_A2NLSF_FIX)))
			}
			NLSF[root_ix] = int16(min_32(LSHIFT(int32(k), 8)+int32(ffrac), int32(int16_MAX)))

			silk_assert(NLSF[root_ix] >= 0)

			root_ix++ // Next root
			if root_ix >= d {
				// Found all roots
				break
			}
			// Alternate pointer to polynomial
			p = PQ[root_ix&1]

			// Evaluate polynomial
			xlo = int32(LSFCosTab_FIX_Q12[k-1]) // Q12
			ylo = LSHIFT(1-(root_ix&2), 12)
		} else {
			// Increment loop counter
			k++
			xlo = xhi
			ylo = yhi
			thr = 0

			if k > LSF_COS_TAB_SZ_FIX {
				i++
				if i > MAX_ITERATIONS_A2NLSF_FIX {
					// Set NLSFs to white spectrum and exit
					NLSF[0] = int16(DIV32_16(1<<15, d+1))
					for k = 1; k < d; k++ {
						NLSF[k] = int16(ADD16(NLSF[k-1], NLSF[0]))
					}
					return
				}

				// Error: Apply progressively more bandwidth expansion and run again
				Bwexpander32(a_Q16, d, 65536-LSHIFT(1, i))

				a2NLSFInit(a_Q16, P[:], Q[:], dd)
				p = P[:]                          // Pointer to polynomial
				xlo = int32(LSFCosTab_FIX_Q12[0]) // Q12
				ylo = a2NLSFEvalPoly(p, xlo, dd)
				if ylo < 0 {
					// Set the first NLSF to zero and move on to the next
					NLSF[0] = 0
					p = Q[:] // Pointer to polynomial
					ylo = a2NLSFEvalPoly(p, xlo, dd)
					root_ix = 1 // Index of current root
				} else {
					root_ix = 0 // Index of current root
				}
				k = 1 // Reset loop counter
			}
		}
	}
}
