package silk

// C: silk/decoder_set_fs.c

/* Set decoder sampling rate */
//
// C: opus_int silk_decoder_set_fs(silk_decoder_state *psDec, opus_int fs_kHz, opus_int32 fs_API_Hz)
func (psDec *DecoderState) DecoderSetFs(fs_kHz int, fs_API_Hz int32) int {
	var frame_length int
	ret := 0

	silk_assert(fs_kHz == 8 || fs_kHz == 12 || fs_kHz == 16)
	silk_assert(psDec.nb_subfr == MAX_NB_SUBFR || psDec.nb_subfr == MAX_NB_SUBFR/2)

	/* New (sub)frame length */
	psDec.subfr_length = int(SMULBB(SUB_FRAME_LENGTH_MS, fs_kHz))
	frame_length = int(SMULBB(psDec.nb_subfr, psDec.subfr_length))

	/* Initialize resampler when switching internal or external sampling frequency */
	if psDec.fs_kHz != fs_kHz || psDec.fs_API_hz != fs_API_Hz {
		/* Initialize the resampler for dec_API.c preparing resampling from fs_kHz to API_fs_Hz */
		ret += psDec.resampler_state.ResamplerInit(SMULBB(fs_kHz, 1000), fs_API_Hz, 0)

		psDec.fs_API_hz = fs_API_Hz
	}

	if psDec.fs_kHz != fs_kHz || frame_length != psDec.frame_length {
		if fs_kHz == 8 {
			if psDec.nb_subfr == MAX_NB_SUBFR {
				psDec.pitch_contour_iCDF = pitch_contour_NB_iCDF[:]
			} else {
				psDec.pitch_contour_iCDF = pitch_contour_10_ms_NB_iCDF[:]
			}
		} else {
			if psDec.nb_subfr == MAX_NB_SUBFR {
				psDec.pitch_contour_iCDF = pitch_contour_iCDF[:]
			} else {
				psDec.pitch_contour_iCDF = pitch_contour_10_ms_iCDF[:]
			}
		}
		if psDec.fs_kHz != fs_kHz {
			psDec.ltp_mem_length = int(SMULBB(LTP_MEM_LENGTH_MS, fs_kHz))
			if fs_kHz == 8 || fs_kHz == 12 {
				psDec.LPC_order = MIN_LPC_ORDER
				psDec.psNLSF_CB = &NLSF_CB_NB_MB
			} else {
				psDec.LPC_order = MAX_LPC_ORDER
				psDec.psNLSF_CB = &NLSF_CB_WB
			}
			if fs_kHz == 16 {
				psDec.pitch_lag_low_bits_iCDF = uniform8_iCDF[:]
			} else if fs_kHz == 12 {
				psDec.pitch_lag_low_bits_iCDF = uniform6_iCDF[:]
			} else if fs_kHz == 8 {
				psDec.pitch_lag_low_bits_iCDF = uniform4_iCDF[:]
			} else {
				/* unsupported sampling rate */
				silk_assert(false)
			}
			psDec.first_frame_after_reset = 1
			psDec.lagPrev = 100
			psDec.LastGainIndex = 10
			psDec.prevSignalType = TYPE_NO_VOICE_ACTIVITY
			clear(psDec.outBuf[:])
			clear(psDec.sLPC_Q14_buf[:])
		}

		psDec.fs_kHz = fs_kHz
		psDec.frame_length = frame_length
	}

	/* Check that settings are valid */
	silk_assert(psDec.frame_length > 0 && psDec.frame_length <= MAX_FRAME_LENGTH)

	return ret
}
