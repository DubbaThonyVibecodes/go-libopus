#!/bin/sh
# Downloads the royalty-free source recordings and cuts the segments that are
# committed under testdata/clips/ as FLAC.  Only needed to regenerate the clips
# from scratch; the corpus generator itself works from testdata/clips/.
#
# Sources (see testdata/clips/ATTRIBUTION.md):
#   - Kimiko Ishizaka, Open Goldberg Variations (CC0 1.0)
#   - LibriVox recordings (public domain)
set -eu
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
DL="$ROOT/build/dl/audio"
OUT="$ROOT/testdata/clips"
mkdir -p "$DL" "$OUT"
fetch() { [ -s "$DL/$1" ] || curl -sS -L -o "$DL/$1" "$2"; }
fetch goldberg_aria.flac 'https://archive.org/download/OpenGoldbergVariations/Kimiko%20Ishizaka%20-%20J.S.%20Bach-%20-Open-%20Goldberg%20Variations%2C%20BWV%20988%20%28Piano%29%20-%2001%20Aria.flac'
fetch goldberg_var4.flac 'https://archive.org/download/OpenGoldbergVariations/Kimiko%20Ishizaka%20-%20J.S.%20Bach-%20-Open-%20Goldberg%20Variations%2C%20BWV%20988%20%28Piano%29%20-%2005%20Variatio%204%20a%201%20Clav..flac'
fetch librivox_meade_01.mp3 'https://archive.org/download/heartofamystery_2005_librivox/heartofamystery_01_meade_64kb.mp3'
fetch librivox_crompton_02.mp3 'https://archive.org/download/williamagain_1902_librivox/williamagain_02_crompton_64kb.mp3'

# All clips: 30 s, 48 kHz, 16-bit FLAC.  Speech stays mono, music stereo.
# -map_metadata -1 / -vn drop embedded art and tags so files stay small.
cut() { # name src start channels
  ffmpeg -v error -y -ss "$3" -t 30 -i "$DL/$2" -vn -map_metadata -1 -ac "$4" -ar 48000 -sample_fmt s16 -compression_level 8 "$OUT/$1.flac"
}
cut speech_a librivox_meade_01.mp3    60  1
cut speech_b librivox_crompton_02.mp3 75  1
cut piano    goldberg_var4.flac       3   2
cut aria     goldberg_aria.flac       20  2
# Mixed speech + piano, speech panned left, piano attenuated: exercises stereo
# prediction and voice/music mode decisions in one clip.
ffmpeg -v error -y -ss 60 -t 30 -i "$DL/librivox_meade_01.mp3" -ss 3 -t 30 -i "$DL/goldberg_var4.flac" \
  -filter_complex '[0:a]aresample=48000,pan=stereo|c0=0.9*c0|c1=0.4*c0[s];[1:a]aresample=48000,volume=0.5[p];[s][p]amix=inputs=2:duration=shortest:normalize=0[o]' \
  -map '[o]' -map_metadata -1 -ac 2 -ar 48000 -sample_fmt s16 -compression_level 8 "$OUT/mix.flac"
ls -la "$OUT"
