# go-libopus guide (libopus 1.4)

This guide covers this module's release line v1.4.z, a pure-Go,
bit-exact port of libopus 1.4. API reference: `go doc` or
[pkg.go.dev](https://pkg.go.dev/github.com/DubbaThonyVibecodes/go-libopus@v1.4.1).

- [Installing and choosing a version](#installing-and-choosing-a-version)
- [Opus in five minutes](#opus-in-five-minutes)
- [Encoding](#encoding)
- [Decoding and packet loss](#decoding-and-packet-loss)
- [More than two channels](#more-than-two-channels)
- [Working with packets](#working-with-packets)
- [The low-level API](#the-low-level-api)
- [What is not included](#what-is-not-included)

## Installing and choosing a version

```
go get github.com/DubbaThonyVibecodes/go-libopus@v1.4
```

The module needs no C compiler and builds with `CGO_ENABLED=0` on every
platform Go supports. Version `vX.Y.z` of the module is bit-exact with
libopus `X.Y.N`: same packets from the encoder, same samples from the
decoder. Pick the version that matches the libopus you interoperate or
compare with; otherwise use the newest. All versions read each other's
packets, because the Opus bitstream (RFC 6716) is fixed.

Features by version (each release includes everything above it):

| Module | libopus | Adds |
|---|---|---|
| v1.0 | 1.0.3 | Encoder and decoder (SILK, CELT, hybrid), FEC, DTX, PLC, multistream and surround, repacketizer |
| v1.1 | 1.1.5 | Encoder music/speech analysis, expert frame duration, prediction disabling, packet padding, soft clipping |
| v1.2 | 1.2.1 | 80/100/120 ms frames, phase-inversion control, better low-bitrate encoding |
| v1.3 | 1.3.1 | Ambisonics (mapping families 2 and 3, projection encoder/decoder), in-DTX query, RFC 8251 fixes |
| v1.4 | 1.4 | FEC mode 2 (FEC without forcing SILK for music), finer DTX |

## Opus in five minutes

* **Sample rates.** The encoder accepts 8, 12, 16, 24 or 48 kHz input, and
  the decoder produces any of those, whatever rate the stream was encoded
  at. Resample other rates (44.1 kHz) yourself.
* **Frames.** Opus codes audio in frames of 2.5, 5, 10, 20, 40 or 60 ms
  (80, 100 and 120 ms too, as multi-frame packets)
  — 20 ms is the default choice. At 48 kHz, 20 ms is 960 samples per
  channel. Samples are always interleaved (`L R L R …`).
* **Modes.** Internally Opus switches between SILK (speech, up to wideband),
  CELT (music, low delay) and a hybrid of both. You steer it with the
  application, bitrate and signal type; you never pick a mode directly
  (except with the testing-only `SetForceMode`).
* **Applications.** `VoIP` favours speech intelligibility, `Audio` favours
  fidelity for music and mixed content, `RestrictedLowDelay` disables the
  speech modes to save 2.5–5 ms of delay.
* **Bitrate.** 6–510 kb/s. Rules of thumb: 16–24 kb/s narrow/wideband
  speech, 32 kb/s fullband speech, 64–128 kb/s stereo music. VBR is the
  default; `SetVBR(false)` gives CBR, `SetVBRConstraint(true)` bounded VBR.

## Encoding

```go
enc, err := libopus.NewEncoder(48000, 2, libopus.Audio)
if err != nil { … }
enc.SetBitrate(96000)
enc.SetComplexity(10) // 0..10, 10 is the default and best quality

packet := make([]byte, libopus.MaxPacket)
for frame := range frames { // each frame: 960 × 2 interleaved int16
	n, err := enc.Encode(frame, packet)
	if err != nil { … }
	send(packet[:n])
}
```

* The packet buffer bounds the packet size; `MaxPacket` (1276 bytes) fits
  any single frame.
  Frames longer than 60 ms produce up to 3 × 1275 + 7 bytes.
* `EncodeFloat` takes float32 samples in [-1, 1].
* `Lookahead()` is the encoder delay in samples. Containers such as Ogg
  store it as "pre-skip" so the decoder can drop that many samples at the
  start.
* `SetSignal(SignalVoice|SignalMusic)` hints the content type;
  `SetBandwidth`/`SetMaxBandwidth` cap the audio bandwidth;
  `SetForceChannels(1)` encodes stereo input as mono.
* **Packet loss resilience:** `SetInbandFEC(true)` with
  `SetPacketLossPerc(p)` makes SILK frames carry a low-rate copy of the
  previous frame.
  `SetInbandFECMode(2)` enables FEC without pushing music into SILK.
  `SetPacketLossPerc` alone also makes the encoder rely less on
  inter-frame prediction.
  `SetPredictionDisabled(true)` goes further, so any packet decodes on its own.
* **Silence:** with `SetDTX(true)` the encoder sends only a tiny packet
  every 400 ms during silence; the decoder conceals the gaps with comfort
  noise.
  `InDTX()` reports whether the encoder is currently in such a period.
* **Frame duration:** `SetExpertFrameDuration` makes the encoder use a fixed
  frame duration regardless of the input length; `Encode` then consumes
  only that many samples (`PacketSamples` of the result), and the caller
  feeds the rest again.
* **Stereo downmix:** `SetPhaseInversionDisabled(true)` avoids phase
  inversion in intensity stereo, so a mono downmix of the decoded signal
  sounds better (on the encoder or on the decoder).
* Everything else in `opus_encoder_ctl` is available through `Ctl`, with
  the request constants from the opus package:

  ```go
  var la int32
  enc.Ctl(opus.OPUS_GET_LOOKAHEAD_REQUEST, &la)
  enc.Ctl(opus.OPUS_SET_BITRATE_REQUEST, int32(64000))
  ```

`Encoder` and `Decoder` hold per-stream state and are not safe for
concurrent use; use one per stream (they are cheap, a few tens of kB).

## Decoding and packet loss

```go
dec, err := libopus.NewDecoder(48000, 2)
pcm := make([]int16, 5760*2) // 120 ms at 48 kHz: enough for any packet
n, err := dec.Decode(packet, pcm, false) // n samples per channel
play(pcm[:n*2])
```

* A decoder can run at a different rate or channel count than the encoder;
  it resamples and up/down-mixes internally.
* **Loss concealment:** for a missing packet call
  `dec.Decode(nil, pcm[:dur*channels], false)`, where `dur` is the missing
  duration in samples per channel (a multiple of 2.5 ms, usually
  `dec.LastPacketDuration()`).
* **FEC:** if packet *k* is lost but *k+1* has arrived, decode *k+1* twice:
  first `Decode(p[k+1], pcm[:dur*channels], true)` recovers *k* from the
  FEC data (or conceals it if there is none), then
  `Decode(p[k+1], pcm, false)` decodes *k+1* itself. This costs one packet
  of latency, so only do it when your jitter buffer already holds *k+1*.
* `DecodeFloat` writes float32 in [-1, 1].
  `SetGain` applies an output gain in Q8 dB (256 = 1 dB).
  `SoftClip` keeps float output within [-1, 1] without hard clipping.
* `FinalRange()` on encoder and decoder must match for the same packet;
  conformance tools use it to check a decoder.

## More than two channels

Multichannel Opus packs several mono or stereo streams into one packet.
The *layout* (stream counts plus the channel mapping) is not in the packet;
containers carry it in their header.

```go
enc, _ := libopus.NewSurroundEncoder(48000, 6, 1, libopus.Audio) // 5.1, Vorbis order
layout := enc.Layout()   // store this in your container header
dec, _ := libopus.NewMultistreamDecoder(48000, layout)
```

* Mapping family 0 is mono/stereo, family 1 the Vorbis surround orders up to
  7.1 (`SurroundLayout(channels)` gives the standard layout), and
  family 2 ambisonics (`AmbisonicsLayout(channels)`, channels =
  (order+1)² optionally plus 2 non-diegetic channels),
  family 255 any explicit layout via `NewMultistreamEncoder`.
* `Stream(i)` gives access to the individual stream encoders/decoders.
* `NewProjectionEncoder` (mapping family 3) codes ambisonics through a
  mixing matrix, which usually sounds better than family 2. The decoder
  needs `DemixingMatrix()` from the encoder side, so store it in the
  container header too.

## Working with packets

Every packet starts with a TOC byte giving mode, bandwidth, frame duration,
stereo flag and frame count. Inspect it without decoding:
`PacketBandwidth`, `PacketChannels`, `PacketFrames`,
`PacketSamplesPerFrame`, `PacketSamples` and `PacketParse` (which also
splits a packet into its frames).

The `Repacketizer` merges consecutive packets of the same configuration into
one (up to 120 ms), or splits a multi-frame packet with `OutRange`. Packets
passed to `Cat` must stay valid until `Out`.

`PacketPad` and `PacketUnpad` add and strip padding in place, e.g. to send
fixed-size packets; `MultistreamPacketPad`/`Unpad` do the same for
multistream packets.

## The low-level API

The `opus` package is libopus' C API, one to one, for code ported from C or
for features without a wrapper method. Errors are negative return codes and
settings go through `Ctl` with the request constants.

| C | Go (`opus` package) |
|---|---|
| `opus_encoder_create(fs, ch, app, &err)` | `opus.EncoderCreate(fs, ch, app, &err)` |
| `opus_encode(st, pcm, frame, data, max)` | `st.Encode(pcm, frame, data, max)` |
| `opus_encode_float(…)` | `st.EncodeFloat(…)` |
| `opus_encoder_ctl(st, OPUS_SET_BITRATE(x))` | `st.Ctl(opus.OPUS_SET_BITRATE_REQUEST, int32(x))` |
| `opus_encoder_ctl(st, OPUS_GET_BITRATE(&x))` | `st.Ctl(opus.OPUS_GET_BITRATE_REQUEST, &x)` with `var x int32` |
| `opus_decoder_create(fs, ch, &err)` | `opus.DecoderCreate(fs, ch, &err)` |
| `opus_decode(st, data, len, pcm, frame, fec)` | `st.Decode(data, len, pcm, frame, fec)` |
| `opus_packet_get_nb_samples(p, len, fs)` | `opus.PacketGetNbSamples(p, len, fs)` |
| `opus_repacketizer_create()` | `opus.RepacketizerCreate()` |
| `opus_multistream_surround_encoder_create(…)` | `opus.MultistreamSurroundEncoderCreate(…)` |
| `opus_strerror(err)` | `opus.Strerror(err)` |

The naming rule is mechanical: drop the `opus_`/`celt_`/`silk_` prefix,
CamelCase the rest, and turn functions whose first argument is the state
into methods on it. `*_destroy` functions exist for parity but do nothing;
the garbage collector frees the state. The wrapper's `Raw()` methods return
these low-level states. The `celt` and `silk` packages are the internal
codec layers, exported for completeness; `PORTING.md` documents the
translation conventions.

## What is not included

* **Containers.** `.opus` files are Ogg streams; WebM/Matroska and RTP also
  carry Opus. Parsing or writing those is out of scope; this module codes
  packets.
* **The float build.** This is a port of libopus' fixed-point
  configuration, so its output is bit-identical to a fixed-point libopus. A
  float libopus (the usual distribution build) produces slightly different
  but equally valid packets and samples.

Performance compared to C is measured in [BENCHMARKS.md](../BENCHMARKS.md).
