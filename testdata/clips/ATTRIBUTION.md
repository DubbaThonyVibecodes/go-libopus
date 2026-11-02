# Test clip attribution

All clips are 30-second excerpts, 48 kHz 16-bit FLAC, cut by
`tools/corpus/fetch-clips.sh`. They are used only as encoder input for the
conformance corpus.

| Clip | Source | Licence |
|------|--------|---------|
| `piano.flac` | Kimiko Ishizaka, *Open Goldberg Variations*, BWV 988, Variatio 4 (archive.org item `OpenGoldbergVariations`) | CC0 1.0 Universal |
| `aria.flac` | Kimiko Ishizaka, *Open Goldberg Variations*, BWV 988, Aria (same item) | CC0 1.0 Universal |
| `speech_a.flac` | LibriVox, *The Heart of a Mystery* (L. T. Meade), chapter 1 (archive.org item `heartofamystery_2005_librivox`) | Public domain |
| `speech_b.flac` | LibriVox, *William Again* (Richmal Crompton), chapter 2 (archive.org item `williamagain_1902_librivox`) | Public domain |
| `mix.flac` | `speech_a` panned left mixed over attenuated `piano` | as above |

No other audio may be committed to this repository. Personal or otherwise
non-redistributable material used for local testing belongs in
`testdata/local/`, which is git-ignored.
