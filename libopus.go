// Package libopus is an idiomatic Go wrapper around the mechanical libopus
// port in the celt, silk and opus sub-packages. It is pure Go (CGO_ENABLED=0).
//
// The low-level opus package mirrors the C API one to one (Ctl requests,
// negative error codes, C naming). This package exposes the same
// functionality with Go types, errors and methods, without hiding any
// configuration: everything reachable through opus_encoder_ctl /
// opus_decoder_ctl has a typed method, and Ctl remains available as an
// escape hatch.
package libopus

import (
	"errors"
	"fmt"

	"github.com/DubbaThonyVibecodes/go-libopus/opus"
)

// Version is the upstream libopus version this port tracks.
func Version() string { return opus.GetVersionString() }

// Error is a libopus error code (the negative OPUS_* values).
type Error int

func (e Error) Error() string { return "opus: " + opus.Strerror(int(e)) }

var (
	ErrBadArg         = Error(opus.OPUS_BAD_ARG)
	ErrBufferTooSmall = Error(opus.OPUS_BUFFER_TOO_SMALL)
	ErrInternal       = Error(opus.OPUS_INTERNAL_ERROR)
	ErrInvalidPacket  = Error(opus.OPUS_INVALID_PACKET)
	ErrUnimplemented  = Error(opus.OPUS_UNIMPLEMENTED)
	ErrInvalidState   = Error(opus.OPUS_INVALID_STATE)
	ErrAllocFail      = Error(opus.OPUS_ALLOC_FAIL)
)

func errOf(code int) error {
	if code >= 0 {
		return nil
	}
	return Error(code)
}

// Application selects the encoder's intended use.
type Application int

const (
	// VoIP: best for most VoIP/videoconference applications where listening
	// quality and intelligibility matter most.
	VoIP Application = opus.OPUS_APPLICATION_VOIP
	// Audio: best for broadcast/high-fidelity application where the decoded
	// audio should be as close as possible to the input.
	Audio Application = opus.OPUS_APPLICATION_AUDIO
	// RestrictedLowDelay: only use when lowest-achievable latency is what
	// matters most. Voice-optimized modes cannot be used.
	RestrictedLowDelay Application = opus.OPUS_APPLICATION_RESTRICTED_LOWDELAY
)

func (a Application) String() string {
	switch a {
	case VoIP:
		return "voip"
	case Audio:
		return "audio"
	case RestrictedLowDelay:
		return "restricted-lowdelay"
	}
	return fmt.Sprintf("Application(%d)", int(a))
}

// Bandwidth is the audio bandwidth of a stream or encoder setting.
type Bandwidth int

const (
	BandwidthAuto Bandwidth = opus.OPUS_AUTO
	Narrowband    Bandwidth = opus.OPUS_BANDWIDTH_NARROWBAND    // 4 kHz
	Mediumband    Bandwidth = opus.OPUS_BANDWIDTH_MEDIUMBAND    // 6 kHz
	Wideband      Bandwidth = opus.OPUS_BANDWIDTH_WIDEBAND      // 8 kHz
	SuperWideband Bandwidth = opus.OPUS_BANDWIDTH_SUPERWIDEBAND // 12 kHz
	Fullband      Bandwidth = opus.OPUS_BANDWIDTH_FULLBAND      // 20 kHz
)

func (b Bandwidth) String() string {
	switch b {
	case BandwidthAuto:
		return "auto"
	case Narrowband:
		return "narrowband"
	case Mediumband:
		return "mediumband"
	case Wideband:
		return "wideband"
	case SuperWideband:
		return "superwideband"
	case Fullband:
		return "fullband"
	}
	return fmt.Sprintf("Bandwidth(%d)", int(b))
}

// Signal hints the encoder about the content type.
type Signal int

const (
	SignalAuto  Signal = opus.OPUS_AUTO
	SignalVoice Signal = opus.OPUS_SIGNAL_VOICE
	SignalMusic Signal = opus.OPUS_SIGNAL_MUSIC
)

// Mode is the codec mode of a packet (SILK, hybrid or CELT).
type Mode int

const (
	ModeSILK   Mode = opus.MODE_SILK_ONLY
	ModeHybrid Mode = opus.MODE_HYBRID
	ModeCELT   Mode = opus.MODE_CELT_ONLY
)

// FrameDuration selects the frame size the encoder uses regardless of the
// number of samples passed to Encode (OPUS_SET_EXPERT_FRAME_DURATION,
// libopus 1.1+). With FrameDurationArg (the default) the frame size is the
// number of samples passed. When the encoder consumes fewer samples than
// offered, the packet says how many were used (see PacketSamples) and the
// caller must feed the rest again. Frames of 80, 100 and 120 ms are
// multi-frame packets (libopus 1.2+).
type FrameDuration int

const (
	FrameDurationArg   FrameDuration = opus.OPUS_FRAMESIZE_ARG
	FrameDuration2_5ms FrameDuration = opus.OPUS_FRAMESIZE_2_5_MS
	FrameDuration5ms   FrameDuration = opus.OPUS_FRAMESIZE_5_MS
	FrameDuration10ms  FrameDuration = opus.OPUS_FRAMESIZE_10_MS
	FrameDuration20ms  FrameDuration = opus.OPUS_FRAMESIZE_20_MS
	FrameDuration40ms  FrameDuration = opus.OPUS_FRAMESIZE_40_MS
	FrameDuration60ms  FrameDuration = opus.OPUS_FRAMESIZE_60_MS
	FrameDuration80ms  FrameDuration = opus.OPUS_FRAMESIZE_80_MS
	FrameDuration100ms FrameDuration = opus.OPUS_FRAMESIZE_100_MS
	FrameDuration120ms FrameDuration = opus.OPUS_FRAMESIZE_120_MS
)

// Auto is the OPUS_AUTO value for settings that accept it (bitrate, forced channels).
const Auto = opus.OPUS_AUTO

// BitrateMax requests the maximum bitrate that fits the output buffer.
const BitrateMax = opus.OPUS_BITRATE_MAX

// MaxPacket is the largest packet a single Opus frame can produce
// (1275 bytes of payload plus the table-of-contents byte); a 120 ms
// multi-frame packet can be up to 3*1275+7 bytes.
const MaxPacket = 1276

// ValidSampleRate reports whether rate is one of the Opus API rates.
func ValidSampleRate(rate int) bool {
	switch rate {
	case 8000, 12000, 16000, 24000, 48000:
		return true
	}
	return false
}

// ValidFrameSize reports whether samplesPerChannel is a legal frame size at
// rate (2.5, 5, 10, 20, 40 or 60 ms).
func ValidFrameSize(rate, samplesPerChannel int) bool {
	n := samplesPerChannel
	return n > 0 && (400*n == rate || 200*n == rate || 100*n == rate || 50*n == rate || 25*n == rate || 50*n == 3*rate)
}

// ---- Encoder ------------------------------------------------------------

// Encoder encodes 16-bit PCM into Opus packets.
type Encoder struct {
	st       *opus.OpusEncoder
	rate     int
	channels int
}

// NewEncoder creates an encoder for the given API sample rate (8000, 12000,
// 16000, 24000 or 48000 Hz) and channel count (1 or 2).
func NewEncoder(sampleRate, channels int, app Application) (*Encoder, error) {
	var code int
	st := opus.EncoderCreate(int32(sampleRate), channels, int(app), &code)
	if st == nil {
		return nil, errOf(code)
	}
	return &Encoder{st: st, rate: sampleRate, channels: channels}, nil
}

// SampleRate returns the API sample rate.
func (e *Encoder) SampleRate() int { return e.rate }

// Channels returns the number of input channels.
func (e *Encoder) Channels() int { return e.channels }

// Encode encodes one frame of interleaved 16-bit PCM. The frame duration is
// len(pcm)/Channels() samples, which must be 2.5, 5, 10, 20, 40 or 60 ms at
// the encoder's sample rate. The packet is written into out and its length
// returned; out limits the packet size (use at least MaxPacket bytes unless
// a smaller maximum is wanted).
func (e *Encoder) Encode(pcm []int16, out []byte) (int, error) {
	if len(pcm)%e.channels != 0 {
		return 0, fmt.Errorf("opus: pcm length %d is not a multiple of %d channels", len(pcm), e.channels)
	}
	n := e.st.Encode(pcm, len(pcm)/e.channels, out, int32(len(out)))
	if n < 0 {
		return 0, Error(n)
	}
	return int(n), nil
}

// EncodeFloat is Encode for float32 input in [-1, 1].
func (e *Encoder) EncodeFloat(pcm []float32, out []byte) (int, error) {
	if len(pcm)%e.channels != 0 {
		return 0, fmt.Errorf("opus: pcm length %d is not a multiple of %d channels", len(pcm), e.channels)
	}
	n := e.st.EncodeFloat(pcm, len(pcm)/e.channels, out, int32(len(out)))
	if n < 0 {
		return 0, Error(n)
	}
	return int(n), nil
}

// Ctl is the raw opus_encoder_ctl escape hatch. Value arguments are int32
// (or int); output arguments are *int32 / *uint32.
func (e *Encoder) Ctl(request int, args ...any) error { return errOf(e.st.Ctl(request, args...)) }

// Raw returns the low-level encoder state.
func (e *Encoder) Raw() *opus.OpusEncoder { return e.st }

func (e *Encoder) getInt(req int) int {
	var v int32
	e.st.Ctl(req, &v)
	return int(v)
}

func b2i(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

// SetBitrate sets the target bitrate in bits per second (Auto or BitrateMax allowed).
func (e *Encoder) SetBitrate(bps int) error { return e.Ctl(opus.OPUS_SET_BITRATE_REQUEST, int32(bps)) }

// Bitrate returns the current bitrate setting.
func (e *Encoder) Bitrate() int { return e.getInt(opus.OPUS_GET_BITRATE_REQUEST) }

// SetComplexity sets the computational complexity, 0 (lowest) to 10 (highest).
func (e *Encoder) SetComplexity(c int) error {
	return e.Ctl(opus.OPUS_SET_COMPLEXITY_REQUEST, int32(c))
}

// Complexity returns the complexity setting.
func (e *Encoder) Complexity() int { return e.getInt(opus.OPUS_GET_COMPLEXITY_REQUEST) }

// SetVBR enables (default) or disables variable bitrate.
func (e *Encoder) SetVBR(on bool) error { return e.Ctl(opus.OPUS_SET_VBR_REQUEST, b2i(on)) }

// VBR reports whether variable bitrate is enabled.
func (e *Encoder) VBR() bool { return e.getInt(opus.OPUS_GET_VBR_REQUEST) != 0 }

// SetVBRConstraint enables (default) or disables constrained VBR.
func (e *Encoder) SetVBRConstraint(on bool) error {
	return e.Ctl(opus.OPUS_SET_VBR_CONSTRAINT_REQUEST, b2i(on))
}

// VBRConstraint reports whether constrained VBR is enabled.
func (e *Encoder) VBRConstraint() bool { return e.getInt(opus.OPUS_GET_VBR_CONSTRAINT_REQUEST) != 0 }

// SetForceChannels forces mono (1) or stereo (2) coding, or Auto.
func (e *Encoder) SetForceChannels(ch int) error {
	return e.Ctl(opus.OPUS_SET_FORCE_CHANNELS_REQUEST, int32(ch))
}

// ForceChannels returns the forced channel setting (Auto if not forced).
func (e *Encoder) ForceChannels() int { return e.getInt(opus.OPUS_GET_FORCE_CHANNELS_REQUEST) }

// SetMaxBandwidth sets the maximum bandwidth the encoder may use.
func (e *Encoder) SetMaxBandwidth(bw Bandwidth) error {
	return e.Ctl(opus.OPUS_SET_MAX_BANDWIDTH_REQUEST, int32(bw))
}

// MaxBandwidth returns the maximum bandwidth setting.
func (e *Encoder) MaxBandwidth() Bandwidth {
	return Bandwidth(e.getInt(opus.OPUS_GET_MAX_BANDWIDTH_REQUEST))
}

// SetBandwidth forces a bandwidth, or BandwidthAuto.
func (e *Encoder) SetBandwidth(bw Bandwidth) error {
	return e.Ctl(opus.OPUS_SET_BANDWIDTH_REQUEST, int32(bw))
}

// Bandwidth returns the bandwidth of the last encoded frame.
func (e *Encoder) Bandwidth() Bandwidth { return Bandwidth(e.getInt(opus.OPUS_GET_BANDWIDTH_REQUEST)) }

// SetSignal hints the content type.
func (e *Encoder) SetSignal(s Signal) error { return e.Ctl(opus.OPUS_SET_SIGNAL_REQUEST, int32(s)) }

// Signal returns the signal hint.
func (e *Encoder) Signal() Signal { return Signal(e.getInt(opus.OPUS_GET_SIGNAL_REQUEST)) }

// SetApplication changes the application; only allowed before the first
// frame or right after Reset.
func (e *Encoder) SetApplication(a Application) error {
	return e.Ctl(opus.OPUS_SET_APPLICATION_REQUEST, int32(a))
}

// Application returns the application setting.
func (e *Encoder) Application() Application {
	return Application(e.getInt(opus.OPUS_GET_APPLICATION_REQUEST))
}

// Lookahead returns the total encoder delay in samples (at the API rate).
func (e *Encoder) Lookahead() int { return e.getInt(opus.OPUS_GET_LOOKAHEAD_REQUEST) }

// SetInbandFEC enables in-band forward error correction (SILK modes).
func (e *Encoder) SetInbandFEC(on bool) error {
	return e.Ctl(opus.OPUS_SET_INBAND_FEC_REQUEST, b2i(on))
}

// InbandFEC reports whether in-band FEC is enabled.
func (e *Encoder) InbandFEC() bool { return e.getInt(opus.OPUS_GET_INBAND_FEC_REQUEST) != 0 }

// SetPacketLossPerc tells the encoder the expected packet loss in percent.
func (e *Encoder) SetPacketLossPerc(p int) error {
	return e.Ctl(opus.OPUS_SET_PACKET_LOSS_PERC_REQUEST, int32(p))
}

// PacketLossPerc returns the expected packet loss setting.
func (e *Encoder) PacketLossPerc() int { return e.getInt(opus.OPUS_GET_PACKET_LOSS_PERC_REQUEST) }

// SetDTX enables discontinuous transmission.
func (e *Encoder) SetDTX(on bool) error { return e.Ctl(opus.OPUS_SET_DTX_REQUEST, b2i(on)) }

// DTX reports whether DTX is enabled.
func (e *Encoder) DTX() bool { return e.getInt(opus.OPUS_GET_DTX_REQUEST) != 0 }

// SetLSBDepth tells the encoder the input's bit depth (8..24).
func (e *Encoder) SetLSBDepth(bits int) error {
	return e.Ctl(opus.OPUS_SET_LSB_DEPTH_REQUEST, int32(bits))
}

// LSBDepth returns the LSB depth setting.
func (e *Encoder) LSBDepth() int { return e.getInt(opus.OPUS_GET_LSB_DEPTH_REQUEST) }

// SetForceMode forces the codec mode (ModeSILK/ModeHybrid/ModeCELT) or Auto.
// This is the private OPUS_SET_FORCE_MODE request; it is mostly useful for
// testing.
func (e *Encoder) SetForceMode(m Mode) error {
	return e.Ctl(opus.OPUS_SET_FORCE_MODE_REQUEST, int32(m))
}

// SetVoiceRatio sets the voice percentage hint (0-100, -1 for auto); the
// private OPUS_SET_VOICE_RATIO request.
func (e *Encoder) SetVoiceRatio(r int) error {
	return e.Ctl(opus.OPUS_SET_VOICE_RATIO_REQUEST, int32(r))
}

// SetExpertFrameDuration sets the frame duration policy (libopus 1.1+).
func (e *Encoder) SetExpertFrameDuration(d FrameDuration) error {
	return e.Ctl(opus.OPUS_SET_EXPERT_FRAME_DURATION_REQUEST, int32(d))
}

// ExpertFrameDuration returns the frame duration policy.
func (e *Encoder) ExpertFrameDuration() FrameDuration {
	return FrameDuration(e.getInt(opus.OPUS_GET_EXPERT_FRAME_DURATION_REQUEST))
}

// SetPredictionDisabled disables almost all inter-frame prediction so that
// any packet can be decoded almost independently (libopus 1.1+). Costs
// bitrate and quality.
func (e *Encoder) SetPredictionDisabled(off bool) error {
	return e.Ctl(opus.OPUS_SET_PREDICTION_DISABLED_REQUEST, b2i(off))
}

// PredictionDisabled reports whether inter-frame prediction is disabled.
func (e *Encoder) PredictionDisabled() bool {
	return e.getInt(opus.OPUS_GET_PREDICTION_DISABLED_REQUEST) != 0
}

// SetPhaseInversionDisabled disables the use of phase inversion for
// intensity stereo, improving the quality of mono downmixes at the cost
// of stereo quality (libopus 1.2+).
func (e *Encoder) SetPhaseInversionDisabled(off bool) error {
	return e.Ctl(opus.OPUS_SET_PHASE_INVERSION_DISABLED_REQUEST, b2i(off))
}

// PhaseInversionDisabled reports whether phase inversion is disabled.
func (e *Encoder) PhaseInversionDisabled() bool {
	return e.getInt(opus.OPUS_GET_PHASE_INVERSION_DISABLED_REQUEST) != 0
}

// FinalRange returns the range coder's final state for the last packet
// (useful for comparing encoder and decoder).
func (e *Encoder) FinalRange() uint32 {
	var v uint32
	e.st.Ctl(opus.OPUS_GET_FINAL_RANGE_REQUEST, &v)
	return v
}

// Reset resets the encoder state (settings are kept).
func (e *Encoder) Reset() error { return e.Ctl(opus.OPUS_RESET_STATE) }

// ---- Decoder ------------------------------------------------------------

// Decoder decodes Opus packets into 16-bit PCM.
type Decoder struct {
	st       *opus.OpusDecoder
	rate     int
	channels int
}

// NewDecoder creates a decoder for the given output sample rate and channel count.
func NewDecoder(sampleRate, channels int) (*Decoder, error) {
	var code int
	st := opus.DecoderCreate(int32(sampleRate), channels, &code)
	if st == nil {
		return nil, errOf(code)
	}
	return &Decoder{st: st, rate: sampleRate, channels: channels}, nil
}

// SampleRate returns the output sample rate.
func (d *Decoder) SampleRate() int { return d.rate }

// Channels returns the number of output channels.
func (d *Decoder) Channels() int { return d.channels }

// Decode decodes one packet into pcm (interleaved) and returns the number
// of samples per channel written. pcm must hold at least one full packet
// (120 ms at the output rate is always enough). A nil or empty packet
// triggers packet loss concealment for len(pcm)/Channels() samples, which
// must then be a multiple of 2.5 ms. With fec set, the in-band FEC data of
// this packet is decoded instead to reconstruct the previous, lost packet.
func (d *Decoder) Decode(packet []byte, pcm []int16, fec bool) (int, error) {
	if len(pcm)%d.channels != 0 {
		return 0, fmt.Errorf("opus: pcm length %d is not a multiple of %d channels", len(pcm), d.channels)
	}
	n := d.st.Decode(packet, int32(len(packet)), pcm, len(pcm)/d.channels, int(b2i(fec)))
	if n < 0 {
		return 0, Error(n)
	}
	return n, nil
}

// DecodeFloat is Decode with float32 output in [-1, 1].
func (d *Decoder) DecodeFloat(packet []byte, pcm []float32, fec bool) (int, error) {
	if len(pcm)%d.channels != 0 {
		return 0, fmt.Errorf("opus: pcm length %d is not a multiple of %d channels", len(pcm), d.channels)
	}
	n := d.st.DecodeFloat(packet, int32(len(packet)), pcm, len(pcm)/d.channels, int(b2i(fec)))
	if n < 0 {
		return 0, Error(n)
	}
	return n, nil
}

// Ctl is the raw opus_decoder_ctl escape hatch.
func (d *Decoder) Ctl(request int, args ...any) error { return errOf(d.st.Ctl(request, args...)) }

// Raw returns the low-level decoder state.
func (d *Decoder) Raw() *opus.OpusDecoder { return d.st }

func (d *Decoder) getInt(req int) int {
	var v int32
	d.st.Ctl(req, &v)
	return int(v)
}

// SetGain sets the output gain in Q8 dB (value/256 dB), -32768..32767.
func (d *Decoder) SetGain(q8dB int) error { return d.Ctl(opus.OPUS_SET_GAIN_REQUEST, int32(q8dB)) }

// Gain returns the output gain setting in Q8 dB.
func (d *Decoder) Gain() int { return d.getInt(opus.OPUS_GET_GAIN_REQUEST) }

// Bandwidth returns the bandwidth of the last decoded packet.
func (d *Decoder) Bandwidth() Bandwidth { return Bandwidth(d.getInt(opus.OPUS_GET_BANDWIDTH_REQUEST)) }

// Pitch returns the pitch period of the last decoded frame in samples at 48 kHz (0 if unvoiced/unknown).
func (d *Decoder) Pitch() int { return d.getInt(opus.OPUS_GET_PITCH_REQUEST) }

// LastPacketDuration returns the duration of the last decoded packet in samples per channel.
func (d *Decoder) LastPacketDuration() int {
	return d.getInt(opus.OPUS_GET_LAST_PACKET_DURATION_REQUEST)
}

// FinalRange returns the range coder's final state for the last packet.
func (d *Decoder) FinalRange() uint32 {
	var v uint32
	d.st.Ctl(opus.OPUS_GET_FINAL_RANGE_REQUEST, &v)
	return v
}

// SetPhaseInversionDisabled disables phase inversion when decoding
// intensity-stereo frames, for better mono downmixes (libopus 1.2+).
func (d *Decoder) SetPhaseInversionDisabled(off bool) error {
	return d.Ctl(opus.OPUS_SET_PHASE_INVERSION_DISABLED_REQUEST, b2i(off))
}

// PhaseInversionDisabled reports whether phase inversion is disabled.
func (d *Decoder) PhaseInversionDisabled() bool {
	return d.getInt(opus.OPUS_GET_PHASE_INVERSION_DISABLED_REQUEST) != 0
}

// Reset resets the decoder state.
func (d *Decoder) Reset() error { return d.Ctl(opus.OPUS_RESET_STATE) }

// PacketSamples returns the number of samples per channel a packet decodes
// to at the decoder's rate.
func (d *Decoder) PacketSamples(packet []byte) (int, error) {
	n := d.st.DecoderGetNbSamples(packet, int32(len(packet)))
	if n < 0 {
		return 0, Error(n)
	}
	return n, nil
}

// ---- Packet helpers -------------------------------------------------------

// PacketBandwidth returns the bandwidth encoded in the packet's TOC byte.
func PacketBandwidth(packet []byte) (Bandwidth, error) {
	if len(packet) < 1 {
		return 0, ErrBadArg
	}
	return Bandwidth(opus.PacketGetBandwidth(packet)), nil
}

// PacketMode returns the codec mode encoded in the packet's TOC byte.
func PacketMode(packet []byte) (Mode, error) {
	if len(packet) < 1 {
		return 0, ErrBadArg
	}
	if packet[0]&0x80 != 0 {
		return ModeCELT, nil
	} else if packet[0]&0x60 == 0x60 {
		return ModeHybrid, nil
	}
	return ModeSILK, nil
}

// PacketSamplesPerFrame returns the samples per channel of one frame of the
// packet at the given rate.
func PacketSamplesPerFrame(packet []byte, rate int) (int, error) {
	if len(packet) < 1 {
		return 0, ErrBadArg
	}
	return opus.PacketGetSamplesPerFrame(packet, int32(rate)), nil
}

// PacketChannels returns the number of channels (1 or 2) signalled by the packet.
func PacketChannels(packet []byte) (int, error) {
	if len(packet) < 1 {
		return 0, ErrBadArg
	}
	return opus.PacketGetNbChannels(packet), nil
}

// PacketFrames returns the number of frames in the packet.
func PacketFrames(packet []byte) (int, error) {
	n := opus.PacketGetNbFrames(packet, int32(len(packet)))
	if n < 0 {
		return 0, Error(n)
	}
	return n, nil
}

// PacketSamples returns the total samples per channel of the packet at rate.
func PacketSamples(packet []byte, rate int) (int, error) {
	n := opus.PacketGetNbSamples(packet, int32(len(packet)), int32(rate))
	if n < 0 {
		return 0, Error(n)
	}
	return n, nil
}

// PacketParse splits a packet into its frames. It returns the TOC byte and
// the frame payloads (sub-slices of packet).
func PacketParse(packet []byte) (toc byte, frames [][]byte, err error) {
	var fr [48][]byte
	var sizes [48]int16
	var off int
	n := opus.PacketParse(packet, int32(len(packet)), &toc, fr[:], sizes[:], &off)
	if n < 0 {
		return 0, nil, Error(n)
	}
	frames = make([][]byte, n)
	for i := range frames {
		frames[i] = fr[i][:sizes[i]]
	}
	return toc, frames, nil
}

// ---- Repacketizer ---------------------------------------------------------

// Repacketizer merges or splits packets of the same stream (same TOC
// configuration) into packets of up to 120 ms.
type Repacketizer struct{ rp *opus.OpusRepacketizer }

// NewRepacketizer creates an empty repacketizer.
func NewRepacketizer() *Repacketizer { return &Repacketizer{rp: opus.RepacketizerCreate()} }

// Reset discards the collected frames.
func (r *Repacketizer) Reset() { r.rp.RepacketizerInit() }

// Cat adds the frames of packet. The packet's bytes must stay valid until Out is called.
func (r *Repacketizer) Cat(packet []byte) error {
	return errOf(r.rp.RepacketizerCat(packet, int32(len(packet))))
}

// Frames returns the number of frames collected.
func (r *Repacketizer) Frames() int { return r.rp.RepacketizerGetNbFrames() }

// Out writes a packet containing all collected frames into out.
func (r *Repacketizer) Out(out []byte) (int, error) {
	n := r.rp.RepacketizerOut(out, int32(len(out)))
	if n < 0 {
		return 0, Error(int(n))
	}
	return int(n), nil
}

// OutRange writes a packet containing frames [begin, end) into out.
func (r *Repacketizer) OutRange(begin, end int, out []byte) (int, error) {
	n := r.rp.RepacketizerOutRange(begin, end, out, int32(len(out)))
	if n < 0 {
		return 0, Error(int(n))
	}
	return int(n), nil
}

// PacketPad pads packet in place to newLen bytes (libopus 1.1+). packet
// must have capacity for newLen bytes; the returned slice has length newLen.
func PacketPad(packet []byte, newLen int) ([]byte, error) {
	if newLen > cap(packet) {
		return nil, ErrBadArg
	}
	p := packet[:newLen]
	if code := opus.PacketPad(p, int32(len(packet)), int32(newLen)); code != opus.OPUS_OK {
		return nil, Error(code)
	}
	return p, nil
}

// PacketUnpad removes all padding from packet in place and returns the
// shortened slice (libopus 1.1+).
func PacketUnpad(packet []byte) ([]byte, error) {
	n := opus.PacketUnpad(packet, int32(len(packet)))
	if n < 0 {
		return nil, Error(int(n))
	}
	return packet[:n], nil
}

// MultistreamPacketPad pads a multistream packet in place to newLen bytes
// (libopus 1.1+); see PacketPad.
func MultistreamPacketPad(packet []byte, newLen, streams int) ([]byte, error) {
	if newLen > cap(packet) {
		return nil, ErrBadArg
	}
	p := packet[:newLen]
	if code := opus.MultistreamPacketPad(p, int32(len(packet)), int32(newLen), streams); code != opus.OPUS_OK {
		return nil, Error(code)
	}
	return p, nil
}

// MultistreamPacketUnpad removes all padding from a multistream packet in
// place (libopus 1.1+); see PacketUnpad.
func MultistreamPacketUnpad(packet []byte, streams int) ([]byte, error) {
	n := opus.MultistreamPacketUnpad(packet, int32(len(packet)), streams)
	if n < 0 {
		return nil, Error(int(n))
	}
	return packet[:n], nil
}

// SoftClip applies soft clipping to bring float PCM into the [-1, 1] range
// (opus_pcm_soft_clip, libopus 1.1+). mem must hold one value per channel
// and carries state between calls; pass nil for stateless use.
func SoftClip(pcm []float32, channels int, mem []float32) {
	if channels <= 0 || len(pcm) < channels {
		return
	}
	if len(mem) < channels {
		mem = make([]float32, channels)
	}
	opus.PcmSoftClip(pcm, len(pcm)/channels, channels, mem)
}

// ---- Multistream ----------------------------------------------------------

// Layout describes a multistream channel layout: how the (up to 255)
// output channels map onto coupled (stereo) and uncoupled (mono) streams.
// See the Ogg Opus specification for the semantics of Mapping.
type Layout struct {
	Channels       int
	Streams        int
	CoupledStreams int
	Mapping        []byte // one entry per channel; 255 = silent channel
}

// SurroundLayout returns the standard (Vorbis-order) layout of mapping
// family 1 for 1 to 8 channels (family 0 for 1-2 channels).
func SurroundLayout(channels int) (Layout, error) {
	if channels < 1 || channels > 8 {
		return Layout{}, ErrBadArg
	}
	var streams, coupled int
	mapping := make([]byte, channels)
	family := 1
	if channels <= 2 {
		family = 0
	}
	var code int
	st := opus.MultistreamSurroundEncoderCreate(48000, channels, family, &streams, &coupled, mapping, opus.OPUS_APPLICATION_AUDIO, &code)
	if st == nil {
		return Layout{}, errOf(code)
	}
	return Layout{Channels: channels, Streams: streams, CoupledStreams: coupled, Mapping: mapping}, nil
}

// MultistreamEncoder encodes multi-channel audio as a set of Opus streams
// in one packet.
type MultistreamEncoder struct {
	st     *opus.OpusMSEncoder
	layout Layout
	rate   int
}

// NewMultistreamEncoder creates a multistream encoder with an explicit layout.
func NewMultistreamEncoder(sampleRate int, layout Layout, app Application) (*MultistreamEncoder, error) {
	if len(layout.Mapping) != layout.Channels {
		return nil, errors.New("opus: layout mapping length must equal channel count")
	}
	var code int
	st := opus.MultistreamEncoderCreate(int32(sampleRate), layout.Channels, layout.Streams, layout.CoupledStreams, layout.Mapping, int(app), &code)
	if st == nil {
		return nil, errOf(code)
	}
	l := layout
	l.Mapping = append([]byte(nil), layout.Mapping...)
	return &MultistreamEncoder{st: st, layout: l, rate: sampleRate}, nil
}

// NewSurroundEncoder creates a multistream encoder for a standard surround
// layout (mapping family 0 for 1-2 channels, 1 for 1-8, 255 for independent
// mono streams) and returns the layout it chose.
func NewSurroundEncoder(sampleRate, channels, mappingFamily int, app Application) (*MultistreamEncoder, error) {
	var streams, coupled, code int
	mapping := make([]byte, max(channels, 1))
	st := opus.MultistreamSurroundEncoderCreate(int32(sampleRate), channels, mappingFamily, &streams, &coupled, mapping, int(app), &code)
	if st == nil {
		return nil, errOf(code)
	}
	return &MultistreamEncoder{st: st, rate: sampleRate, layout: Layout{Channels: channels, Streams: streams, CoupledStreams: coupled, Mapping: mapping[:channels]}}, nil
}

// Layout returns the encoder's channel layout.
func (e *MultistreamEncoder) Layout() Layout { return e.layout }

// Encode encodes one frame of interleaved PCM (len(pcm)/Layout().Channels samples per channel).
func (e *MultistreamEncoder) Encode(pcm []int16, out []byte) (int, error) {
	ch := e.layout.Channels
	if len(pcm)%ch != 0 {
		return 0, fmt.Errorf("opus: pcm length %d is not a multiple of %d channels", len(pcm), ch)
	}
	n := e.st.MultistreamEncode(pcm, len(pcm)/ch, out, int32(len(out)))
	if n < 0 {
		return 0, Error(n)
	}
	return n, nil
}

// EncodeFloat is Encode for float32 input.
func (e *MultistreamEncoder) EncodeFloat(pcm []float32, out []byte) (int, error) {
	ch := e.layout.Channels
	if len(pcm)%ch != 0 {
		return 0, fmt.Errorf("opus: pcm length %d is not a multiple of %d channels", len(pcm), ch)
	}
	n := e.st.MultistreamEncodeFloat(pcm, len(pcm)/ch, out, int32(len(out)))
	if n < 0 {
		return 0, Error(n)
	}
	return n, nil
}

// Ctl is the raw opus_multistream_encoder_ctl escape hatch; SET requests
// apply to every stream.
func (e *MultistreamEncoder) Ctl(request int, args ...any) error {
	return errOf(e.st.Ctl(request, args...))
}

// SetBitrate sets the total bitrate (Auto or BitrateMax allowed).
func (e *MultistreamEncoder) SetBitrate(bps int) error {
	return e.Ctl(opus.OPUS_SET_BITRATE_REQUEST, int32(bps))
}

// SetComplexity sets the complexity of every stream.
func (e *MultistreamEncoder) SetComplexity(c int) error {
	return e.Ctl(opus.OPUS_SET_COMPLEXITY_REQUEST, int32(c))
}

// SetVBR sets VBR on every stream.
func (e *MultistreamEncoder) SetVBR(on bool) error { return e.Ctl(opus.OPUS_SET_VBR_REQUEST, b2i(on)) }

// Stream returns the wrapped elementary encoder of stream i, for per-stream settings.
func (e *MultistreamEncoder) Stream(i int) (*Encoder, error) {
	var st *opus.OpusEncoder
	if code := e.st.Ctl(opus.OPUS_MULTISTREAM_GET_ENCODER_STATE_REQUEST, int32(i), &st); code != 0 {
		return nil, Error(code)
	}
	ch := 1
	if i < e.layout.CoupledStreams {
		ch = 2
	}
	return &Encoder{st: st, rate: e.rate, channels: ch}, nil
}

// SetExpertFrameDuration sets the frame duration policy of every stream (libopus 1.1+).
func (e *MultistreamEncoder) SetExpertFrameDuration(d FrameDuration) error {
	return e.Ctl(opus.OPUS_SET_EXPERT_FRAME_DURATION_REQUEST, int32(d))
}

// SetPredictionDisabled disables inter-frame prediction in every stream (libopus 1.1+).
func (e *MultistreamEncoder) SetPredictionDisabled(off bool) error {
	return e.Ctl(opus.OPUS_SET_PREDICTION_DISABLED_REQUEST, b2i(off))
}

// SetPhaseInversionDisabled disables intensity-stereo phase inversion in every stream (libopus 1.2+).
func (e *MultistreamEncoder) SetPhaseInversionDisabled(off bool) error {
	return e.Ctl(opus.OPUS_SET_PHASE_INVERSION_DISABLED_REQUEST, b2i(off))
}

// FinalRange returns the XOR of the streams' final range states.
func (e *MultistreamEncoder) FinalRange() uint32 {
	var v uint32
	e.st.Ctl(opus.OPUS_GET_FINAL_RANGE_REQUEST, &v)
	return v
}

// MultistreamDecoder decodes multistream packets into multi-channel PCM.
type MultistreamDecoder struct {
	st     *opus.OpusMSDecoder
	layout Layout
	rate   int
}

// NewMultistreamDecoder creates a multistream decoder for the given layout.
func NewMultistreamDecoder(sampleRate int, layout Layout) (*MultistreamDecoder, error) {
	if len(layout.Mapping) != layout.Channels {
		return nil, errors.New("opus: layout mapping length must equal channel count")
	}
	var code int
	st := opus.MultistreamDecoderCreate(int32(sampleRate), layout.Channels, layout.Streams, layout.CoupledStreams, layout.Mapping, &code)
	if st == nil {
		return nil, errOf(code)
	}
	l := layout
	l.Mapping = append([]byte(nil), layout.Mapping...)
	return &MultistreamDecoder{st: st, layout: l, rate: sampleRate}, nil
}

// Layout returns the decoder's channel layout.
func (d *MultistreamDecoder) Layout() Layout { return d.layout }

// Decode decodes one multistream packet (nil for loss concealment) and
// returns the samples per channel written.
func (d *MultistreamDecoder) Decode(packet []byte, pcm []int16, fec bool) (int, error) {
	ch := d.layout.Channels
	if len(pcm)%ch != 0 {
		return 0, fmt.Errorf("opus: pcm length %d is not a multiple of %d channels", len(pcm), ch)
	}
	n := d.st.MultistreamDecode(packet, int32(len(packet)), pcm, len(pcm)/ch, int(b2i(fec)))
	if n < 0 {
		return 0, Error(n)
	}
	return n, nil
}

// DecodeFloat is Decode with float32 output.
func (d *MultistreamDecoder) DecodeFloat(packet []byte, pcm []float32, fec bool) (int, error) {
	ch := d.layout.Channels
	if len(pcm)%ch != 0 {
		return 0, fmt.Errorf("opus: pcm length %d is not a multiple of %d channels", len(pcm), ch)
	}
	n := d.st.MultistreamDecodeFloat(packet, int32(len(packet)), pcm, len(pcm)/ch, int(b2i(fec)))
	if n < 0 {
		return 0, Error(n)
	}
	return n, nil
}

// Ctl is the raw opus_multistream_decoder_ctl escape hatch.
func (d *MultistreamDecoder) Ctl(request int, args ...any) error {
	return errOf(d.st.Ctl(request, args...))
}

// SetGain sets the output gain (Q8 dB) of every stream.
func (d *MultistreamDecoder) SetGain(q8dB int) error {
	return d.Ctl(opus.OPUS_SET_GAIN_REQUEST, int32(q8dB))
}

// Gain returns the output gain (Q8 dB) (libopus 1.1+).
func (d *MultistreamDecoder) Gain() int {
	var v int32
	d.st.Ctl(opus.OPUS_GET_GAIN_REQUEST, &v)
	return int(v)
}

// LastPacketDuration returns the duration in samples of the last decoded
// packet (libopus 1.1+).
func (d *MultistreamDecoder) LastPacketDuration() int {
	var v int32
	d.st.Ctl(opus.OPUS_GET_LAST_PACKET_DURATION_REQUEST, &v)
	return int(v)
}

// Bandwidth returns the bandwidth of the last decoded packet (libopus 1.1+).
func (d *MultistreamDecoder) Bandwidth() Bandwidth {
	var v int32
	d.st.Ctl(opus.OPUS_GET_BANDWIDTH_REQUEST, &v)
	return Bandwidth(v)
}

// SetPhaseInversionDisabled disables phase inversion in every stream decoder (libopus 1.2+).
func (d *MultistreamDecoder) SetPhaseInversionDisabled(off bool) error {
	return d.Ctl(opus.OPUS_SET_PHASE_INVERSION_DISABLED_REQUEST, b2i(off))
}

// Reset resets every stream decoder.
func (d *MultistreamDecoder) Reset() error { return d.Ctl(opus.OPUS_RESET_STATE) }

// Stream returns the wrapped elementary decoder of stream i.
func (d *MultistreamDecoder) Stream(i int) (*Decoder, error) {
	var st *opus.OpusDecoder
	if code := d.st.Ctl(opus.OPUS_MULTISTREAM_GET_DECODER_STATE_REQUEST, int32(i), &st); code != 0 {
		return nil, Error(code)
	}
	ch := 1
	if i < d.layout.CoupledStreams {
		ch = 2
	}
	return &Decoder{st: st, rate: d.rate, channels: ch}, nil
}
