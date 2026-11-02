# Benchmarks

How fast is the pure-Go port compared to upstream libopus? Every scenario
below runs the same workload through this module and through upstream C
built from the same release, and checks that both produced the same bytes.

The C columns are three builds of the same upstream source:

* **C fixed, no SIMD**: `--enable-fixed-point --disable-intrinsics --disable-asm --disable-rtcd`.
  This is exactly the code this module ports, so it is the like-for-like
  comparison. It produces bit-identical output to the Go port.
* **C fixed, SIMD**: the same with upstream's default SSE/AVX2 paths and
  run-time CPU detection. Same bitstream as the port, faster.
* **C float, SIMD**: upstream's default configuration, which is what a
  distribution's libopus package is. Different (floating-point) output.

The Go numbers come from the default Go toolchain settings
(`GOAMD64=v1`, `CGO_ENABLED=0`, default GOMAXPROCS).

## libopus 1.0.3 (libopus 1.0.3-fixed)

Measured on AMD Ryzen Threadripper 7960X 24-Cores, linux/amd64, go1.26.3-X:nodwarf5, C built with cc (GCC) 16.1.1 20260430 at `-O2`. Each cell is the best of 15 runs over a 6-second clip, as a real-time factor (seconds of audio processed per second of one core; higher is faster). The ratio columns divide the Go time by the C time (1.00 = as fast as C; 1.50 = 50% slower).

| Scenario | | Go (this port) | C fixed, no SIMD | C fixed, SIMD | C float, SIMD | Go / C fixed no-SIMD | Go / C float SIMD |
|---|---|--:|--:|--:|--:|--:|--:|
| music 48 kHz stereo 128 kb/s | CELT encode | 63× | 166× | 166× | 128× | 2.63 | 2.03 |
| music 48 kHz stereo 64 kb/s | CELT encode | 71× | 208× | 208× | 157× | 2.91 | 2.19 |
| music 48 kHz stereo 64 kb/s, complexity 5 | CELT encode | 72× | 209× | 209× | 157× | 2.92 | 2.19 |
| low delay 48 kHz stereo 96 kb/s, 10 ms | CELT encode | 64× | 165× | 166× | 136× | 2.60 | 2.14 |
| speech 48 kHz mono 32 kb/s | hybrid encode | 54× | 86× | 86× | 99× | 1.58 | 1.82 |
| speech 16 kHz mono 24 kb/s | SILK encode | 71× | 103× | 102× | 114× | 1.46 | 1.62 |
| speech 8 kHz mono 12 kb/s | SILK encode | 136× | 193× | 194× | 226× | 1.42 | 1.66 |
| music 48 kHz stereo 128 kb/s | CELT decode | 138× | 285× | 287× | 382× | 2.06 | 2.77 |
| music 48 kHz stereo 64 kb/s | CELT decode | 156× | 343× | 341× | 470× | 2.20 | 3.02 |
| music 48 kHz stereo 64 kb/s, complexity 5 | CELT decode | 161× | 343× | 342× | 471× | 2.13 | 2.92 |
| low delay 48 kHz stereo 96 kb/s, 10 ms | CELT decode | 141× | 301× | 302× | 443× | 2.13 | 3.13 |
| speech 48 kHz mono 32 kb/s | hybrid decode | 239× | 502× | 505× | 579× | 2.10 | 2.42 |
| speech 16 kHz mono 24 kb/s | SILK decode | 1177× | 1834× | 1815× | 1725× | 1.56 | 1.47 |
| speech 8 kHz mono 12 kb/s | SILK decode | 2816× | 4621× | 4632× | 4357× | 1.64 | 1.55 |

### Reading the numbers

The port is a line-by-line translation and has not been hand-optimised.
The remaining gap to C is general code generation: GCC vectorises and
schedules these fixed-point loops more aggressively than the Go compiler,
and the hot loops are not restructured for Go. Go's bounds checks account
for only about 5-8% (measured by building with `-gcflags=all=-B`), and
`GOAMD64=v3` makes no measurable difference. Even so, one core
encodes high-quality stereo music many tens of times faster than real time,
so typical real-time uses (VoIP, streaming, file transcoding) are far from
CPU-bound.

### Reproducing

```
make -C tools/bench            # builds the four upstream variants for the checked-out release
go run ./tools/bench -out BENCHMARKS.md
go test -run '^$' -bench . .   # Go-only microbenchmarks (x_realtime metric)
```
