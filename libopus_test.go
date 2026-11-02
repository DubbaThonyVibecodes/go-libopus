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
	if err := enc.SetExpertFrameDuration(FrameDuration120ms); err != nil {
		t.Fatal(err)
	}
	if enc.ExpertFrameDuration() != FrameDuration120ms {
		t.Fatal("frame duration not applied")
	}
	if err := enc.SetPhaseInversionDisabled(true); err != nil {
		t.Fatal(err)
	}
	if !enc.PhaseInversionDisabled() {
		t.Fatal("phase inversion flag not applied")
	}
	if err := enc.SetPredictionDisabled(true); err != nil {
		t.Fatal(err)
	}
	if !enc.PredictionDisabled() {
		t.Fatal("prediction flag not applied")
	}
	pcm := make([]int16, 5760)
	for i := range pcm {
		pcm[i] = int16(8000 * math.Sin(float64(i)*2*math.Pi*440/48000))
	}
	pkt := make([]byte, 3*1275+7)
	n, err := enc.Encode(pcm, pkt)
	if err != nil || n < 1 {
		t.Fatalf("encode: n=%d err=%v", n, err)
	}
	used, err := PacketSamples(pkt[:n], 48000)
	if err != nil || used != 5760 {
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

// TestProjection round-trips a first-order ambisonics signal through the
// projection (mapping family 3) encoder and decoder (libopus 1.3+).
func TestProjection(t *testing.T) {
	const rate, ch, frame = 48000, 4, 960
	enc, err := NewProjectionEncoder(rate, ch, Audio)
	if err != nil {
		t.Fatal(err)
	}
	if err := enc.SetBitrate(128000); err != nil {
		t.Fatal(err)
	}
	streams, coupled := enc.Streams()
	matrix, err := enc.DemixingMatrix()
	if err != nil || len(matrix) != (streams+coupled)*ch*2 {
		t.Fatalf("matrix: %v len %d", err, len(matrix))
	}
	dec, err := NewProjectionDecoder(rate, ch, streams, coupled, matrix)
	if err != nil {
		t.Fatal(err)
	}
	pcm := make([]int16, frame*ch)
	out := make([]int16, 5760*ch)
	pkt := make([]byte, 4*MaxPacket)
	var energy float64
	for f := 0; f < 20; f++ {
		for i := 0; i < frame; i++ {
			pcm[i*ch] = int16(8000 * math.Sin(float64(f*frame+i)*2*math.Pi*440/rate))
		}
		n, err := enc.Encode(pcm, pkt)
		if err != nil {
			t.Fatal(err)
		}
		m, err := dec.Decode(pkt[:n], out, false)
		if err != nil || m != frame {
			t.Fatalf("decode: m=%d err=%v", m, err)
		}
		for i := 0; i < m*ch; i++ {
			energy += float64(out[i]) * float64(out[i])
		}
	}
	if energy == 0 {
		t.Fatal("silent output")
	}
	if l, err := AmbisonicsLayout(6); err != nil || l.Streams != 5 || l.CoupledStreams != 1 {
		t.Fatalf("ambisonics layout: %+v %v", l, err)
	}
	e2, err := NewSurroundEncoder(rate, 9, 2, Audio)
	if err != nil || e2.Layout().Streams != 9 {
		t.Fatalf("family 2 encoder: %v", err)
	}
	e, _ := NewEncoder(rate, 1, VoIP)
	if e.InDTX() {
		t.Fatal("fresh encoder reports DTX")
	}
}

// TestPCM24 round-trips through the 24-bit API (libopus 1.6+) and checks the
// restricted applications and the ignore-extensions ctl.
func TestPCM24(t *testing.T) {
	const rate, ch, frame = 48000, 2, 960
	enc, err := NewEncoder(rate, ch, RestrictedCELT)
	if err != nil {
		t.Fatal(err)
	}
	if enc.Application() != RestrictedCELT {
		t.Fatalf("application %v", enc.Application())
	}
	if _, err := NewEncoder(rate, 1, RestrictedSILK); err != nil {
		t.Fatal(err)
	}
	dec, _ := NewDecoder(rate, ch)
	if err := dec.SetIgnoreExtensions(true); err != nil || !dec.IgnoreExtensions() {
		t.Fatalf("ignore extensions: %v", err)
	}
	pcm := make([]int32, frame*ch)
	for i := 0; i < frame; i++ {
		v := int32(4000000 * math.Sin(float64(i)*2*math.Pi*440/rate))
		pcm[i*ch], pcm[i*ch+1] = v, v/2
	}
	pkt := make([]byte, MaxPacket)
	out := make([]int32, 5760*ch)
	var energy float64
	for f := 0; f < 10; f++ {
		n, err := enc.Encode24(pcm, pkt)
		if err != nil {
			t.Fatal(err)
		}
		m, err := dec.Decode24(pkt[:n], out, false)
		if err != nil || m != frame {
			t.Fatalf("decode24: m=%d err=%v", m, err)
		}
		for _, v := range out[:m*ch] {
			energy += float64(v) * float64(v)
		}
	}
	if energy == 0 {
		t.Fatal("silent output")
	}
}
