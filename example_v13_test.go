package libopus_test

import (
	"fmt"
	"log"

	libopus "github.com/DubbaThonyVibecodes/go-libopus"
)

// Encode first-order ambisonics (4 channels) with the projection encoder,
// channel mapping family 3 (libopus 1.3+).
func ExampleNewProjectionEncoder() {
	const rate, channels = 48000, 4
	enc, err := libopus.NewProjectionEncoder(rate, channels, libopus.Audio)
	if err != nil {
		log.Fatal(err)
	}
	streams, coupled := enc.Streams()
	matrix, _ := enc.DemixingMatrix() // goes to the decoder, e.g. in the Ogg header

	dec, err := libopus.NewProjectionDecoder(rate, channels, streams, coupled, matrix)
	if err != nil {
		log.Fatal(err)
	}
	packet := make([]byte, 4*libopus.MaxPacket)
	n, _ := enc.Encode(tone(rate, channels, 960, 0), packet)
	out := make([]int16, 5760*channels)
	m, _ := dec.Decode(packet[:n], out, false)
	fmt.Println(streams, "streams,", coupled, "coupled,", len(matrix), "matrix bytes,", m, "samples")
	// Output: 2 streams, 2 coupled, 32 matrix bytes, 960 samples
}
