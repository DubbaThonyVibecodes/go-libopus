package silk

// C: silk/LP_variable_cutoff.c

/*
   Elliptic/Cauer filters designed with 0.1 dB passband ripple,
   80 dB minimum stopband attenuation, and
   [0.95 : 0.15 : 0.35] normalized cut off frequencies.
*/

// Helper function, interpolates the filter taps
//
// C: static inline void silk_LP_interpolate_filter_taps(
//
//	opus_int32           B_Q28[ TRANSITION_NB ],
//	opus_int32           A_Q28[ TRANSITION_NA ],
//	const opus_int       ind,
//	const opus_int32     fac_Q16
//
// )
func lpInterpolateFilterTaps(B_Q28 []int32, A_Q28 []int32, ind int, fac_Q16 int32) {
	var nb, na int

	if ind < TRANSITION_INT_NUM-1 {
		if fac_Q16 > 0 {
			if fac_Q16 < 32768 { /* fac_Q16 is in range of a 16-bit int */
				/* Piece-wise linear interpolation of B and A */
				for nb = 0; nb < TRANSITION_NB; nb++ {
					B_Q28[nb] = SMLAWB(
						Transition_LP_B_Q28[ind][nb],
						Transition_LP_B_Q28[ind+1][nb]-
							Transition_LP_B_Q28[ind][nb],
						fac_Q16)
				}
				for na = 0; na < TRANSITION_NA; na++ {
					A_Q28[na] = SMLAWB(
						Transition_LP_A_Q28[ind][na],
						Transition_LP_A_Q28[ind+1][na]-
							Transition_LP_A_Q28[ind][na],
						fac_Q16)
				}
			} else { /* ( fac_Q16 - ( 1 << 16 ) ) is in range of a 16-bit int */
				silk_assert(fac_Q16-(1<<16) == SAT16(fac_Q16-(1<<16)))
				/* Piece-wise linear interpolation of B and A */
				for nb = 0; nb < TRANSITION_NB; nb++ {
					B_Q28[nb] = SMLAWB(
						Transition_LP_B_Q28[ind+1][nb],
						Transition_LP_B_Q28[ind+1][nb]-
							Transition_LP_B_Q28[ind][nb],
						fac_Q16-(int32(1)<<16))
				}
				for na = 0; na < TRANSITION_NA; na++ {
					A_Q28[na] = SMLAWB(
						Transition_LP_A_Q28[ind+1][na],
						Transition_LP_A_Q28[ind+1][na]-
							Transition_LP_A_Q28[ind][na],
						fac_Q16-(int32(1)<<16))
				}
			}
		} else {
			copy(B_Q28[:TRANSITION_NB], Transition_LP_B_Q28[ind][:])
			copy(A_Q28[:TRANSITION_NA], Transition_LP_A_Q28[ind][:])
		}
	} else {
		copy(B_Q28[:TRANSITION_NB], Transition_LP_B_Q28[TRANSITION_INT_NUM-1][:])
		copy(A_Q28[:TRANSITION_NA], Transition_LP_A_Q28[TRANSITION_INT_NUM-1][:])
	}
}

// Low-pass filter with variable cutoff frequency based on
// piece-wise linear interpolation between elliptic filters
// Start by setting psEncC->mode <> 0;
// Deactivate by setting psEncC->mode = 0;
//
// C: void silk_LP_variable_cutoff(
//
//	silk_LP_state               *psLP,                          /* I/O  LP filter state                             */
//	opus_int16                  *frame,                         /* I/O  Low-pass filtered output signal             */
//	const opus_int              frame_length                    /* I    Frame length                                */
//
// )
func (psLP *LPState) LPVariableCutoff(frame []int16, frame_length int) {
	var B_Q28 [TRANSITION_NB]int32
	var A_Q28 [TRANSITION_NA]int32
	var fac_Q16 int32 = 0
	var ind int = 0

	silk_assert(psLP.transition_frame_no >= 0 && psLP.transition_frame_no <= TRANSITION_FRAMES)

	/* Run filter if needed */
	if psLP.mode != 0 {
		/* Calculate index and interpolation factor for interpolation */
		if TRANSITION_INT_STEPS == 64 { /* C: #if( TRANSITION_INT_STEPS == 64 ) */
			fac_Q16 = LSHIFT(TRANSITION_FRAMES-psLP.transition_frame_no, 16-6)
		} else {
			fac_Q16 = DIV32_16(LSHIFT(TRANSITION_FRAMES-psLP.transition_frame_no, 16), TRANSITION_FRAMES)
		}
		ind = int(RSHIFT(fac_Q16, 16))
		fac_Q16 -= LSHIFT(ind, 16)

		silk_assert(ind >= 0)
		silk_assert(ind < TRANSITION_INT_NUM)

		/* Interpolate filter coefficients */
		lpInterpolateFilterTaps(B_Q28[:], A_Q28[:], ind, fac_Q16)

		/* Update transition frame number for next frame */
		psLP.transition_frame_no = LIMIT(psLP.transition_frame_no+int32(psLP.mode), 0, TRANSITION_FRAMES)

		/* ARMA low-pass filtering */
		silk_assert(TRANSITION_NB == 3 && TRANSITION_NA == 2)
		BiquadAlt(frame, B_Q28[:], A_Q28[:], psLP.In_LP_State[:], frame, int32(frame_length), 1)
	}
}
