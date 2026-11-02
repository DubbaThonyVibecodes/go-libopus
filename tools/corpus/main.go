// Command corpus generates the conformance corpus for go-libopus by driving
// the C oracle (tools/corpus/cgen/gen.c, linked against an out-of-tree build
// of upstream libopus) over a fixed list of cases.
//
//	go run ./tools/corpus            # hermetic subset  -> testdata/corpus
//	go run ./tools/corpus -extended  # full set         -> testdata/generated
//
// Both modes use the same case list; the extended set runs longer inputs and
// keeps the full decoded PCM for every decoder case.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/DubbaThonyVibecodes/go-libopus/internal/testvec"
)

var (
	flagExtended = flag.Bool("extended", false, "generate the extended corpus into testdata/generated")
	flagOut      = flag.String("out", "", "output directory (default depends on -extended)")
	flagGen      = flag.String("gen", "build/gen-fixed", "path to the oracle driver")
	flagJobs     = flag.Int("jobs", runtime.NumCPU(), "parallel jobs")
	flagFilter   = flag.String("filter", "", "only run cases whose name contains this substring")
	flagScratch  = flag.String("scratch", "build/corpus-scratch", "directory for synthetic inputs and other temporaries")
)

type input struct {
	src  string // "clip:name" or "synth:spec"
	rate int
	ch   int
}

func (in input) secs() float64 {
	ext := *flagExtended
	if strings.HasPrefix(in.src, "synth:") {
		if ext {
			return 10
		}
		if in.ch > 2 {
			return 1.5
		}
		return 2
	}
	if ext {
		return 30
	}
	return 6
}

func (in input) synth() bool { return strings.HasPrefix(in.src, "synth:") }

// file is the input path relative to the corpus dir for clips, and relative
// to the scratch dir for synthetic inputs (which are not committed).
func (in input) file() string {
	name := strings.NewReplacer(":", "_", ".", "p").Replace(in.src)
	return fmt.Sprintf("in/%s_%d_%dch_%gs.s16", name, in.rate, in.ch, in.secs())
}

type decVariant struct {
	name string
	args []string // extra/overriding args; "--rate"/"--ch" override the encoder's
}

type encCase struct {
	name string
	in   input
	args []string
	decs []decVariant // in addition to the always-present plain decode
	// Extra derived cases on the produced packet file.
	info   bool
	repack []string // e.g. "merge2", "split"
}

var plain = decVariant{name: "plain"}

func a(s string) []string { return strings.Fields(s) }

func loss(p, seed int) decVariant {
	return decVariant{fmt.Sprintf("loss%d_s%d", p, seed), a(fmt.Sprintf("--lossperc %d --seed %d", p, seed))}
}
func fecloss(p, seed int) decVariant {
	return decVariant{fmt.Sprintf("fec_loss%d_s%d", p, seed), a(fmt.Sprintf("--fec --lossperc %d --seed %d", p, seed))}
}
func atRate(r int) decVariant {
	return decVariant{fmt.Sprintf("at%dk", r/1000), a(fmt.Sprintf("--rate %d", r))}
}
func atCh(c int) decVariant {
	return decVariant{fmt.Sprintf("as%dch", c), a(fmt.Sprintf("--ch %d", c))}
}

var gain = decVariant{"gain", a("--gain -6000")}
var gainSched = decVariant{"gainsched", []string{"--gain", "3000", "--sched", "f=40:gain=-12000,f=80:gain=0,f=120:gain=1000"}}
var asFloat = decVariant{"float", a("--float")}
var decReset = decVariant{"reset", []string{"--sched", "f=50:reset=1,f=51:reset=1,f=130:reset=1"}}
var noPhaseInv = decVariant{"nophaseinv", a("--nophaseinv")}

func clip(name string, rate, ch int) input  { return input{"clip:" + name, rate, ch} }
func synth(spec string, rate, ch int) input { return input{"synth:" + spec, rate, ch} }

// cases builds the full case list.  Keep this table-driven and boring: every
// entry is one oracle invocation plus its decoder variants.
func cases() []encCase {
	var cs []encCase
	add := func(name string, in input, args string, decs ...decVariant) *encCase {
		cs = append(cs, encCase{name: name, in: in, args: a(args), decs: decs})
		return &cs[len(cs)-1]
	}

	// ---- SILK / hybrid speech paths ---------------------------------------
	for _, sp := range []string{"speech_a", "speech_b"} {
		add(sp+"_8k_voip_12k", clip(sp, 8000, 1), "--app voip --bitrate 12000", loss(10, 1), atRate(48000), atCh(2))
		add(sp+"_12k_voip_16k", clip(sp, 12000, 1), "--app voip --bitrate 16000", loss(20, 2))
		add(sp+"_16k_voip_24k", clip(sp, 16000, 1), "--app voip --bitrate 24000", loss(10, 1), loss(40, 9), atRate(8000), atRate(48000), asFloat, decReset)
		add(sp+"_24k_voip_32k", clip(sp, 24000, 1), "--app voip --bitrate 32000", loss(10, 3), atRate(16000))
		add(sp+"_48k_voip_40k", clip(sp, 48000, 1), "--app voip --bitrate 40000", loss(10, 4), atRate(8000), atRate(12000), atRate(16000), atRate(24000), gain)
	}
	sa16 := clip("speech_a", 16000, 1)
	for _, fs := range []string{"10", "40", "60"} {
		c := add("speech_a_16k_voip_fs"+fs, sa16, "--app voip --bitrate 24000 --framesize "+fs, loss(10, 5))
		c.info = true
		if fs != "10" {
			c.repack = []string{"split"}
		}
	}
	for _, fs := range []string{"2.5", "5"} { // SILK can't do these: must fall back to CELT
		add("speech_a_16k_voip_fs"+fs, sa16, "--app voip --bitrate 24000 --framesize "+fs)
	}
	add("speech_a_16k_voip_cbr", sa16, "--app voip --bitrate 24000 --cbr")
	add("speech_a_16k_voip_cvbr", sa16, "--app voip --bitrate 24000 --cvbr")
	add("speech_a_24k_voip_cbr", clip("speech_a", 24000, 1), "--app voip --bitrate 32000 --cbr")
	for _, cx := range []int{0, 2, 5, 8} {
		add(fmt.Sprintf("speech_a_16k_voip_cplx%d", cx), sa16, fmt.Sprintf("--app voip --bitrate 24000 --complexity %d", cx))
	}
	for _, cx := range []int{0, 5} {
		add(fmt.Sprintf("speech_b_8k_voip_cplx%d", cx), clip("speech_b", 8000, 1), fmt.Sprintf("--app voip --bitrate 12000 --complexity %d", cx))
	}
	add("speech_a_16k_voip_fec_loss10", sa16, "--app voip --bitrate 24000 --fec --lossperc 10", fecloss(10, 1), fecloss(30, 2), loss(10, 1))
	add("speech_a_16k_voip_fec_loss30", sa16, "--app voip --bitrate 24000 --fec --lossperc 30", fecloss(30, 3))
	add("speech_b_24k_voip_fec_loss20", clip("speech_b", 24000, 1), "--app voip --bitrate 32000 --fec --lossperc 20", fecloss(20, 4))
	add("speech_b_48k_voip_fec_loss10", clip("speech_b", 48000, 1), "--app voip --bitrate 32000 --fec --lossperc 10", fecloss(10, 5))
	add("speech_a_16k_voip_dtx", sa16, "--app voip --bitrate 24000 --dtx", loss(10, 6))
	add("speech_b_8k_voip_dtx", clip("speech_b", 8000, 1), "--app voip --bitrate 10000 --dtx")
	add("speech_a_16k_voip_6k", sa16, "--app voip --bitrate 6000")
	add("speech_a_16k_voip_max", sa16, "--app voip --bitrate max")
	add("speech_a_48k_voip_max", clip("speech_a", 48000, 1), "--app voip --bitrate max")
	for _, bw := range []string{"NB", "MB", "WB", "SWB", "FB"} {
		add("speech_a_48k_voip_bw"+bw, clip("speech_a", 48000, 1), "--app voip --bitrate 32000 --bw "+bw)
	}
	add("speech_a_48k_voip_maxbwWB", clip("speech_a", 48000, 1), "--app voip --bitrate 64000 --maxbw WB")
	add("speech_a_48k_voip_maxbwNB", clip("speech_a", 48000, 1), "--app voip --bitrate 64000 --maxbw NB")
	add("speech_a_48k_voip_sigvoice", clip("speech_a", 48000, 1), "--app voip --bitrate 32000 --signal voice")
	add("speech_a_48k_voip_sigmusic", clip("speech_a", 48000, 1), "--app voip --bitrate 32000 --signal music")
	add("speech_a_48k_audio_sigvoice", clip("speech_a", 48000, 1), "--app audio --bitrate 32000 --signal voice")
	add("speech_a_16k_voip_lsb8", sa16, "--app voip --bitrate 24000 --lsbdepth 8")
	add("speech_a_16k_voip_lsb24", sa16, "--app voip --bitrate 24000 --lsbdepth 24")
	add("speech_a_16k_voip_maxpayload60", sa16, "--app voip --bitrate 48000 --maxpayload 60")
	add("speech_a_48k_audio_maxpayload100", clip("speech_a", 48000, 1), "--app audio --bitrate 128000 --maxpayload 100")
	add("speech_a_16k_voip_voiceratio0", sa16, "--app voip --bitrate 24000 --voiceratio 0")
	add("speech_a_16k_voip_voiceratio100", sa16, "--app voip --bitrate 24000 --voiceratio 100")
	add("speech_a_16k_audio_24k", sa16, "--app audio --bitrate 24000")
	add("speech_a_16k_lowdelay_24k", sa16, "--app lowdelay --bitrate 24000")
	add("speech_b_8k_audio_12k", clip("speech_b", 8000, 1), "--app audio --bitrate 12000")

	// ---- CELT / music paths ------------------------------------------------
	p48 := clip("piano", 48000, 2)
	for _, br := range []string{"24000", "48000", "64000", "96000", "128000", "192000", "256000", "max"} {
		c := add("piano_48k_audio_"+br, p48, "--app audio --bitrate "+br)
		if br == "64000" {
			c.decs = []decVariant{loss(10, 1), loss(40, 2), atRate(8000), atRate(16000), atRate(24000), atCh(1), gain, gainSched, asFloat, decReset}
			c.info = true
		}
	}
	for _, fs := range []string{"2.5", "5", "10", "20", "40", "60"} {
		c := add("piano_48k_audio_fs"+fs, p48, "--app audio --bitrate 64000 --framesize "+fs, loss(10, 3))
		if fs == "40" || fs == "60" {
			c.info = true
			c.repack = []string{"split"}
		}
		if fs == "10" {
			c.repack = []string{"merge2", "merge6"}
		}
		if fs == "20" {
			c.repack = []string{"merge3"}
		}
		if fs == "2.5" {
			c.repack = []string{"merge8"}
		}
	}
	add("piano_48k_audio_cbr", p48, "--app audio --bitrate 64000 --cbr", loss(10, 4))
	add("piano_48k_audio_cvbr", p48, "--app audio --bitrate 64000 --cvbr")
	add("piano_48k_audio_cbr_fs10", p48, "--app audio --bitrate 96000 --cbr --framesize 10")
	add("piano_48k_audio_cplx0", p48, "--app audio --bitrate 64000 --complexity 0")
	add("piano_48k_audio_cplx5", p48, "--app audio --bitrate 64000 --complexity 5")
	add("piano_48k_audio_forcemono", p48, "--app audio --bitrate 64000 --forcech 1")
	for _, bw := range []string{"NB", "MB", "WB", "SWB"} {
		add("piano_48k_audio_bw"+bw, p48, "--app audio --bitrate 64000 --bw "+bw)
	}
	add("piano_48k_audio_lsb8", p48, "--app audio --bitrate 64000 --lsbdepth 8")
	add("piano_48k_audio_lsb24", p48, "--app audio --bitrate 64000 --lsbdepth 24")
	add("piano_48k_audio_sigvoice", p48, "--app audio --bitrate 48000 --signal voice")
	add("piano_48k_voip_32k", p48, "--app voip --bitrate 32000")
	add("piano_48k_voip_64k", p48, "--app voip --bitrate 64000")
	add("piano_24k_audio_48k", clip("piano", 24000, 2), "--app audio --bitrate 48000", atRate(48000))
	add("piano_16k_audio_32k", clip("piano", 16000, 2), "--app audio --bitrate 32000")
	add("piano_12k_audio_24k", clip("piano", 12000, 2), "--app audio --bitrate 24000")
	add("piano_8k_audio_16k", clip("piano", 8000, 2), "--app audio --bitrate 16000")
	add("piano_48k_audio_mono_64k", clip("piano", 48000, 1), "--app audio --bitrate 64000", atCh(2))
	add("piano_48k_audio_mono_32k_fs10", clip("piano", 48000, 1), "--app audio --bitrate 32000 --framesize 10")
	add("aria_48k_audio_64k", clip("aria", 48000, 2), "--app audio --bitrate 64000", loss(10, 5))
	add("aria_48k_audio_32k", clip("aria", 48000, 2), "--app audio --bitrate 32000")
	add("aria_48k_audio_dtx", clip("aria", 48000, 2), "--app audio --bitrate 48000 --dtx")
	add("aria_48k_voip_dtx", clip("aria", 48000, 2), "--app voip --bitrate 24000 --dtx")
	add("mix_48k_audio_64k", clip("mix", 48000, 2), "--app audio --bitrate 64000", loss(10, 6), atCh(1))
	add("mix_48k_audio_32k", clip("mix", 48000, 2), "--app audio --bitrate 32000")
	add("mix_48k_voip_24k", clip("mix", 48000, 2), "--app voip --bitrate 24000")
	add("mix_48k_voip_48k", clip("mix", 48000, 2), "--app voip --bitrate 48000")
	add("mix_48k_voip_48k_fec", clip("mix", 48000, 2), "--app voip --bitrate 48000 --fec --lossperc 15", fecloss(15, 7))
	add("mix_48k_audio_forcemono", clip("mix", 48000, 2), "--app audio --bitrate 48000 --forcech 1")
	add("mix_48k_audio_forcestereo_mono", clip("mix", 48000, 1), "--app audio --bitrate 48000 --forcech 1")
	add("mix_16k_voip_20k", clip("mix", 16000, 2), "--app voip --bitrate 20000")
	add("mix_8k_voip_14k", clip("mix", 8000, 2), "--app voip --bitrate 14000")
	for _, fs := range []string{"2.5", "5", "10", "20"} {
		add("piano_48k_lowdelay_fs"+fs, p48, "--app lowdelay --bitrate 96000 --framesize "+fs)
	}
	add("piano_48k_lowdelay_64k", p48, "--app lowdelay --bitrate 64000", loss(10, 7), atRate(16000))
	add("piano_48k_lowdelay_cbr_fs5", p48, "--app lowdelay --bitrate 128000 --cbr --framesize 5")
	add("speech_a_48k_lowdelay_mono_32k", clip("speech_a", 48000, 1), "--app lowdelay --bitrate 32000")
	add("piano_24k_lowdelay_48k", clip("piano", 24000, 2), "--app lowdelay --bitrate 48000")
	add("piano_8k_lowdelay_24k", clip("piano", 8000, 1), "--app lowdelay --bitrate 24000")

	// ---- mode lists from opus_demo (forced modes, all frame sizes, both channel counts)
	m48 := clip("mix", 48000, 2)
	for _, ml := range []string{"silk8", "silk12", "silk16", "hybrid24", "hybrid48", "celt", "celthq"} {
		c := add("mix_48k_modelist_"+ml+"_32k", m48, "--app audio --bitrate 32000 --mode-list "+ml, loss(10, 8))
		c.info = true
		add("mix_48k_modelist_"+ml+"_64k", m48, "--app voip --bitrate 64000 --mode-list "+ml)
		add("piano_48k_modelist_"+ml+"_96k", p48, "--app audio --bitrate 96000 --mode-list "+ml)
	}
	add("mix_48k_force_silk", m48, "--app audio --bitrate 48000 --mode silk")
	add("mix_48k_force_hybrid", m48, "--app audio --bitrate 48000 --mode hybrid")
	add("mix_48k_force_celt", m48, "--app audio --bitrate 48000 --mode celt", loss(10, 9))
	add("speech_a_16k_force_celt", sa16, "--app voip --bitrate 24000 --mode celt")
	add("speech_a_48k_force_hybrid_fec", clip("speech_a", 48000, 1), "--app voip --bitrate 32000 --mode hybrid --fec --lossperc 10", fecloss(10, 10))

	// ---- scheduled control changes: transitions are where the bugs live ----
	sch := func(name string, in input, base, sched string, decs ...decVariant) {
		cs = append(cs, encCase{name: name, in: in, args: append(a(base), "--sched", sched), decs: decs})
	}
	sch("mix_48k_sched_modes", m48, "--app audio --bitrate 48000", "f=25:mode=silk,f=50:mode=hybrid,f=75:mode=celt,f=100:mode=silk,f=125:mode=celt,f=150:mode=hybrid,f=175:mode=auto", loss(10, 11))
	sch("mix_48k_sched_modes_fs10", m48, "--app audio --bitrate 48000 --framesize 10", "f=30:mode=silk,f=60:mode=celt,f=90:mode=hybrid,f=120:mode=celt,f=150:mode=silk,f=180:mode=auto")
	sch("mix_48k_sched_bw", m48, "--app audio --bitrate 48000", "f=20:bw=NB,f=40:bw=WB,f=60:bw=SWB,f=80:bw=FB,f=100:bw=MB,f=120:bw=auto,f=140:bw=NB,f=160:bw=FB")
	sch("mix_48k_sched_bw_voip", m48, "--app voip --bitrate 32000", "f=20:bw=NB,f=40:bw=WB,f=60:bw=SWB,f=80:bw=FB,f=100:bw=MB,f=120:bw=auto")
	sch("mix_48k_sched_bitrate", m48, "--app audio --bitrate 48000", "f=10:bitrate=8000,f=30:bitrate=16000,f=50:bitrate=128000,f=70:bitrate=6000,f=90:bitrate=max,f=110:bitrate=24000,f=130:bitrate=300000,f=150:bitrate=12000")
	sch("speech_a_48k_sched_bitrate", clip("speech_a", 48000, 1), "--app voip --bitrate 24000", "f=10:bitrate=6000,f=30:bitrate=12000,f=50:bitrate=20000,f=70:bitrate=40000,f=90:bitrate=96000,f=110:bitrate=8000,f=130:bitrate=max")
	sch("mix_48k_sched_forcech", m48, "--app audio --bitrate 48000", "f=30:forcech=1,f=60:forcech=2,f=90:forcech=auto,f=120:forcech=1,f=150:forcech=auto", loss(10, 12))
	sch("mix_48k_sched_forcech_voip", m48, "--app voip --bitrate 32000", "f=30:forcech=1,f=60:forcech=2,f=90:forcech=auto,f=120:forcech=1")
	sch("mix_48k_sched_framesize", m48, "--app audio --bitrate 64000", "f=20:framesize=10,f=40:framesize=2.5,f=60:framesize=60,f=80:framesize=5,f=100:framesize=40,f=120:framesize=20,f=140:framesize=2.5,f=160:framesize=60")
	sch("speech_a_16k_sched_framesize", sa16, "--app voip --bitrate 24000", "f=20:framesize=10,f=40:framesize=40,f=60:framesize=60,f=80:framesize=5,f=100:framesize=20,f=120:framesize=2.5,f=140:framesize=60")
	sch("mix_48k_sched_reset", m48, "--app audio --bitrate 48000", "f=50:reset=1,f=51:reset=1,f=120:reset=1", decReset)
	sch("speech_a_16k_sched_reset", sa16, "--app voip --bitrate 24000", "f=40:reset=1,f=41:reset=1,f=100:reset=1", decReset)
	// The application can only change before the first frame, i.e. in the same frame as a reset.
	sch("mix_48k_sched_app", m48, "--app audio --bitrate 48000", "f=40:reset=1,f=40:app=voip,f=80:reset=1,f=80:app=lowdelay,f=120:reset=1,f=120:app=audio,f=160:reset=1,f=160:app=voip")
	sch("mix_48k_sched_fec", m48, "--app voip --bitrate 32000", "f=20:fec=1,f=40:lossperc=20,f=60:fec=0,f=80:fec=1,f=100:lossperc=50,f=120:lossperc=0,f=140:fec=0", fecloss(20, 13), loss(20, 13))
	sch("speech_a_16k_sched_fec", sa16, "--app voip --bitrate 24000", "f=20:fec=1,f=40:lossperc=20,f=60:fec=0,f=80:fec=1,f=100:lossperc=50,f=120:fec=0", fecloss(20, 14))
	sch("speech_a_16k_sched_dtx", sa16, "--app voip --bitrate 24000", "f=30:dtx=1,f=90:dtx=0,f=150:dtx=1")
	sch("mix_48k_sched_cplx", m48, "--app audio --bitrate 48000", "f=20:complexity=0,f=40:complexity=3,f=60:complexity=10,f=80:complexity=7,f=100:complexity=1")
	sch("speech_a_16k_sched_cplx", sa16, "--app voip --bitrate 24000", "f=20:complexity=0,f=40:complexity=3,f=60:complexity=10,f=80:complexity=6,f=100:complexity=1")
	sch("mix_48k_sched_signal", m48, "--app audio --bitrate 40000", "f=30:signal=voice,f=60:signal=music,f=90:signal=auto,f=120:signal=voice")
	sch("mix_48k_sched_vbr", m48, "--app audio --bitrate 48000", "f=30:vbr=0,f=60:cvbr=1,f=90:vbr=1,f=120:cvbr=0,f=150:vbr=0")
	sch("speech_a_16k_sched_vbr", sa16, "--app voip --bitrate 24000", "f=30:vbr=0,f=60:cvbr=1,f=90:vbr=1,f=120:vbr=0")
	sch("mix_48k_sched_maxbw", m48, "--app audio --bitrate 64000", "f=20:maxbw=NB,f=40:maxbw=WB,f=60:maxbw=FB,f=80:maxbw=MB,f=100:maxbw=SWB")
	sch("mix_48k_sched_lsb", m48, "--app audio --bitrate 64000", "f=30:lsbdepth=8,f=60:lsbdepth=24,f=90:lsbdepth=16")
	sch("mix_48k_sched_everything", m48, "--app audio --bitrate 48000", "f=10:mode=silk,f=20:bw=WB,f=30:bitrate=12000,f=40:mode=celt,f=50:framesize=10,f=60:forcech=1,f=70:mode=hybrid,f=80:framesize=20,f=90:bitrate=64000,f=100:bw=auto,f=110:mode=auto,f=120:forcech=auto,f=130:reset=1,f=130:app=voip,f=150:fec=1,f=160:lossperc=30", loss(10, 15))
	sch("speech_a_16k_sched_silk_celt", sa16, "--app voip --bitrate 24000 --fec --lossperc 10", "f=30:mode=celt,f=60:mode=silk,f=90:mode=celt,f=120:mode=auto", fecloss(10, 16))
	sch("speech_a_48k_sched_silk_hybrid", clip("speech_a", 48000, 1), "--app voip --bitrate 28000", "f=30:bw=WB,f=60:bw=FB,f=90:mode=silk,f=120:mode=hybrid,f=150:bw=NB,f=180:mode=auto,f=210:bw=auto")
	add("mix_48k_randfs_sweep", m48, "--app audio --bitrate 20000 --random-framesize --seed 5 --sweep 2000 --sweep-max 120000", loss(10, 17))
	add("mix_48k_randfs_voip", m48, "--app voip --bitrate 24000 --random-framesize --seed 9")
	add("speech_a_48k_randfs_randfec", clip("speech_a", 48000, 1), "--app voip --bitrate 24000 --random-framesize --random-fec --seed 11", fecloss(10, 18))
	add("speech_a_16k_randfec", sa16, "--app voip --bitrate 24000 --random-fec --seed 13", fecloss(20, 19))
	add("speech_a_16k_sweep", sa16, "--app voip --bitrate 6000 --sweep 500 --sweep-max 60000")
	add("piano_48k_sweep", p48, "--app audio --bitrate 8000 --sweep 3000 --sweep-max 256000")
	add("piano_48k_sweep_cbr", p48, "--app audio --bitrate 8000 --sweep 3000 --sweep-max 256000 --cbr")

	// ---- libopus 1.1: expert frame duration, prediction disabled, padding ----
	for _, fd := range []string{"2.5", "5", "10", "20", "40", "60", "80", "100", "120"} {
		n := strings.ReplaceAll(fd, ".", "p")
		c := add("mix_48k_audio_framedur"+n, m48, "--app audio --bitrate 64000 --framesize 60 --framedur "+fd, loss(10, 40))
		c.info = true
		add("speech_a_16k_voip_framedur"+n, sa16, "--app voip --bitrate 24000 --framesize 60 --framedur "+fd)
		add("piano_48k_lowdelay_framedur"+n, p48, "--app lowdelay --bitrate 96000 --framesize 40 --framedur "+fd)
	}
	// libopus 1.2: 80/100/120 ms frames (multi-frame packets via the repacketizer)
	for _, fs := range []string{"80", "100", "120"} {
		c := add("mix_48k_audio_fs"+fs, m48, "--app audio --bitrate 64000 --framesize "+fs, loss(10, 42))
		c.info = true
		c.repack = []string{"split"}
		add("speech_a_16k_voip_fs"+fs, sa16, "--app voip --bitrate 24000 --framesize "+fs, fecloss(10, 43))
		add("piano_48k_audio_cbr_fs"+fs, p48, "--app audio --bitrate 96000 --cbr --framesize "+fs)
		add("mix_48k_voip_dtx_fs"+fs, m48, "--app voip --bitrate 32000 --dtx --framesize "+fs)
	}
	add("piano_48k_audio_framedur120_fs60", p48, "--app audio --bitrate 64000 --framesize 60 --framedur 120")
	sch("mix_48k_sched_framedur", m48, "--app audio --bitrate 64000 --framesize 120", "f=10:framedur=10,f=30:framedur=120,f=60:framedur=60,f=80:framedur=2.5,f=100:framedur=arg,f=120:framedur=40,f=140:framedur=100")
	sch("mix_48k_sched_framesize_long", m48, "--app audio --bitrate 48000 --framesize 20", "f=10:framesize=80,f=20:framesize=10,f=30:framesize=120,f=40:framesize=2.5,f=50:framesize=100,f=60:framesize=60,f=70:framesize=120", loss(10, 44))
	// libopus 1.2: phase inversion disabling (encoder and decoder side)
	add("piano_48k_audio_nophaseinv", p48, "--app audio --bitrate 64000 --nophaseinv", loss(10, 45), noPhaseInv)
	add("mix_48k_audio_nophaseinv_32k", m48, "--app audio --bitrate 32000 --nophaseinv", noPhaseInv)
	add("mix_48k_voip_nophaseinv", m48, "--app voip --bitrate 32000 --nophaseinv")
	add("piano_48k_audio_64k_decnophaseinv", p48, "--app audio --bitrate 64000", noPhaseInv, decVariant{"phaseinvsched", []string{"--sched", "f=30:phaseinv=1,f=60:phaseinv=0,f=90:phaseinv=1"}})
	sch("mix_48k_sched_phaseinv", m48, "--app audio --bitrate 48000", "f=20:phaseinv=1,f=50:phaseinv=0,f=80:phaseinv=1,f=110:bitrate=24000,f=140:phaseinv=0")
	add("mix_48k_audio_nopred", m48, "--app audio --bitrate 64000 --nopred", loss(10, 41))
	add("piano_48k_audio_nopred_fs10", p48, "--app audio --bitrate 96000 --framesize 10 --nopred")
	add("speech_a_16k_voip_nopred", sa16, "--app voip --bitrate 24000 --nopred")
	add("mix_48k_lowdelay_nopred", m48, "--app lowdelay --bitrate 64000 --nopred")
	sch("mix_48k_sched_pred", m48, "--app audio --bitrate 48000", "f=20:pred=1,f=50:pred=0,f=80:pred=1,f=110:mode=silk,f=140:pred=0")
	{
		c := add("mix_48k_audio_pad", m48, "--app audio --bitrate 48000")
		c.repack = []string{"pad1", "pad7", "pad300"}
		c = add("speech_a_16k_voip_pad_fs40", sa16, "--app voip --bitrate 24000 --framesize 40")
		c.repack = []string{"pad2", "pad260", "split", "merge3"}
		c = add("mix_48k_audio_pad_fs10_cbr", m48, "--app audio --bitrate 64000 --framesize 10 --cbr")
		c.repack = []string{"pad5", "merge2", "merge6"}
	}

	// ---- synthetic signals -------------------------------------------------
	for _, k := range []string{"silence", "dc", "sine", "sine:50", "sine:19000", "multitone", "sweep", "noise", "pink", "impulses", "impulses:40", "square", "clip", "bursts", "lowlevel", "fade"} {
		n := strings.ReplaceAll(k, ":", "")
		add("synth_"+n+"_48k_audio_st", synth(k, 48000, 2), "--app audio --bitrate 64000", loss(10, 20))
		add("synth_"+n+"_48k_audio_mono", synth(k, 48000, 1), "--app audio --bitrate 48000")
		add("synth_"+n+"_16k_voip", synth(k, 16000, 1), "--app voip --bitrate 20000", loss(10, 21))
	}
	for _, k := range []string{"silence", "lowlevel", "bursts", "fade"} {
		add("synth_"+k+"_16k_voip_dtx", synth(k, 16000, 1), "--app voip --bitrate 20000 --dtx")
		add("synth_"+k+"_8k_voip_dtx", synth(k, 8000, 1), "--app voip --bitrate 10000 --dtx")
		add("synth_"+k+"_48k_audio_dtx", synth(k, 48000, 2), "--app audio --bitrate 48000 --dtx")
		add("synth_"+k+"_48k_voip_dtx_fec", synth(k, 48000, 1), "--app voip --bitrate 24000 --dtx --fec --lossperc 20", fecloss(20, 22))
	}
	add("synth_impulses_48k_lowdelay_fs2p5", synth("impulses:40", 48000, 2), "--app lowdelay --bitrate 128000 --framesize 2.5")
	add("synth_sweep_48k_audio_max", synth("sweep", 48000, 2), "--app audio --bitrate max")
	add("synth_noise_48k_audio_cbr_6k", synth("noise", 48000, 2), "--app audio --bitrate 6000 --cbr")
	add("synth_clip_48k_voip", synth("clip", 48000, 2), "--app voip --bitrate 32000")
	add("synth_clip_8k_voip", synth("clip", 8000, 1), "--app voip --bitrate 12000")

	// ---- multistream / surround --------------------------------------------
	ms := func(name string, in input, args string, decArgs string, extra ...decVariant) {
		d := []decVariant{{name: "plain", args: a(decArgs)}}
		for _, e := range extra {
			d = append(d, decVariant{e.name, append(append([]string{}, a(decArgs)...), e.args...)})
		}
		cs = append(cs, encCase{name: name, in: in, args: a(args), decs: d, info: true})
	}
	ms("ms_mono_1s", synth("bursts", 48000, 1), "--app audio --bitrate 32000 --streams 1 --coupled 0 --mapping 0", "--streams 1 --coupled 0 --mapping 0", loss(10, 30))
	ms("ms_stereo_coupled", synth("multitone", 48000, 2), "--app audio --bitrate 64000 --streams 1 --coupled 1 --mapping 0,1", "--streams 1 --coupled 1 --mapping 0,1", loss(10, 31))
	ms("ms_stereo_2mono", synth("multitone", 48000, 2), "--app audio --bitrate 64000 --streams 2 --coupled 0 --mapping 0,1", "--streams 2 --coupled 0 --mapping 0,1")
	ms("ms_stereo_swapped", synth("sweep", 48000, 2), "--app audio --bitrate 64000 --streams 1 --coupled 1 --mapping 1,0", "--streams 1 --coupled 1 --mapping 1,0")
	ms("ms_3ch", synth("bursts", 48000, 3), "--app audio --bitrate 96000 --streams 2 --coupled 1 --mapping 0,1,2", "--streams 2 --coupled 1 --mapping 0,1,2", loss(10, 32))
	ms("ms_3ch_omit", synth("bursts", 48000, 3), "--app audio --bitrate 64000 --streams 1 --coupled 1 --mapping 0,255,1", "--streams 1 --coupled 1 --mapping 0,255,1")
	ms("ms_4ch", synth("multitone", 48000, 4), "--app audio --bitrate 128000 --streams 2 --coupled 2 --mapping 0,1,2,3", "--streams 2 --coupled 2 --mapping 0,1,2,3")
	ms("ms_51", synth("pink", 48000, 6), "--app audio --bitrate 192000 --streams 4 --coupled 2 --mapping 0,4,1,2,3,5", "--streams 4 --coupled 2 --mapping 0,4,1,2,3,5", loss(10, 33))
	ms("ms_51_voip_fs10", synth("bursts", 48000, 6), "--app voip --bitrate 160000 --framesize 10 --streams 4 --coupled 2 --mapping 0,4,1,2,3,5", "--streams 4 --coupled 2 --mapping 0,4,1,2,3,5")
	ms("ms_51_16k", synth("bursts", 16000, 6), "--app audio --bitrate 96000 --streams 4 --coupled 2 --mapping 0,4,1,2,3,5", "--streams 4 --coupled 2 --mapping 0,4,1,2,3,5")
	ms("ms_8ch", synth("sweep", 48000, 8), "--app audio --bitrate 256000 --streams 5 --coupled 3 --mapping 0,6,1,2,3,4,5,7", "--streams 5 --coupled 3 --mapping 0,6,1,2,3,4,5,7")
	msPad := func(name string, pads ...string) {
		for i := range cs {
			if cs[i].name == name {
				cs[i].repack = append(cs[i].repack, pads...)
			}
		}
	}
	ms("ms_51_cbr", synth("pink", 48000, 6), "--app audio --bitrate 192000 --cbr --streams 4 --coupled 2 --mapping 0,4,1,2,3,5", "--streams 4 --coupled 2 --mapping 0,4,1,2,3,5")
	ms("ms_51_lowdelay_fs5", synth("impulses", 48000, 6), "--app lowdelay --bitrate 256000 --framesize 5 --streams 4 --coupled 2 --mapping 0,4,1,2,3,5", "--streams 4 --coupled 2 --mapping 0,4,1,2,3,5")
	// Surround encoder (mapping family 1 picks streams/coupled/mapping itself; the
	// trace header records what it chose, and the decoder uses the standard layouts).
	famMaps := map[int]string{
		1: "--streams 1 --coupled 0 --mapping 0",
		2: "--streams 1 --coupled 1 --mapping 0,1",
		3: "--streams 2 --coupled 1 --mapping 0,2,1",
		4: "--streams 2 --coupled 2 --mapping 0,1,2,3",
		5: "--streams 3 --coupled 2 --mapping 0,4,1,2,3",
		6: "--streams 4 --coupled 2 --mapping 0,4,1,2,3,5",
		7: "--streams 4 --coupled 4 --mapping 0,4,1,2,3,5,6",
		8: "--streams 5 --coupled 3 --mapping 0,6,1,2,3,4,5,7",
	}
	for ch := 1; ch <= 8; ch++ {
		ms(fmt.Sprintf("surround_fam1_%dch", ch), synth("bursts", 48000, ch), fmt.Sprintf("--app audio --bitrate %d --family 1", 32000*ch), famMaps[ch], loss(10, 40+ch))
	}
	ms("surround_fam0_mono", synth("sine", 48000, 1), "--app voip --bitrate 24000 --family 0", "--streams 1 --coupled 0 --mapping 0")
	ms("surround_fam0_stereo", synth("multitone", 48000, 2), "--app audio --bitrate 64000 --family 0", "--streams 1 --coupled 1 --mapping 0,1")
	ms("surround_fam1_6ch_16k", synth("bursts", 16000, 6), "--app audio --bitrate 128000 --family 1", famMaps[6])
	ms("surround_fam1_6ch_voip_fs40", synth("bursts", 48000, 6), "--app voip --bitrate 128000 --framesize 40 --family 1", famMaps[6])

	msPad("ms_stereo_coupled", "pad3", "pad200")
	msPad("ms_51", "pad9")
	msPad("ms_3ch_omit", "pad40")
	return cs
}

// ---- execution --------------------------------------------------------------

type job struct {
	c   testvec.Case
	dep string // name of the case whose output this one consumes ("" if none)
}

func main() {
	flag.Parse()
	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	out := *flagOut
	if out == "" {
		if *flagExtended {
			out = "testdata/generated"
		} else {
			out = "testdata/corpus"
		}
	}
	gen, err := filepath.Abs(*flagGen)
	if err != nil {
		log.Fatal(err)
	}
	ver, err := exec.Command(gen, "version").Output()
	if err != nil {
		log.Fatalf("oracle driver %s: %v (run `make -C tools/corpus gen` first)", gen, err)
	}
	for _, d := range []string{"in", "enc", "dec", "info", "repack", "vec"} {
		if err := os.MkdirAll(filepath.Join(out, d), 0o755); err != nil {
			log.Fatal(err)
		}
	}
	if *flagExtended {
		os.MkdirAll(filepath.Join(out, "pcm"), 0o755)
	}

	scratch, err := filepath.Abs(*flagScratch)
	if err != nil {
		log.Fatal(err)
	}
	os.MkdirAll(filepath.Join(scratch, "in"), 0o755)
	cs := cases()
	// 1. inputs
	inputs := map[string]input{}
	for _, c := range cs {
		inputs[c.in.file()] = c.in
	}
	log.Printf("%d cases, %d distinct inputs", len(cs), len(inputs))
	prepareInputs(root, out, scratch, inputs)

	// 2. build manifest-level case list.
	man := &testvec.Manifest{Oracle: strings.TrimSpace(string(ver))}
	var first, second, third []testvec.Case // each stage depends on the previous one's outputs
	for _, c := range cs {
		if *flagFilter != "" && !strings.Contains(c.name, *flagFilter) {
			continue
		}
		encArgs := append([]string{"--rate", strconv.Itoa(c.in.rate), "--ch", strconv.Itoa(c.in.ch)}, c.args...)
		pkt := "enc/" + c.name + ".pkt"
		ec := testvec.Case{Name: "enc/" + c.name, Kind: "enc", Args: encArgs, OutPkt: pkt, Trace: "enc/" + c.name + ".trace.gz"}
		if c.in.synth() {
			ec.Synth = strings.TrimPrefix(c.in.src, "synth:")
			ec.SynthSecs = c.in.secs()
		} else {
			ec.In = c.in.file()
		}
		first = append(first, ec)
		decs := append([]decVariant{plain}, c.decs...)
		if len(c.decs) > 0 && c.decs[0].name == "plain" { // multistream cases carry their own plain variant
			decs = c.decs
		}
		for _, d := range decs {
			rate, ch := c.in.rate, c.in.ch
			args := []string{}
			rest := []string{}
			for i := 0; i < len(d.args); i++ {
				switch d.args[i] {
				case "--rate":
					rate, _ = strconv.Atoi(d.args[i+1])
					i++
				case "--ch":
					ch, _ = strconv.Atoi(d.args[i+1])
					i++
				default:
					rest = append(rest, d.args[i])
				}
			}
			args = append(args, "--rate", strconv.Itoa(rate), "--ch", strconv.Itoa(ch))
			args = append(args, rest...)
			n := "dec/" + c.name + "." + d.name
			second = append(second, testvec.Case{Name: n, Kind: "dec", Args: args, Pkt: pkt, Trace: n + ".trace.gz"})
		}
		if c.info {
			n := "info/" + c.name
			second = append(second, testvec.Case{Name: n, Kind: "info", Args: []string{"--rate", strconv.Itoa(c.in.rate)}, Pkt: pkt, Trace: n + ".trace.gz"})
		}
		for _, r := range c.repack {
			var args []string
			switch {
			case r == "split":
				args = []string{"--split"}
			case strings.HasPrefix(r, "pad"):
				args = []string{"--pad", strings.TrimPrefix(r, "pad")}
				for i, a := range c.args { // multistream: pad the last stream
					if a == "--streams" && i+1 < len(c.args) {
						args = append(args, "--streams", c.args[i+1])
					}
				}
			default:
				args = []string{"--merge", strings.TrimPrefix(r, "merge")}
			}
			n := "repack/" + c.name + "." + r
			second = append(second, testvec.Case{Name: n, Kind: "repack", Args: args, Pkt: pkt, OutPkt: n + ".pkt", Trace: n + ".trace.gz"})
			// and decode the repacketized stream
			dn := "dec/" + c.name + "." + r
			dargs := []string{"--rate", strconv.Itoa(c.in.rate), "--ch", strconv.Itoa(c.in.ch)}
			for _, d := range c.decs { // multistream layouts live in the plain decoder variant
				if d.name == "plain" {
					dargs = append(dargs, d.args...)
				}
			}
			third = append(third, testvec.Case{Name: dn, Kind: "dec", Args: dargs, Pkt: n + ".pkt", Trace: dn + ".trace.gz"})
		}
	}
	// RFC 6716 decoder vectors.
	for i := 1; i <= 12; i++ {
		bit := fmt.Sprintf("../vectors/rfc6716/testvector%02d.bit", i)
		if *flagFilter == "" || strings.Contains(*flagFilter, "vec") {
			for _, rc := range [][2]int{{48000, 2}, {48000, 1}, {16000, 1}, {8000, 2}} {
				n := fmt.Sprintf("vec/testvector%02d_%dk_%dch", i, rc[0]/1000, rc[1])
				first = append(first, testvec.Case{Name: n, Kind: "dec", Args: []string{"--rate", strconv.Itoa(rc[0]), "--ch", strconv.Itoa(rc[1])}, Pkt: bit, Trace: n + ".trace.gz"})
			}
			n := fmt.Sprintf("vec/testvector%02d_48k_2ch_loss", i)
			first = append(first, testvec.Case{Name: n, Kind: "dec", Args: []string{"--rate", "48000", "--ch", "2", "--lossperc", "10", "--seed", strconv.Itoa(100 + i)}, Pkt: bit, Trace: n + ".trace.gz"})
			n = fmt.Sprintf("vec/testvector%02d_info", i)
			first = append(first, testvec.Case{Name: n, Kind: "info", Args: []string{"--rate", "48000"}, Pkt: bit, Trace: n + ".trace.gz"})
		}
	}

	// 3. run
	run := func(list []testvec.Case) {
		var wg sync.WaitGroup
		sem := make(chan struct{}, *flagJobs)
		var mu sync.Mutex
		var failed []string
		for i := range list {
			wg.Add(1)
			sem <- struct{}{}
			go func(c *testvec.Case) {
				defer wg.Done()
				defer func() { <-sem }()
				if err := runCase(gen, out, scratch, c); err != nil {
					mu.Lock()
					failed = append(failed, c.Name+": "+err.Error())
					mu.Unlock()
				}
			}(&list[i])
		}
		wg.Wait()
		if len(failed) > 0 {
			sort.Strings(failed)
			log.Fatalf("%d cases failed:\n%s", len(failed), strings.Join(failed, "\n"))
		}
	}
	run(first)
	run(second)
	run(third)
	man.Cases = append(append(append(man.Cases, first...), second...), third...)
	sort.Slice(man.Cases, func(i, j int) bool { return man.Cases[i].Name < man.Cases[j].Name })
	if err := man.Save(out); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %d cases to %s", len(man.Cases), out)
}

func prepareInputs(root, out, scratch string, inputs map[string]input) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, *flagJobs)
	for file, in := range inputs {
		path := filepath.Join(out, file)
		if in.synth() {
			path = filepath.Join(scratch, file)
		}
		if st, err := os.Stat(path); err == nil && st.Size() > 0 {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(path string, in input) {
			defer wg.Done()
			defer func() { <-sem }()
			if strings.HasPrefix(in.src, "synth:") {
				pcm, err := testvec.Synth(strings.TrimPrefix(in.src, "synth:"), in.rate, in.ch, in.secs())
				if err != nil {
					log.Fatal(err)
				}
				if err := os.WriteFile(path, testvec.S16Bytes(pcm), 0o644); err != nil {
					log.Fatal(err)
				}
				return
			}
			src := filepath.Join(root, "testdata/clips", strings.TrimPrefix(in.src, "clip:")+".flac")
			cmd := exec.Command("ffmpeg", "-v", "error", "-y", "-i", src, "-t", fmt.Sprint(in.secs()),
				"-ac", strconv.Itoa(in.ch), "-ar", strconv.Itoa(in.rate), "-af", "aresample=resampler=swr:filter_size=64:cutoff=0.95",
				"-f", "s16le", "-acodec", "pcm_s16le", path)
			if b, err := cmd.CombinedOutput(); err != nil {
				log.Fatalf("ffmpeg %s: %v\n%s", in.src, err, b)
			}
		}(path, in)
	}
	wg.Wait()
}

func runCase(gen, out, scratch string, c *testvec.Case) error {
	tmpTrace := filepath.Join(out, c.Trace+".tmp")
	var args []string
	switch c.Kind {
	case "enc":
		args = append([]string{"enc"}, c.Args...)
		in := filepath.Join(out, c.In)
		if c.Synth != "" {
			rate, ch := argInt(c.Args, "--rate"), argInt(c.Args, "--ch")
			in = filepath.Join(scratch, input{"synth:" + c.Synth, rate, ch}.file())
		}
		args = append(args, in, filepath.Join(out, c.OutPkt), tmpTrace)
	case "dec":
		args = append([]string{"dec"}, c.Args...)
		pcm := filepath.Join(out, "pcm", strings.TrimPrefix(c.Name, "dec/")+".s16")
		if !*flagExtended {
			pcm = tmpTrace + ".pcm"
		} else {
			os.MkdirAll(filepath.Dir(pcm), 0o755)
		}
		args = append(args, filepath.Join(out, c.Pkt), tmpTrace, pcm)
		defer func() {
			if b, err := os.ReadFile(pcm); err == nil {
				h := sha256.Sum256(b)
				c.PcmSHA = hex.EncodeToString(h[:])
				c.PcmBytes = int64(len(b))
				if *flagExtended {
					c.OutPcm = "pcm/" + strings.TrimPrefix(c.Name, "dec/") + ".s16"
				} else {
					os.Remove(pcm)
				}
			}
		}()
	case "info":
		args = append([]string{"info"}, c.Args...)
		args = append(args, filepath.Join(out, c.Pkt), tmpTrace)
	case "repack":
		args = append([]string{"repack"}, c.Args...)
		args = append(args, filepath.Join(out, c.Pkt), filepath.Join(out, c.OutPkt), tmpTrace)
	}
	cmd := exec.Command(gen, args...)
	if b, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %v\n%s", gen, strings.Join(args, " "), err, b)
	}
	lines, err := testvec.ReadTrace(tmpTrace)
	if err != nil {
		return err
	}
	os.Remove(tmpTrace)
	return testvec.WriteTrace(filepath.Join(out, c.Trace), lines)
}

func argInt(args []string, key string) int {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key {
			v, _ := strconv.Atoi(args[i+1])
			return v
		}
	}
	return 0
}
