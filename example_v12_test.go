package libopus_test

import (
	"fmt"

	libopus "github.com/DubbaThonyVibecodes/go-libopus"
)

// Frames longer than 60 ms (80, 100, 120 ms) save overhead on low-rate
// links; the encoder packs them as several frames in one packet (libopus 1.2+).
func Example_longFrames() {
	enc, _ := libopus.NewEncoder(16000, 1, libopus.VoIP)
	enc.SetBitrate(12000)
	buf := make([]byte, 3*libopus.MaxPacket)
	n, _ := enc.Encode(tone(16000, 1, 1920, 0), buf) // 120 ms
	frames, _ := libopus.PacketFrames(buf[:n])
	samples, _ := libopus.PacketSamples(buf[:n], 16000)
	fmt.Println(samples, "samples in", frames, "frames")
	// Output: 1920 samples in 2 frames
}
