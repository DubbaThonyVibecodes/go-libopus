package silk

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// C: silk/encode_pulses.c

/*********************************************/
/* Encode quantization indices of excitation */
/*********************************************/

// C: static inline opus_int combine_and_check(opus_int *pulses_comb, const opus_int *pulses_in, opus_int max_pulses, opus_int len)
//
// return ok
func combineAndCheck(
	pulses_comb []int, /* O                                   */
	pulses_in []int, /* I                                   */
	max_pulses int, /* I    max value for sum of pulses    */
	len int, /* I    number of output values        */
) int {
	var k, sum int

	for k = 0; k < len; k++ {
		sum = pulses_in[2*k] + pulses_in[2*k+1]
		if sum > max_pulses {
			return 1
		}
		pulses_comb[k] = sum
	}

	return 0
}

// Encode quantization indices of excitation
//
// C: void silk_encode_pulses(ec_enc *psRangeEnc, const opus_int signalType, const opus_int quantOffsetType, opus_int8 pulses[], const opus_int frame_length)
func EncodePulses(psRangeEnc *celt.EcEnc, signalType int, quantOffsetType int, pulses []int8, frame_length int) {
	var i, k, j, iter, bit, nLS, scale_down int
	var RateLevelIndex int = 0
	var abs_q, minSumBits_Q5, sumBits_Q5 int32
	var abs_pulses [MAX_FRAME_LENGTH]int
	var sum_pulses [MAX_NB_SHELL_BLOCKS]int
	var nRshifts [MAX_NB_SHELL_BLOCKS]int
	var pulses_comb [8]int
	var abs_pulses_ptr []int
	var pulses_ptr []int8
	var cdf_ptr []uint8
	var nBits_ptr []uint8

	clear(pulses_comb[:8]) /* Fixing Valgrind reported problem*/

	/****************************/
	/* Prepare for shell coding */
	/****************************/
	/* Calculate number of shell blocks */
	silk_assert(1<<LOG2_SHELL_CODEC_FRAME_LENGTH == SHELL_CODEC_FRAME_LENGTH)
	iter = RSHIFT(frame_length, LOG2_SHELL_CODEC_FRAME_LENGTH)
	if iter*SHELL_CODEC_FRAME_LENGTH < frame_length {
		silk_assert(frame_length == 12*10) /* Make sure only happens for 10 ms @ 12 kHz */
		iter++
		clear(pulses[frame_length : frame_length+SHELL_CODEC_FRAME_LENGTH])
	}

	/* Take the absolute value of the pulses */
	for i = 0; i < iter*SHELL_CODEC_FRAME_LENGTH; i += 4 {
		abs_pulses[i+0] = abs(int(pulses[i+0]))
		abs_pulses[i+1] = abs(int(pulses[i+1]))
		abs_pulses[i+2] = abs(int(pulses[i+2]))
		abs_pulses[i+3] = abs(int(pulses[i+3]))
	}

	/* Calc sum pulses per shell code frame */
	abs_pulses_ptr = abs_pulses[:]
	for i = 0; i < iter; i++ {
		nRshifts[i] = 0

		for {
			/* 1+1 -> 2 */
			scale_down = combineAndCheck(pulses_comb[:], abs_pulses_ptr, int(max_pulses_table[0]), 8)
			/* 2+2 -> 4 */
			scale_down += combineAndCheck(pulses_comb[:], pulses_comb[:], int(max_pulses_table[1]), 4)
			/* 4+4 -> 8 */
			scale_down += combineAndCheck(pulses_comb[:], pulses_comb[:], int(max_pulses_table[2]), 2)
			/* 8+8 -> 16 */
			scale_down += combineAndCheck(sum_pulses[i:], pulses_comb[:], int(max_pulses_table[3]), 1)

			if scale_down != 0 {
				/* We need to downscale the quantization signal */
				nRshifts[i]++
				for k = 0; k < SHELL_CODEC_FRAME_LENGTH; k++ {
					abs_pulses_ptr[k] = RSHIFT(abs_pulses_ptr[k], 1)
				}
			} else {
				/* Jump out of while(1) loop and go to next shell coding frame */
				break
			}
		}
		abs_pulses_ptr = abs_pulses_ptr[SHELL_CODEC_FRAME_LENGTH:]
	}

	/**************/
	/* Rate level */
	/**************/
	/* find rate level that leads to fewest bits for coding of pulses per block info */
	minSumBits_Q5 = int32_MAX
	for k = 0; k < N_RATE_LEVELS-1; k++ {
		nBits_ptr = pulses_per_block_BITS_Q5[k][:]
		sumBits_Q5 = int32(rate_levels_BITS_Q5[signalType>>1][k])
		for i = 0; i < iter; i++ {
			if nRshifts[i] > 0 {
				sumBits_Q5 += int32(nBits_ptr[MAX_PULSES+1])
			} else {
				sumBits_Q5 += int32(nBits_ptr[sum_pulses[i]])
			}
		}
		if sumBits_Q5 < minSumBits_Q5 {
			minSumBits_Q5 = sumBits_Q5
			RateLevelIndex = k
		}
	}
	psRangeEnc.EncIcdf(RateLevelIndex, rate_levels_iCDF[signalType>>1][:], 8)

	/***************************************************/
	/* Sum-Weighted-Pulses Encoding                    */
	/***************************************************/
	cdf_ptr = pulses_per_block_iCDF[RateLevelIndex][:]
	for i = 0; i < iter; i++ {
		if nRshifts[i] == 0 {
			psRangeEnc.EncIcdf(sum_pulses[i], cdf_ptr, 8)
		} else {
			psRangeEnc.EncIcdf(MAX_PULSES+1, cdf_ptr, 8)
			for k = 0; k < nRshifts[i]-1; k++ {
				psRangeEnc.EncIcdf(MAX_PULSES+1, pulses_per_block_iCDF[N_RATE_LEVELS-1][:], 8)
			}
			psRangeEnc.EncIcdf(sum_pulses[i], pulses_per_block_iCDF[N_RATE_LEVELS-1][:], 8)
		}
	}

	/******************/
	/* Shell Encoding */
	/******************/
	for i = 0; i < iter; i++ {
		if sum_pulses[i] > 0 {
			ShellEncoder(psRangeEnc, abs_pulses[i*SHELL_CODEC_FRAME_LENGTH:])
		}
	}

	/****************/
	/* LSB Encoding */
	/****************/
	for i = 0; i < iter; i++ {
		if nRshifts[i] > 0 {
			pulses_ptr = pulses[i*SHELL_CODEC_FRAME_LENGTH:]
			nLS = nRshifts[i] - 1
			for k = 0; k < SHELL_CODEC_FRAME_LENGTH; k++ {
				abs_q = int32(int8(abs(int(pulses_ptr[k]))))
				for j = nLS; j > 0; j-- {
					bit = int(RSHIFT(abs_q, j) & 1)
					psRangeEnc.EncIcdf(bit, lsb_iCDF[:], 8)
				}
				bit = int(abs_q & 1)
				psRangeEnc.EncIcdf(bit, lsb_iCDF[:], 8)
			}
		}
	}

	/****************/
	/* Encode signs */
	/****************/
	EncodeSigns(psRangeEnc, pulses, frame_length, signalType, quantOffsetType, sum_pulses[:])
}
