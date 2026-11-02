package celt

import (
	"fmt"
	"hash/crc32"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DubbaThonyVibecodes/go-libopus/internal/testvec"
)

// TestCeltOnlyCorpus decodes the CELT-only packets of the conformance corpus
// directly with the CELT decoder (doing the little bit of TOC handling the
// Opus layer would do) and compares per-frame range state and output CRC
// with the oracle trace. It is an early smoke test for the CELT port; the
// real conformance suite lives in the conformance package.
func TestCeltOnlyCorpus(t *testing.T) {
	dir := filepath.Join("..", "testdata", "corpus")
	man, err := testvec.Load(dir)
	if err != nil {
		t.Skip("corpus not available:", err)
	}
	var ran int
	for _, c := range man.Cases {
		if c.Kind != "dec" || !strings.HasSuffix(c.Name, ".plain") {
			continue
		}
		n := strings.TrimPrefix(c.Name, "dec/")
		if !(strings.Contains(n, "force_celt") || strings.Contains(n, "modelist_celt") || strings.Contains(n, "lowdelay")) {
			continue
		}
		c := c
		t.Run(n, func(t *testing.T) {
			if runCeltCase(t, dir, c) {
				ran++
			}
		})
	}
	if ran == 0 {
		t.Fatal("no CELT-only cases found")
	}
}

func runCeltCase(t *testing.T, dir string, c testvec.Case) bool {
	rate := argInt(c.Args, "--rate")
	ch := argInt(c.Args, "--ch")
	if ch > 2 {
		t.Skip("multistream case")
	}
	pkts, err := testvec.ReadPacketFile(filepath.Join(dir, c.Pkt))
	if err != nil {
		t.Fatal(err)
	}
	want, err := testvec.ReadTrace(filepath.Join(dir, c.Trace))
	if err != nil {
		t.Fatal(err)
	}
	st := new(CELTDecoder)
	if r := st.DecoderInit(int32(rate), ch); r != OPUS_OK {
		t.Fatal("init:", r)
	}
	for i, p := range pkts {
		line := want[i+1] // skip header
		if len(p.Data) == 0 {
			t.Skip("case has lost packets")
		}
		toc := p.Data[0]
		config := int(toc >> 3)
		if config < 16 || toc&3 != 0 {
			t.Skipf("packet %d: not a single-frame CELT packet (toc=%02x)", i, toc)
		}
		streamCh := 1 + int(toc>>2)&1
		frame := []int{120, 240, 480, 960}[config&3] * rate / 48000
		var endband int32
		switch (config - 16) / 4 {
		case 0:
			endband = 13
		case 1:
			endband = 17
		case 2:
			endband = 19
		default:
			endband = 21
		}
		st.Ctl(CELT_SET_START_BAND_REQUEST, int32(0))
		st.Ctl(CELT_SET_END_BAND_REQUEST, endband)
		st.Ctl(CELT_SET_CHANNELS_REQUEST, int32(streamCh))
		pcm := make([]int32, frame*ch) // opus_res (24-bit resolution since 1.6)
		var dec EcDec
		dec.DecInit(p.Data[1:], uint32(len(p.Data)-1))
		n := st.DecodeWithEc(p.Data[1:], len(p.Data)-1, pcm, frame, &dec, 0)
		if n != frame {
			t.Fatalf("packet %d: decode returned %d, want %d", i, n, frame)
		}
		got := fmt.Sprintf("n=%d rng=%08x", n, dec.Rng())
		pcm16 := make([]int16, len(pcm))
		for j := range pcm {
			pcm16[j] = RES2INT16(pcm[j])
		}
		crc := crc32.ChecksumIEEE(testvec.S16Bytes(pcm16))
		gotCrc := fmt.Sprintf("crc=%08x", crc)
		if !strings.Contains(line, got) || !strings.Contains(line, gotCrc) {
			t.Fatalf("packet %d mismatch:\n got %s %s\nwant %s", i, got, gotCrc, line)
		}
	}
	return true
}

func argInt(args []string, key string) int {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key {
			var v int
			fmt.Sscan(args[i+1], &v)
			return v
		}
	}
	return 0
}
