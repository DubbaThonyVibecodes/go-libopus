package libopus_test

import (
	"errors"
	"fmt"
	"log"
	"math"

	libopus "github.com/DubbaThonyVibecodes/go-libopus"
)

// tone returns n samples per channel of a 440 Hz tone, interleaved.
func tone(rate, channels, n, offset int) []int16 {
	pcm := make([]int16, n*channels)
	for i := 0; i < n; i++ {
		v := int16(8000 * math.Sin(2*math.Pi*440*float64(offset+i)/float64(rate)))
		for c := 0; c < channels; c++ {
			pcm[i*channels+c] = v
		}
	}
	return pcm
}

// Encode a stereo stream in 20 ms frames and decode it again.
func Example() {
	const rate, channels = 48000, 2
	const frame = rate / 50 // 20 ms = 960 samples per channel

	enc, err := libopus.NewEncoder(rate, channels, libopus.Audio)
	if err != nil {
		log.Fatal(err)
	}
	if err := enc.SetBitrate(96000); err != nil {
		log.Fatal(err)
	}
	dec, err := libopus.NewDecoder(rate, channels)
	if err != nil {
		log.Fatal(err)
	}

	packet := make([]byte, libopus.MaxPacket)
	out := make([]int16, 5760*channels) // room for the longest packet (120 ms)
	total := 0
	for f := 0; f < 50; f++ { // one second
		n, err := enc.Encode(tone(rate, channels, frame, f*frame), packet)
		if err != nil {
			log.Fatal(err)
		}
		samples, err := dec.Decode(packet[:n], out, false)
		if err != nil {
			log.Fatal(err)
		}
		total += samples
	}
	fmt.Println("decoded samples per channel:", total)
	// Output: decoded samples per channel: 48000
}

// Conceal a lost packet, and recover one from in-band FEC data.
func ExampleDecoder_Decode_packetLoss() {
	const rate, frame = 16000, 320 // 20 ms
	enc, _ := libopus.NewEncoder(rate, 1, libopus.VoIP)
	enc.SetBitrate(24000)
	enc.SetInbandFEC(true)    // carry a low-rate copy of each frame in the next packet
	enc.SetPacketLossPerc(20) // FEC is only used when losses are expected

	var packets [][]byte
	for f := 0; f < 10; f++ {
		buf := make([]byte, libopus.MaxPacket)
		n, _ := enc.Encode(tone(rate, 1, frame, f*frame), buf)
		packets = append(packets, buf[:n])
	}

	dec, _ := libopus.NewDecoder(rate, 1)
	pcm := make([]int16, 5760)
	for i, p := range packets {
		switch i {
		case 3: // lost, and the next packet has not arrived yet: conceal it
			n, _ := dec.Decode(nil, pcm[:dec.LastPacketDuration()], false)
			fmt.Println("packet 3 concealed:", n)
		case 6: // lost, but packet 7 is already here: recover 6 from 7's FEC data
		case 7:
			n, _ := dec.Decode(p, pcm[:dec.LastPacketDuration()], true)
			fmt.Println("packet 6 recovered:", n)
			n, _ = dec.Decode(p, pcm, false)
			fmt.Println("packet 7 decoded:", n)
		default:
			dec.Decode(p, pcm, false)
		}
	}
	// Output:
	// packet 3 concealed: 320
	// packet 6 recovered: 320
	// packet 7 decoded: 320
}

// Float samples in [-1, 1] work the same way as int16.
func ExampleEncoder_EncodeFloat() {
	enc, _ := libopus.NewEncoder(48000, 1, libopus.Audio)
	dec, _ := libopus.NewDecoder(48000, 1)
	in := make([]float32, 960)
	for i := range in {
		in[i] = float32(0.25 * math.Sin(2*math.Pi*1000*float64(i)/48000))
	}
	packet := make([]byte, libopus.MaxPacket)
	n, err := enc.EncodeFloat(in, packet)
	if err != nil {
		log.Fatal(err)
	}
	out := make([]float32, 5760)
	m, _ := dec.DecodeFloat(packet[:n], out, false)
	fmt.Println(m)
	// Output: 960
}

// Encode 5.1 surround with the standard (Vorbis) channel layout.
func ExampleNewSurroundEncoder() {
	const rate, channels, frame = 48000, 6, 960
	enc, err := libopus.NewSurroundEncoder(rate, channels, 1, libopus.Audio)
	if err != nil {
		log.Fatal(err)
	}
	enc.SetBitrate(256000)
	layout := enc.Layout() // what the decoder needs (sent in the container header)
	fmt.Printf("%d streams, %d coupled, mapping %v\n", layout.Streams, layout.CoupledStreams, layout.Mapping)

	dec, err := libopus.NewMultistreamDecoder(rate, layout)
	if err != nil {
		log.Fatal(err)
	}
	packet := make([]byte, 6*libopus.MaxPacket)
	n, _ := enc.Encode(tone(rate, channels, frame, 0), packet)
	out := make([]int16, 5760*channels)
	m, _ := dec.Decode(packet[:n], out, false)
	fmt.Println("decoded", m, "samples per channel")
	// Output:
	// 4 streams, 2 coupled, mapping [0 4 1 2 3 5]
	// decoded 960 samples per channel
}

// Merge three 20 ms packets into one 60 ms packet and look inside it.
func ExampleRepacketizer() {
	enc, _ := libopus.NewEncoder(48000, 1, libopus.Audio)
	rp := libopus.NewRepacketizer()
	var bufs [][]byte // packets must stay valid until Out
	for f := 0; f < 3; f++ {
		buf := make([]byte, libopus.MaxPacket)
		n, _ := enc.Encode(tone(48000, 1, 960, f*960), buf)
		bufs = append(bufs, buf[:n])
		if err := rp.Cat(bufs[f]); err != nil {
			log.Fatal(err)
		}
	}
	merged := make([]byte, 3*libopus.MaxPacket)
	n, err := rp.Out(merged)
	if err != nil {
		log.Fatal(err)
	}
	frames, _ := libopus.PacketFrames(merged[:n])
	samples, _ := libopus.PacketSamples(merged[:n], 48000)
	bw, _ := libopus.PacketBandwidth(merged[:n])
	fmt.Println(frames, "frames,", samples, "samples,", bw)
	// Output: 3 frames, 2880 samples, fullband
}

// Errors are libopus error codes, comparable with errors.Is.
func ExampleError() {
	_, err := libopus.NewEncoder(44100, 2, libopus.Audio) // 44.1 kHz is not an Opus rate
	fmt.Println(err, errors.Is(err, libopus.ErrBadArg))
	// Output: opus: invalid argument true
}
