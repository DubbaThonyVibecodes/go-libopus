# Brief for porting SILK files (libopus 1.0.3 → Go)

You are porting a batch of C files from `third_party/opus/silk/` (and
`third_party/opus/silk/fixed/`) into the Go package `silk/` of this module
(`github.com/DubbaThonyVibecodes/go-libopus`). Read `PORTING.md` first.
This is a *mechanical, line-by-line* port of the fixed-point build; the goal
is bit-exact behaviour, not pretty Go. Keep the C structure, loops, variable
names and comments. Every function gets a doc comment starting with the C
signature (`// C: void silk_foo(...)`).

## What already exists (do not modify these files)

* `silk/typedef.go`, `macros.go`, `SigProc_FIX.go`, `Inlines.go`: every C
  macro / inline helper with the **same upper-case name minus the `silk_`
  prefix**: `SMULWB`, `SMLAWB`, `SMULBB`, `SMLABB`, `SMULWW`, `SMLAWW`, `MLA`,
  `MUL`, `ADD32`, `SUB32`, `ADD16`, `ADD64`, `ADD_SAT32`, `SUB_SAT32`,
  `ADD_SAT16`, `ADD_POS_SAT32`, `SAT16`, `SAT32`, `LSHIFT`, `LSHIFT32`,
  `LSHIFT16`, `LSHIFT_SAT32`, `LSHIFT_ovflw`, `RSHIFT`, `RSHIFT32`, `RSHIFT64`,
  `RSHIFT_ROUND`, `RSHIFT_ROUND64`, `ADD_LSHIFT32`, `ADD_RSHIFT32`,
  `SUB_LSHIFT32`, `SUB_RSHIFT32`, `ADD_LSHIFT`, `ADD_RSHIFT`, `DIV32`,
  `DIV32_16`, `SMULL`, `SMMUL`, `SMLAL`, `SMULTT`, `SMLATT`, `SMULWT`,
  `SMLAWT`, `SMULBT`, `SMLABT`, `SMLALBB`, `CLZ16`, `CLZ32`, `CLZ64`,
  `CLZ_FRAC`, `ROR32`, `SQRT_APPROX`, `DIV32_varQ`, `INVERSE32_varQ`,
  `LIMIT`, `LIMIT_32`, `LIMIT_16`, `LIMIT_int`, `RAND`, `SILK_FIX_CONST`,
  `NSHIFT_MUL_32_32`, `NSHIFT_MUL_16_16`, `abs`, `abs_int32`, `sign`,
  `min_int`, `max_int`, `min_32`, `max_32`, `min_16`, `max_16`, `matrix_ptr`,
  `matrix_adr`, `matrix_c_ptr`, `silk_assert`, `b2i(bool) int`, `int16_MAX`,
  `int32_MAX`, `int32_MIN`, …  They are generic over integer types, so you
  can pass `int`, `int16`, `int32` operands directly; the *result* type is
  the C expression's type (`int32` for most, `int16` for ADD16/SUB16,
  `int64` for the 64-bit ones, same type as input for RSHIFT/RSHIFT_ROUND/
  SAT*/LIMIT). `silk_min(a,b)` / `silk_max(a,b)` are the Go builtins
  `min` / `max` (operands must have the same type).
* `define.go`, `errors.go`, `control.go`, `tuning_parameters.go`,
  `pitch_est_defines.go`, `resampler_rom.go`, `resampler_private.go`,
  `PLC_h.go`: all `#define` constants with their C names. Constants that are
  `float` literals in C (`1e-5f`) are typed `float32` so `SILK_FIX_CONST`
  reproduces single-precision rounding; pass them straight through.
* `structs.go`, `structs_FIX.go`: all state structs. Type names are the C
  names minus `silk_`, CamelCased: `EncoderState` (silk_encoder_state),
  `EncoderStateFIX`, `EncoderControlFIX`, `Encoder` (silk_encoder, super
  struct with `state_Fxx [2]EncoderStateFIX`), `DecoderState`, `Decoder`
  (silk_decoder, with `channel_state [2]DecoderState`), `DecoderControl`,
  `NsqState`, `VADState`, `LPState`, `NLSFCBStruct`, `StereoEncState`,
  `StereoDecState`, `SideInfoIndices`, `PLCStruct`, `CNGStruct`,
  `ResamplerStateStruct` (the C `sFIR` union is split into `sFIR_i32` and
  `sFIR_i16`), `EncControlStruct`, `DecControlStruct` (exported field names:
  `NChannelsAPI`, `API_sampleRate`, `PayloadSize_ms`, `BitRate`, …).
  Field names are the C names. Fixed-size C arrays are Go arrays: use
  `x.arr[:]` to pass them as slices, `x.arr[off:]` for `&x.arr[off]`.
  Pointer fields (`pitch_contour_iCDF`, `psNLSF_CB`, `Coefs`) are slices /
  pointers.
* Tables (`tables_*.go`, `table_LSF_cos.go`, `pitch_est_tables.go`,
  `resampler_rom_tables.go`): C name minus `silk_` prefix, **underscores
  kept**: `gain_iCDF`, `delta_gain_iCDF`, `pitch_lag_iCDF`,
  `pitch_contour_iCDF`, `pulses_per_block_iCDF`, `shell_code_table0`,
  `sign_iCDF`, `uniform4_iCDF`, `NLSF_EXT_iCDF`, `LTP_per_index_iCDF`,
  `LTP_gain_iCDF_ptrs` (`[3][]uint8`), `LTP_gain_BITS_Q5_ptrs`,
  `LTP_vq_ptrs_Q7` (`[3][]int8`, each a *flat* codebook as in C),
  `LTP_vq_sizes`, `LTPscale_iCDF`, `LTPScales_table_Q14`,
  `type_offset_VAD_iCDF`, `stereo_pred_quant_Q13`, `LBRR_flags_iCDF_ptr`,
  `NLSF_interpolation_factor_iCDF`, `NLSF_CB_WB`, `NLSF_CB_NB_MB`,
  `TargetRate_table_NB/MB/WB`, `SNR_table_Q1`, `Quantization_Offsets_Q10`,
  `Transition_LP_B_Q28`, `Transition_LP_A_Q28`, `LSFCosTab_FIX_Q12`,
  `CB_lags_stage2`, `CB_lags_stage3`, `Lag_range_stage3`,
  `nb_cbk_searchs_stage3`, `CB_lags_stage2_10_ms`, …, `Resampler_3_4_COEFS`,
  `Resampler_2_3_COEFS`, `Resampler_1_2_COEFS`, …, `resampler_frac_FIR_12`,
  `resampler_down2_0`, `resampler_down2_1`, `resampler_up2_hq_0/1`.
  These are Go arrays; slice them (`gain_iCDF[i][:]`) where C passes a
  pointer.
* The range coder is `celt.EcEnc` / `celt.EcDec` (same struct) from
  `github.com/DubbaThonyVibecodes/go-libopus/celt`: `ec_enc_icdf(psRangeEnc,
  s, icdf, 8)` → `psRangeEnc.EncIcdf(s, icdf, 8)`; `ec_dec_icdf` →
  `DecIcdf(icdf []uint8, ftb uint) int`; `ec_enc_bit_logp` → `EncBitLogp(val
  int, logp uint)`; `ec_dec_bit_logp(dec, logp)` → `DecBitLogp(logp uint)
  int`; `ec_tell` → `Tell() int`; `ec_enc_uint`/`ec_dec_uint` →
  `EncUint(fl, ft uint32)` / `DecUint(ft uint32) uint32`; `ec_enc_bits` /
  `ec_dec_bits` → `EncBits(fl uint32, bits uint)` / `DecBits(bits uint)
  uint32`; `ec_enc_patch_initial_bits` → `EncPatchInitialBits(val uint32,
  nbits uint)`; `ec_range_bytes` → `RangeBytes() uint32`; `ec_get_buffer` →
  `GetBuffer() []byte`.

## The stubs: your signatures are fixed

Every non-static SILK function already has a **stub** in
`silk/<cfile>.go` (one Go file per C file, same base name) with the agreed
Go signature and a `panic("TODO")` body. Other people are porting the
callers and callees in parallel against exactly these signatures, so:

* **Replace the stub body, keep the signature exactly** (receiver, parameter
  names, types, order, return type).
* Functions whose first C parameter is a state pointer are methods on that
  state (e.g. `func (psDec *DecoderState) DecodeFrame(...)`,
  `func (psEncC *EncoderState) NSQ(NSQ *NsqState, ...)`); the C call
  `silk_decode_frame(psDec, ...)` becomes `psDec.DecodeFrame(...)`.
* Non-method function names are the C name minus `silk_`, CamelCased without
  underscores (`silk_NLSF2A` → `NLSF2A`, `silk_LPC_inverse_pred_gain_Q24` →
  `LPCInversePredGainQ24`, `silk_k2a_Q16` → `K2aQ16`). Look at the stubs for
  the exact spelling before calling anything.
* `static` helpers inside a C file are yours to add in your Go file, as
  unexported functions named after the C name (`silk_A2NLSF_trans_poly` →
  `a2NLSFTransPoly` or `silk_A2NLSF_trans_poly` minus the prefix in
  lowerCamel). Keep them in the same file.
* C arrays / pointers are slices (`[]int16`); pointer arithmetic `x + n`
  is `x[n:]`; negative indexing (`x[-1]`) means the caller must pass a slice
  with headroom and an explicit offset — look at how the stub's callers in
  the C code use it and keep an index variable rather than re-slicing when
  the C code reads before the pointer.
* `opus_int` is `int`, `opus_int32` is `int32`, `opus_int16` is `int16`,
  `opus_int8` is `int8`, `opus_int64` is `int64`. Keep local variable types
  exactly as in C; where C silently truncates on assignment
  (`opus_int16 x = expr32;`) write the explicit conversion
  (`x = int16(expr32)`). Never widen arithmetic that C does in 32 bits.
* `silk_memcpy(dst, src, n*sizeof(T))` → `copy(dst[:n], src[:n])`;
  `silk_memset(x, 0, n*sizeof(T))` → `clear(x[:n])`; `silk_memmove` → `copy`
  (Go's copy handles overlap). Struct copies `*a = *b` work as in C.
* Stack arrays (`opus_int16 tmp[ MAX_FRAME_LENGTH ]`) become
  `var tmp [MAX_FRAME_LENGTH]int16` and are passed as `tmp[:]`.
* `#ifdef FIXED_POINT` branches: port the fixed-point branch only; drop the
  float branches. Drop `SAVE_STACK`, `RESTORE_STACK`, `silk_assert`
  can stay as `silk_assert(cond)`. Drop DEBUG_STORE_DATA / MACRO_COUNT.
* Boolean results used as ints: `b2i(cond)`.
* Do not "improve" anything. Do not add tests. Do not touch files outside
  your list (if you believe a shared file is wrong, say so in your report).

## Verifying

Run `gofmt -w silk/<yourfiles>.go` and `go build ./silk` from the module
root. Other people are editing other files in the package at the same
time, so a build error in a file that is not yours is not your problem:
retry a few seconds later and only fix errors in your files. Finish when
`go vet ./silk` reports nothing for your files.

Report back with: the list of files ported, anything you were unsure about
(with the C line numbers), and any shared-file change you think is needed.
