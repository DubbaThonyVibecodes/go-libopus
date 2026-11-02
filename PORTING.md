# Porting conventions

The Go code is a *mechanical* translation of upstream libopus. The goal is
that a reader with the C file open next to the Go file can follow line by
line, and that an upstream diff can be applied to the port by hand.

## Files and packages

| Upstream | Go |
|----------|----|
| `celt/foo.c` + `celt/foo.h` | `celt/foo.go` |
| `silk/foo.c`, `silk/fixed/foo_FIX.c` | `silk/foo.go`, `silk/foo_FIX.go` (one package; Go cannot have the cyclic imports a sub-package would need) |
| `src/foo.c`, `include/foo.h` | `opus/foo.go` |
| `celt/arch.h`, `fixed_generic.h`, … | `celt/arch.go`, `celt/fixed_generic.go`, … |
| header-only tables (`static_modes_fixed.h`) | same base name `.go` |

Only the fixed-point build is ported for now. File names keep the upstream
`_FIX` / `fixed_` markers so the float variant can be added behind build
tags later.

## Names

* C non-`static` → Go exported, C `static` → Go unexported.
* The module prefix (`ec_`, `celt_`, `silk_`, `opus_`) is dropped and the
  rest is CamelCased: `silk_NLSF2A` → `NLSF2A`, `celt_decode_with_ec` →
  `DecodeWithEc`, `ec_enc_bit_logp` → `EncBitLogp`.
* If the first parameter is a pointer to a state struct that the function
  operates on, the function becomes a method on that struct:
  `ec_decode(ec_dec *this, unsigned ft)` → `func (d *EcDec) Decode(ft uint32)`.
* Macros that compute values (`MULT16_16_Q15`, `SHL32`, `IMIN`, `EC_ILOG`,
  `silk_SMULWB`, …) become functions with the *same upper-case name*, so the
  arithmetic reads exactly like the C. They are generic over integer types
  where the C macro is untyped; the result type is the C expression's type.
* `#define` constants keep their names (`EC_CODE_TOP`, `OPUS_BAD_ARG`).
* Struct fields keep their C names (`st.mode`, `psDec.nFramesDecoded`).
* Every ported function carries a doc comment that starts with the C
  signature, e.g. `// C: unsigned ec_decode(ec_dec *_this, unsigned _ft)`.
  Upstream comments that explain the algorithm are kept.

## Types

| C | Go |
|---|----|
| `int`, `unsigned` (counts, indices, flags) | `int`, `uint32` |
| `opus_int16/32`, `opus_uint16/32` | `int16/32`, `uint16/32` |
| `opus_val16`, `opus_val32`, `celt_sig`, `celt_norm`, `celt_ener` (fixed) | `int16`, `int32`, `int32`, `int16`, `int32` |
| `opus_int64` | `int64` |
| `T *` array parameter / pointer arithmetic | `[]T` / re-slicing `x[i:]` |
| `T *` scalar out-parameter | `*T`, or an extra return value when the C returns void |
| `T x[N]` inside a struct | `[N]T` |
| `ALLOC(x, n, T)` | `make([]T, n)` |
| `OPUS_COPY/OPUS_MOVE/OPUS_CLEAR` | `copy` / `clear` |
| `celt_assert` | `celt_assert` (compiled out unless `enableAssertions`) |
| `opus_encoder_ctl(st, OPUS_SET_X(v))` | `st.Ctl(OPUS_SET_X_REQUEST, v)` in the low-level package; typed setters live in the wrapper |

C `int` maps to Go `int` (64-bit) because upstream is free of signed
overflow (it is tested with UBSan); all intentional wrap-around arithmetic
is done on explicitly sized unsigned types in both. Assignments that
truncate in C (`opus_int16 x = expr32;`) become explicit `int16(expr)`
conversions in Go, which have identical semantics.

## Build configuration

The port follows upstream built with `--enable-fixed-point
--disable-intrinsics --disable-asm --disable-rtcd` on a 64-bit host, i.e.
`FIXED_POINT`, `OPUS_FAST_INT64`, no `CUSTOM_MODES`, no SIMD/asm
(`opus_select_arch()` is 0, and every `int arch` parameter upstream added in
1.1 is kept as `arch int` so that later diffs still apply) and **with** the
float API, so the float code that the fixed-point build compiles
(`src/analysis.c`, `mlp.c`, the `AnalysisInfo` consumers in
`celt_encoder.c`, `opus_pcm_soft_clip`, …) is ported too.

* `foo_c()` plus a `foo(..., arch)` dispatch macro become one Go function
  named after `foo` with the macro's parameter list.
* Float code uses `float32`/`float64` exactly as C uses `float`/`double`
  (an unsuffixed C literal promotes to double), every product that feeds an
  addition is wrapped in an explicit conversion (`float32(a*b) + c`) so that
  Go cannot fuse it into an FMA on arm64/ppc64/s390x, and libm calls go
  through `math` on float64. `log`/`pow`/`exp` are not correctly rounded in
  either libm or Go's `math`, so a last-ulp difference is possible in
  theory; the conformance corpus has not shown one.

## Not ported: the DNN features of 1.5+

libopus 1.5 added neural extensions (DRED, deep PLC, OSCE, the loss
generator) under `dnn/`, all of which are opt-in at configure time
(`--enable-dred`, `--enable-deep-plc`, `--enable-osce`) and default to off.
The port follows the default build: the `dnn/` tree is not ported, the
`#ifdef ENABLE_DRED` / `ENABLE_DEEP_PLC` / `ENABLE_OSCE` blocks are skipped,
and the public DRED entry points behave as upstream's stubs do in that
configuration (unimplemented / no-op). The bitstream and the output of the
default build are unaffected by these features.

## What is *not* mechanical

* Memory: no `get_size`/`init` split is needed in Go; `opus_encoder_create`
  allocates with `new`. The `*_get_size` functions are kept for API parity.
* Variadic `ctl` becomes `Ctl(request int, args ...any) int` with the same
  request constants and the same big switch.
* Error codes stay negative ints in the low-level packages; the wrapper turns
  them into `error` values.

## Known deviations from a literal translation

* `silk.StereoLRToMS` / `silk.StereoMSToLR`: the C code indexes two samples
  before the `x1`/`x2` pointers. The Go slices start two samples *before*
  the frame (callers pass `inputBuf[:]`, not `inputBuf[2:]`); see the doc
  comments.
* `silk.LTPAnalysisFilterFIX` takes an explicit `x_idx` because the C reads
  up to `max(pitchL)+2` samples behind `x`.
* `celt.comb_filter` / `celt.comb_filter_const` take explicit `(slice, offset)`
  pairs for `x` and `y` because the C reads up to `T+2` samples *behind* the
  input pointer.
* `celt.Fir` (1.2+) reads `ord` samples behind its input, so the Go input
  slice starts `ord` samples *before* the C pointer, like `comb_filter`.
* `silk.FindLTPFIX` / `silk.FindPredCoefsFIX` (1.2+) take an explicit index
  of the C pointer inside the base buffer because the C reads behind it.
* The oracle twin (`internal/oracle`) mirrors two quirks of the C driver:
  the `__opus_check_int` macro evaluates its argument twice (an extra LCG
  draw in `--random-fec`), and `gen info` prints the previous packet's TOC
  when parsing fails.
