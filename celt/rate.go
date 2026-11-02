package celt

// C: celt/rate.h, celt/rate.c — bit allocation.
// compute_pulse_cache() is CUSTOM_MODES only and not ported; the static mode
// carries a precomputed cache.

const (
	MAX_PSEUDO     = 40
	LOG_MAX_PSEUDO = 6

	CELT_MAX_PULSES = 128

	MAX_FINE_BITS = 8

	FINE_OFFSET            = 21
	QTHETA_OFFSET          = 4
	QTHETA_OFFSET_TWOPHASE = 16
)

// C: static OPUS_INLINE int get_pulses(int i)
func get_pulses(i int) int {
	if i < 8 {
		return i
	}
	return (8 + (i & 7)) << uint((i>>3)-1)
}

// C: static OPUS_INLINE int bits2pulses(const CELTMode *m, int band, int LM, int bits)
func bits2pulses(m *CELTMode, band, LM, bits int) int {
	var i, lo, hi int
	LM++
	cache := m.cache.bits[m.cache.index[LM*m.nbEBands+band]:]

	lo = 0
	hi = int(cache[0])
	bits--
	for i = 0; i < LOG_MAX_PSEUDO; i++ {
		mid := (lo + hi + 1) >> 1
		// OPT: Make sure this is implemented with a conditional move
		if int(cache[mid]) >= bits {
			hi = mid
		} else {
			lo = mid
		}
	}
	var clo int
	if lo == 0 {
		clo = -1
	} else {
		clo = int(cache[lo])
	}
	if bits-clo <= int(cache[hi])-bits {
		return lo
	}
	return hi
}

// C: static OPUS_INLINE int pulses2bits(const CELTMode *m, int band, int LM, int pulses)
func pulses2bits(m *CELTMode, band, LM, pulses int) int {
	LM++
	cache := m.cache.bits[m.cache.index[LM*m.nbEBands+band]:]
	if pulses == 0 {
		return 0
	}
	return int(cache[pulses]) + 1
}

var LOG2_FRAC_TABLE = [24]uint8{
	0,
	8, 13,
	16, 19, 21, 23,
	24, 26, 27, 28, 29, 30, 31, 32,
	32, 33, 34, 34, 35, 36, 36, 37, 37,
}

const ALLOC_STEPS = 6

// C: static OPUS_INLINE int interp_bits2pulses(const CELTMode *m, int start, int end, int skip_start,
//
//	const int *bits1, const int *bits2, const int *thresh, const int *cap, opus_int32 total, opus_int32 *_balance,
//	int skip_rsv, int *intensity, int intensity_rsv, int *dual_stereo, int dual_stereo_rsv, int *bits,
//	int *ebits, int *fine_priority, int C, int LM, ec_ctx *ec, int encode, int prev)
func interp_bits2pulses(m *CELTMode, start, end, skip_start int,
	bits1, bits2, thresh, cap []int, total int32, _balance *int32,
	skip_rsv int, intensity *int, intensity_rsv int, dual_stereo *int, dual_stereo_rsv int, bits []int,
	ebits, fine_priority []int, C, LM int, ec *EcCtx, encode, prev int, signalBandwidth int) int {
	var psum int32
	var lo, hi, i, j, logM, stereo int
	var codedBands int = -1
	var alloc_floor int
	var left, percoeff int32
	var done int
	var balance int32

	alloc_floor = C << BITRES
	if C > 1 {
		stereo = 1
	}

	logM = LM << BITRES
	lo = 0
	hi = 1 << ALLOC_STEPS
	for i = 0; i < ALLOC_STEPS; i++ {
		mid := (lo + hi) >> 1
		psum = 0
		done = 0
		for j = end; j > start; {
			j--
			tmp := bits1[j] + int(int32(mid)*int32(bits2[j])>>ALLOC_STEPS)
			if tmp >= thresh[j] || done != 0 {
				done = 1
				// Don't allocate more than we can actually use
				psum += int32(IMIN(tmp, cap[j]))
			} else {
				if tmp >= alloc_floor {
					psum += int32(alloc_floor)
				}
			}
		}
		if psum > total {
			hi = mid
		} else {
			lo = mid
		}
	}
	psum = 0
	// printf ("interp bisection gave %d\n", lo);
	done = 0
	for j = end; j > start; {
		j--
		tmp := bits1[j] + int(int32(lo)*int32(bits2[j])>>ALLOC_STEPS)
		if tmp < thresh[j] && done == 0 {
			if tmp >= alloc_floor {
				tmp = alloc_floor
			} else {
				tmp = 0
			}
		} else {
			done = 1
		}
		// Don't allocate more than we can actually use
		tmp = IMIN(tmp, cap[j])
		bits[j] = tmp
		psum += int32(tmp)
	}

	// Decide which bands to skip, working backwards from the end.
	for codedBands = end; ; codedBands-- {
		var band_width, band_bits, rem int
		j = codedBands - 1
		// Never skip the first band, nor a band that has been boosted by
		// dynalloc.
		// In the first case, we'd be coding a bit to signal we're going to waste
		// all the other bits.
		// In the second case, we'd be coding a bit to redistribute all the bits
		// we just signaled should be cocentrated in this band.
		if j <= skip_start {
			// Give the bit we reserved to end skipping back.
			total += int32(skip_rsv)
			break
		}
		// Figure out how many left-over bits we would be adding to this band.
		// This can include bits we've stolen back from higher, skipped bands.
		left = total - psum
		percoeff = int32(Udiv(uint32(left), uint32(m.eBands[codedBands]-m.eBands[start])))
		left -= int32(m.eBands[codedBands]-m.eBands[start]) * percoeff
		rem = IMAX(int(left)-int(m.eBands[j]-m.eBands[start]), 0)
		band_width = int(m.eBands[codedBands] - m.eBands[j])
		band_bits = int(int32(bits[j]) + percoeff*int32(band_width) + int32(rem))
		// Only code a skip decision if we're above the threshold for this band.
		// Otherwise it is force-skipped.
		// This ensures that we have enough bits to code the skip flag.
		if band_bits >= IMAX(thresh[j], alloc_floor+(1<<BITRES)) {
			if encode != 0 {
				// This if() block is the only part of the allocation function that
				// is not a mandatory part of the bitstream: any bands we choose to
				// skip here must be explicitly signaled.
				var depth_threshold int
				// We choose a threshold with some hysteresis to keep bands from
				// fluctuating in and out, but we try not to fold below a certain point.
				if codedBands > 17 {
					if j < prev {
						depth_threshold = 7
					} else {
						depth_threshold = 9
					}
				} else {
					depth_threshold = 0
				}
				if codedBands <= start+2 || (band_bits > ((depth_threshold*band_width<<uint(LM)<<BITRES)>>4) && j <= signalBandwidth) {
					ec.EncBitLogp(1, 1)
					break
				}
				ec.EncBitLogp(0, 1)
			} else if ec.DecBitLogp(1) != 0 {
				break
			}
			// We used a bit to skip this band.
			psum += 1 << BITRES
			band_bits -= 1 << BITRES
		}
		// Reclaim the bits originally allocated to this band.
		psum -= int32(bits[j] + intensity_rsv)
		if intensity_rsv > 0 {
			intensity_rsv = int(LOG2_FRAC_TABLE[j-start])
		}
		psum += int32(intensity_rsv)
		if band_bits >= alloc_floor {
			// If we have enough for a fine energy bit per channel, use it.
			psum += int32(alloc_floor)
			bits[j] = alloc_floor
		} else {
			// Otherwise this band gets nothing at all.
			bits[j] = 0
		}
	}

	celt_assert(codedBands > start)
	// Code the intensity and dual stereo parameters.
	if intensity_rsv > 0 {
		if encode != 0 {
			*intensity = IMIN(*intensity, codedBands)
			ec.EncUint(uint32(*intensity-start), uint32(codedBands+1-start))
		} else {
			*intensity = start + int(ec.DecUint(uint32(codedBands+1-start)))
		}
	} else {
		*intensity = 0
	}
	if *intensity <= start {
		total += int32(dual_stereo_rsv)
		dual_stereo_rsv = 0
	}
	if dual_stereo_rsv > 0 {
		if encode != 0 {
			ec.EncBitLogp(*dual_stereo, 1)
		} else {
			*dual_stereo = ec.DecBitLogp(1)
		}
	} else {
		*dual_stereo = 0
	}

	// Allocate the remaining bits
	left = total - psum
	percoeff = int32(Udiv(uint32(left), uint32(m.eBands[codedBands]-m.eBands[start])))
	left -= int32(m.eBands[codedBands]-m.eBands[start]) * percoeff
	for j = start; j < codedBands; j++ {
		bits[j] += int(percoeff) * int(m.eBands[j+1]-m.eBands[j])
	}
	for j = start; j < codedBands; j++ {
		tmp := int(IMIN(left, int32(m.eBands[j+1]-m.eBands[j])))
		bits[j] += tmp
		left -= int32(tmp)
	}
	// for (j=0;j<end;j++)printf("%d ", bits[j]);printf("\n");

	balance = 0
	for j = start; j < codedBands; j++ {
		var N0, N, den, offset, NClogN int
		var excess, bit int32
		celt_assert(bits[j] >= 0)
		N0 = int(m.eBands[j+1] - m.eBands[j])
		N = N0 << uint(LM)
		bit = int32(bits[j]) + balance

		if N > 1 {
			excess = MAX32(bit-int32(cap[j]), 0)
			bits[j] = int(bit - excess)

			// Compensate for the extra DoF in stereo
			den = C * N
			if C == 2 && N > 2 && *dual_stereo == 0 && j < *intensity {
				den++
			}

			NClogN = den * (int(m.logN[j]) + logM)

			// Offset for the number of fine bits by log2(N)/2 + FINE_OFFSET
			// compared to their "fair share" of total/N
			offset = (NClogN >> 1) - den*FINE_OFFSET

			// N=2 is the only point that doesn't match the curve
			if N == 2 {
				offset += den << BITRES >> 2
			}

			// Changing the offset for allocating the second and third
			// fine energy bit
			if bits[j]+offset < den*2<<BITRES {
				offset += NClogN >> 2
			} else if bits[j]+offset < den*3<<BITRES {
				offset += NClogN >> 3
			}

			// Divide with rounding
			ebits[j] = IMAX(0, (bits[j] + offset + (den << (BITRES - 1))))
			ebits[j] = int(Udiv(uint32(ebits[j]), uint32(den))) >> BITRES

			// Make sure not to bust
			if C*ebits[j] > (bits[j] >> BITRES) {
				ebits[j] = bits[j] >> uint(stereo) >> BITRES
			}

			// More than that is useless because that's about as far as PVQ can go
			ebits[j] = IMIN(ebits[j], MAX_FINE_BITS)

			// If we rounded down or capped this band, make it a candidate for the
			// final fine energy pass
			if ebits[j]*(den<<BITRES) >= bits[j]+offset {
				fine_priority[j] = 1
			} else {
				fine_priority[j] = 0
			}

			// Remove the allocated fine bits; the rest are assigned to PVQ
			bits[j] -= C * ebits[j] << BITRES

		} else {
			// For N=1, all bits go to fine energy except for a single sign bit
			excess = MAX32(0, bit-int32(C<<BITRES))
			bits[j] = int(bit - excess)
			ebits[j] = 0
			fine_priority[j] = 1
		}

		// Fine energy can't take advantage of the re-balancing in
		// quant_all_bands().
		// Instead, do the re-balancing here.
		if excess > 0 {
			var extra_fine, extra_bits int
			extra_fine = IMIN(int(excess>>uint(stereo+BITRES)), MAX_FINE_BITS-ebits[j])
			ebits[j] += extra_fine
			extra_bits = extra_fine * C << BITRES
			if int32(extra_bits) >= excess-balance {
				fine_priority[j] = 1
			} else {
				fine_priority[j] = 0
			}
			excess -= int32(extra_bits)
		}
		balance = excess

		celt_assert(bits[j] >= 0)
		celt_assert(ebits[j] >= 0)
	}
	// Save any remaining bits over the cap for the rebalancing in
	// quant_all_bands().
	*_balance = balance

	// The skipped bands use all their bits for fine energy.
	for ; j < end; j++ {
		ebits[j] = bits[j] >> uint(stereo) >> BITRES
		celt_assert(C*ebits[j]<<BITRES == bits[j])
		bits[j] = 0
		if ebits[j] < 1 {
			fine_priority[j] = 1
		} else {
			fine_priority[j] = 0
		}
	}
	return codedBands
}

// CltComputeAllocation computes the pulse allocation, i.e. how many pulses will
// go in each band. Returns the number of coded bands.
//
// C: int clt_compute_allocation(const CELTMode *m, int start, int end, const int *offsets, const int *cap, int alloc_trim, int *intensity, int *dual_stereo,
//
//	opus_int32 total, opus_int32 *balance, int *pulses, int *ebits, int *fine_priority, int C, int LM, ec_ctx *ec, int encode, int prev, int signalBandwidth)
func CltComputeAllocation(m *CELTMode, start, end int, offsets, cap []int, alloc_trim int, intensity, dual_stereo *int,
	total int32, balance *int32, pulses, ebits, fine_priority []int, C, LM int, ec *EcCtx, encode, prev int, signalBandwidth int) int {
	var lo, hi, length, j, codedBands, skip_start, skip_rsv, intensity_rsv, dual_stereo_rsv int

	total = IMAX(total, 0)
	length = m.nbEBands
	skip_start = start
	// Reserve a bit to signal the end of manually skipped bands.
	if total >= 1<<BITRES {
		skip_rsv = 1 << BITRES
	}
	total -= int32(skip_rsv)
	// Reserve bits for the intensity and dual stereo parameters.
	intensity_rsv = 0
	dual_stereo_rsv = 0
	if C == 2 {
		intensity_rsv = int(LOG2_FRAC_TABLE[end-start])
		if int32(intensity_rsv) > total {
			intensity_rsv = 0
		} else {
			total -= int32(intensity_rsv)
			if total >= 1<<BITRES {
				dual_stereo_rsv = 1 << BITRES
			}
			total -= int32(dual_stereo_rsv)
		}
	}
	bits1 := make([]int, length)
	bits2 := make([]int, length)
	thresh := make([]int, length)
	trim_offset := make([]int, length)

	for j = start; j < end; j++ {
		// Below this threshold, we're sure not to allocate any PVQ bits
		thresh[j] = IMAX(C<<BITRES, (3*int(m.eBands[j+1]-m.eBands[j])<<uint(LM)<<BITRES)>>4)
		// Tilt of the allocation curve
		trim_offset[j] = C * int(m.eBands[j+1]-m.eBands[j]) * (alloc_trim - 5 - LM) * (end - j - 1) * (1 << uint(LM+BITRES)) >> 6
		// Giving less resolution to single-coefficient bands because they get
		// more benefit from having one coarse value per coefficient
		if int(m.eBands[j+1]-m.eBands[j])<<uint(LM) == 1 {
			trim_offset[j] -= C << BITRES
		}
	}
	lo = 1
	hi = m.nbAllocVectors - 1
	for {
		var done int = 0
		var psum int = 0
		mid := (lo + hi) >> 1
		for j = end; j > start; {
			j--
			var bitsj int
			N := int(m.eBands[j+1] - m.eBands[j])
			bitsj = C * N * int(m.allocVectors[mid*length+j]) << uint(LM) >> 2
			if bitsj > 0 {
				bitsj = IMAX(0, bitsj+trim_offset[j])
			}
			bitsj += offsets[j]
			if bitsj >= thresh[j] || done != 0 {
				done = 1
				// Don't allocate more than we can actually use
				psum += IMIN(bitsj, cap[j])
			} else {
				if bitsj >= C<<BITRES {
					psum += C << BITRES
				}
			}
		}
		if int32(psum) > total {
			hi = mid - 1
		} else {
			lo = mid + 1
		}
		// printf ("lo = %d, hi = %d\n", lo, hi);
		if lo > hi {
			break
		}
	}
	hi = lo
	lo--
	// printf ("interp between %d and %d\n", lo, hi);
	for j = start; j < end; j++ {
		var bits1j, bits2j int
		N := int(m.eBands[j+1] - m.eBands[j])
		bits1j = C * N * int(m.allocVectors[lo*length+j]) << uint(LM) >> 2
		if hi >= m.nbAllocVectors {
			bits2j = cap[j]
		} else {
			bits2j = C * N * int(m.allocVectors[hi*length+j]) << uint(LM) >> 2
		}
		if bits1j > 0 {
			bits1j = IMAX(0, bits1j+trim_offset[j])
		}
		if bits2j > 0 {
			bits2j = IMAX(0, bits2j+trim_offset[j])
		}
		if lo > 0 {
			bits1j += offsets[j]
		}
		bits2j += offsets[j]
		if offsets[j] > 0 {
			skip_start = j
		}
		bits2j = IMAX(0, bits2j-bits1j)
		bits1[j] = bits1j
		bits2[j] = bits2j
	}
	codedBands = interp_bits2pulses(m, start, end, skip_start, bits1, bits2, thresh, cap,
		total, balance, skip_rsv, intensity, intensity_rsv, dual_stereo, dual_stereo_rsv,
		pulses, ebits, fine_priority, C, LM, ec, encode, prev, signalBandwidth)
	return codedBands
}
