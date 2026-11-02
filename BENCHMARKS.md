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

## libopus 1.3.1 (libopus 1.3.1-fixed)

Measured on AMD Ryzen Threadripper 7960X 24-Cores, linux/amd64, go1.26.3-X:nodwarf5, C built with cc (GCC) 16.1.1 20260430 at `-O2`. Each cell is the best of 15 runs over a 6-second clip, as a real-time factor (seconds of audio processed per second of one core; higher is faster). The ratio columns divide the Go time by the C time (1.00 = as fast as C; 1.50 = 50% slower).

| Scenario | | Go (this port) | C fixed, no SIMD | C fixed, SIMD | C float, SIMD | Go / C fixed no-SIMD | Go / C float SIMD |
|---|---|--:|--:|--:|--:|--:|--:|
| music 48 kHz stereo 128 kb/s | CELT encode | 68× | 144× | n/a | 182× | 2.13 | 2.70 |
| music 48 kHz stereo 64 kb/s | CELT encode | 74× | 172× | n/a | 219× | 2.31 | 2.94 |
| music 48 kHz stereo 64 kb/s, complexity 5 | CELT encode | 106× | 246× | n/a | 318× | 2.32 | 2.99 |
| low delay 48 kHz stereo 96 kb/s, 10 ms | CELT encode | 75× | 172× | n/a | 218× | 2.30 | 2.91 |
| speech 48 kHz mono 32 kb/s | hybrid encode | 55× | 93× | n/a | 100× | 1.68 | 1.81 |
| speech 16 kHz mono 24 kb/s | SILK encode | 68× | 105× | n/a | 112× | 1.55 | 1.66 |
| speech 8 kHz mono 12 kb/s | SILK encode | 157× | 226× | n/a | 248× | 1.44 | 1.58 |
| music 48 kHz stereo 128 kb/s | CELT decode | 291× | 496× | n/a | 578× | 1.70 | 1.99 |
| music 48 kHz stereo 64 kb/s | CELT decode | 368× | 655× | n/a | 755× | 1.78 | 2.05 |
| music 48 kHz stereo 64 kb/s, complexity 5 | CELT decode | 352× | 649× | n/a | 745× | 1.84 | 2.11 |
| low delay 48 kHz stereo 96 kb/s, 10 ms | CELT decode | 324× | 595× | n/a | 718× | 1.83 | 2.21 |
| speech 48 kHz mono 32 kb/s | hybrid decode | 451× | 862× | n/a | 896× | 1.91 | 1.99 |
| speech 16 kHz mono 24 kb/s | SILK decode | 1373× | 2487× | n/a | 2299× | 1.81 | 1.67 |
| speech 8 kHz mono 12 kb/s | SILK decode | 2979× | 5710× | n/a | 5258× | 1.92 | 1.76 |

*n/a*: the upstream "C fixed, SIMD" build crashed on the workloads marked n/a (a bug in upstream's own SIMD code with this compiler, not in the benchmark), so those cells were not measured.

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
