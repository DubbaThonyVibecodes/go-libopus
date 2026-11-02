// Package oracle is the Go twin of tools/corpus/cgen/gen.c: it drives the
// Go port with exactly the same command-line semantics as the C oracle
// driver and produces the same trace lines, so the conformance tests can
// diff the two.
package oracle

import (
	"fmt"
	"hash/crc32"
	"strconv"
	"strings"

	"github.com/DubbaThonyVibecodes/go-libopus/internal/testvec"
	"github.com/DubbaThonyVibecodes/go-libopus/opus"
)

const (
	maxPacket = 1500
	maxFrame  = 960 * 6
	maxCh     = 36
)

func die(format string, a ...any) error { return fmt.Errorf("oracle: "+format, a...) }

func parseBw(s string) (int32, error) {
	switch s {
	case "NB":
		return opus.OPUS_BANDWIDTH_NARROWBAND, nil
	case "MB":
		return opus.OPUS_BANDWIDTH_MEDIUMBAND, nil
	case "WB":
		return opus.OPUS_BANDWIDTH_WIDEBAND, nil
	case "SWB":
		return opus.OPUS_BANDWIDTH_SUPERWIDEBAND, nil
	case "FB":
		return opus.OPUS_BANDWIDTH_FULLBAND, nil
	case "auto":
		return opus.OPUS_AUTO, nil
	}
	return 0, die("bad bandwidth %q", s)
}

func parseApp(s string) (int32, error) {
	switch s {
	case "voip":
		return opus.OPUS_APPLICATION_VOIP, nil
	case "audio":
		return opus.OPUS_APPLICATION_AUDIO, nil
	case "lowdelay":
		return opus.OPUS_APPLICATION_RESTRICTED_LOWDELAY, nil
	}
	return 0, die("bad application %q", s)
}

func parseSignal(s string) (int32, error) {
	switch s {
	case "auto":
		return opus.OPUS_AUTO, nil
	case "voice":
		return opus.OPUS_SIGNAL_VOICE, nil
	case "music":
		return opus.OPUS_SIGNAL_MUSIC, nil
	}
	return 0, die("bad signal %q", s)
}

func parseMode(s string) (int32, error) {
	switch s {
	case "auto":
		return opus.OPUS_AUTO, nil
	case "silk":
		return opus.MODE_SILK_ONLY, nil
	case "hybrid":
		return opus.MODE_HYBRID, nil
	case "celt":
		return opus.MODE_CELT_ONLY, nil
	}
	return 0, die("bad mode %q", s)
}

func parseFramesize(s string, rate int) (int, error) {
	switch s {
	case "2.5":
		return rate / 400, nil
	case "5":
		return rate / 200, nil
	case "10":
		return rate / 100, nil
	case "20":
		return rate / 50, nil
	case "40":
		return rate / 25, nil
	case "60":
		return 3 * rate / 50, nil
	case "80":
		return 4 * rate / 50, nil
	case "100":
		return rate / 10, nil
	case "120":
		return 6 * rate / 50, nil
	}
	return 0, die("bad framesize %q", s)
}

func parseBitrate(s string) int32 {
	if s == "max" {
		return opus.OPUS_BITRATE_MAX
	}
	if s == "auto" {
		return opus.OPUS_AUTO
	}
	v, _ := strconv.Atoi(s)
	return int32(v)
}

func atoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

// Mode lists from upstream opus_demo.c: {mode, bandwidth, frame size @48k, channels}.
var silk8_test = [][4]int32{
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_NARROWBAND, 960 * 3, 1},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_NARROWBAND, 960 * 2, 1},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_NARROWBAND, 960, 1},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_NARROWBAND, 480, 1},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_NARROWBAND, 960 * 3, 2},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_NARROWBAND, 960 * 2, 2},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_NARROWBAND, 960, 2},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_NARROWBAND, 480, 2},
}
var silk12_test = [][4]int32{
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_MEDIUMBAND, 960 * 3, 1},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_MEDIUMBAND, 960 * 2, 1},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_MEDIUMBAND, 960, 1},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_MEDIUMBAND, 480, 1},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_MEDIUMBAND, 960 * 3, 2},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_MEDIUMBAND, 960 * 2, 2},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_MEDIUMBAND, 960, 2},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_MEDIUMBAND, 480, 2},
}
var silk16_test = [][4]int32{
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_WIDEBAND, 960 * 3, 1},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_WIDEBAND, 960 * 2, 1},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_WIDEBAND, 960, 1},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_WIDEBAND, 480, 1},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_WIDEBAND, 960 * 3, 2},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_WIDEBAND, 960 * 2, 2},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_WIDEBAND, 960, 2},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_WIDEBAND, 480, 2},
}
var hybrid24_test = [][4]int32{
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_SUPERWIDEBAND, 960, 1},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_SUPERWIDEBAND, 480, 1},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_SUPERWIDEBAND, 960, 2},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_SUPERWIDEBAND, 480, 2},
}
var hybrid48_test = [][4]int32{
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_FULLBAND, 960, 1},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_FULLBAND, 480, 1},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_FULLBAND, 960, 2},
	{opus.MODE_SILK_ONLY, opus.OPUS_BANDWIDTH_FULLBAND, 480, 2},
}
var celt_test = [][4]int32{
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_FULLBAND, 960, 1},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_SUPERWIDEBAND, 960, 1},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_WIDEBAND, 960, 1},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_NARROWBAND, 960, 1},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_FULLBAND, 480, 1},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_SUPERWIDEBAND, 480, 1},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_WIDEBAND, 480, 1},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_NARROWBAND, 480, 1},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_FULLBAND, 240, 1},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_SUPERWIDEBAND, 240, 1},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_WIDEBAND, 240, 1},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_NARROWBAND, 240, 1},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_FULLBAND, 120, 1},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_SUPERWIDEBAND, 120, 1},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_WIDEBAND, 120, 1},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_NARROWBAND, 120, 1},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_FULLBAND, 960, 2},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_SUPERWIDEBAND, 960, 2},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_WIDEBAND, 960, 2},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_NARROWBAND, 960, 2},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_FULLBAND, 480, 2},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_SUPERWIDEBAND, 480, 2},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_WIDEBAND, 480, 2},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_NARROWBAND, 480, 2},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_FULLBAND, 240, 2},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_SUPERWIDEBAND, 240, 2},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_WIDEBAND, 240, 2},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_NARROWBAND, 240, 2},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_FULLBAND, 120, 2},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_SUPERWIDEBAND, 120, 2},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_WIDEBAND, 120, 2},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_NARROWBAND, 120, 2},
}
var celt_hq_test = [][4]int32{
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_FULLBAND, 960, 2},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_FULLBAND, 480, 2},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_FULLBAND, 240, 2},
	{opus.MODE_CELT_ONLY, opus.OPUS_BANDWIDTH_FULLBAND, 120, 2},
}

// sched is a scheduled CTL change: at frame index `frame`, apply what=val.
type sched struct {
	frame int
	what  string
	val   int32
}

func parseFramedur(v string) (int32, error) {
	switch v {
	case "arg":
		return opus.OPUS_FRAMESIZE_ARG, nil
	case "2.5":
		return opus.OPUS_FRAMESIZE_2_5_MS, nil
	case "5":
		return opus.OPUS_FRAMESIZE_5_MS, nil
	case "10":
		return opus.OPUS_FRAMESIZE_10_MS, nil
	case "20":
		return opus.OPUS_FRAMESIZE_20_MS, nil
	case "40":
		return opus.OPUS_FRAMESIZE_40_MS, nil
	case "60":
		return opus.OPUS_FRAMESIZE_60_MS, nil
	case "80":
		return opus.OPUS_FRAMESIZE_80_MS, nil
	case "100":
		return opus.OPUS_FRAMESIZE_100_MS, nil
	case "120":
		return opus.OPUS_FRAMESIZE_120_MS, nil
	}
	return 0, die("bad frame duration %q", v)
}

func parseSchedVal(what, v string, rate int) (int32, error) {
	switch what {
	case "framedur":
		return parseFramedur(v)
	case "bw", "maxbw":
		return parseBw(v)
	case "mode":
		return parseMode(v)
	case "signal":
		return parseSignal(v)
	case "app":
		return parseApp(v)
	case "bitrate":
		return parseBitrate(v), nil
	case "framesize":
		f, err := parseFramesize(v, rate)
		return int32(f), err
	case "forcech":
		if v == "auto" {
			return opus.OPUS_AUTO, nil
		}
		return int32(atoi(v)), nil
	}
	return int32(atoi(v)), nil
}

// "f=100:bitrate=32000,f=200:bw=NB"
func parseSched(s string, rate int) ([]sched, error) {
	var out []sched
	for _, tok := range strings.Split(s, ",") {
		if tok == "" {
			continue
		}
		f, rest, ok := strings.Cut(tok, ":")
		if !ok || !strings.HasPrefix(f, "f=") {
			return nil, die("bad sched %q", tok)
		}
		what, v, ok := strings.Cut(rest, "=")
		if !ok {
			return nil, die("bad sched %q", tok)
		}
		val, err := parseSchedVal(what, v, rate)
		if err != nil {
			return nil, err
		}
		out = append(out, sched{atoi(f[2:]), what, val})
	}
	return out, nil
}

// encoder abstracts the elementary and multistream encoders (ENC_CTL in gen.c).
type encoder interface {
	Ctl(request int, args ...any) int
	encode(pcm []int16, frameSize int, data []byte, maxBytes int) int
}

type elemEnc struct{ *opus.OpusEncoder }

func (e elemEnc) encode(pcm []int16, frameSize int, data []byte, maxBytes int) int {
	return int(e.Encode(pcm, frameSize, data, int32(maxBytes)))
}

type msEnc struct{ *opus.OpusMSEncoder }

func (e msEnc) encode(pcm []int16, frameSize int, data []byte, maxBytes int) int {
	return e.MultistreamEncode(pcm, frameSize, data, int32(maxBytes))
}

type projEnc struct{ *opus.OpusProjectionEncoder }

func (e projEnc) encode(pcm []int16, frameSize int, data []byte, maxBytes int) int {
	return e.ProjectionEncode(pcm, frameSize, data, int32(maxBytes))
}

// getDemixingMatrix mirrors get_demixing_matrix in gen.c: the demixing matrix
// of the projection (family 3) encoder for this channel count.
func getDemixingMatrix(rate, ch int, app int) (matrix []byte, streams, coupled int, gain int32, err error) {
	var errc int
	pe := opus.ProjectionAmbisonicsEncoderCreate(int32(rate), ch, 3, &streams, &coupled, app, &errc)
	if errc != opus.OPUS_OK {
		return nil, 0, 0, 0, die("projection create: %s", opus.Strerror(errc))
	}
	var size int32
	if pe.Ctl(opus.OPUS_PROJECTION_GET_DEMIXING_MATRIX_SIZE_REQUEST, &size) != opus.OPUS_OK {
		return nil, 0, 0, 0, die("matrix size")
	}
	matrix = make([]byte, size)
	if pe.Ctl(opus.OPUS_PROJECTION_GET_DEMIXING_MATRIX_REQUEST, matrix, size) != opus.OPUS_OK {
		return nil, 0, 0, 0, die("matrix")
	}
	if pe.Ctl(opus.OPUS_PROJECTION_GET_DEMIXING_MATRIX_GAIN_REQUEST, &gain) != opus.OPUS_OK {
		return nil, 0, 0, 0, die("matrix gain")
	}
	return matrix, streams, coupled, gain, nil
}

func applySched(enc encoder, s *sched, frameSize *int, framedur *int32) error {
	r := opus.OPUS_OK
	switch s.what {
	case "bitrate":
		r = enc.Ctl(opus.OPUS_SET_BITRATE_REQUEST, s.val)
	case "bw":
		r = enc.Ctl(opus.OPUS_SET_BANDWIDTH_REQUEST, s.val)
	case "maxbw":
		r = enc.Ctl(opus.OPUS_SET_MAX_BANDWIDTH_REQUEST, s.val)
	case "mode":
		r = enc.Ctl(opus.OPUS_SET_FORCE_MODE_REQUEST, s.val)
	case "signal":
		r = enc.Ctl(opus.OPUS_SET_SIGNAL_REQUEST, s.val)
	case "app":
		r = enc.Ctl(opus.OPUS_SET_APPLICATION_REQUEST, s.val)
	case "complexity":
		r = enc.Ctl(opus.OPUS_SET_COMPLEXITY_REQUEST, s.val)
	case "fec":
		r = enc.Ctl(opus.OPUS_SET_INBAND_FEC_REQUEST, s.val)
	case "lossperc":
		r = enc.Ctl(opus.OPUS_SET_PACKET_LOSS_PERC_REQUEST, s.val)
	case "dtx":
		r = enc.Ctl(opus.OPUS_SET_DTX_REQUEST, s.val)
	case "vbr":
		r = enc.Ctl(opus.OPUS_SET_VBR_REQUEST, s.val)
	case "cvbr":
		r = enc.Ctl(opus.OPUS_SET_VBR_CONSTRAINT_REQUEST, s.val)
	case "forcech":
		r = enc.Ctl(opus.OPUS_SET_FORCE_CHANNELS_REQUEST, s.val)
	case "lsbdepth":
		r = enc.Ctl(opus.OPUS_SET_LSB_DEPTH_REQUEST, s.val)
	case "voiceratio":
		r = enc.Ctl(opus.OPUS_SET_VOICE_RATIO_REQUEST, s.val)
	case "reset":
		r = enc.Ctl(opus.OPUS_RESET_STATE)
	case "framedur":
		r = enc.Ctl(opus.OPUS_SET_EXPERT_FRAME_DURATION_REQUEST, s.val)
		*framedur = s.val
	case "pred":
		r = enc.Ctl(opus.OPUS_SET_PREDICTION_DISABLED_REQUEST, s.val)
	case "phaseinv":
		r = enc.Ctl(opus.OPUS_SET_PHASE_INVERSION_DISABLED_REQUEST, s.val)
	case "framesize":
		*frameSize = int(s.val)
	default:
		return die("unknown sched key %q", s.what)
	}
	if r != opus.OPUS_OK {
		return die("sched %s=%d failed: %s", s.what, s.val, opus.Strerror(r))
	}
	return nil
}

func parseMapping(v string) ([]byte, error) {
	var m []byte
	for _, t := range strings.Split(v, ",") {
		if len(m) >= maxCh {
			return nil, die("mapping too long")
		}
		m = append(m, byte(atoi(t)))
	}
	return m, nil
}

// EncResult is what gen enc produces.
type EncResult struct {
	Packets []testvec.Packet
	Trace   []string
}

// RunEnc mirrors cmd_enc in gen.c. args are the gen command-line options
// (without the positional file names).
func RunEnc(args []string, in []int16) (*EncResult, error) {
	rate, ch := 48000, 1
	app := int32(opus.OPUS_APPLICATION_AUDIO)
	bitrate := int32(64000)
	frameSize := -1
	vbr, cvbr := int32(1), int32(0)
	bw, maxbw := int32(opus.OPUS_AUTO), int32(opus.OPUS_AUTO)
	complexity := int32(10)
	fec, lossperc, dtx := int32(0), int32(0), int32(0)
	framedur, nopred, nophaseinv := int32(opus.OPUS_FRAMESIZE_ARG), int32(0), int32(0)
	proj := false
	remaining := 0
	forcech := int32(opus.OPUS_AUTO)
	sig := int32(opus.OPUS_AUTO)
	lsbdepth := int32(16)
	maxpayload := maxPacket
	mode := int32(opus.OPUS_AUTO)
	voiceratio := int32(-1)
	sweepBps, sweepMax, sweepMin := 0, 0, 0
	randomFramesize, randomFec := false, false
	var modeList [][4]int32
	var scheds []sched
	ms := false
	streams, coupled, family := 0, 0, -1
	var mapping []byte
	lcg := testvec.LCG{State: 1}

	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func() string {
			i++
			if i >= len(args) {
				return ""
			}
			return args[i]
		}
		var err error
		switch a {
		case "--rate":
			rate = atoi(next())
		case "--ch":
			ch = atoi(next())
		case "--app":
			app, err = parseApp(next())
		case "--bitrate":
			bitrate = parseBitrate(next())
		case "--framesize":
			frameSize, err = parseFramesize(next(), rate)
		case "--cbr":
			vbr = 0
		case "--cvbr":
			cvbr = 1
		case "--bw":
			bw, err = parseBw(next())
		case "--maxbw":
			maxbw, err = parseBw(next())
		case "--complexity":
			complexity = int32(atoi(next()))
		case "--fec":
			fec = 1
		case "--fec2":
			fec = 2
		case "--lossperc":
			lossperc = int32(atoi(next()))
		case "--dtx":
			dtx = 1
		case "--forcech":
			v := next()
			if v == "auto" {
				forcech = opus.OPUS_AUTO
			} else {
				forcech = int32(atoi(v))
			}
		case "--signal":
			sig, err = parseSignal(next())
		case "--mode":
			mode, err = parseMode(next())
		case "--lsbdepth":
			lsbdepth = int32(atoi(next()))
		case "--maxpayload":
			maxpayload = atoi(next())
		case "--framedur":
			framedur, err = parseFramedur(next())
		case "--nopred":
			nopred = 1
		case "--nophaseinv":
			nophaseinv = 1
		case "--voiceratio":
			voiceratio = int32(atoi(next()))
		case "--sweep":
			sweepBps = atoi(next())
		case "--sweep-max":
			sweepMax = atoi(next())
		case "--random-framesize":
			randomFramesize = true
		case "--random-fec":
			randomFec = true
		case "--seed":
			v, _ := strconv.ParseUint(next(), 0, 32)
			lcg.State = uint32(v)
		case "--sched":
			scheds, err = parseSched(next(), rate)
		case "--mode-list":
			switch next() {
			case "silk8":
				modeList = silk8_test
			case "silk12":
				modeList = silk12_test
			case "silk16":
				modeList = silk16_test
			case "hybrid24":
				modeList = hybrid24_test
			case "hybrid48":
				modeList = hybrid48_test
			case "celt":
				modeList = celt_test
			case "celthq":
				modeList = celt_hq_test
			default:
				err = die("bad mode-list")
			}
		case "--streams":
			ms = true
			streams = atoi(next())
		case "--coupled":
			ms = true
			coupled = atoi(next())
		case "--family":
			ms = true
			family = atoi(next())
		case "--projection":
			ms, proj = true, true
			family = 3
		case "--mapping":
			ms = true
			mapping, err = parseMapping(next())
		default:
			return nil, die("unknown option %q", a)
		}
		if err != nil {
			return nil, err
		}
	}
	if frameSize < 0 {
		frameSize = rate / 50
	}
	if ch > maxCh {
		return nil, die("too many channels")
	}
	if ch > 2 && !ms {
		return nil, die("channels > 2 requires multistream options")
	}

	res := &EncResult{}
	var enc encoder
	var errc int
	if proj {
		var s2, c2 int
		e := opus.ProjectionAmbisonicsEncoderCreate(int32(rate), ch, family, &s2, &c2, int(app), &errc)
		if errc != opus.OPUS_OK {
			return nil, die("projection create: %s", opus.Strerror(errc))
		}
		streams, coupled = s2, c2
		matrix, _, _, gain, err := getDemixingMatrix(rate, ch, int(app))
		if err != nil {
			return nil, err
		}
		res.Trace = append(res.Trace, fmt.Sprintf("H projection family=%d streams=%d coupled=%d gain=%d size=%d matrix=%x", family, streams, coupled, gain, len(matrix), matrix))
		enc = projEnc{e}
	} else if ms {
		if family >= 0 {
			var s2, c2 int
			map2 := make([]byte, maxCh)
			e := opus.MultistreamSurroundEncoderCreate(int32(rate), ch, family, &s2, &c2, map2, int(app), &errc)
			if errc != opus.OPUS_OK {
				return nil, die("surround create: %s", opus.Strerror(errc))
			}
			streams, coupled = s2, c2
			var sb strings.Builder
			fmt.Fprintf(&sb, "H surround family=%d streams=%d coupled=%d mapping=", family, s2, c2)
			for i := 0; i < ch; i++ {
				if i > 0 {
					sb.WriteByte(',')
				}
				fmt.Fprintf(&sb, "%d", map2[i])
			}
			res.Trace = append(res.Trace, sb.String())
			enc = msEnc{e}
		} else {
			if len(mapping) != ch {
				return nil, die("mapping length must equal channels")
			}
			e := opus.MultistreamEncoderCreate(int32(rate), ch, streams, coupled, mapping, int(app), &errc)
			if errc != opus.OPUS_OK {
				return nil, die("ms create: %s", opus.Strerror(errc))
			}
			enc = msEnc{e}
		}
	} else {
		e := opus.EncoderCreate(int32(rate), ch, int(app), &errc)
		if errc != opus.OPUS_OK {
			return nil, die("create: %s", opus.Strerror(errc))
		}
		enc = elemEnc{e}
	}
	set := func(req int, args ...any) error {
		if r := enc.Ctl(req, args...); r != opus.OPUS_OK {
			return die("ctl %d failed: %s", req, opus.Strerror(r))
		}
		return nil
	}
	steps := []func() error{
		func() error { return set(opus.OPUS_SET_BITRATE_REQUEST, bitrate) },
		func() error { return set(opus.OPUS_SET_BANDWIDTH_REQUEST, bw) },
		func() error {
			if maxbw != opus.OPUS_AUTO {
				return set(opus.OPUS_SET_MAX_BANDWIDTH_REQUEST, maxbw)
			}
			return nil
		},
		func() error { return set(opus.OPUS_SET_VBR_REQUEST, vbr) },
		func() error { return set(opus.OPUS_SET_VBR_CONSTRAINT_REQUEST, cvbr) },
		func() error { return set(opus.OPUS_SET_COMPLEXITY_REQUEST, complexity) },
		func() error { return set(opus.OPUS_SET_INBAND_FEC_REQUEST, fec) },
		func() error { return set(opus.OPUS_SET_FORCE_CHANNELS_REQUEST, forcech) },
		func() error { return set(opus.OPUS_SET_DTX_REQUEST, dtx) },
		func() error { return set(opus.OPUS_SET_PACKET_LOSS_PERC_REQUEST, lossperc) },
		func() error { return set(opus.OPUS_SET_SIGNAL_REQUEST, sig) },
		func() error { return set(opus.OPUS_SET_LSB_DEPTH_REQUEST, lsbdepth) },
		func() error {
			if mode != opus.OPUS_AUTO {
				return set(opus.OPUS_SET_FORCE_MODE_REQUEST, mode)
			}
			return nil
		},
		func() error {
			if voiceratio >= 0 && !ms {
				return set(opus.OPUS_SET_VOICE_RATIO_REQUEST, voiceratio)
			}
			return nil
		},
		func() error {
			if framedur != opus.OPUS_FRAMESIZE_ARG {
				return set(opus.OPUS_SET_EXPERT_FRAME_DURATION_REQUEST, framedur)
			}
			return nil
		},
		func() error {
			if nopred != 0 {
				return set(opus.OPUS_SET_PREDICTION_DISABLED_REQUEST, nopred)
			}
			return nil
		},
		func() error {
			if nophaseinv != 0 {
				return set(opus.OPUS_SET_PHASE_INVERSION_DISABLED_REQUEST, nophaseinv)
			}
			return nil
		},
	}
	for _, s := range steps {
		if err := s(); err != nil {
			return nil, err
		}
	}
	var lookahead int32
	if err := set(opus.OPUS_GET_LOOKAHEAD_REQUEST, &lookahead); err != nil {
		return nil, err
	}
	{
		var sb strings.Builder
		var v int32
		fmt.Fprintf(&sb, "H lookahead=%d", lookahead)
		get := func(req int, name string) error {
			if err := set(req, &v); err != nil {
				return err
			}
			fmt.Fprintf(&sb, " %s=%d", name, v)
			return nil
		}
		for _, g := range []struct {
			req  int
			name string
		}{
			{opus.OPUS_GET_SAMPLE_RATE_REQUEST, "fs"},
			{opus.OPUS_GET_BITRATE_REQUEST, "bitrate"},
			{opus.OPUS_GET_BANDWIDTH_REQUEST, "bw"},
			{opus.OPUS_GET_APPLICATION_REQUEST, "app"},
			{opus.OPUS_GET_COMPLEXITY_REQUEST, "cplx"},
			{opus.OPUS_GET_VBR_REQUEST, "vbr"},
			{opus.OPUS_GET_SIGNAL_REQUEST, "signal"},
			{opus.OPUS_GET_FORCE_CHANNELS_REQUEST, "forcech"},
			{opus.OPUS_GET_LSB_DEPTH_REQUEST, "lsb"},
			{opus.OPUS_GET_EXPERT_FRAME_DURATION_REQUEST, "framedur"},
			{opus.OPUS_GET_PREDICTION_DISABLED_REQUEST, "pred"},
			{opus.OPUS_GET_PHASE_INVERSION_DISABLED_REQUEST, "phaseinv"},
		} {
			if err := get(g.req, g.name); err != nil {
				return nil, err
			}
		}
		res.Trace = append(res.Trace, sb.String())
	}
	if sweepMax != 0 {
		sweepMin = int(bitrate)
	}
	modeSwitchTime := 0
	if modeList != nil {
		modeSwitchTime = len(in) / ch / len(modeList)
	}
	buf := make([]int16, maxFrame*ch)
	data := make([]byte, maxPacket)
	count, currMode, currModeCount := 0, 0, 0
	stop, delayedCelt, newsize := false, false, 0
	pos := 0 // read position in `in`
	bitrateCur := int(bitrate)

	for !stop {
		for s := range scheds {
			if scheds[s].frame == count {
				if err := applySched(enc, &scheds[s], &frameSize, &framedur); err != nil {
					return nil, err
				}
			}
		}
		if delayedCelt {
			frameSize = newsize
			delayedCelt = false
		} else if randomFramesize && lcg.Next()%20 == 0 {
			newsize = int(lcg.Next() % 6)
			switch newsize {
			case 0:
				newsize = rate / 400
			case 1:
				newsize = rate / 200
			case 2:
				newsize = rate / 100
			case 3:
				newsize = rate / 50
			case 4:
				newsize = rate / 25
			case 5:
				newsize = 3 * rate / 50
			}
			sb := sweepBps
			if sb < 0 {
				sb = -sb
			}
			for newsize < rate/25 && bitrateCur-sb <= 3*12*rate/newsize {
				newsize *= 2
			}
			if newsize < rate/100 && frameSize >= rate/100 {
				if err := set(opus.OPUS_SET_FORCE_MODE_REQUEST, int32(opus.MODE_CELT_ONLY)); err != nil {
					return nil, err
				}
				delayedCelt = true
			} else {
				frameSize = newsize
			}
		}
		if randomFec && lcg.Next()%30 == 0 {
			// gen.c writes OPUS_SET_INBAND_FEC(lcg()%4 == 0); the C macro
			// __opus_check_int(x) evaluates x twice (once in a discarded
			// type check, once for the value), so the oracle draws twice.
			lcg.Next()
			v := int32(0)
			if lcg.Next()%4 == 0 {
				v = 1
			}
			if err := set(opus.OPUS_SET_INBAND_FEC_REQUEST, v); err != nil {
				return nil, err
			}
		}
		if modeList != nil {
			m := modeList[currMode]
			if err := set(opus.OPUS_SET_BANDWIDTH_REQUEST, m[1]); err != nil {
				return nil, err
			}
			if err := set(opus.OPUS_SET_FORCE_MODE_REQUEST, m[0]); err != nil {
				return nil, err
			}
			if err := set(opus.OPUS_SET_FORCE_CHANNELS_REQUEST, m[3]); err != nil {
				return nil, err
			}
			frameSize = int(m[2]) * rate / 48000
		}
		// read_s16: like opus_demo the last (possibly empty) frame is zero padded and still encoded
		if remaining > frameSize {
			remaining = frameSize
		}
		n := (frameSize - remaining) * ch
		avail := len(in) - pos
		if avail > n {
			avail = n
		}
		copy(buf[remaining*ch:remaining*ch+avail], in[pos:pos+avail])
		pos += avail
		read := remaining + avail/ch
		if read < frameSize {
			clear(buf[read*ch : frameSize*ch])
			stop = true
		}
		length := enc.encode(buf[:frameSize*ch], frameSize, data, maxpayload)
		if length < 0 {
			res.Trace = append(res.Trace, fmt.Sprintf("E i=%d err=%d", count, length))
			break
		}
		if sweepBps != 0 {
			bitrateCur += sweepBps
			if sweepMax != 0 {
				if bitrateCur > sweepMax {
					sweepBps = -sweepBps
				} else if bitrateCur < sweepMin {
					sweepBps = -sweepBps
				}
			}
			if bitrateCur < 1000 {
				bitrateCur = 1000
			}
			if err := set(opus.OPUS_SET_BITRATE_REQUEST, int32(bitrateCur)); err != nil {
				return nil, err
			}
		}
		var rng uint32
		var ebw int32
		if err := set(opus.OPUS_GET_FINAL_RANGE_REQUEST, &rng); err != nil {
			return nil, err
		}
		if err := set(opus.OPUS_GET_BANDWIDTH_REQUEST, &ebw); err != nil {
			return nil, err
		}
		pkt := testvec.Packet{Data: append([]byte(nil), data[:length]...), Rng: rng}
		res.Packets = append(res.Packets, pkt)
		line := fmt.Sprintf("E i=%d fs=%d len=%d rng=%08x bw=%d", count, frameSize, length, rng, ebw)
		if length > 0 {
			line += fmt.Sprintf(" toc=%02x", data[0])
		}
		if !ms {
			var indtx int32
			if err := set(opus.OPUS_GET_IN_DTX_REQUEST, &indtx); err != nil {
				return nil, err
			}
			line += fmt.Sprintf(" indtx=%d", indtx)
		}
		remaining = 0
		if framedur != opus.OPUS_FRAMESIZE_ARG {
			// like opus_demo: the encoder may consume fewer samples than offered; keep the rest
			nbEncoded := frameSize
			if length > 1 {
				nbEncoded = int(opus.PacketGetSamplesPerFrame(data, int32(rate))) * opus.PacketGetNbFrames(data, int32(length))
			}
			if nbEncoded <= 0 || nbEncoded > frameSize {
				nbEncoded = frameSize
			}
			remaining = frameSize - nbEncoded
			copy(buf[:remaining*ch], buf[nbEncoded*ch:nbEncoded*ch+remaining*ch])
			line += fmt.Sprintf(" enc=%d", nbEncoded)
		}
		res.Trace = append(res.Trace, line)
		currModeCount += frameSize
		if modeList != nil && currModeCount > modeSwitchTime && currMode < len(modeList)-1 {
			currMode++
			currModeCount = 0
		}
		count++
	}
	return res, nil
}

// DecResult is what gen dec produces.
type DecResult struct {
	Trace []string
	Pcm   []int16 // concatenation of all decoded frames (gen writes it when an output file is given)
}

type decoder interface {
	Ctl(request int, args ...any) int
	decode(data []byte, length int, pcm []int16, frameSize int, decodeFec int) int
	decodeFloat(data []byte, length int, pcm []float32, frameSize int, decodeFec int) int
}

type elemDec struct{ *opus.OpusDecoder }

func (d elemDec) decode(data []byte, length int, pcm []int16, frameSize int, decodeFec int) int {
	return d.Decode(data, int32(length), pcm, frameSize, decodeFec)
}
func (d elemDec) decodeFloat(data []byte, length int, pcm []float32, frameSize int, decodeFec int) int {
	return d.DecodeFloat(data, int32(length), pcm, frameSize, decodeFec)
}

type msDec struct{ *opus.OpusMSDecoder }

type projDec struct{ *opus.OpusProjectionDecoder }

func (d projDec) decode(data []byte, length int, pcm []int16, frameSize int, decodeFec int) int {
	return d.ProjectionDecode(data, int32(length), pcm, frameSize, decodeFec)
}
func (d projDec) decodeFloat(data []byte, length int, pcm []float32, frameSize int, decodeFec int) int {
	return d.ProjectionDecodeFloat(data, int32(length), pcm, frameSize, decodeFec)
}

func (d msDec) decode(data []byte, length int, pcm []int16, frameSize int, decodeFec int) int {
	return d.MultistreamDecode(data, int32(length), pcm, frameSize, decodeFec)
}
func (d msDec) decodeFloat(data []byte, length int, pcm []float32, frameSize int, decodeFec int) int {
	return d.MultistreamDecodeFloat(data, int32(length), pcm, frameSize, decodeFec)
}

// RunDec mirrors cmd_dec in gen.c.
func RunDec(args []string, pkts []testvec.Packet) (*DecResult, error) {
	rate, ch := 48000, 1
	lossperc, fec, gain, skip := 0, 0, int32(0), 0
	useFloat := false
	nophaseinv := int32(0)
	ms, proj := false, false
	streams, coupled := 0, 0
	var mapping []byte
	var scheds []sched
	lcg := testvec.LCG{State: 1}

	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func() string {
			i++
			if i >= len(args) {
				return ""
			}
			return args[i]
		}
		var err error
		switch a {
		case "--rate":
			rate = atoi(next())
		case "--ch":
			ch = atoi(next())
		case "--lossperc":
			lossperc = atoi(next())
		case "--fec":
			fec = 1
		case "--gain":
			gain = int32(atoi(next()))
		case "--skip":
			skip = atoi(next())
		case "--float":
			useFloat = true
		case "--nophaseinv":
			nophaseinv = 1
		case "--seed":
			v, _ := strconv.ParseUint(next(), 0, 32)
			lcg.State = uint32(v)
		case "--sched":
			scheds, err = parseSched(next(), rate)
		case "--projection":
			ms, proj = true, true
		case "--streams":
			ms = true
			streams = atoi(next())
		case "--coupled":
			ms = true
			coupled = atoi(next())
		case "--mapping":
			ms = true
			mapping, err = parseMapping(next())
		default:
			return nil, die("unknown option %q", a)
		}
		if err != nil {
			return nil, err
		}
	}
	if ch > maxCh {
		return nil, die("too many channels")
	}
	res := &DecResult{}
	var dec decoder
	var errc int
	if proj {
		matrix, s2, c2, _, err := getDemixingMatrix(rate, ch, opus.OPUS_APPLICATION_AUDIO)
		if err != nil {
			return nil, err
		}
		streams, coupled = s2, c2
		d := opus.ProjectionDecoderCreate(int32(rate), ch, streams, coupled, matrix, int32(len(matrix)), &errc)
		if errc != opus.OPUS_OK {
			return nil, die("projection dec create: %s", opus.Strerror(errc))
		}
		dec = projDec{d}
	} else if ms {
		if len(mapping) != ch {
			return nil, die("mapping length must equal channels")
		}
		d := opus.MultistreamDecoderCreate(int32(rate), ch, streams, coupled, mapping, &errc)
		if errc != opus.OPUS_OK {
			return nil, die("ms create: %s", opus.Strerror(errc))
		}
		dec = msDec{d}
	} else {
		d := opus.DecoderCreate(int32(rate), ch, &errc)
		if errc != opus.OPUS_OK {
			return nil, die("create: %s", opus.Strerror(errc))
		}
		dec = elemDec{d}
	}
	set := func(req int, args ...any) error {
		if r := dec.Ctl(req, args...); r != opus.OPUS_OK {
			return die("dec ctl %d failed: %s", req, opus.Strerror(r))
		}
		return nil
	}
	if gain != 0 {
		if err := set(opus.OPUS_SET_GAIN_REQUEST, gain); err != nil {
			return nil, err
		}
	}
	if nophaseinv != 0 {
		if err := set(opus.OPUS_SET_PHASE_INVERSION_DISABLED_REQUEST, nophaseinv); err != nil {
			return nil, err
		}
	}
	{
		var v int32
		if err := set(opus.OPUS_GET_SAMPLE_RATE_REQUEST, &v); err != nil {
			return nil, err
		}
		line := fmt.Sprintf("H fs=%d", v)
		if err := set(opus.OPUS_GET_GAIN_REQUEST, &v); err != nil {
			return nil, err
		}
		line += fmt.Sprintf(" gain=%d", v)
		if err := set(opus.OPUS_GET_PHASE_INVERSION_DISABLED_REQUEST, &v); err != nil {
			return nil, err
		}
		res.Trace = append(res.Trace, fmt.Sprintf("%s phaseinv=%d", line, v))
	}
	getDur := func() (int32, error) {
		var v int32
		err := set(opus.OPUS_GET_LAST_PACKET_DURATION_REQUEST, &v)
		return v, err
	}
	var data [2][]byte
	var length [2]int
	out := make([]int16, maxFrame*ch)
	var outf []float32
	if useFloat {
		outf = make([]float32, maxFrame*ch)
	}
	count, toggle, lost, lostPrev := 0, 0, false, false

	for _, p := range pkts {
		length[toggle] = len(p.Data)
		data[toggle] = p.Data
		for s := range scheds {
			if scheds[s].frame == count {
				switch scheds[s].what {
				case "gain":
					if err := set(opus.OPUS_SET_GAIN_REQUEST, scheds[s].val); err != nil {
						return nil, err
					}
				case "reset":
					if err := set(opus.OPUS_RESET_STATE); err != nil {
						return nil, err
					}
				case "phaseinv":
					if err := set(opus.OPUS_SET_PHASE_INVERSION_DISABLED_REQUEST, scheds[s].val); err != nil {
						return nil, err
					}
				default:
					return nil, die("unknown dec sched key %q", scheds[s].what)
				}
			}
		}
		lost = length[toggle] == 0 || (lossperc > 0 && int(lcg.Next()%100) < lossperc)
		if count < fec { // FEC: delay by one packet, exactly like opus_demo
			res.Trace = append(res.Trace, fmt.Sprintf("D i=%d delayed", count))
			lostPrev = lost
			count++
			toggle = (toggle + fec) & 1
			continue
		}
		var pkt []byte
		var pktlen, n int
		decodeFec := 0
		if fec != 0 {
			if lostPrev {
				dur, err := getDur()
				if err != nil {
					return nil, err
				}
				if !lost {
					pkt = data[toggle]
				}
				pktlen = length[toggle]
				n = int(dur)
				decodeFec = 1
			} else {
				pkt = data[1-toggle]
				pktlen = length[1-toggle]
				n = maxFrame
				if pktlen == 0 {
					pkt = nil
				}
			}
		} else {
			if lost {
				dur, err := getDur()
				if err != nil {
					return nil, err
				}
				n = int(dur)
				pkt = nil
				pktlen = 0
			} else {
				n = maxFrame
				pkt = data[toggle]
				pktlen = length[toggle]
			}
		}
		if pkt == nil {
			pktlen = 0
		}
		if useFloat {
			n = dec.decodeFloat(pkt, pktlen, outf, n, decodeFec)
			if n > 0 {
				for i := 0; i < n*ch; i++ {
					x := outf[i] * 32768.0
					var v int
					if x < 0 {
						v = int(x - 0.5)
					} else {
						v = int(x + 0.5)
					}
					if v > 32767 {
						v = 32767
					}
					if v < -32768 {
						v = -32768
					}
					out[i] = int16(v)
				}
			}
		} else {
			n = dec.decode(pkt, pktlen, out, n, decodeFec)
		}
		var drng uint32
		var dbw, dur, pitch int32
		if err := set(opus.OPUS_GET_FINAL_RANGE_REQUEST, &drng); err != nil {
			return nil, err
		}
		if err := set(opus.OPUS_GET_BANDWIDTH_REQUEST, &dbw); err != nil {
			return nil, err
		}
		var err error
		if dur, err = getDur(); err != nil {
			return nil, err
		}
		if !ms {
			if err := set(opus.OPUS_GET_PITCH_REQUEST, &pitch); err != nil {
				return nil, err
			}
		}
		if n < 0 {
			res.Trace = append(res.Trace, fmt.Sprintf("D i=%d lost=%d fec=%d err=%d", count, b2i(lost), decodeFec, n))
		} else {
			crc := crc32.ChecksumIEEE(testvec.S16Bytes(out[:n*ch]))
			res.Trace = append(res.Trace, fmt.Sprintf("D i=%d lost=%d fec=%d n=%d rng=%08x bw=%d dur=%d pitch=%d crc=%08x",
				count, b2i(lost), decodeFec, n, drng, dbw, dur, pitch, crc))
			if n > skip {
				res.Pcm = append(res.Pcm, out[skip*ch:n*ch]...)
			}
			if n < skip {
				skip -= n
			} else {
				skip = 0
			}
		}
		lostPrev = lost
		count++
		toggle = (toggle + fec) & 1
	}
	return res, nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// RunInfo mirrors cmd_info in gen.c.
func RunInfo(args []string, pkts []testvec.Packet) ([]string, error) {
	rate := 48000
	for i := 0; i < len(args); i++ {
		if args[i] == "--rate" && i+1 < len(args) {
			rate = atoi(args[i+1])
			i++
		}
	}
	var trace []string
	dec := opus.DecoderCreate(int32(rate), 1, nil)
	// gen.c declares `toc` inside the loop but never initialises it, so on a
	// parse failure it prints the previous packet's value (same stack slot);
	// keep it across iterations to mirror that.
	var toc byte
	for count, p := range pkts {
		if len(p.Data) == 0 {
			trace = append(trace, fmt.Sprintf("P i=%d empty", count))
			continue
		}
		data := p.Data
		ulen := int32(len(data))
		var frames [48][]byte
		var sizes [48]int16
		var payloadOffset int
		nf := opus.PacketParse(data, ulen, &toc, frames[:], sizes[:], &payloadOffset)
		bw := opus.PacketGetBandwidth(data)
		spf := opus.PacketGetSamplesPerFrame(data, int32(rate))
		nch := opus.PacketGetNbChannels(data)
		ns := opus.PacketGetNbSamples(data, ulen, int32(rate))
		dns := dec.DecoderGetNbSamples(data, ulen)
		off := -1
		if nf > 0 {
			off = payloadOffset
		}
		var sb strings.Builder
		lbrr := opus.PacketHasLbrr(data, ulen)
		fmt.Fprintf(&sb, "P i=%d len=%d toc=%02x nf=%d bw=%d spf=%d nch=%d ns=%d dns=%d lbrr=%d off=%d sizes=", count, ulen, toc, nf, bw, spf, nch, ns, dns, lbrr, off)
		if nf > 0 {
			for j := 0; j < nf; j++ {
				if j > 0 {
					sb.WriteByte(',')
				}
				fmt.Fprintf(&sb, "%d@%d", sizes[j], len(data)-len(frames[j]))
			}
		}
		trace = append(trace, sb.String())
	}
	return trace, nil
}

// RepackResult is what gen repack produces.
type RepackResult struct {
	Packets []testvec.Packet
	Trace   []string
}

// RunRepack mirrors cmd_repack in gen.c.
func RunRepack(args []string, pkts []testvec.Packet) (*RepackResult, error) {
	merge, split, pad, streams := 1, false, 0, 0
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--merge":
			if i+1 < len(args) {
				merge = atoi(args[i+1])
				i++
			}
		case "--split":
			split = true
		case "--pad":
			if i+1 < len(args) {
				pad = atoi(args[i+1])
				i++
			}
		case "--streams":
			if i+1 < len(args) {
				streams = atoi(args[i+1])
				i++
			}
		default:
			return nil, die("unknown option %q", args[i])
		}
	}
	if merge < 1 || merge > 48 {
		return nil, die("merge out of range")
	}
	res := &RepackResult{}
	var bufs [48][]byte
	outbuf := make([]byte, maxPacket*48)
	rp := opus.RepacketizerCreate()
	inrp := 0
	emit := func(ol int32) {
		res.Trace = append(res.Trace, fmt.Sprintf("O len=%d", ol))
		if ol > 0 {
			res.Packets = append(res.Packets, testvec.Packet{Data: append([]byte(nil), outbuf[:ol]...), Rng: 0})
		}
	}
	for count, p := range pkts {
		ulen := int32(len(p.Data))
		if ulen == 0 {
			res.Trace = append(res.Trace, fmt.Sprintf("R i=%d empty", count))
			continue
		}
		// gen.c reads into bufs[inrp] (a fresh MAX_PACKET buffer per slot)
		bufs[inrp] = append([]byte(nil), p.Data...)
		if pad != 0 {
			// opus_packet_pad / opus_multistream_packet_pad in place, then unpad a copy
			newlen := ulen + int32(pad)
			if int(newlen) > len(outbuf) {
				return nil, die("pad too large")
			}
			copy(outbuf, bufs[0][:ulen])
			var r int
			if streams > 0 {
				r = opus.MultistreamPacketPad(outbuf, ulen, newlen, streams)
			} else {
				r = opus.PacketPad(outbuf, ulen, newlen)
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "R i=%d pad=%d", count, r)
			if r == opus.OPUS_OK {
				tmp := append([]byte(nil), outbuf[:newlen]...)
				var ul int32
				if streams > 0 {
					ul = opus.MultistreamPacketUnpad(tmp, newlen, streams)
				} else {
					ul = opus.PacketUnpad(tmp, newlen)
				}
				fmt.Fprintf(&sb, " unpad=%d", ul)
				if ul > 0 {
					fmt.Fprintf(&sb, " ucrc=%08x", crc32.ChecksumIEEE(tmp[:ul]))
				}
				res.Packets = append(res.Packets, testvec.Packet{Data: append([]byte(nil), outbuf[:newlen]...), Rng: 0})
			}
			res.Trace = append(res.Trace, sb.String())
			continue
		}
		if split {
			rp.RepacketizerInit()
			r := rp.RepacketizerCat(bufs[0], ulen) // the C reads into bufs[inrp] which is bufs[0] in split mode
			nf := rp.RepacketizerGetNbFrames()
			var sb strings.Builder
			fmt.Fprintf(&sb, "R i=%d cat=%d nf=%d outs=", count, r, nf)
			if r == opus.OPUS_OK {
				for j := 0; j < nf; j++ {
					ol := rp.RepacketizerOutRange(j, j+1, outbuf, int32(len(outbuf)))
					if j > 0 {
						sb.WriteByte(',')
					}
					fmt.Fprintf(&sb, "%d", ol)
					if ol > 0 {
						res.Packets = append(res.Packets, testvec.Packet{Data: append([]byte(nil), outbuf[:ol]...), Rng: 0})
					}
				}
			}
			res.Trace = append(res.Trace, sb.String())
			continue
		}
		if inrp == 0 {
			rp.RepacketizerInit()
		}
		r := rp.RepacketizerCat(bufs[inrp], ulen)
		res.Trace = append(res.Trace, fmt.Sprintf("R i=%d cat=%d nf=%d", count, r, rp.RepacketizerGetNbFrames()))
		if r == opus.OPUS_OK {
			inrp++
		} else { // flush what we have and start over with this packet
			emit(rp.RepacketizerOut(outbuf, int32(len(outbuf))))
			rp.RepacketizerInit()
			// the packet becomes frame 0: it moves to slot 0 so that later packets cannot overwrite it
			bufs[0] = bufs[inrp]
			r = rp.RepacketizerCat(bufs[0], ulen)
			res.Trace = append(res.Trace, fmt.Sprintf("R i=%d cat=%d nf=%d", count, r, rp.RepacketizerGetNbFrames()))
			if r == opus.OPUS_OK {
				inrp = 1
			} else {
				inrp = 0
			}
		}
		if inrp == merge {
			emit(rp.RepacketizerOut(outbuf, int32(len(outbuf))))
			inrp = 0
		}
	}
	if inrp > 0 {
		emit(rp.RepacketizerOut(outbuf, int32(len(outbuf))))
	}
	return res, nil
}
