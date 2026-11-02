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
| `testdata/` | – | Committed conformance corpus, clips, RFC 6716 vectors |

## Status

Tracks libopus **1.0.3**. The decoder (SILK, CELT, hybrid, PLC, FEC,
multistream, repacketizer) is bit-exact against the upstream fixed-point
build on every corpus case; the encoder is being verified.

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
