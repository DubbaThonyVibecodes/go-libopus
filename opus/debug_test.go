package opus

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/DubbaThonyVibecodes/go-libopus/internal/testvec"
)

// TestDebugDecState mirrors build/dbg/decstate.c. Run with
// DBG_PKT=file DBG_RATE=.. DBG_CH=.. DBG_LOSS=.. DBG_SEED=..
func TestDebugDecState(t *testing.T) {
	pktPath := os.Getenv("DBG_PKT")
	if pktPath == "" {
		t.Skip("DBG_PKT not set")
	}
	rate, _ := strconv.Atoi(os.Getenv("DBG_RATE"))
	ch, _ := strconv.Atoi(os.Getenv("DBG_CH"))
	lossperc, _ := strconv.Atoi(os.Getenv("DBG_LOSS"))
	seed, _ := strconv.ParseUint(os.Getenv("DBG_SEED"), 0, 32)
	f, err := os.Open(pktPath)
	if err != nil {
		t.Fatal(err)
	}
	pkts, err := testvec.ReadPackets(f)
	if err != nil {
		t.Fatal(err)
	}
	lcg := testvec.LCG{State: uint32(seed)}
	var errc int
	st := DecoderCreate(int32(rate), ch, &errc)
	out := make([]int16, 5760*8)
	w, _ := os.Create(os.Getenv("DBG_OUT"))
	defer w.Close()
	for count, p := range pkts {
		lost := len(p.Data) == 0 || (lossperc > 0 && int(lcg.Next()%100) < lossperc)
		var n int
		if lost {
			var dur int32
			st.Ctl(OPUS_GET_LAST_PACKET_DURATION_REQUEST, &dur)
			n = st.Decode(nil, 0, out, int(dur), 0)
		} else {
			n = st.Decode(p.Data, int32(len(p.Data)), out, 5760, 0)
		}
		var h int64
		if n > 0 {
			for _, v := range out[:n*ch] {
				h = h*31 + int64(v)
			}
		}
		fmt.Fprintf(w, "F %d lost=%d n=%d mode=%d pcm=%d | %s\n", count, b2i(lost), n, st.mode, h, st.silk_dec.DebugState())
	}
}
