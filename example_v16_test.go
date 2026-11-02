package libopus_test

import (
	"fmt"

	libopus "github.com/DubbaThonyVibecodes/go-libopus"
)

// 24-bit samples in int32 (range ±8388607) keep the extra resolution of
// libopus 1.6's 24-bit internal path.
func ExampleEncoder_Encode24() {
	enc, _ := libopus.NewEncoder(48000, 2, libopus.Audio)
	dec, _ := libopus.NewDecoder(48000, 2)
	in := make([]int32, 960*2)
	for i, v := range tone(48000, 2, 960, 0) {
		in[i] = int32(v) << 8
	}
	buf := make([]byte, libopus.MaxPacket)
	n, _ := enc.Encode24(in, buf)
	out := make([]int32, 5760*2)
	m, _ := dec.Decode24(buf[:n], out, false)
	fmt.Println(m)
	// Output: 960
}
