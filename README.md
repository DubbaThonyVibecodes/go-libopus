# go-libopus

A pure Go (`CGO_ENABLED=0`) port of [libopus](https://opus-codec.org/), the
reference implementation of the Opus audio codec (RFC 6716).

The port is deliberately *mechanical*: files, functions and control flow
follow upstream C one to one, so that every upstream release can be tracked
as a diff. On top of that low-level port sits an idiomatic wrapper package.

## Versioning

Module tags are `vX.Y.Z` where `X.Y` is the upstream libopus minor version
the port tracks: `v1.0.z` is bit-compatible with libopus 1.0.N, `v1.6.z`
with libopus 1.6.N, and so on. `master` tracks the newest upstream release;
each supported minor has its own branch.

## Layout

| Path | Mirrors upstream | Purpose |
|------|------------------|---------|
| `celt/` | `celt/` | CELT layer (MDCT codec, range coder, PLC) |
| `silk/` | `silk/`, `silk/fixed/` | SILK layer (LPC speech codec) |
| `opus/` | `src/`, `include/` | Opus framing, mode switching, multistream, repacketizer, C-style API |
| `libopus` (root) | – | Idiomatic wrapper |
| `third_party/opus` | – | Upstream source, git submodule pinned to the ported release |
| `tools/corpus` | – | Conformance-corpus generator (C oracle driver + Go orchestrator) |
| `tools/bench` | – | Speed comparison with upstream C (`BENCHMARKS.md`) |
| `docs/` | – | Usage guide |
| `testdata/` | – | Committed conformance corpus, clips, RFC 6716 vectors |

## Status

Tracks libopus **1.5.2** on this branch (fixed-point build); each ported
minor keeps a `release/X.Y` branch and `vX.Y.z` tags. Encoder and decoder
(SILK, CELT, hybrid, mode switching with redundancy, PLC, in-band FEC, DTX,
multistream/surround, ambisonics (mapping families 2 and 3 with the
projection encoder/decoder), repacketizer, packet padding, packet
helpers, the RNN tonality analysis, 80/100/120 ms frames, phase-inversion
control, RFC 8251 fixes, FEC mode 2, packet extensions, 4th/5th-order
ambisonics) are bit-exact against the upstream build on all
cases of the committed corpus and on the extended 30-second corpus, on
both 64-bit and 32-bit (`GOARCH=386`) builds. The module has no
dependencies outside the standard library and builds with `CGO_ENABLED=0`.

| Branch / tag | libopus | Corpus cases |
|--------------|---------|--------------|
| `release/1.5`, `v1.5.1` | 1.5.2 (default build: no DRED/deep PLC/OSCE, see PORTING.md) | 1050 |
| `release/1.4`, `v1.4.1` | 1.4 | 1040 |
| `release/1.3`, `v1.3.1` | 1.3.1 | 1023 |
| `release/1.2`, `v1.2.1` | 1.2.1 | 968 |
| `release/1.1`, `v1.1.1` | 1.1.5 | 899 |
| `release/1.0`, `v1.0.1` | 1.0.3 | 790 |

## Getting started

```sh
go get github.com/DubbaThonyVibecodes/go-libopus@v1.5
```

```go
import libopus "github.com/DubbaThonyVibecodes/go-libopus"

enc, err := libopus.NewEncoder(48000, 2, libopus.Audio)
enc.SetBitrate(96000)
packet := make([]byte, libopus.MaxPacket)
n, err := enc.Encode(pcm, packet)              // pcm: 20 ms = 960×2 interleaved int16

dec, err := libopus.NewDecoder(48000, 2)
out := make([]int16, 5760*2)                   // room for the longest packet (120 ms)
m, err := dec.Decode(packet[:n], out, false)   // m samples per channel
lost, err := dec.Decode(nil, out[:960*2], false) // conceal a lost 20 ms packet
```

## Documentation

* **API reference:** `go doc github.com/DubbaThonyVibecodes/go-libopus` or
  [pkg.go.dev](https://pkg.go.dev/github.com/DubbaThonyVibecodes/go-libopus@v1.5),
  with runnable examples (encode/decode, packet loss and FEC, float input,
  surround, repacketizing, ambisonics).
  The reference follows the module version, so each `v1.Y` documents
  exactly what libopus 1.Y offers.
* **Guide:** [docs/guide.md](docs/guide.md) explains the concepts (sample
  rates, frames, applications, bitrates), encoding and decoding, packet loss
  handling, multichannel, packet manipulation, the C-to-Go mapping of the
  low-level `opus` package, and what is out of scope (containers such as Ogg).
* **Porting conventions:** [PORTING.md](PORTING.md).

## Performance

[BENCHMARKS.md](BENCHMARKS.md) compares this port with upstream C built from
the same release, on identical workloads with identical output: the port
runs at roughly 0.4–0.7× the speed of the equivalent C build (no SIMD),
and still encodes stereo music tens of times faster than real time on one
core. Reproduce with `make -C tools/bench && go run ./tools/bench`.

## Testing

Correctness is defined as bit-exactness against the upstream *fixed-point*
build at the same version. `tools/corpus/cgen/gen.c` links against an
out-of-tree build of the submodule (never the host's libopus) and records
packets, per-frame range-coder state and per-frame output CRCs for several
hundred encoder/decoder configurations; the Go tests replay the same cases
and diff the traces, which pinpoints the first diverging frame.

```sh
make -C tools/corpus gen       # build upstream + oracle driver (needs autotools, gcc)
go run ./tools/corpus          # regenerate the committed hermetic corpus
go run ./tools/corpus -extended  # bigger local corpus with full PCM (git-ignored)
go test ./...
```

See `PORTING.md` for the porting conventions.

## Licence

BSD-3-Clause, same as upstream; see `LICENSE`. Opus is additionally covered
by royalty-free patent licences from Xiph.Org, Broadcom and Microsoft, see
https://opus-codec.org/license/.
