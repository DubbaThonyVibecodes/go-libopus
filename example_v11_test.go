package libopus_test

import (
	"fmt"

	libopus "github.com/DubbaThonyVibecodes/go-libopus"
)

// Pad a packet to a fixed size (e.g. for constant-rate transports) and strip
// the padding again (libopus 1.1+).
func ExamplePacketPad() {
	enc, _ := libopus.NewEncoder(48000, 1, libopus.Audio)
	buf := make([]byte, 400)
	n, _ := enc.Encode(tone(48000, 1, 960, 0), buf)

	padded, err := libopus.PacketPad(buf[:n], 400)
	if err != nil {
		panic(err)
	}
	stripped, _ := libopus.PacketUnpad(padded)
	fmt.Println(len(padded), len(stripped) == n)
	// Output: 400 true
}

// Let the encoder decide the frame duration (here: always 20 ms, whatever
// the caller passes in). Encode then consumes only that many samples
// (libopus 1.1+).
func ExampleEncoder_SetExpertFrameDuration() {
	enc, _ := libopus.NewEncoder(48000, 1, libopus.Audio)
	enc.SetExpertFrameDuration(libopus.FrameDuration20ms)
	buf := make([]byte, libopus.MaxPacket)
	n, _ := enc.Encode(tone(48000, 1, 2880, 0), buf) // 60 ms offered
	used, _ := libopus.PacketSamples(buf[:n], 48000)
	fmt.Println("encoded", used, "of 2880 samples")
	// Output: encoded 960 of 2880 samples
}
