package silk

// C: silk/decode_pitch.c

/***********************************************************
* Pitch analyser function
********************************************************** */
//
// C: void silk_decode_pitch(opus_int16 lagIndex, opus_int8 contourIndex, opus_int pitch_lags[],
// const opus_int Fs_kHz, const opus_int nb_subfr)
func DecodePitch(lagIndex int16, contourIndex int8, pitch_lags []int, Fs_kHz int, nb_subfr int) {
	var lag, k, min_lag, max_lag, cbk_size int
	/* The C code points at the flat codebook and indexes it with matrix_ptr( ptr, k, contourIndex,
	   cbk_size ); the Go tables are 2-D arrays with row width cbk_size, so keep the rows. */
	var Lag_CB_ptr [PE_MAX_NB_SUBFR][]int8

	if Fs_kHz == 8 {
		if nb_subfr == PE_MAX_NB_SUBFR {
			for k = 0; k < PE_MAX_NB_SUBFR; k++ {
				Lag_CB_ptr[k] = CB_lags_stage2[k][:]
			}
			cbk_size = PE_NB_CBKS_STAGE2_EXT
		} else {
			silk_assert(nb_subfr == PE_MAX_NB_SUBFR>>1)
			for k = 0; k < PE_MAX_NB_SUBFR>>1; k++ {
				Lag_CB_ptr[k] = CB_lags_stage2_10_ms[k][:]
			}
			cbk_size = PE_NB_CBKS_STAGE2_10MS
		}
	} else {
		if nb_subfr == PE_MAX_NB_SUBFR {
			for k = 0; k < PE_MAX_NB_SUBFR; k++ {
				Lag_CB_ptr[k] = CB_lags_stage3[k][:]
			}
			cbk_size = PE_NB_CBKS_STAGE3_MAX
		} else {
			silk_assert(nb_subfr == PE_MAX_NB_SUBFR>>1)
			for k = 0; k < PE_MAX_NB_SUBFR>>1; k++ {
				Lag_CB_ptr[k] = CB_lags_stage3_10_ms[k][:]
			}
			cbk_size = PE_NB_CBKS_STAGE3_10MS
		}
	}

	min_lag = int(SMULBB(PE_MIN_LAG_MS, Fs_kHz))
	max_lag = int(SMULBB(PE_MAX_LAG_MS, Fs_kHz))
	lag = min_lag + int(lagIndex)

	for k = 0; k < nb_subfr; k++ {
		/* matrix_ptr( Lag_CB_ptr, k, contourIndex, cbk_size ): row k of the codebook, cbk_size entries wide */
		pitch_lags[k] = lag + int(Lag_CB_ptr[k][:cbk_size][contourIndex])
		pitch_lags[k] = LIMIT(pitch_lags[k], min_lag, max_lag)
	}
}
