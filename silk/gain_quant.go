package silk

// C: silk/gain_quant.c

const (
	OFFSET        = ((MIN_QGAIN_DB*128)/6 + 16*128)
	SCALE_Q16     = ((65536 * (N_LEVELS_QGAIN - 1)) / (((MAX_QGAIN_DB - MIN_QGAIN_DB) * 128) / 6))
	INV_SCALE_Q16 = ((65536 * (((MAX_QGAIN_DB - MIN_QGAIN_DB) * 128) / 6)) / (N_LEVELS_QGAIN - 1))
)

/* Gain scalar quantization with hysteresis, uniform on log scale */
//
// C: void silk_gains_quant(opus_int8 ind[ MAX_NB_SUBFR ], opus_int32 gain_Q16[ MAX_NB_SUBFR ],
// opus_int8 *prev_ind, const opus_int conditional, const opus_int nb_subfr)
func GainsQuant(ind []int8, gain_Q16 []int32, prev_ind *int8, conditional int, nb_subfr int) {
	var k, double_step_size_threshold int

	for k = 0; k < nb_subfr; k++ {
		/* Convert to log scale, scale, floor() */
		ind[k] = int8(SMULWB(SCALE_Q16, Lin2log(gain_Q16[k])-OFFSET))

		/* Round towards previous quantized gain (hysteresis) */
		if ind[k] < *prev_ind {
			ind[k]++
		}
		ind[k] = int8(LIMIT_int(int(ind[k]), 0, N_LEVELS_QGAIN-1))

		/* Compute delta indices and limit */
		if k == 0 && conditional == 0 {
			/* Full index */
			ind[k] = int8(LIMIT_int(int(ind[k]), int(*prev_ind)+MIN_DELTA_GAIN_QUANT, N_LEVELS_QGAIN-1))
			*prev_ind = ind[k]
		} else {
			/* Delta index */
			ind[k] = int8(int(ind[k]) - int(*prev_ind))

			/* Double the quantization step size for large gain increases, so that the max gain level can be reached */
			double_step_size_threshold = 2*MAX_DELTA_GAIN_QUANT - N_LEVELS_QGAIN + int(*prev_ind)
			if int(ind[k]) > double_step_size_threshold {
				ind[k] = int8(double_step_size_threshold + RSHIFT(int(ind[k])-double_step_size_threshold+1, 1))
			}

			ind[k] = int8(LIMIT_int(int(ind[k]), MIN_DELTA_GAIN_QUANT, MAX_DELTA_GAIN_QUANT))

			/* Accumulate deltas */
			if int(ind[k]) > double_step_size_threshold {
				*prev_ind = int8(int(*prev_ind) + int(LSHIFT(ind[k], 1)) - double_step_size_threshold)
			} else {
				*prev_ind += ind[k]
			}

			/* Shift to make non-negative */
			ind[k] -= MIN_DELTA_GAIN_QUANT
		}

		/* Scale and convert to linear scale */
		gain_Q16[k] = Log2lin(min_32(SMULWB(INV_SCALE_Q16, *prev_ind)+OFFSET, 3967)) /* 3967 = 31 in Q7 */
	}
}

/* Gains scalar dequantization, uniform on log scale */
//
// C: void silk_gains_dequant(opus_int32 gain_Q16[ MAX_NB_SUBFR ], const opus_int8 ind[ MAX_NB_SUBFR ],
// opus_int8 *prev_ind, const opus_int conditional, const opus_int nb_subfr)
func GainsDequant(gain_Q16 []int32, ind []int8, prev_ind *int8, conditional int, nb_subfr int) {
	var k, ind_tmp, double_step_size_threshold int

	for k = 0; k < nb_subfr; k++ {
		if k == 0 && conditional == 0 {
			/* Gain index is not allowed to go down more than 16 steps (~21.8 dB) */
			*prev_ind = int8(max_int(int(ind[k]), int(*prev_ind)-16))
		} else {
			/* Delta index */
			ind_tmp = int(ind[k]) + MIN_DELTA_GAIN_QUANT

			/* Accumulate deltas */
			double_step_size_threshold = 2*MAX_DELTA_GAIN_QUANT - N_LEVELS_QGAIN + int(*prev_ind)
			if ind_tmp > double_step_size_threshold {
				*prev_ind = int8(int(*prev_ind) + int(LSHIFT(ind_tmp, 1)) - double_step_size_threshold)
			} else {
				*prev_ind = int8(int(*prev_ind) + ind_tmp)
			}
		}
		*prev_ind = int8(LIMIT_int(int(*prev_ind), 0, N_LEVELS_QGAIN-1))

		/* Scale and convert to linear scale */
		gain_Q16[k] = Log2lin(min_32(SMULWB(INV_SCALE_Q16, *prev_ind)+OFFSET, 3967)) /* 3967 = 31 in Q7 */
	}
}

/* Compute unique identifier of gain indices vector */
//
// C: opus_int32 silk_gains_ID(const opus_int8 ind[ MAX_NB_SUBFR ], const opus_int nb_subfr)
func GainsID(ind []int8, nb_subfr int) int32 {
	var k int
	var gainsID int32

	gainsID = 0
	for k = 0; k < nb_subfr; k++ {
		gainsID = ADD_LSHIFT32(ind[k], gainsID, 8)
	}

	return gainsID
}
