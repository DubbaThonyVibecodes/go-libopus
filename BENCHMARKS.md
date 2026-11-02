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

## libopus 1.4 (libopus 1.4-fixed)

Measured on AMD Ryzen Threadripper 7960X 24-Cores, linux/amd64, go1.26.3-X:nodwarf5, C built with cc (GCC) 16.1.1 20260430 at `-O2`. Each cell is the best of 15 runs over a 6-second clip, as a real-time factor (seconds of audio processed per second of one core; higher is faster). The ratio columns divide the Go time by the C time (1.00 = as fast as C; 1.50 = 50% slower).

| Scenario | | Go (this port) | C fixed, no SIMD | C fixed, SIMD | C float, SIMD | Go / C fixed no-SIMD | Go / C float SIMD |
|---|---|--:|--:|--:|--:|--:|--:|
| music 48 kHz stereo 128 kb/s | CELT encode | 65× | 145× | 164× | 182× | 2.23 | 2.80 |
| music 48 kHz stereo 64 kb/s | CELT encode | 73× | 172× | 206× | 219× | 2.35 | 2.99 |
| music 48 kHz stereo 64 kb/s, complexity 5 | CELT encode | 104× | 247× | 310× | 319× | 2.37 | 3.07 |
| low delay 48 kHz stereo 96 kb/s, 10 ms | CELT encode | 74× | 169× | 201× | 218× | 2.27 | 2.93 |
| speech 48 kHz mono 32 kb/s | hybrid encode | 54× | 90× | 95× | 98× | 1.66 | 1.82 |
| speech 16 kHz mono 24 kb/s | SILK encode | 66× | 101× | 107× | 110× | 1.53 | 1.67 |
| speech 8 kHz mono 12 kb/s | SILK encode | 153× | 213× | 222× | 237× | 1.39 | 1.55 |
| music 48 kHz stereo 128 kb/s | CELT decode | 305× | 498× | 501× | 567× | 1.63 | 1.86 |
| music 48 kHz stereo 64 kb/s | CELT decode | 373× | 655× | 651× | 756× | 1.76 | 2.02 |
| music 48 kHz stereo 64 kb/s, complexity 5 | CELT decode | 358× | 640× | 647× | 751× | 1.79 | 2.10 |
| low delay 48 kHz stereo 96 kb/s, 10 ms | CELT decode | 339× | 607× | 607× | 724× | 1.79 | 2.14 |
| speech 48 kHz mono 32 kb/s | hybrid decode | 446× | 866× | 865× | 896× | 1.94 | 2.01 |
| speech 16 kHz mono 24 kb/s | SILK decode | 1393× | 2473× | 2468× | 2306× | 1.78 | 1.66 |
| speech 8 kHz mono 12 kb/s | SILK decode | 3058× | 5455× | 5378× | 5005× | 1.78 | 1.64 |

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
