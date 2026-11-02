package conformance

import (
	"testing"

	"github.com/DubbaThonyVibecodes/go-libopus/internal/testvec"
	"github.com/DubbaThonyVibecodes/go-libopus/opus"
)

// TestVersionMatchesOracle checks that the version string reported by the
// port is the one of the upstream build the corpus was generated with.
func TestVersionMatchesOracle(t *testing.T) {
	m, err := testvec.Load("../testdata/corpus")
	if err != nil {
		t.Fatal(err)
	}
	if got := opus.GetVersionString(); got != m.Oracle {
		t.Fatalf("GetVersionString() = %q, corpus oracle is %q", got, m.Oracle)
	}
}
