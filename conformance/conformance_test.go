// Package conformance replays every case of the committed corpus
// (testdata/corpus) and, when present, the locally generated extended
// corpus (testdata/generated) through the Go port and diffs the traces
// against the C oracle. Each failure reports the first diverging frame.
package conformance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DubbaThonyVibecodes/go-libopus/internal/oracle"
	"github.com/DubbaThonyVibecodes/go-libopus/internal/testvec"
)

var filter = flag.String("case", "", "only run cases whose name contains this substring")

func TestCorpus(t *testing.T)   { runCorpus(t, filepath.Join("..", "testdata", "corpus")) }
func TestExtended(t *testing.T) { runCorpus(t, filepath.Join("..", "testdata", "generated")) }

func runCorpus(t *testing.T, dir string) {
	man, err := testvec.Load(dir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("no corpus in %s (run `go run ./tools/corpus`)", dir)
		}
		t.Fatal(err)
	}
	// Outputs of enc/repack cases are consumed by dec cases; run them from
	// the oracle's files rather than from our own output so every case is
	// independent (and failures are localised).
	for _, c := range man.Cases {
		if *filter != "" && !strings.Contains(c.Name, *filter) {
			continue
		}
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			runCase(t, dir, c)
		})
	}
}

func argInt(args []string, key string) int {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key {
			var v int
			for _, ch := range args[i+1] {
				v = v*10 + int(ch-'0')
			}
			return v
		}
	}
	return 0
}

func runCase(t *testing.T, dir string, c testvec.Case) {
	want, err := testvec.ReadTrace(filepath.Join(dir, c.Trace))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	switch c.Kind {
	case "enc":
		var in []int16
		if c.Synth != "" {
			in, err = testvec.Synth(c.Synth, argInt(c.Args, "--rate"), argInt(c.Args, "--ch"), c.SynthSecs)
		} else {
			in, err = testvec.ReadS16File(filepath.Join(dir, c.In))
		}
		if err != nil {
			t.Fatal(err)
		}
		res, err := oracle.RunEnc(c.Args, in)
		if err != nil {
			t.Fatal(err)
		}
		got = res.Trace
		compareTrace(t, got, want)
		wantPkts, err := testvec.ReadPacketFile(filepath.Join(dir, c.OutPkt))
		if err != nil {
			t.Fatal(err)
		}
		comparePackets(t, res.Packets, wantPkts)
	case "dec":
		pkts, err := testvec.ReadPacketFile(filepath.Join(dir, c.Pkt))
		if err != nil {
			t.Fatal(err)
		}
		res, err := oracle.RunDec(c.Args, pkts)
		if err != nil {
			t.Fatal(err)
		}
		compareTrace(t, res.Trace, want)
		if c.PcmSHA != "" {
			sum := sha256.Sum256(testvec.S16Bytes(res.Pcm))
			if hex.EncodeToString(sum[:]) != c.PcmSHA {
				t.Errorf("decoded PCM differs (%d bytes, want %d)", 2*len(res.Pcm), c.PcmBytes)
			}
		}
	case "info":
		pkts, err := testvec.ReadPacketFile(filepath.Join(dir, c.Pkt))
		if err != nil {
			t.Fatal(err)
		}
		got, err = oracle.RunInfo(c.Args, pkts)
		if err != nil {
			t.Fatal(err)
		}
		compareTrace(t, got, want)
	case "repack":
		pkts, err := testvec.ReadPacketFile(filepath.Join(dir, c.Pkt))
		if err != nil {
			t.Fatal(err)
		}
		res, err := oracle.RunRepack(c.Args, pkts)
		if err != nil {
			t.Fatal(err)
		}
		compareTrace(t, res.Trace, want)
		wantPkts, err := testvec.ReadPacketFile(filepath.Join(dir, c.OutPkt))
		if err != nil {
			t.Fatal(err)
		}
		comparePackets(t, res.Packets, wantPkts)
	default:
		t.Fatalf("unknown case kind %q", c.Kind)
	}
}

func compareTrace(t *testing.T, got, want []string) {
	t.Helper()
	if i := testvec.FirstDiff(got, want); i >= 0 {
		g, w := "<missing>", "<missing>"
		if i < len(got) {
			g = got[i]
		}
		if i < len(want) {
			w = want[i]
		}
		t.Errorf("trace differs at line %d (of %d/%d):\n got: %s\nwant: %s", i, len(got), len(want), g, w)
	}
}

func comparePackets(t *testing.T, got, want []testvec.Packet) {
	t.Helper()
	n := len(got)
	if len(want) < n {
		n = len(want)
	}
	for i := 0; i < n; i++ {
		if got[i].Rng != want[i].Rng || !bytes.Equal(got[i].Data, want[i].Data) {
			t.Errorf("packet %d differs: len %d/%d rng %08x/%08x", i, len(got[i].Data), len(want[i].Data), got[i].Rng, want[i].Rng)
			return
		}
	}
	if len(got) != len(want) {
		t.Errorf("packet count differs: got %d, want %d", len(got), len(want))
	}
}
