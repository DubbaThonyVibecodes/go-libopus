package silk

// C: silk/control_SNR.c

// Control SNR of redidual quantizer
//
// C: opus_int silk_control_SNR(silk_encoder_state *psEncC, opus_int32 TargetRate_bps)
func (psEncC *EncoderState) ControlSNR(TargetRate_bps int32) int {
	var k, ret int = 0, SILK_NO_ERROR
	var frac_Q6 int32
	var rateTable []int32

	/* Set bitrate/coding quality */
	TargetRate_bps = LIMIT(TargetRate_bps, MIN_TARGET_RATE_BPS, MAX_TARGET_RATE_BPS)
	if TargetRate_bps != psEncC.TargetRate_bps {
		psEncC.TargetRate_bps = TargetRate_bps

		/* If new TargetRate_bps, translate to SNR_dB value */
		if psEncC.fs_kHz == 8 {
			rateTable = TargetRate_table_NB[:]
		} else if psEncC.fs_kHz == 12 {
			rateTable = TargetRate_table_MB[:]
		} else {
			rateTable = TargetRate_table_WB[:]
		}

		/* Reduce bitrate for 10 ms modes in these calculations */
		if psEncC.nb_subfr == 2 {
			TargetRate_bps -= REDUCE_BITRATE_10_MS_BPS
		}

		/* Find bitrate interval in table and interpolate */
		for k = 1; k < TARGET_RATE_TAB_SZ; k++ {
			if TargetRate_bps <= rateTable[k] {
				frac_Q6 = DIV32(LSHIFT(TargetRate_bps-rateTable[k-1], 6),
					rateTable[k]-rateTable[k-1])
				psEncC.SNR_dB_Q7 = int(LSHIFT(SNR_table_Q1[k-1], 6) + MUL(frac_Q6, int32(SNR_table_Q1[k])-int32(SNR_table_Q1[k-1])))
				break
			}
		}
	}

	return ret
}
