package celt

// C: celt/quant_bands.h, celt/quant_bands.c — coarse/fine energy quantisation.

// Mean energy in each band quantized in Q4
var eMeans = [25]int8{
	103, 100, 92, 85, 81,
	77, 72, 70, 78, 75,
	73, 71, 78, 74, 69,
	72, 70, 74, 76, 71,
	60, 60, 60, 60, 60,
}

// prediction coefficients: 0.9, 0.8, 0.65, 0.5
var pred_coef = [4]int16{29440, 26112, 21248, 16384}
var beta_coef = [4]int16{30147, 22282, 12124, 6554}

const beta_intra int16 = 4915

// Parameters of the Laplace-like probability models used for the coarse energy.
// There is one pair of parameters for each frame size, prediction type
// (inter/intra), and band number.
// The first number of each pair is the probability of 0, and the second is the
// decay rate, both in Q8 precision.
var e_prob_model = [4][2][42]uint8{
	// 120 sample frames.
	{
		// Inter
		{
			72, 127, 65, 129, 66, 128, 65, 128, 64, 128, 62, 128, 64, 128,
			64, 128, 92, 78, 92, 79, 92, 78, 90, 79, 116, 41, 115, 40,
			114, 40, 132, 26, 132, 26, 145, 17, 161, 12, 176, 10, 177, 11,
		},
		// Intra
		{
			24, 179, 48, 138, 54, 135, 54, 132, 53, 134, 56, 133, 55, 132,
			55, 132, 61, 114, 70, 96, 74, 88, 75, 88, 87, 74, 89, 66,
			91, 67, 100, 59, 108, 50, 120, 40, 122, 37, 97, 43, 78, 50,
		},
	},
	// 240 sample frames.
	{
		// Inter
		{
			83, 78, 84, 81, 88, 75, 86, 74, 87, 71, 90, 73, 93, 74,
			93, 74, 109, 40, 114, 36, 117, 34, 117, 34, 143, 17, 145, 18,
			146, 19, 162, 12, 165, 10, 178, 7, 189, 6, 190, 8, 177, 9,
		},
		// Intra
		{
			23, 178, 54, 115, 63, 102, 66, 98, 69, 99, 74, 89, 71, 91,
			73, 91, 78, 89, 86, 80, 92, 66, 93, 64, 102, 59, 103, 60,
			104, 60, 117, 52, 123, 44, 138, 35, 133, 31, 97, 38, 77, 45,
		},
	},
	// 480 sample frames.
	{
		// Inter
		{
			61, 90, 93, 60, 105, 42, 107, 41, 110, 45, 116, 38, 113, 38,
			112, 38, 124, 26, 132, 27, 136, 19, 140, 20, 155, 14, 159, 16,
			158, 18, 170, 13, 177, 10, 187, 8, 192, 6, 175, 9, 159, 10,
		},
		// Intra
		{
			21, 178, 59, 110, 71, 86, 75, 85, 84, 83, 91, 66, 88, 73,
			87, 72, 92, 75, 98, 72, 105, 58, 107, 54, 115, 52, 114, 55,
			112, 56, 129, 51, 132, 40, 150, 33, 140, 29, 98, 35, 77, 42,
		},
	},
	// 960 sample frames.
	{
		// Inter
		{
			42, 121, 96, 66, 108, 43, 111, 40, 117, 44, 123, 32, 120, 36,
			119, 33, 127, 33, 134, 34, 139, 21, 147, 23, 152, 20, 158, 25,
			154, 26, 166, 21, 173, 16, 184, 13, 184, 10, 150, 13, 139, 15,
		},
		// Intra
		{
			22, 178, 63, 114, 74, 82, 84, 83, 92, 82, 103, 62, 96, 72,
			96, 67, 101, 73, 107, 72, 113, 55, 118, 52, 125, 52, 118, 52,
			117, 55, 135, 49, 137, 39, 157, 32, 145, 29, 97, 33, 77, 40,
		},
	},
}

var small_energy_icdf = [3]uint8{2, 1, 0}

// C: static opus_val32 loss_distortion(const opus_val16 *eBands, opus_val16 *oldEBands, int start, int end, int len, int C)
func loss_distortion(eBands, oldEBands []int16, start, end, length, C int) int32 {
	var c, i int
	var dist int32 = 0
	c = 0
	for {
		for i = start; i < end; i++ {
			var d int16 = int16(SUB16(SHR16(eBands[i+c*length], 3), SHR16(oldEBands[i+c*length], 3)))
			dist = MAC16_16(dist, d, d)
		}
		c++
		if c >= C {
			break
		}
	}
	return MIN32(200, SHR32(dist, 2*DB_SHIFT-6))
}

// C: static int quant_coarse_energy_impl(const CELTMode *m, int start, int end,
//
//	const opus_val16 *eBands, opus_val16 *oldEBands,
//	opus_int32 budget, opus_int32 tell,
//	const unsigned char *prob_model, opus_val16 *error, ec_enc *enc,
//	int C, int LM, int intra, opus_val16 max_decay, int lfe)
func quant_coarse_energy_impl(m *CELTMode, start, end int,
	eBands, oldEBands []int16,
	budget, tell int32,
	prob_model []uint8, error []int16, enc *EcEnc,
	C, LM, intra int, max_decay int16, lfe int) int {
	var i, c int
	var badness int = 0
	var prev = [2]int32{0, 0}
	var coef, beta int16

	if tell+3 <= budget {
		enc.EncBitLogp(intra, 3)
	}
	if intra != 0 {
		coef = 0
		beta = beta_intra
	} else {
		beta = beta_coef[LM]
		coef = pred_coef[LM]
	}

	// Encode at a fixed coarse resolution
	for i = start; i < end; i++ {
		c = 0
		for {
			var bits_left, qi, qi0 int
			var q int32
			var x int16
			var f, tmp int32
			var oldE, decay_bound int16
			x = eBands[i+c*m.nbEBands]
			oldE = MAX16(-QCONST16(9.0, DB_SHIFT), oldEBands[i+c*m.nbEBands])
			f = SHL32(EXTEND32(x), 7) - PSHR32(MULT16_16(coef, oldE), 8) - prev[c]
			// Rounding to nearest integer here is really important!
			qi = int((f + int32(QCONST32(.5, DB_SHIFT+7))) >> (DB_SHIFT + 7))
			decay_bound = EXTRACT16(MAX32(-int32(QCONST16(28.0, DB_SHIFT)),
				SUB32(int32(oldEBands[i+c*m.nbEBands]), max_decay)))
			// Prevent the energy from going down too quickly (e.g. for bands
			// that have just one bin)
			if qi < 0 && int32(x) < int32(decay_bound) {
				qi += int(SHR16(SUB16(decay_bound, x), DB_SHIFT))
				if qi > 0 {
					qi = 0
				}
			}
			qi0 = qi
			// If we don't have enough bits to encode all the energy, just assume
			// something safe.
			tell = int32(enc.Tell())
			bits_left = int(budget) - int(tell) - 3*C*(end-i)
			if i != start && bits_left < 30 {
				if bits_left < 24 {
					qi = IMIN(1, qi)
				}
				if bits_left < 16 {
					qi = IMAX(-1, qi)
				}
			}
			if lfe != 0 && i >= 2 {
				qi = IMIN(qi, 0)
			}
			if budget-tell >= 15 {
				var pi int
				pi = 2 * IMIN(i, 20)
				enc.LaplaceEncode(&qi, uint32(prob_model[pi])<<7, int(prob_model[pi+1])<<6)
			} else if budget-tell >= 2 {
				qi = IMAX(-1, IMIN(qi, 1))
				var s int
				if qi < 0 {
					s = -1
				}
				enc.EncIcdf(2*qi^s, small_energy_icdf[:], 2)
			} else if budget-tell >= 1 {
				qi = IMIN(0, qi)
				enc.EncBitLogp(-qi, 1)
			} else {
				qi = -1
			}
			error[i+c*m.nbEBands] = int16(PSHR32(f, 7) - int32(SHL16(qi, DB_SHIFT)))
			badness += ABS(qi0 - qi)
			q = SHL32(EXTEND32(qi), DB_SHIFT)

			tmp = PSHR32(MULT16_16(coef, oldE), 8) + prev[c] + SHL32(q, 7)
			tmp = MAX32(-QCONST32(28.0, DB_SHIFT+7), tmp)
			oldEBands[i+c*m.nbEBands] = int16(PSHR32(tmp, 7))
			prev[c] = prev[c] + SHL32(q, 7) - MULT16_16(beta, PSHR32(q, 8))
			c++
			if c >= C {
				break
			}
		}
	}
	if lfe != 0 {
		return 0
	}
	return badness
}

// C: void quant_coarse_energy(const CELTMode *m, int start, int end, int effEnd,
//
//	const opus_val16 *eBands, opus_val16 *oldEBands, opus_uint32 budget,
//	opus_val16 *error, ec_enc *enc, int C, int LM, int nbAvailableBytes,
//	int force_intra, opus_val32 *delayedIntra, int two_pass, int loss_rate, int lfe)
func QuantCoarseEnergy(m *CELTMode, start, end, effEnd int,
	eBands, oldEBands []int16, budget uint32,
	error []int16, enc *EcEnc, C, LM, nbAvailableBytes int,
	force_intra int, delayedIntra *int32, two_pass, loss_rate int, lfe int) {
	var intra int
	var max_decay int16
	var enc_start_state EcEnc
	var tell uint32
	var badness1 int = 0
	var intra_bias int32
	var new_distortion int32

	if force_intra != 0 || (two_pass == 0 && *delayedIntra > int32(2*C*(end-start)) && nbAvailableBytes > (end-start)*C) {
		intra = 1
	}
	intra_bias = int32((int64(budget) * int64(*delayedIntra) * int64(loss_rate)) / int64(C*512))
	new_distortion = loss_distortion(eBands, oldEBands, start, effEnd, m.nbEBands, C)

	tell = uint32(enc.Tell())
	if tell+3 > budget {
		two_pass = 0
		intra = 0
	}

	max_decay = QCONST16(16.0, DB_SHIFT)
	if end-start > 10 {
		max_decay = int16(MIN32(int32(max_decay), SHL32(EXTEND32(nbAvailableBytes), DB_SHIFT-3)))
	}
	if lfe != 0 {
		max_decay = QCONST16(3.0, DB_SHIFT)
	}
	enc_start_state = *enc

	oldEBands_intra := make([]int16, C*m.nbEBands)
	error_intra := make([]int16, C*m.nbEBands)
	copy(oldEBands_intra, oldEBands[:C*m.nbEBands])

	if two_pass != 0 || intra != 0 {
		badness1 = quant_coarse_energy_impl(m, start, end, eBands, oldEBands_intra, int32(budget),
			int32(tell), e_prob_model[LM][1][:], error_intra, enc, C, LM, 1, max_decay, lfe)
	}

	if intra == 0 {
		var intra_buf []byte
		var enc_intra_state EcEnc
		var tell_intra int32
		var nstart_bytes, nintra_bytes uint32
		var save_bytes uint32
		var badness2 int

		tell_intra = int32(enc.TellFrac())

		enc_intra_state = *enc

		nstart_bytes = enc_start_state.RangeBytes()
		nintra_bytes = enc_intra_state.RangeBytes()
		intra_buf = enc_intra_state.GetBuffer()[nstart_bytes:]
		save_bytes = nintra_bytes - nstart_bytes
		intra_bits := make([]byte, save_bytes)
		// Copy bits from intra bit-stream
		copy(intra_bits, intra_buf[:nintra_bytes-nstart_bytes])

		*enc = enc_start_state

		badness2 = quant_coarse_energy_impl(m, start, end, eBands, oldEBands, int32(budget),
			int32(tell), e_prob_model[LM][intra][:], error, enc, C, LM, 0, max_decay, lfe)

		if two_pass != 0 && (badness1 < badness2 || (badness1 == badness2 && int32(enc.TellFrac())+intra_bias > tell_intra)) {
			*enc = enc_intra_state
			// Copy intra bits to bit-stream
			copy(intra_buf[:nintra_bytes-nstart_bytes], intra_bits)
			copy(oldEBands[:C*m.nbEBands], oldEBands_intra)
			copy(error[:C*m.nbEBands], error_intra)
			intra = 1
		}
	} else {
		copy(oldEBands[:C*m.nbEBands], oldEBands_intra)
		copy(error[:C*m.nbEBands], error_intra)
	}

	if intra != 0 {
		*delayedIntra = new_distortion
	} else {
		*delayedIntra = ADD32(MULT16_32_Q15(MULT16_16_Q15(pred_coef[LM], pred_coef[LM]), *delayedIntra),
			new_distortion)
	}
}

// C: void quant_fine_energy(const CELTMode *m, int start, int end, opus_val16 *oldEBands, opus_val16 *error, int *fine_quant, ec_enc *enc, int C)
func QuantFineEnergy(m *CELTMode, start, end int, oldEBands, error []int16, fine_quant []int, enc *EcEnc, C int) {
	var i, c int

	// Encode finer resolution
	for i = start; i < end; i++ {
		var frac int16 = 1 << uint(fine_quant[i])
		if fine_quant[i] <= 0 {
			continue
		}
		c = 0
		for {
			var q2 int
			var offset int16
			// Has to be without rounding
			q2 = int((int32(error[i+c*m.nbEBands]) + int32(QCONST16(.5, DB_SHIFT))) >> uint(DB_SHIFT-fine_quant[i]))
			if q2 > int(frac)-1 {
				q2 = int(frac) - 1
			}
			if q2 < 0 {
				q2 = 0
			}
			enc.EncBits(uint32(q2), uint(fine_quant[i]))
			offset = int16(SUB16(SHR32(SHL32(EXTEND32(q2), DB_SHIFT)+int32(QCONST16(.5, DB_SHIFT)), fine_quant[i]), QCONST16(.5, DB_SHIFT)))
			oldEBands[i+c*m.nbEBands] += offset
			error[i+c*m.nbEBands] -= offset
			// printf ("%f ", error[i] - offset);
			c++
			if c >= C {
				break
			}
		}
	}
}

// C: void quant_energy_finalise(const CELTMode *m, int start, int end, opus_val16 *oldEBands, opus_val16 *error, int *fine_quant, int *fine_priority, int bits_left, ec_enc *enc, int C)
func QuantEnergyFinalise(m *CELTMode, start, end int, oldEBands, error []int16, fine_quant, fine_priority []int, bits_left int, enc *EcEnc, C int) {
	var i, prio, c int

	// Use up the remaining bits
	for prio = 0; prio < 2; prio++ {
		for i = start; i < end && bits_left >= C; i++ {
			if fine_quant[i] >= MAX_FINE_BITS || fine_priority[i] != prio {
				continue
			}
			c = 0
			for {
				var q2 int
				var offset int16
				if error[i+c*m.nbEBands] < 0 {
					q2 = 0
				} else {
					q2 = 1
				}
				enc.EncBits(uint32(q2), 1)
				offset = int16(SHR16(int32(SHL16(q2, DB_SHIFT))-int32(QCONST16(.5, DB_SHIFT)), fine_quant[i]+1))
				oldEBands[i+c*m.nbEBands] += offset
				bits_left--
				c++
				if c >= C {
					break
				}
			}
		}
	}
}

// C: void unquant_coarse_energy(const CELTMode *m, int start, int end, opus_val16 *oldEBands, int intra, ec_dec *dec, int C, int LM)
func UnquantCoarseEnergy(m *CELTMode, start, end int, oldEBands []int16, intra int, dec *EcDec, C, LM int) {
	prob_model := e_prob_model[LM][intra][:]
	var i, c int
	var prev = [2]int32{0, 0}
	var coef, beta int16
	var budget, tell int32

	if intra != 0 {
		coef = 0
		beta = beta_intra
	} else {
		beta = beta_coef[LM]
		coef = pred_coef[LM]
	}

	budget = int32(dec.storage * 8)

	// Decode at a fixed coarse resolution
	for i = start; i < end; i++ {
		c = 0
		for {
			var qi int
			var q, tmp int32
			// It would be better to express this invariant as a
			// test on C at function entry, but that isn't enough
			// to make the static analyzer happy.
			celt_assert(c < 2)
			tell = int32(dec.Tell())
			if budget-tell >= 15 {
				var pi int
				pi = 2 * IMIN(i, 20)
				qi = dec.LaplaceDecode(uint32(prob_model[pi])<<7, int(prob_model[pi+1])<<6)
			} else if budget-tell >= 2 {
				qi = dec.DecIcdf(small_energy_icdf[:], 2)
				qi = (qi >> 1) ^ -(qi & 1)
			} else if budget-tell >= 1 {
				qi = -dec.DecBitLogp(1)
			} else {
				qi = -1
			}
			q = SHL32(EXTEND32(qi), DB_SHIFT)

			oldEBands[i+c*m.nbEBands] = MAX16(-QCONST16(9.0, DB_SHIFT), oldEBands[i+c*m.nbEBands])
			tmp = PSHR32(MULT16_16(coef, oldEBands[i+c*m.nbEBands]), 8) + prev[c] + SHL32(q, 7)
			tmp = MAX32(-QCONST32(28.0, DB_SHIFT+7), tmp)
			oldEBands[i+c*m.nbEBands] = int16(PSHR32(tmp, 7))
			prev[c] = prev[c] + SHL32(q, 7) - MULT16_16(beta, PSHR32(q, 8))
			c++
			if c >= C {
				break
			}
		}
	}
}

// C: void unquant_fine_energy(const CELTMode *m, int start, int end, opus_val16 *oldEBands, int *fine_quant, ec_dec *dec, int C)
func UnquantFineEnergy(m *CELTMode, start, end int, oldEBands []int16, fine_quant []int, dec *EcDec, C int) {
	var i, c int
	// Decode finer resolution
	for i = start; i < end; i++ {
		if fine_quant[i] <= 0 {
			continue
		}
		c = 0
		for {
			var q2 int
			var offset int16
			q2 = int(dec.DecBits(uint(fine_quant[i])))
			offset = int16(SUB16(SHR32(SHL32(EXTEND32(q2), DB_SHIFT)+int32(QCONST16(.5, DB_SHIFT)), fine_quant[i]), QCONST16(.5, DB_SHIFT)))
			oldEBands[i+c*m.nbEBands] += offset
			c++
			if c >= C {
				break
			}
		}
	}
}

// C: void unquant_energy_finalise(const CELTMode *m, int start, int end, opus_val16 *oldEBands, int *fine_quant,  int *fine_priority, int bits_left, ec_dec *dec, int C)
func UnquantEnergyFinalise(m *CELTMode, start, end int, oldEBands []int16, fine_quant, fine_priority []int, bits_left int, dec *EcDec, C int) {
	var i, prio, c int

	// Use up the remaining bits
	for prio = 0; prio < 2; prio++ {
		for i = start; i < end && bits_left >= C; i++ {
			if fine_quant[i] >= MAX_FINE_BITS || fine_priority[i] != prio {
				continue
			}
			c = 0
			for {
				var q2 int
				var offset int16
				q2 = int(dec.DecBits(1))
				offset = int16(SHR16(int32(SHL16(q2, DB_SHIFT))-int32(QCONST16(.5, DB_SHIFT)), fine_quant[i]+1))
				oldEBands[i+c*m.nbEBands] += offset
				bits_left--
				c++
				if c >= C {
					break
				}
			}
		}
	}
}

// C: void amp2Log2(const CELTMode *m, int effEnd, int end, celt_ener *bandE, opus_val16 *bandLogE, int C)
func Amp2Log2(m *CELTMode, effEnd, end int, bandE []int32, bandLogE []int16, C int) {
	var c, i int
	c = 0
	for {
		for i = 0; i < effEnd; i++ {
			bandLogE[i+c*m.nbEBands] = int16(int32(Log2(SHL32(bandE[i+c*m.nbEBands], 2))) - int32(SHL16(int16(eMeans[i]), 6)))
		}
		for i = effEnd; i < end; i++ {
			bandLogE[c*m.nbEBands+i] = -QCONST16(14.0, DB_SHIFT)
		}
		c++
		if c >= C {
			break
		}
	}
}
