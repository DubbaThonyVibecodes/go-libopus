package libopus

import (
	"encoding/binary"
	"os"
	"testing"
)

// The benchmarks below run the same workloads as tools/bench (which also
// times upstream C for comparison). Each op is one 20 ms frame; the
// x_realtime metric is seconds of audio processed per second of CPU.

func benchClip(b *testing.B, name string) []int16 {
	raw, err := os.ReadFile("testdata/corpus/in/" + name)
	if err != nil {
		b.Skip(err)
	}
	pcm := make([]int16, len(raw)/2)
	for i := range pcm {
		pcm[i] = int16(binary.LittleEndian.Uint16(raw[2*i:]))
	}
	return pcm
}

func benchEncode(b *testing.B, clip string, rate, ch int, app Application, bitrate, cplx int) {
	pcm := benchClip(b, clip)
	frame := rate / 50
	nframes := len(pcm) / ch / frame
	enc, err := NewEncoder(rate, ch, app)
	if err != nil {
		b.Fatal(err)
	}
	enc.SetBitrate(bitrate)
	enc.SetComplexity(cplx)
	pkt := make([]byte, 1500)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f := i % nframes
		if _, err := enc.Encode(pcm[f*frame*ch:(f+1)*frame*ch], pkt); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(b.N)*0.02/b.Elapsed().Seconds(), "x_realtime")
}

func benchDecode(b *testing.B, clip string, rate, ch int, app Application, bitrate int) {
	pcm := benchClip(b, clip)
	frame := rate / 50
	nframes := len(pcm) / ch / frame
	enc, err := NewEncoder(rate, ch, app)
	if err != nil {
		b.Fatal(err)
	}
	enc.SetBitrate(bitrate)
	pkts := make([][]byte, nframes)
	for f := range pkts {
		buf := make([]byte, 1500)
		n, err := enc.Encode(pcm[f*frame*ch:(f+1)*frame*ch], buf)
		if err != nil {
			b.Fatal(err)
		}
		pkts[f] = buf[:n]
	}
	dec, err := NewDecoder(rate, ch)
	if err != nil {
		b.Fatal(err)
	}
	out := make([]int16, 5760*ch)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := dec.Decode(pkts[i%nframes], out, false); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(b.N)*0.02/b.Elapsed().Seconds(), "x_realtime")
}

func BenchmarkEncodeMusic48kStereo128k(b *testing.B) {
	benchEncode(b, "clip_piano_48000_2ch_6s.s16", 48000, 2, Audio, 128000, 10)
}
func BenchmarkEncodeMusic48kStereo64kCplx5(b *testing.B) {
	benchEncode(b, "clip_mix_48000_2ch_6s.s16", 48000, 2, Audio, 64000, 5)
}
func BenchmarkEncodeSpeech16kMono24k(b *testing.B) {
	benchEncode(b, "clip_speech_b_16000_1ch_6s.s16", 16000, 1, VoIP, 24000, 10)
}
func BenchmarkEncodeSpeech48kMono32k(b *testing.B) {
	benchEncode(b, "clip_speech_a_48000_1ch_6s.s16", 48000, 1, VoIP, 32000, 10)
}
func BenchmarkDecodeMusic48kStereo128k(b *testing.B) {
	benchDecode(b, "clip_piano_48000_2ch_6s.s16", 48000, 2, Audio, 128000)
}
func BenchmarkDecodeSpeech16kMono24k(b *testing.B) {
	benchDecode(b, "clip_speech_b_16000_1ch_6s.s16", 16000, 1, VoIP, 24000)
}
