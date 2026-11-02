package opus

// C: include/opus_defines.h

// Error codes
const (
	// No error
	OPUS_OK = 0
	// One or more invalid/out of range arguments
	OPUS_BAD_ARG = -1
	// The mode struct passed is invalid
	OPUS_BUFFER_TOO_SMALL = -2
	// An internal error was detected
	OPUS_INTERNAL_ERROR = -3
	// The compressed data passed is corrupted
	OPUS_INVALID_PACKET = -4
	// Invalid/unsupported request number
	OPUS_UNIMPLEMENTED = -5
	// An encoder or decoder structure is invalid or already freed
	OPUS_INVALID_STATE = -6
	// Memory allocation has failed
	OPUS_ALLOC_FAIL = -7
)

// Request codes (these are the values of the C OPUS_*_REQUEST macros; the
// value/pointer-checking macros OPUS_SET_X(x) collapse to these).
const (
	OPUS_SET_APPLICATION_REQUEST          = 4000
	OPUS_GET_APPLICATION_REQUEST          = 4001
	OPUS_SET_BITRATE_REQUEST              = 4002
	OPUS_GET_BITRATE_REQUEST              = 4003
	OPUS_SET_MAX_BANDWIDTH_REQUEST        = 4004
	OPUS_GET_MAX_BANDWIDTH_REQUEST        = 4005
	OPUS_SET_VBR_REQUEST                  = 4006
	OPUS_GET_VBR_REQUEST                  = 4007
	OPUS_SET_BANDWIDTH_REQUEST            = 4008
	OPUS_GET_BANDWIDTH_REQUEST            = 4009
	OPUS_SET_COMPLEXITY_REQUEST           = 4010
	OPUS_GET_COMPLEXITY_REQUEST           = 4011
	OPUS_SET_INBAND_FEC_REQUEST           = 4012
	OPUS_GET_INBAND_FEC_REQUEST           = 4013
	OPUS_SET_PACKET_LOSS_PERC_REQUEST     = 4014
	OPUS_GET_PACKET_LOSS_PERC_REQUEST     = 4015
	OPUS_SET_DTX_REQUEST                  = 4016
	OPUS_GET_DTX_REQUEST                  = 4017
	OPUS_SET_VBR_CONSTRAINT_REQUEST       = 4020
	OPUS_GET_VBR_CONSTRAINT_REQUEST       = 4021
	OPUS_SET_FORCE_CHANNELS_REQUEST       = 4022
	OPUS_GET_FORCE_CHANNELS_REQUEST       = 4023
	OPUS_SET_SIGNAL_REQUEST               = 4024
	OPUS_GET_SIGNAL_REQUEST               = 4025
	OPUS_GET_LOOKAHEAD_REQUEST            = 4027
	OPUS_RESET_STATE                      = 4028
	OPUS_GET_SAMPLE_RATE_REQUEST          = 4029
	OPUS_GET_FINAL_RANGE_REQUEST          = 4031
	OPUS_GET_PITCH_REQUEST                = 4033
	OPUS_SET_GAIN_REQUEST                 = 4034
	OPUS_GET_GAIN_REQUEST                 = 4045 /* Should have been 4035 */
	OPUS_SET_LSB_DEPTH_REQUEST            = 4036
	OPUS_GET_LSB_DEPTH_REQUEST            = 4037
	OPUS_GET_LAST_PACKET_DURATION_REQUEST = 4039
)

// Values for the various encoder CTLs
const (
	OPUS_AUTO        = -1000 // Auto/default setting
	OPUS_BITRATE_MAX = -1    // Maximum bitrate

	// Best for most VoIP/videoconference applications where listening quality and intelligibility matter most
	OPUS_APPLICATION_VOIP = 2048
	// Best for broadcast/high-fidelity application where the decoded audio should be as close as possible to the input
	OPUS_APPLICATION_AUDIO = 2049
	// Only use when lowest-achievable latency is what matters most. Voice-optimized modes cannot be used.
	OPUS_APPLICATION_RESTRICTED_LOWDELAY = 2051

	OPUS_SIGNAL_VOICE = 3001 // Signal being encoded is voice
	OPUS_SIGNAL_MUSIC = 3002 // Signal being encoded is music

	OPUS_BANDWIDTH_NARROWBAND    = 1101 // 4 kHz bandpass
	OPUS_BANDWIDTH_MEDIUMBAND    = 1102 // 6 kHz bandpass
	OPUS_BANDWIDTH_WIDEBAND      = 1103 // 8 kHz bandpass
	OPUS_BANDWIDTH_SUPERWIDEBAND = 1104 // 12 kHz bandpass
	OPUS_BANDWIDTH_FULLBAND      = 1105 // 20 kHz bandpass
)

// C: include/opus_multistream.h
const (
	OPUS_MULTISTREAM_GET_ENCODER_STATE_REQUEST = 5120
	OPUS_MULTISTREAM_GET_DECODER_STATE_REQUEST = 5122
)

// C: src/opus_private.h
const (
	MODE_SILK_ONLY = 1000
	MODE_HYBRID    = 1001
	MODE_CELT_ONLY = 1002

	OPUS_SET_VOICE_RATIO_REQUEST = 11018
	OPUS_GET_VOICE_RATIO_REQUEST = 11019

	OPUS_SET_FORCE_MODE_REQUEST = 11002
)
