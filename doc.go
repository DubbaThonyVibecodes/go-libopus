// Package libopus is a pure-Go (CGO_ENABLED=0) implementation of the Opus
// audio codec, ported line by line from the reference implementation,
// libopus 1.1.5, and verified to produce bit-identical output to it.
//
// This package is the convenient, idiomatic API. Everything it does is
// built on the low-level packages, which you can also use directly:
//
//   - [github.com/DubbaThonyVibecodes/go-libopus/opus]: the C API of
//     libopus one to one (opus_encode → (*OpusEncoder).Encode, the ctl
//     requests, negative error codes, C naming).
//   - [github.com/DubbaThonyVibecodes/go-libopus/celt] and
//     [github.com/DubbaThonyVibecodes/go-libopus/silk]: the two codec
//     layers. Most users never need them.
//
// # Versions
//
// The module's version follows libopus: v1.1.z of this module is
// bit-compatible with libopus 1.1.N, so pick the module version that
// matches the libopus you need to interoperate or compare with. Opus is a
// fixed bitstream format (RFC 6716): every version decodes every other
// version's packets, newer releases mainly improve encoder quality and add
// features.
//
// # Encoding
//
// Create an [Encoder] for a sample rate (8000, 12000, 16000, 24000 or 48000
// Hz), one or two channels and an [Application], then pass it one frame of
// interleaved samples at a time. The frame duration is inferred from the
// slice length and must be 2.5, 5, 10, 20, 40 or 60 ms
// — 20 ms is the usual choice. Each call returns one packet.
//
//	enc, err := libopus.NewEncoder(48000, 2, libopus.Audio)
//	enc.SetBitrate(96000)
//	packet := make([]byte, libopus.MaxPacket)
//	n, err := enc.Encode(pcm, packet) // pcm: 960 samples × 2 channels
//	send(packet[:n])
//
// Encoder settings mirror the libopus CTLs: [Encoder.SetBitrate],
// [Encoder.SetComplexity], [Encoder.SetVBR], [Encoder.SetBandwidth],
// [Encoder.SetSignal], [Encoder.SetInbandFEC], [Encoder.SetDTX],
// [Encoder.SetPacketLossPerc] and more. Anything without a typed method is
// reachable through [Encoder.Ctl] with the request constants of the opus
// package.
//
// # Decoding
//
// A [Decoder] needs only the output sample rate and channel count; it can
// decode packets produced at any rate. Give it room for the longest
// possible packet, 120 ms (5760 samples per channel at 48 kHz):
//
//	dec, err := libopus.NewDecoder(48000, 2)
//	pcm := make([]int16, 5760*2)
//	n, err := dec.Decode(packet, pcm, false) // n samples per channel
//
// To conceal a lost packet, call Decode with a nil packet and a pcm slice
// sized to the missing duration ([Decoder.LastPacketDuration] is the usual
// choice). If the encoder used in-band FEC, the packet after a loss also
// carries a lower-quality copy of the lost one: decode it first with
// fec=true into a slice sized to the lost duration, then decode it again
// normally. See the Decoder examples.
//
// [Decoder.DecodeFloat] and [Encoder.EncodeFloat] use float32 samples in
// [-1, 1] instead of int16.
//
// # Multichannel
//
// [MultistreamEncoder] and [MultistreamDecoder] code more than two channels
// as several Opus streams in one packet. [NewSurroundEncoder] picks the
// standard layouts (channel mapping family 1: Vorbis order, up to 7.1)
// and [SurroundLayout] returns the layout a decoder needs.
//
// # Packets
//
// Helpers such as [PacketSamples], [PacketBandwidth], [PacketParse] and
// [Repacketizer] inspect, split and merge packets without decoding them.
// [PacketPad] and [PacketUnpad] add and remove padding.
//
// This module implements the codec only. Opus files (.opus) and most
// streaming formats wrap packets in a container such as Ogg, Matroska or
// RTP, which is out of scope here.
//
// # Errors
//
// Functions return the libopus errors as values of type [Error], with
// sentinels such as [ErrBadArg] and [ErrBufferTooSmall] for errors.Is.
//
// A longer guide, including the libopus-to-Go mapping table, is docs/guide.md
// in the source tree; performance compared to C is in BENCHMARKS.md.
package libopus
