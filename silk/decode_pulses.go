package silk

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// C: silk/decode_pulses.c

/*********************************************/
/* Decode quantization indices of excitation */
/*********************************************/
//
// C: void silk_decode_pulses(ec_dec *psRangeDec, opus_int16 pulses[], const opus_int signalType,
// const opus_int quantOffsetType, const opus_int frame_length)
func DecodePulses(psRangeDec *celt.EcDec, pulses []int16, signalType int, quantOffsetType int, frame_length int) {
	var i, j, k, iter, abs_q, nLS, RateLevelIndex int
	var sum_pulses, nLshifts [MAX_NB_SHELL_BLOCKS]int
	var pulses_ptr []int16
	var cdf_ptr []uint8

	/*********************/
	/* Decode rate level */
	/*********************/
	RateLevelIndex = psRangeDec.DecIcdf(rate_levels_iCDF[signalType>>1][:], 8)

	/* Calculate number of shell blocks */
	silk_assert(1<<LOG2_SHELL_CODEC_FRAME_LENGTH == SHELL_CODEC_FRAME_LENGTH)
	iter = RSHIFT(frame_length, LOG2_SHELL_CODEC_FRAME_LENGTH)
	if iter*SHELL_CODEC_FRAME_LENGTH < frame_length {
		silk_assert(frame_length == 12*10) /* Make sure only happens for 10 ms @ 12 kHz */
		iter++
	}

	/***************************************************/
	/* Sum-Weighted-Pulses Decoding                    */
	/***************************************************/
	cdf_ptr = pulses_per_block_iCDF[RateLevelIndex][:]
	for i = 0; i < iter; i++ {
		nLshifts[i] = 0
		sum_pulses[i] = psRangeDec.DecIcdf(cdf_ptr, 8)

		/* LSB indication */
		for sum_pulses[i] == SILK_MAX_PULSES+1 {
			nLshifts[i]++
			/* When we've already got 10 LSBs, we shift the table to not allow (SILK_MAX_PULSES + 1) */
			sum_pulses[i] = psRangeDec.DecIcdf(
				pulses_per_block_iCDF[N_RATE_LEVELS-1][b2i(nLshifts[i] == 10):], 8)
		}
	}

	/***************************************************/
	/* Shell decoding                                  */
	/***************************************************/
	for i = 0; i < iter; i++ {
		if sum_pulses[i] > 0 {
			ShellDecoder(pulses[SMULBB(i, SHELL_CODEC_FRAME_LENGTH):], psRangeDec, sum_pulses[i])
		} else {
			clear(pulses[SMULBB(i, SHELL_CODEC_FRAME_LENGTH):][:SHELL_CODEC_FRAME_LENGTH])
		}
	}

	/***************************************************/
	/* LSB Decoding                                    */
	/***************************************************/
	for i = 0; i < iter; i++ {
		if nLshifts[i] > 0 {
			nLS = nLshifts[i]
			pulses_ptr = pulses[SMULBB(i, SHELL_CODEC_FRAME_LENGTH):]
			for k = 0; k < SHELL_CODEC_FRAME_LENGTH; k++ {
				abs_q = int(pulses_ptr[k])
				for j = 0; j < nLS; j++ {
					abs_q = int(LSHIFT(abs_q, 1))
					abs_q += psRangeDec.DecIcdf(lsb_iCDF[:], 8)
				}
				pulses_ptr[k] = int16(abs_q)
			}
			/* Mark the number of pulses non-zero for sign decoding. */
			sum_pulses[i] |= nLS << 5
		}
	}

	/****************************************/
	/* Decode and add signs to pulse signal */
	/****************************************/
	DecodeSigns(psRangeDec, pulses, frame_length, signalType, quantOffsetType, sum_pulses[:])
}
