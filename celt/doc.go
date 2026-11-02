// Package celt is a line-by-line Go port of libopus' celt/ directory: the
// CELT codec (MDCT transform coding used for music and low-delay audio),
// the range coder shared with SILK, and the fixed-point math helpers.
//
// It is an internal layer of the Opus codec, exported because the opus and
// silk packages build on it; applications should use the root package
// github.com/DubbaThonyVibecodes/go-libopus. Function and macro names follow
// the C source (MULT16_16_Q15, (*EcEnc).EncBits for ec_enc_bits, …); see
// PORTING.md in the module root.
package celt
