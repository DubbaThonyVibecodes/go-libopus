// Package silk is a line-by-line Go port of libopus' silk/ and silk/fixed/
// directories: the SILK speech codec (linear prediction, noise shaping
// quantisation, packet loss concealment) in its fixed-point configuration.
//
// It is an internal layer of the Opus codec, exported because the opus
// package builds on it; applications should use the root package
// github.com/DubbaThonyVibecodes/go-libopus. Names follow the C source minus
// the silk_ prefix (silk_NLSF2A is NLSF2A); see PORTING.md in the module
// root.
package silk
