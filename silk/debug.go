package silk

import "fmt"

// DebugState is a temporary helper used by the differential test harness.
func (d *Decoder) DebugState() string {
	s := &d.channel_state[0]
	h16 := func(p []int16) int64 {
		var h int64
		for _, v := range p {
			h = h*31 + int64(v)
		}
		return h
	}
	h32 := func(p []int32) int64 {
		var h int64
		for _, v := range p {
			h = h*31 + int64(v)
		}
		return h
	}
	return fmt.Sprintf("lossCnt=%d sig=%d lagPrev=%d lgi=%d ffar=%d fs=%d nsf=%d sfl=%d exc=%d sLPC=%d outBuf=%d NLSF=%d pg=%d | PLC pitch=%d ltp=%d lpc=%d lfl=%d seed=%d rs=%d ce=%d ces=%d pls=%d pg0=%d pg1=%d | CNG g=%d seed=%d syn=%d nlsf=%d exc=%d",
		s.lossCnt, s.prevSignalType, s.lagPrev, s.LastGainIndex, s.first_frame_after_reset, s.fs_kHz, s.nb_subfr, s.subfr_length,
		h32(s.exc_Q14[:]), h32(s.sLPC_Q14_buf[:]), h16(s.outBuf[:]), h16(s.prevNLSF_Q15[:]), s.prev_gain_Q16,
		s.sPLC.pitchL_Q8, h16(s.sPLC.LTPCoef_Q14[:]), h16(s.sPLC.prevLPC_Q12[:]), s.sPLC.last_frame_lost, s.sPLC.rand_seed, s.sPLC.randScale_Q14, s.sPLC.conc_energy, s.sPLC.conc_energy_shift, s.sPLC.prevLTP_scale_Q14, s.sPLC.prevGain_Q16[0], s.sPLC.prevGain_Q16[1],
		s.sCNG.CNG_smth_Gain_Q16, s.sCNG.rand_seed, h32(s.sCNG.CNG_synth_state[:]), h16(s.sCNG.CNG_smth_NLSF_Q15[:]), h32(s.sCNG.CNG_exc_buf_Q14[:]))
}
