package silk

// C: silk/control.h

// Decoder API flags
const (
	FLAG_DECODE_NORMAL = 0
	FLAG_PACKET_LOST   = 1
	FLAG_DECODE_LBRR   = 2
)

// EncControlStruct is the C silk_EncControlStruct: structure for controlling encoder operation.
type EncControlStruct struct {
	// I:   Number of channels; 1/2
	NChannelsAPI int32
	// I:   Number of channels; 1/2
	NChannelsInternal int32
	// I:   Input signal sampling rate in Hertz; 8000/12000/16000/24000/32000/44100/48000
	API_sampleRate int32
	// I:   Maximum internal sampling rate in Hertz; 8000/12000/16000
	MaxInternalSampleRate int32
	// I:   Minimum internal sampling rate in Hertz; 8000/12000/16000
	MinInternalSampleRate int32
	// I:   Soft request for internal sampling rate in Hertz; 8000/12000/16000
	DesiredInternalSampleRate int32
	// I:   Number of samples per packet in milliseconds; 10/20/40/60
	PayloadSize_ms int
	// I:   Bitrate during active speech in bits/second; internally limited
	BitRate int32
	// I:   Uplink packet loss in percent (0-100)
	PacketLossPercentage int
	// I:   Complexity mode; 0 is lowest, 10 is highest complexity
	Complexity int
	// I:   Flag to enable in-band Forward Error Correction (FEC); 0/1
	UseInBandFEC int
	// I:   Flag to enable discontinuous transmission (DTX); 0/1
	UseDTX int
	// I:   Flag to use constant bitrate
	UseCBR int
	// I:   Maximum number of bits allowed for the frame
	MaxBits int
	// I:   Causes a smooth downmix to mono
	ToMono int
	// I:   Opus encoder is allowing us to switch bandwidth
	OpusCanSwitch int
	// I: Make frames as independent as possible (but still use LPC)
	ReducedDependency int32
	// O:   Internal sampling rate used, in Hertz; 8000/12000/16000
	InternalSampleRate int32
	// O: Flag that bandwidth switching is allowed (because low voice activity)
	AllowBandwidthSwitch int
	// O:   Flag that SILK runs in WB mode without variable LP filter (use for switching between WB/SWB/FB)
	InWBmodeWithoutVariableLP int
	// O:   Stereo width
	StereoWidth_Q14 int
	// O:   Tells the Opus encoder we're ready to switch
	SwitchReady int
}

// DecControlStruct is the C silk_DecControlStruct: structure for controlling
// decoder operation and reading decoder status.
type DecControlStruct struct {
	// I:   Number of channels; 1/2
	NChannelsAPI int32
	// I:   Number of channels; 1/2
	NChannelsInternal int32
	// I:   Output signal sampling rate in Hertz; 8000/12000/16000/24000/32000/44100/48000
	API_sampleRate int32
	// I:   Internal sampling rate used, in Hertz; 8000/12000/16000
	InternalSampleRate int32
	// I:   Number of samples per packet in milliseconds; 10/20/40/60
	PayloadSize_ms int
	// O:   Pitch lag of previous frame (0 if unvoiced), measured in samples at 48 kHz
	PrevPitchLag int
}
