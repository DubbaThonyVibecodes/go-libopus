package testvec

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Manifest lists every case in a corpus directory.  Paths are relative to the
// directory holding manifest.json.
type Manifest struct {
	// Upstream version string the corpus was generated with, e.g. "libopus 1.0.3-fixed".
	Oracle string `json:"oracle"`
	Cases  []Case `json:"cases"`
}

// Case is one invocation of the oracle driver.  Args are the exact gen.c
// command-line arguments (after the sub-command); the Go oracle twin parses
// the same arguments, so the semantics live in one place.
type Case struct {
	Name string `json:"name"`
	// Kind is "enc", "dec", "info" or "repack".
	Kind string   `json:"kind"`
	Args []string `json:"args,omitempty"`

	// Input for enc: raw s16 file, or a synthetic signal rendered on the fly
	// by Synth (Synth spec + SynthSecs; rate and channels come from Args).
	In        string  `json:"in,omitempty"`
	Synth     string  `json:"synth,omitempty"`
	SynthSecs float64 `json:"synthSecs,omitempty"`
	// Input for dec/info/repack: packet file.
	Pkt string `json:"pkt,omitempty"`

	// Outputs.
	OutPkt   string `json:"outPkt,omitempty"`    // enc, repack
	Trace    string `json:"trace"`               // all kinds (gzip if it ends in .gz)
	OutPcm   string `json:"outPcm,omitempty"`    // dec (extended corpus only)
	PcmSHA   string `json:"pcmSha256,omitempty"` // dec: sha256 of the full decoded s16 stream
	PcmBytes int64  `json:"pcmBytes,omitempty"`  // dec: length of that stream in bytes
}

// Load reads manifest.json from dir.
func Load(dir string) (*Manifest, error) {
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Save writes manifest.json into dir.
func (m *Manifest) Save(dir string) error {
	b, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.json"), append(b, '\n'), 0o644)
}
