# Brief for moving the Go port from one upstream libopus release to the next

You are updating a batch of Go files so that they match the **next** upstream
libopus release. The Go code in this module (`github.com/DubbaThonyVibecodes/go-libopus`)
is a *mechanical, line-by-line* port of upstream (`third_party/opus`, a git
submodule). Read `PORTING.md` first and `tools/port/SILK_AGENT_BRIEF.md` for
the naming of the macro helpers (it is written for `silk/` but the same
conventions apply everywhere: C macro `MULT16_16_Q15` is Go `MULT16_16_Q15`,
`silk_SMULWB` is `silk.SMULWB`, …).

The task: for every C file in your list, apply the upstream change set
`git -C third_party/opus diff <OLD> <NEW> -- <file>` to the Go twin of that
file, so that afterwards the Go file is a faithful port of the `<NEW>`
version. The submodule is already checked out at `<NEW>`, so
`third_party/opus/...` *is* the new source; `git -C third_party/opus show
<OLD>:path` gives the old one. When a diff is large, it is usually quicker
to re-port the function from the new source than to patch the Go.

## The build configuration we port

The oracle the tests compare against is upstream built with
`--enable-fixed-point --disable-intrinsics --disable-asm --disable-rtcd`
on x86-64, so port exactly the code that this configuration compiles:

* `FIXED_POINT` **defined**; `DISABLE_FLOAT_API` **not defined** (float-API
  code that lives in `#ifndef DISABLE_FLOAT_API` *is* compiled and *is*
  ported, including `analysis.c`, the `float` paths of `opus_encode_float`
  and the parts of the CELT encoder that read `AnalysisInfo`).
* `CUSTOM_MODES` not defined, `OPUS_HAVE_RTCD` not defined, no
  `OPUS_X86_MAY_HAVE_*`, no `OPUS_ARM_*`, no `MIPSr1_ASM`, no `HAVE_ARM_NE10`.
  Skip every file under `*/x86/`, `*/arm/`, `*/mips/` and every
  `#if defined(OPUS_X86_...)` branch. `opus_select_arch()` returns 0.
* `OPUS_FAST_INT64` is 1 (64-bit host): use the `opus_int64` forms of the
  `MULT16_32_Q15`-style macros (they are bit-identical anyway).
* `ENABLE_ASSERTIONS` off, `FUZZING` off, `RESYNTH` off.

## `arch` parameters and `_c` dispatch

Upstream now threads an `int arch` argument through many functions and
dispatches `foo(..., arch)` macros to `foo_c(...)`. Port this as:

* A C function that gains an `int arch` parameter gains `arch int` in the
  same position in Go (and every Go call site passes it on). Keep it even
  though it is always 0: the point is that the next upstream diff still
  applies.
* `foo_c(...)` + `#define foo(..., arch) ((void)(arch), foo_c(...))` becomes
  **one** Go function named after `foo` (no `_c` suffix) with the macro's
  parameter list (so including `arch`), whose body is `foo_c`. Example:
  `celt_inner_prod(x, y, N, arch)` → `func InnerProd(x, y []int16, N int, arch int) int32`.
* `opus_select_arch()` is `func SelectArch() int { return 0 }` (celt
  package, `cpu_support.go`).

## Float code

Some of the code compiled in this configuration is genuine C `float` /
`double` arithmetic (`src/analysis.c`, `src/mlp.c`, a few blocks in
`celt_encoder.c`, `opus_encoder.c`, `opus_multistream_encoder.c`). The
oracle runs with SSE math and `-ffp-contract=off`, i.e. every operation is
rounded to its C type and nothing is fused. To get the same bits in Go:

* C `float` is Go `float32`, C `double` is Go `float64`. A C literal with an
  `f` suffix (`.5f`) is a float; a literal **without** suffix (`.5`,
  `1e-10`) is a *double* and promotes the whole subexpression it is in to
  double: `floor(.5+x)` with `float x` is `math.Floor(0.5 + float64(x))`;
  `x*.5f` is `x * 0.5` on a float32 `x`.
* Go may fuse `a*b + c` into one FMA on some targets (arm64, ppc64, s390x,
  amd64 with GOAMD64=v3). Prevent it by converting every product that feeds
  an addition/subtraction: write `float32(a*b) + c` (an explicit conversion
  forces rounding per the Go spec). Same for float64 (`float64(a*b) + c`).
* libm: `log` → `math.Log`, `exp` → `math.Exp`, `sqrt` → `math.Sqrt`, `pow`
  → `math.Pow`, `log10` → `math.Log10`, `floor` → `math.Floor`, `fabs` →
  `math.Abs`, all on float64 with explicit conversions as in C
  (`(float)log(E+1e-10f)` → `float32(math.Log(float64(E + 1e-10)))`, note
  the float32 addition happens first). `floorf`/`sqrtf` etc. are the
  float32 versions; do the math in float64 and convert the result.
* Casts: C `(int)f` truncates toward zero like Go `int(f)`; `(opus_val16)f`
  → `int16(f)`; `(kiss_fft_scalar)(w*x)` → `int32(w * x)` in the fixed
  build. Integer → float: `(float)i` → `float32(i)`.
* Keep the C evaluation order (left to right, parentheses as written).

## Conventions reminder

* One Go file per C file, same base name (`celt/celt_encoder.c` →
  `celt/celt_encoder.go`; header-only code goes in the `.go` named after
  the header). When upstream splits or renames a file, do the same in Go
  and move the functions.
* Names: drop `ec_`/`celt_`/`silk_`/`opus_` prefix, CamelCase;
  non-static → exported; static → unexported (lowerCamel or the C name).
  First parameter a state pointer → method. New struct fields keep their C
  names. New `#define` constants keep their C names.
* Every function has a `// C: <signature>` doc comment; update it when the
  C signature changes.
* Arrays → slices; pointer arithmetic → re-slicing; out-params → pointers;
  `ALLOC` → `make`; `OPUS_COPY/MOVE/CLEAR` → `copy`/`clear`; `celt_assert`
  → `celt_assert`. Drop `SAVE_STACK`/`RESTORE_STACK`, `VARDECL`, `ALLOC_STACK`.
* Do not "improve" anything, do not reorder, do not add tests, do not touch
  files outside your list unless the brief says so. Keep deleted C code
  deleted (do not keep both old and new versions).
* Finish with `gofmt -w` on your files and `go vet ./<pkg>`. Other agents are
  editing the *other* packages at the same time, so if `go build` fails in
  a package that is not yours, retry later and only fix your own files; if
  it still fails at the end, say so in your report.

Report back with: the files you changed, every function whose Go signature
changed (old → new), anything you were unsure about (C file + line), and
shared-file changes you think other packages need.
