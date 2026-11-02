// Package opus is a line-by-line Go port of libopus' Opus layer (src/ and
// include/ upstream): the encoder and decoder that choose between SILK and
// CELT, multistream and projection coding, the repacketizer and the packet
// helpers.
//
// It mirrors the C API one to one, for code ported from C and for anything
// the friendlier root package does not wrap:
//
//	C                                         Go
//	opus_encoder_create(fs, ch, app, &err)    opus.EncoderCreate(fs, ch, app, &err)
//	opus_encode(st, pcm, n, data, max)        st.Encode(pcm, n, data, max)
//	opus_encoder_ctl(st, OPUS_SET_BITRATE(x)) st.Ctl(opus.OPUS_SET_BITRATE_REQUEST, int32(x))
//	opus_encoder_ctl(st, OPUS_GET_BITRATE(&x)) st.Ctl(opus.OPUS_GET_BITRATE_REQUEST, &x) // var x int32
//	opus_decode(st, data, len, pcm, n, fec)   st.Decode(data, len, pcm, n, fec)
//
// Naming is mechanical: the opus_ prefix is dropped, the rest is
// CamelCased, and functions whose first argument is the state become
// methods. Errors are the negative libopus codes (OPUS_BAD_ARG, …);
// Strerror describes them. Ctl takes the same request constants as C, with
// int32 values and *int32 (or *uint32 for final range) outputs. The
// *_destroy functions exist for parity and do nothing.
//
// Most programs should use the root package
// github.com/DubbaThonyVibecodes/go-libopus, which wraps all of this in Go
// types. PORTING.md in the module root documents the translation rules.
package opus
