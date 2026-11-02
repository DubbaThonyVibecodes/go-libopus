package libopus

import (
	"math"
	"testing"
)

// TestRoundTrip encodes a tone with the wrapper and decodes it back; it is
// a smoke test of the API, the bit-exact checks live in the conformance package.
func TestRoundTrip(t *testing.T) {
	const rate, ch, frame = 48000, 2, 960
	enc, err := NewEncoder(rate, ch, Audio)
	if err != nil {
		t.Fatal(err)
	}
	if err := enc.SetBitrate(64000); err != nil {
		t.Fatal(err)
	}
	if enc.Bitrate() != 64000 || enc.Lookahead() <= 0 {
		t.Fatalf("settings not applied: bitrate %d lookahead %d", enc.Bitrate(), enc.Lookahead())
	}
	dec, err := NewDecoder(rate, ch)
	if err != nil {
		t.Fatal(err)
	}
	pcm := make([]int16, frame*ch)
	out := make([]int16, 5760*ch)
	pkt := make([]byte, MaxPacket)
	var energyIn, energyOut float64
	for f := 0; f < 50; f++ {
		for i := 0; i < frame; i++ {
			v := int16(8000 * math.Sin(2*math.Pi*440*float64(f*frame+i)/rate))
			pcm[2*i], pcm[2*i+1] = v, v/2
			energyIn += float64(v) * float64(v)
		}
		n, err := enc.Encode(pcm, pkt)
		if err != nil {
			t.Fatal(err)
		}
		m, err := dec.Decode(pkt[:n], out, false)
		if err != nil {
			t.Fatal(err)
		}
		if m != frame {
			t.Fatalf("decoded %d samples, want %d", m, frame)
		}
		if enc.FinalRange() != dec.FinalRange() {
			t.Fatalf("frame %d: range mismatch %08x vs %08x", f, enc.FinalRange(), dec.FinalRange())
		}
		for i := 0; i < frame; i++ {
			energyOut += float64(out[2*i]) * float64(out[2*i])
		}
	}
	if energyOut < energyIn/4 || energyOut > energyIn*4 {
		t.Fatalf("output energy %g too far from input %g", energyOut, energyIn)
	}
	// PLC and FEC paths must not fail.
	if _, err := dec.Decode(nil, out[:frame*ch], false); err != nil {
		t.Fatal("plc:", err)
	}
	if _, err := dec.Decode(pkt[:1], out[:frame*ch], true); err != nil {
		t.Fatal("fec:", err)
	}
	if bw, err := PacketBandwidth(pkt); err != nil || bw != Fullband {
		t.Fatalf("bandwidth %v %v", bw, err)
	}
}

func TestSurround(t *testing.T) {
	l, err := SurroundLayout(6)
	if err != nil {
		t.Fatal(err)
	}
	if l.Streams != 4 || l.CoupledStreams != 2 {
		t.Fatalf("unexpected 5.1 layout %+v", l)
	}
	enc, err := NewSurroundEncoder(48000, 6, 1, Audio)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := NewMultistreamDecoder(48000, enc.Layout())
	if err != nil {
		t.Fatal(err)
	}
	pcm := make([]int16, 960*6)
	for i := range pcm {
		pcm[i] = int16((i * 37) % 2000)
	}
	pkt := make([]byte, 4*MaxPacket)
	n, err := enc.Encode(pcm, pkt)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]int16, 5760*6)
	if m, err := dec.Decode(pkt[:n], out, false); err != nil || m != 960 {
		t.Fatalf("decode: %d %v", m, err)
	}
}

// TestFrameDurationAndPadding exercises the libopus 1.1 additions of the
// wrapper: variable frame duration, prediction disabling and packet padding.
func TestFrameDurationAndPadding(t *testing.T) {
	enc, err := NewEncoder(48000, 1, Audio)
	if err != nil {
		t.Fatal(err)
	}
	if err := enc.SetExpertFrameDuration(FrameDurationVariable); err != nil {
		t.Fatal(err)
	}
	if enc.ExpertFrameDuration() != FrameDurationVariable {
		t.Fatal("frame duration not applied")
	}
	if err := enc.SetPredictionDisabled(true); err != nil {
		t.Fatal(err)
	}
	if !enc.PredictionDisabled() {
		t.Fatal("prediction flag not applied")
	}
	pcm := make([]int16, 2880)
	for i := range pcm {
		pcm[i] = int16(8000 * math.Sin(float64(i)*2*math.Pi*440/48000))
	}
	pkt := make([]byte, MaxPacket)
	n, err := enc.Encode(pcm, pkt)
	if err != nil || n < 1 {
		t.Fatalf("encode: n=%d err=%v", n, err)
	}
	used, err := PacketSamples(pkt[:n], 48000)
	if err != nil || used < 120 || used > 2880 {
		t.Fatalf("packet samples %d err %v", used, err)
	}
	padded, err := PacketPad(pkt[:n], n+37)
	if err != nil || len(padded) != n+37 {
		t.Fatalf("pad: %v len %d", err, len(padded))
	}
	dec, _ := NewDecoder(48000, 1)
	out := make([]int16, 5760)
	if m, err := dec.Decode(padded, out, false); err != nil || m != used {
		t.Fatalf("decode padded: m=%d err=%v", m, err)
	}
	unpadded, err := PacketUnpad(padded)
	if err != nil || len(unpadded) != n || string(unpadded) != string(pkt[:n]) {
		t.Fatalf("unpad: %v len %d", err, len(unpadded))
	}
	f := make([]float32, 480)
	for i := range f {
		f[i] = 1.5
	}
	SoftClip(f, 1, nil)
	if f[100] > 1 || f[100] < 0.5 {
		t.Fatalf("soft clip: %v", f[100])
	}
}
