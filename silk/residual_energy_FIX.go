package silk

// C: silk/fixed/residual_energy_FIX.c

// Calculates residual energies of input subframes where all subframes have LPC_order
// of preceding samples
//
// C: void silk_residual_energy_FIX(opus_int32 nrgs[ MAX_NB_SUBFR ], opus_int nrgsQ[ MAX_NB_SUBFR ], const opus_int16 x[], opus_int16 a_Q12[ 2 ][ MAX_LPC_ORDER ], const opus_int32 gains[ MAX_NB_SUBFR ], const opus_int subfr_length, const opus_int nb_subfr, const opus_int LPC_order, int arch)
func ResidualEnergyFIX(
	nrgs []int32, // O    Residual energy per subframe
	nrgsQ []int, // O    Q value per subframe
	x []int16, // I    Input signal
	a_Q12 *[2][MAX_LPC_ORDER]int16, // I    AR coefs for each frame half
	gains []int32, // I    Quantization gains
	subfr_length int, // I    Subframe length
	nb_subfr int, // I    Number of subframes
	LPC_order int, // I    LPC order
	arch int, // I    Run-time architecture
) {
	var offset, i, j, rshift, lz1, lz2 int
	var LPC_res_ptr int // C: opus_int16 *LPC_res_ptr; index into LPC_res[]
	var LPC_res []int16
	var x_ptr int // C: const opus_int16 *x_ptr; index into x[]
	var tmp32 int32

	x_ptr = 0
	offset = LPC_order + subfr_length

	// Filter input to create the LPC residual for each frame half, and measure subframe energies
	LPC_res = make([]int16, (MAX_NB_SUBFR>>1)*offset)
	silk_assert((nb_subfr>>1)*(MAX_NB_SUBFR>>1) == nb_subfr)
	for i = 0; i < nb_subfr>>1; i++ {
		// Calculate half frame LPC residual signal including preceding samples
		LPCAnalysisFilter(LPC_res, x[x_ptr:], a_Q12[i][:], int32((MAX_NB_SUBFR>>1)*offset), int32(LPC_order), arch)

		// Point to first subframe of the just calculated LPC residual signal
		LPC_res_ptr = LPC_order
		for j = 0; j < (MAX_NB_SUBFR >> 1); j++ {
			// Measure subframe energy
			SumSqrShift(&nrgs[i*(MAX_NB_SUBFR>>1)+j], &rshift, LPC_res[LPC_res_ptr:], subfr_length)

			// Set Q values for the measured energy
			nrgsQ[i*(MAX_NB_SUBFR>>1)+j] = -rshift

			// Move to next subframe
			LPC_res_ptr += offset
		}
		// Move to next frame half
		x_ptr += (MAX_NB_SUBFR >> 1) * offset
	}

	// Apply the squared subframe gains
	for i = 0; i < nb_subfr; i++ {
		// Fully upscale gains and energies
		lz1 = int(CLZ32(nrgs[i])) - 1
		lz2 = int(CLZ32(gains[i])) - 1

		tmp32 = LSHIFT32(gains[i], lz2)

		// Find squared gains
		tmp32 = SMMUL(tmp32, tmp32) // Q( 2 * lz2 - 32 )

		// Scale energies
		nrgs[i] = SMMUL(tmp32, LSHIFT32(nrgs[i], lz1)) // Q( nrgsQ[ i ] + lz1 + 2 * lz2 - 32 - 32 )
		nrgsQ[i] += lz1 + 2*lz2 - 32 - 32
	}
}
