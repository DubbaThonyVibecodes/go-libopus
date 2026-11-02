/*
 * gen: oracle driver for go-libopus test corpus generation.
 *
 * Links statically against an out-of-tree build of upstream libopus
 * (never the host's system library) and exercises the public API in a
 * fully deterministic way.  Every decision that upstream opus_demo makes
 * with libc rand() is made here with a small LCG so the Go test harness can
 * reproduce the exact same sequence of calls.
 *
 * Packet file format (identical to opus_demo): per packet
 *   4 bytes big-endian length, 4 bytes big-endian encoder final range,
 *   <length> payload bytes.  A length of 0 means "packet lost".
 *
 * Trace file format: one text line per frame, see trace_* below.  The Go
 * tests produce the same lines and diff them, which pinpoints the first
 * diverging frame.
 */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include "opus.h"
#include "opus_multistream.h"
#include "opus_projection.h"
#include "opus_private.h"

#define MAX_PACKET 1500
#define MAX_FRAME (960*6)
#define MAX_CH 36

static void die(const char *m) { fprintf(stderr, "gen: %s\n", m); exit(2); }
static void dief(const char *m, const char *a) { fprintf(stderr, "gen: %s: %s\n", m, a); exit(2); }

/* ---- deterministic helpers -------------------------------------------- */
static uint32_t lcg_state = 1;
static uint32_t lcg(void) { lcg_state = lcg_state*1664525u + 1013904223u; return lcg_state >> 8; }

static uint32_t crc_table[256];
static void crc_init(void) {
   uint32_t i, j, c;
   for (i = 0; i < 256; i++) {
      c = i;
      for (j = 0; j < 8; j++) c = (c & 1) ? (0xEDB88320u ^ (c >> 1)) : (c >> 1);
      crc_table[i] = c;
   }
}
static uint32_t crc_s16(const opus_int16 *p, int n) {
   uint32_t c = 0xFFFFFFFFu;
   int i;
   for (i = 0; i < n; i++) {
      unsigned v = (unsigned short)p[i];
      c = crc_table[(c ^ (v & 0xFF)) & 0xFF] ^ (c >> 8);
      c = crc_table[(c ^ (v >> 8)) & 0xFF] ^ (c >> 8);
   }
   return c ^ 0xFFFFFFFFu;
}

static uint32_t crc_s32(const opus_int32 *p, int n) {
   uint32_t c = 0xFFFFFFFFu;
   int i, k;
   for (i = 0; i < n; i++) {
      uint32_t v = (uint32_t)p[i];
      for (k = 0; k < 4; k++) { c = crc_table[(c ^ (v & 0xFF)) & 0xFF] ^ (c >> 8); v >>= 8; }
   }
   return c ^ 0xFFFFFFFFu;
}
static uint32_t crc_bytes(const unsigned char *p, int n) {
   uint32_t c = 0xFFFFFFFFu;
   int i;
   for (i = 0; i < n; i++) c = crc_table[(c ^ p[i]) & 0xFF] ^ (c >> 8);
   return c ^ 0xFFFFFFFFu;
}

static void put_be32(FILE *f, uint32_t v) {
   unsigned char b[4] = { v>>24, (v>>16)&0xFF, (v>>8)&0xFF, v&0xFF };
   if (fwrite(b, 1, 4, f) != 4) die("write");
}
static int get_be32(FILE *f, uint32_t *v) {
   unsigned char b[4];
   if (fread(b, 1, 4, f) != 4) return 0;
   *v = ((uint32_t)b[0]<<24)|((uint32_t)b[1]<<16)|((uint32_t)b[2]<<8)|b[3];
   return 1;
}
static void write_s16(FILE *f, const opus_int16 *p, int n) {
   int i;
   for (i = 0; i < n; i++) {
      unsigned char b[2] = { p[i] & 0xFF, (p[i] >> 8) & 0xFF };
      if (fwrite(b, 1, 2, f) != 2) die("write");
   }
}
static int read_s16(FILE *f, opus_int16 *p, int n) {
   int i, got = 0;
   for (i = 0; i < n; i++) {
      unsigned char b[2];
      if (fread(b, 1, 2, f) != 2) break;
      p[i] = (opus_int16)(b[0] | (b[1] << 8));
      got++;
   }
   return got;
}

/* ---- option parsing ----------------------------------------------------- */
static int parse_bw(const char *s) {
   if (!strcmp(s, "NB")) return OPUS_BANDWIDTH_NARROWBAND;
   if (!strcmp(s, "MB")) return OPUS_BANDWIDTH_MEDIUMBAND;
   if (!strcmp(s, "WB")) return OPUS_BANDWIDTH_WIDEBAND;
   if (!strcmp(s, "SWB")) return OPUS_BANDWIDTH_SUPERWIDEBAND;
   if (!strcmp(s, "FB")) return OPUS_BANDWIDTH_FULLBAND;
   if (!strcmp(s, "auto")) return OPUS_AUTO;
   dief("bad bandwidth", s); return 0;
}
static int parse_app(const char *s) {
   if (!strcmp(s, "voip")) return OPUS_APPLICATION_VOIP;
   if (!strcmp(s, "audio")) return OPUS_APPLICATION_AUDIO;
   if (!strcmp(s, "lowdelay")) return OPUS_APPLICATION_RESTRICTED_LOWDELAY;
   if (!strcmp(s, "silk")) return OPUS_APPLICATION_RESTRICTED_SILK; /* libopus 1.6 */
   if (!strcmp(s, "celt")) return OPUS_APPLICATION_RESTRICTED_CELT;
   dief("bad application", s); return 0;
}
static int parse_signal(const char *s) {
   if (!strcmp(s, "auto")) return OPUS_AUTO;
   if (!strcmp(s, "voice")) return OPUS_SIGNAL_VOICE;
   if (!strcmp(s, "music")) return OPUS_SIGNAL_MUSIC;
   dief("bad signal", s); return 0;
}
static int parse_mode(const char *s) {
   if (!strcmp(s, "auto")) return OPUS_AUTO;
   if (!strcmp(s, "silk")) return MODE_SILK_ONLY;
   if (!strcmp(s, "hybrid")) return MODE_HYBRID;
   if (!strcmp(s, "celt")) return MODE_CELT_ONLY;
   dief("bad mode", s); return 0;
}
static int parse_framedur(const char *s) {
   if (!strcmp(s, "arg")) return OPUS_FRAMESIZE_ARG;
   if (!strcmp(s, "2.5")) return OPUS_FRAMESIZE_2_5_MS;
   if (!strcmp(s, "5"))   return OPUS_FRAMESIZE_5_MS;
   if (!strcmp(s, "10"))  return OPUS_FRAMESIZE_10_MS;
   if (!strcmp(s, "20"))  return OPUS_FRAMESIZE_20_MS;
   if (!strcmp(s, "40"))  return OPUS_FRAMESIZE_40_MS;
   if (!strcmp(s, "60"))  return OPUS_FRAMESIZE_60_MS;
   if (!strcmp(s, "80"))  return OPUS_FRAMESIZE_80_MS;
   if (!strcmp(s, "100")) return OPUS_FRAMESIZE_100_MS;
   if (!strcmp(s, "120")) return OPUS_FRAMESIZE_120_MS;
   dief("bad frame duration", s); return 0;
}
static int parse_framesize(const char *s, int rate) {
   if (!strcmp(s, "2.5")) return rate/400;
   if (!strcmp(s, "5"))   return rate/200;
   if (!strcmp(s, "10"))  return rate/100;
   if (!strcmp(s, "20"))  return rate/50;
   if (!strcmp(s, "40"))  return rate/25;
   if (!strcmp(s, "60"))  return 3*rate/50;
   if (!strcmp(s, "80"))  return 4*rate/50;
   if (!strcmp(s, "100")) return rate/10;
   if (!strcmp(s, "120")) return 6*rate/50;
   dief("bad framesize", s); return 0;
}
static int parse_bitrate(const char *s) {
   if (!strcmp(s, "max")) return OPUS_BITRATE_MAX;
   if (!strcmp(s, "auto")) return OPUS_AUTO;
   return atoi(s);
}

/* Mode lists from upstream opus_demo.c: {mode, bandwidth, frame size @48k, channels}. */
static const int silk8_test[][4] = {
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_NARROWBAND, 960*3, 1},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_NARROWBAND, 960*2, 1},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_NARROWBAND, 960,   1},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_NARROWBAND, 480,   1},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_NARROWBAND, 960*3, 2},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_NARROWBAND, 960*2, 2},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_NARROWBAND, 960,   2},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_NARROWBAND, 480,   2}
};
static const int silk12_test[][4] = {
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_MEDIUMBAND, 960*3, 1},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_MEDIUMBAND, 960*2, 1},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_MEDIUMBAND, 960,   1},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_MEDIUMBAND, 480,   1},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_MEDIUMBAND, 960*3, 2},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_MEDIUMBAND, 960*2, 2},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_MEDIUMBAND, 960,   2},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_MEDIUMBAND, 480,   2}
};
static const int silk16_test[][4] = {
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_WIDEBAND, 960*3, 1},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_WIDEBAND, 960*2, 1},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_WIDEBAND, 960,   1},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_WIDEBAND, 480,   1},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_WIDEBAND, 960*3, 2},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_WIDEBAND, 960*2, 2},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_WIDEBAND, 960,   2},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_WIDEBAND, 480,   2}
};
static const int hybrid24_test[][4] = {
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_SUPERWIDEBAND, 960, 1},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_SUPERWIDEBAND, 480, 1},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_SUPERWIDEBAND, 960, 2},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_SUPERWIDEBAND, 480, 2}
};
static const int hybrid48_test[][4] = {
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_FULLBAND, 960, 1},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_FULLBAND, 480, 1},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_FULLBAND, 960, 2},
      {MODE_SILK_ONLY, OPUS_BANDWIDTH_FULLBAND, 480, 2}
};
static const int celt_test[][4] = {
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_FULLBAND,      960, 1},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_SUPERWIDEBAND, 960, 1},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_WIDEBAND,      960, 1},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_NARROWBAND,    960, 1},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_FULLBAND,      480, 1},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_SUPERWIDEBAND, 480, 1},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_WIDEBAND,      480, 1},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_NARROWBAND,    480, 1},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_FULLBAND,      240, 1},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_SUPERWIDEBAND, 240, 1},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_WIDEBAND,      240, 1},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_NARROWBAND,    240, 1},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_FULLBAND,      120, 1},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_SUPERWIDEBAND, 120, 1},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_WIDEBAND,      120, 1},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_NARROWBAND,    120, 1},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_FULLBAND,      960, 2},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_SUPERWIDEBAND, 960, 2},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_WIDEBAND,      960, 2},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_NARROWBAND,    960, 2},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_FULLBAND,      480, 2},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_SUPERWIDEBAND, 480, 2},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_WIDEBAND,      480, 2},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_NARROWBAND,    480, 2},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_FULLBAND,      240, 2},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_SUPERWIDEBAND, 240, 2},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_WIDEBAND,      240, 2},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_NARROWBAND,    240, 2},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_FULLBAND,      120, 2},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_SUPERWIDEBAND, 120, 2},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_WIDEBAND,      120, 2},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_NARROWBAND,    120, 2},
};
static const int celt_hq_test[][4] = {
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_FULLBAND,      960, 2},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_FULLBAND,      480, 2},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_FULLBAND,      240, 2},
      {MODE_CELT_ONLY, OPUS_BANDWIDTH_FULLBAND,      120, 2},
};

/* A scheduled CTL change: at frame index `frame`, apply `what`=`val`. */
struct sched { int frame; char what[16]; int val; };
#define MAX_SCHED 64

static int parse_sched_val(const char *what, const char *v, int rate) {
   if (!strcmp(what, "bw") || !strcmp(what, "maxbw")) return parse_bw(v);
   if (!strcmp(what, "mode")) return parse_mode(v);
   if (!strcmp(what, "signal")) return parse_signal(v);
   if (!strcmp(what, "app")) return parse_app(v);
   if (!strcmp(what, "bitrate")) return parse_bitrate(v);
   if (!strcmp(what, "framesize")) return parse_framesize(v, rate);
   if (!strcmp(what, "framedur")) return parse_framedur(v);
   if (!strcmp(what, "forcech")) return strcmp(v, "auto") ? atoi(v) : OPUS_AUTO;
   return atoi(v);
}
/* "f=100:bitrate=32000,f=200:bw=NB" */
static int parse_sched(char *s, struct sched *out, int rate) {
   int n = 0;
   char *tok = strtok(s, ",");
   while (tok) {
      char *colon = strchr(tok, ':'), *eq;
      if (!colon || strncmp(tok, "f=", 2)) dief("bad sched", tok);
      *colon = 0;
      eq = strchr(colon+1, '=');
      if (!eq) dief("bad sched", tok);
      *eq = 0;
      if (n >= MAX_SCHED) die("too many sched");
      out[n].frame = atoi(tok+2);
      strncpy(out[n].what, colon+1, 15); out[n].what[15] = 0;
      out[n].val = parse_sched_val(out[n].what, eq+1, rate);
      n++;
      tok = strtok(NULL, ",");
   }
   return n;
}

/* Generic encoder-ish control applied to either an OpusEncoder or an
 * OpusMSEncoder (which forwards to all its streams). */
/* g_proj: the state is an OpusProjectionEncoder/Decoder (libopus 1.3 ambisonics, mapping family 3) */
static int g_proj = 0;
#define ENC_CTL(st, ms, ...) (g_proj ? opus_projection_encoder_ctl((OpusProjectionEncoder*)(st), __VA_ARGS__) : (ms) ? opus_multistream_encoder_ctl((OpusMSEncoder*)(st), __VA_ARGS__) : opus_encoder_ctl((OpusEncoder*)(st), __VA_ARGS__))
#define DEC_CTL(st, ms, ...) (g_proj ? opus_projection_decoder_ctl((OpusProjectionDecoder*)(st), __VA_ARGS__) : (ms) ? opus_multistream_decoder_ctl((OpusMSDecoder*)(st), __VA_ARGS__) : opus_decoder_ctl((OpusDecoder*)(st), __VA_ARGS__))

/* Demixing matrix of the projection (family 3) encoder for this channel count; it is
 * deterministic, so the decoder side regenerates it instead of carrying it out of band. */
static int get_demixing_matrix(int rate, int ch, int app, unsigned char **matrix, opus_int32 *size, int *streams, int *coupled, int *gain) {
   int err;
   OpusProjectionEncoder *pe = opus_projection_ambisonics_encoder_create(rate, ch, 3, streams, coupled, app, &err);
   if (err != OPUS_OK) dief("projection create", opus_strerror(err));
   if (opus_projection_encoder_ctl(pe, OPUS_PROJECTION_GET_DEMIXING_MATRIX_SIZE(size)) != OPUS_OK) die("matrix size");
   *matrix = malloc(*size);
   if (opus_projection_encoder_ctl(pe, OPUS_PROJECTION_GET_DEMIXING_MATRIX(*matrix, *size)) != OPUS_OK) die("matrix");
   if (opus_projection_encoder_ctl(pe, OPUS_PROJECTION_GET_DEMIXING_MATRIX_GAIN(gain)) != OPUS_OK) die("matrix gain");
   opus_projection_encoder_destroy(pe);
   return 0;
}

static void apply_sched(void *enc, int ms, const struct sched *s, int *frame_size, int *framedur, int rate) {
   int r = OPUS_OK;
   (void)rate;
   if (!strcmp(s->what, "bitrate")) r = ENC_CTL(enc, ms, OPUS_SET_BITRATE(s->val));
   else if (!strcmp(s->what, "bw")) r = ENC_CTL(enc, ms, OPUS_SET_BANDWIDTH(s->val));
   else if (!strcmp(s->what, "maxbw")) r = ENC_CTL(enc, ms, OPUS_SET_MAX_BANDWIDTH(s->val));
   else if (!strcmp(s->what, "mode")) r = ENC_CTL(enc, ms, OPUS_SET_FORCE_MODE(s->val));
   else if (!strcmp(s->what, "signal")) r = ENC_CTL(enc, ms, OPUS_SET_SIGNAL(s->val));
   else if (!strcmp(s->what, "app")) r = ENC_CTL(enc, ms, OPUS_SET_APPLICATION(s->val));
   else if (!strcmp(s->what, "complexity")) r = ENC_CTL(enc, ms, OPUS_SET_COMPLEXITY(s->val));
   else if (!strcmp(s->what, "fec")) r = ENC_CTL(enc, ms, OPUS_SET_INBAND_FEC(s->val));
   else if (!strcmp(s->what, "lossperc")) r = ENC_CTL(enc, ms, OPUS_SET_PACKET_LOSS_PERC(s->val));
   else if (!strcmp(s->what, "dtx")) r = ENC_CTL(enc, ms, OPUS_SET_DTX(s->val));
   else if (!strcmp(s->what, "vbr")) r = ENC_CTL(enc, ms, OPUS_SET_VBR(s->val));
   else if (!strcmp(s->what, "cvbr")) r = ENC_CTL(enc, ms, OPUS_SET_VBR_CONSTRAINT(s->val));
   else if (!strcmp(s->what, "forcech")) r = ENC_CTL(enc, ms, OPUS_SET_FORCE_CHANNELS(s->val));
   else if (!strcmp(s->what, "lsbdepth")) r = ENC_CTL(enc, ms, OPUS_SET_LSB_DEPTH(s->val));
   else if (!strcmp(s->what, "voiceratio")) r = ENC_CTL(enc, ms, OPUS_SET_VOICE_RATIO(s->val));
   else if (!strcmp(s->what, "reset")) r = ENC_CTL(enc, ms, OPUS_RESET_STATE);
   else if (!strcmp(s->what, "framedur")) { r = ENC_CTL(enc, ms, OPUS_SET_EXPERT_FRAME_DURATION(s->val)); *framedur = s->val; }
   else if (!strcmp(s->what, "pred")) r = ENC_CTL(enc, ms, OPUS_SET_PREDICTION_DISABLED(s->val));
   else if (!strcmp(s->what, "phaseinv")) r = ENC_CTL(enc, ms, OPUS_SET_PHASE_INVERSION_DISABLED(s->val));
   else if (!strcmp(s->what, "framesize")) *frame_size = s->val;
   else dief("unknown sched key", s->what);
   if (r != OPUS_OK) { fprintf(stderr, "gen: sched %s=%d failed: %s\n", s->what, s->val, opus_strerror(r)); exit(2); }
}

/* ---- enc ----------------------------------------------------------------- */
static int cmd_enc(int argc, char **argv) {
   int rate = 48000, ch = 1, app = OPUS_APPLICATION_AUDIO, bitrate = 64000, frame_size = -1;
   int vbr = 1, cvbr = 0, bw = OPUS_AUTO, maxbw = OPUS_AUTO, complexity = 10, fec = 0, lossperc = 0, dtx = 0;
   int forcech = OPUS_AUTO, sig = OPUS_AUTO, lsbdepth = 16, maxpayload = MAX_PACKET, mode = OPUS_AUTO;
   int voiceratio = -1, framedur = OPUS_FRAMESIZE_ARG, nopred = 0, remaining = 0, nophaseinv = 0, pcm24 = 0;
   opus_int32 *in32 = NULL;
   int sweep_bps = 0, sweep_max = 0, sweep_min = 0, random_framesize = 0, random_fec = 0;
   const int (*mode_list)[4] = NULL; int nb_modes = 0;
   struct sched sched[MAX_SCHED]; int nsched = 0;
   /* multistream */
   int ms = 0, streams = 0, coupled = 0, family = -1; unsigned char mapping[MAX_CH]; int nmap = 0;
   const char *in_path = NULL, *pkt_path = NULL, *trace_path = NULL;
   int i, err;
   void *enc; FILE *fin, *fpkt, *ftr;
   opus_int16 *in; unsigned char *data;
   int count = 0, curr_mode = 0, curr_mode_count = 0, mode_switch_time = 0, stop = 0, delayed_celt = 0, newsize = 0;
   int lookahead = 0;

   lcg_state = 1;
   for (i = 0; i < argc; i++) {
      const char *a = argv[i];
#define NEXT (i+1 < argc ? argv[++i] : (dief("missing value for", a), (char*)0))
      if (!strcmp(a, "--rate")) rate = atoi(NEXT);
      else if (!strcmp(a, "--ch")) ch = atoi(NEXT);
      else if (!strcmp(a, "--app")) app = parse_app(NEXT);
      else if (!strcmp(a, "--bitrate")) bitrate = parse_bitrate(NEXT);
      else if (!strcmp(a, "--framesize")) { const char *v = NEXT; frame_size = parse_framesize(v, rate); }
      else if (!strcmp(a, "--cbr")) vbr = 0;
      else if (!strcmp(a, "--cvbr")) cvbr = 1;
      else if (!strcmp(a, "--bw")) bw = parse_bw(NEXT);
      else if (!strcmp(a, "--maxbw")) maxbw = parse_bw(NEXT);
      else if (!strcmp(a, "--complexity")) complexity = atoi(NEXT);
      else if (!strcmp(a, "--fec")) fec = 1;
      else if (!strcmp(a, "--fec2")) fec = 2; /* libopus 1.4: FEC without forcing SILK for music */
      else if (!strcmp(a, "--lossperc")) lossperc = atoi(NEXT);
      else if (!strcmp(a, "--dtx")) dtx = 1;
      else if (!strcmp(a, "--forcech")) { const char *v = NEXT; forcech = strcmp(v, "auto") ? atoi(v) : OPUS_AUTO; }
      else if (!strcmp(a, "--signal")) sig = parse_signal(NEXT);
      else if (!strcmp(a, "--mode")) mode = parse_mode(NEXT);
      else if (!strcmp(a, "--lsbdepth")) lsbdepth = atoi(NEXT);
      else if (!strcmp(a, "--maxpayload")) maxpayload = atoi(NEXT);
      else if (!strcmp(a, "--voiceratio")) voiceratio = atoi(NEXT);
      else if (!strcmp(a, "--framedur")) framedur = parse_framedur(NEXT);
      else if (!strcmp(a, "--nopred")) nopred = 1;
      else if (!strcmp(a, "--nophaseinv")) nophaseinv = 1;
      else if (!strcmp(a, "--pcm24")) pcm24 = 1; /* libopus 1.6: 24-bit API (samples << 8) */
      else if (!strcmp(a, "--sweep")) sweep_bps = atoi(NEXT);
      else if (!strcmp(a, "--sweep-max")) sweep_max = atoi(NEXT);
      else if (!strcmp(a, "--random-framesize")) random_framesize = 1;
      else if (!strcmp(a, "--random-fec")) random_fec = 1;
      else if (!strcmp(a, "--seed")) lcg_state = (uint32_t)strtoul(NEXT, NULL, 0);
      else if (!strcmp(a, "--sched")) nsched = parse_sched(NEXT, sched, rate);
      else if (!strcmp(a, "--mode-list")) {
         const char *v = NEXT;
         if (!strcmp(v, "silk8")) { mode_list = silk8_test; nb_modes = 8; }
         else if (!strcmp(v, "silk12")) { mode_list = silk12_test; nb_modes = 8; }
         else if (!strcmp(v, "silk16")) { mode_list = silk16_test; nb_modes = 8; }
         else if (!strcmp(v, "hybrid24")) { mode_list = hybrid24_test; nb_modes = 4; }
         else if (!strcmp(v, "hybrid48")) { mode_list = hybrid48_test; nb_modes = 4; }
         else if (!strcmp(v, "celt")) { mode_list = celt_test; nb_modes = 32; }
         else if (!strcmp(v, "celthq")) { mode_list = celt_hq_test; nb_modes = 4; }
         else dief("bad mode-list", v);
      }
      else if (!strcmp(a, "--streams")) { ms = 1; streams = atoi(NEXT); }
      else if (!strcmp(a, "--coupled")) { ms = 1; coupled = atoi(NEXT); }
      else if (!strcmp(a, "--family")) { ms = 1; family = atoi(NEXT); }
      else if (!strcmp(a, "--projection")) { ms = 1; g_proj = 1; family = 3; }
      else if (!strcmp(a, "--mapping")) {
         char *v = NEXT, *t = strtok(v, ",");
         ms = 1;
         while (t) { if (nmap >= MAX_CH) die("mapping too long"); mapping[nmap++] = (unsigned char)atoi(t); t = strtok(NULL, ","); }
      }
      else if (a[0] == '-' && a[1] == '-') dief("unknown option", a);
      else if (!in_path) in_path = a;
      else if (!pkt_path) pkt_path = a;
      else if (!trace_path) trace_path = a;
      else dief("extra arg", a);
#undef NEXT
   }
   if (!in_path || !pkt_path || !trace_path) die("usage: gen enc [opts] in.s16 out.pkt out.trace");
   if (frame_size < 0) frame_size = rate/50;
   if (ch > MAX_CH) die("too many channels");
   if (ch > 2 && !ms) die("channels > 2 requires multistream options");

   fin = fopen(in_path, "rb"); if (!fin) dief("open", in_path);
   fpkt = fopen(pkt_path, "wb"); if (!fpkt) dief("open", pkt_path);
   ftr = fopen(trace_path, "w"); if (!ftr) dief("open", trace_path);

   if (g_proj) {
      int s2, c2, gain, i2; unsigned char *matrix; opus_int32 msize;
      enc = opus_projection_ambisonics_encoder_create(rate, ch, family, &s2, &c2, app, &err);
      if (err != OPUS_OK) dief("projection create", opus_strerror(err));
      streams = s2; coupled = c2;
      get_demixing_matrix(rate, ch, app, &matrix, &msize, &s2, &c2, &gain);
      fprintf(ftr, "H projection family=%d streams=%d coupled=%d gain=%d size=%d matrix=", family, streams, coupled, gain, (int)msize);
      for (i2 = 0; i2 < msize; i2++) fprintf(ftr, "%02x", matrix[i2]);
      fprintf(ftr, "\n");
      free(matrix);
   } else if (ms) {
      if (family >= 0) {
         int s2, c2; unsigned char map2[MAX_CH];
         enc = opus_multistream_surround_encoder_create(rate, ch, family, &s2, &c2, map2, app, &err);
         if (err != OPUS_OK) dief("surround create", opus_strerror(err));
         streams = s2; coupled = c2;
         fprintf(ftr, "H surround family=%d streams=%d coupled=%d mapping=", family, s2, c2);
         for (i = 0; i < ch; i++) fprintf(ftr, "%s%d", i ? "," : "", map2[i]);
         fprintf(ftr, "\n");
      } else {
         if (nmap != ch) die("mapping length must equal channels");
         enc = opus_multistream_encoder_create(rate, ch, streams, coupled, mapping, app, &err);
         if (err != OPUS_OK) dief("ms create", opus_strerror(err));
      }
   } else {
      enc = opus_encoder_create(rate, ch, app, &err);
      if (err != OPUS_OK) dief("create", opus_strerror(err));
   }
#define SET(...) do { int r_ = ENC_CTL(enc, ms, __VA_ARGS__); if (r_ != OPUS_OK) { fprintf(stderr, "gen: ctl failed: %s\n", opus_strerror(r_)); exit(2);} } while (0)
   SET(OPUS_SET_BITRATE(bitrate));
   SET(OPUS_SET_BANDWIDTH(bw));
   if (maxbw != OPUS_AUTO) SET(OPUS_SET_MAX_BANDWIDTH(maxbw));
   SET(OPUS_SET_VBR(vbr));
   SET(OPUS_SET_VBR_CONSTRAINT(cvbr));
   SET(OPUS_SET_COMPLEXITY(complexity));
   SET(OPUS_SET_INBAND_FEC(fec));
   SET(OPUS_SET_FORCE_CHANNELS(forcech));
   SET(OPUS_SET_DTX(dtx));
   SET(OPUS_SET_PACKET_LOSS_PERC(lossperc));
   SET(OPUS_SET_SIGNAL(sig));
   SET(OPUS_SET_LSB_DEPTH(lsbdepth));
   if (mode != OPUS_AUTO) SET(OPUS_SET_FORCE_MODE(mode));
   if (voiceratio >= 0 && !ms) SET(OPUS_SET_VOICE_RATIO(voiceratio));
   if (framedur != OPUS_FRAMESIZE_ARG) SET(OPUS_SET_EXPERT_FRAME_DURATION(framedur));
   if (nopred) SET(OPUS_SET_PREDICTION_DISABLED(nopred));
   if (nophaseinv) SET(OPUS_SET_PHASE_INVERSION_DISABLED(nophaseinv));
   SET(OPUS_GET_LOOKAHEAD(&lookahead));
   {
      int v;
      fprintf(ftr, "H lookahead=%d", lookahead);
      SET(OPUS_GET_SAMPLE_RATE(&v)); fprintf(ftr, " fs=%d", v);
      SET(OPUS_GET_BITRATE(&v)); fprintf(ftr, " bitrate=%d", v);
      SET(OPUS_GET_BANDWIDTH(&v)); fprintf(ftr, " bw=%d", v);
      SET(OPUS_GET_APPLICATION(&v)); fprintf(ftr, " app=%d", v);
      SET(OPUS_GET_COMPLEXITY(&v)); fprintf(ftr, " cplx=%d", v);
      SET(OPUS_GET_VBR(&v)); fprintf(ftr, " vbr=%d", v);
      SET(OPUS_GET_SIGNAL(&v)); fprintf(ftr, " signal=%d", v);
      SET(OPUS_GET_FORCE_CHANNELS(&v)); fprintf(ftr, " forcech=%d", v);
      SET(OPUS_GET_LSB_DEPTH(&v)); fprintf(ftr, " lsb=%d", v);
      SET(OPUS_GET_EXPERT_FRAME_DURATION(&v)); fprintf(ftr, " framedur=%d", v);
      SET(OPUS_GET_PREDICTION_DISABLED(&v)); fprintf(ftr, " pred=%d", v);
      SET(OPUS_GET_PHASE_INVERSION_DISABLED(&v)); fprintf(ftr, " phaseinv=%d", v);
      fprintf(ftr, "\n");
   }
   if (sweep_max) sweep_min = bitrate;
   if (mode_list) {
      long size;
      fseek(fin, 0, SEEK_END); size = ftell(fin); fseek(fin, 0, SEEK_SET);
      mode_switch_time = (int)(size/sizeof(short)/ch/nb_modes);
   }
   in = malloc(sizeof(opus_int16)*MAX_FRAME*ch);
   in32 = malloc(sizeof(opus_int32)*MAX_FRAME*ch);
   data = malloc(MAX_PACKET);

   while (!stop) {
      int len, read, s_i; opus_uint32 rng; int ebw;
      for (s_i = 0; s_i < nsched; s_i++) if (sched[s_i].frame == count) apply_sched(enc, ms, &sched[s_i], &frame_size, &framedur, rate);
      if (delayed_celt) { frame_size = newsize; delayed_celt = 0; }
      else if (random_framesize && lcg()%20 == 0) {
         newsize = lcg()%6;
         switch (newsize) {
         case 0: newsize = rate/400; break;
         case 1: newsize = rate/200; break;
         case 2: newsize = rate/100; break;
         case 3: newsize = rate/50; break;
         case 4: newsize = rate/25; break;
         case 5: newsize = 3*rate/50; break;
         }
         while (newsize < rate/25 && bitrate - abs(sweep_bps) <= 3*12*rate/newsize) newsize *= 2;
         if (newsize < rate/100 && frame_size >= rate/100) { SET(OPUS_SET_FORCE_MODE(MODE_CELT_ONLY)); delayed_celt = 1; }
         else frame_size = newsize;
      }
      if (random_fec && lcg()%30 == 0) SET(OPUS_SET_INBAND_FEC(lcg()%4 == 0));
      if (mode_list) {
         SET(OPUS_SET_BANDWIDTH(mode_list[curr_mode][1]));
         SET(OPUS_SET_FORCE_MODE(mode_list[curr_mode][0]));
         SET(OPUS_SET_FORCE_CHANNELS(mode_list[curr_mode][3]));
         frame_size = mode_list[curr_mode][2] * rate / 48000;
      }
      if (remaining > frame_size) remaining = frame_size;
      read = remaining + read_s16(fin, in + remaining*ch, (frame_size-remaining)*ch) / ch;
      if (read < frame_size) { /* like opus_demo: the last (possibly empty) frame is zero padded and still encoded */
         for (i = read*ch; i < frame_size*ch; i++) in[i] = 0;
         stop = 1;
      }
      if (pcm24) {
         for (i = 0; i < frame_size*ch; i++) in32[i] = (opus_int32)in[i] << 8;
         if (g_proj) len = opus_projection_encode24((OpusProjectionEncoder*)enc, in32, frame_size, data, maxpayload);
         else if (ms) len = opus_multistream_encode24((OpusMSEncoder*)enc, in32, frame_size, data, maxpayload);
         else len = opus_encode24((OpusEncoder*)enc, in32, frame_size, data, maxpayload);
      }
      else if (g_proj) len = opus_projection_encode((OpusProjectionEncoder*)enc, in, frame_size, data, maxpayload);
      else if (ms) len = opus_multistream_encode((OpusMSEncoder*)enc, in, frame_size, data, maxpayload);
      else len = opus_encode((OpusEncoder*)enc, in, frame_size, data, maxpayload);
      if (len < 0) { fprintf(ftr, "E i=%d err=%d\n", count, len); fprintf(stderr, "gen: opus_encode: %s\n", opus_strerror(len)); break; }
      if (sweep_bps) {
         bitrate += sweep_bps;
         if (sweep_max) { if (bitrate > sweep_max) sweep_bps = -sweep_bps; else if (bitrate < sweep_min) sweep_bps = -sweep_bps; }
         if (bitrate < 1000) bitrate = 1000;
         SET(OPUS_SET_BITRATE(bitrate));
      }
      SET(OPUS_GET_FINAL_RANGE(&rng));
      SET(OPUS_GET_BANDWIDTH(&ebw));
      put_be32(fpkt, (uint32_t)len); put_be32(fpkt, rng);
      if (len && fwrite(data, 1, len, fpkt) != (size_t)len) die("write");
      fprintf(ftr, "E i=%d fs=%d len=%d rng=%08x bw=%d", count, frame_size, len, (unsigned)rng, ebw);
      if (len > 0) fprintf(ftr, " toc=%02x", data[0]);
      if (!ms) { int indtx; SET(OPUS_GET_IN_DTX(&indtx)); fprintf(ftr, " indtx=%d", indtx); }
      remaining = 0;
      if (framedur != OPUS_FRAMESIZE_ARG) {
         /* like opus_demo: the encoder may consume fewer samples than offered; keep the rest */
         int nb_encoded = len > 1 ? opus_packet_get_samples_per_frame(data, rate)*opus_packet_get_nb_frames(data, len) : frame_size;
         if (nb_encoded <= 0 || nb_encoded > frame_size) nb_encoded = frame_size;
         remaining = frame_size - nb_encoded;
         for (i = 0; i < remaining*ch; i++) in[i] = in[nb_encoded*ch + i];
         fprintf(ftr, " enc=%d", nb_encoded);
      }
      fprintf(ftr, "\n");
      curr_mode_count += frame_size;
      if (mode_list && curr_mode_count > mode_switch_time && curr_mode < nb_modes-1) { curr_mode++; curr_mode_count = 0; }
      count++;
   }
#undef SET
   if (g_proj) opus_projection_encoder_destroy((OpusProjectionEncoder*)enc); else if (ms) opus_multistream_encoder_destroy((OpusMSEncoder*)enc); else opus_encoder_destroy((OpusEncoder*)enc);
   fclose(fin); fclose(fpkt); fclose(ftr); free(in); free(in32); free(data);
   return 0;
}

/* ---- dec ----------------------------------------------------------------- */
static int cmd_dec(int argc, char **argv) {
   int rate = 48000, ch = 1, lossperc = 0, fec = 0, gain = 0, use_float = 0, nophaseinv = 0, pcm24 = 0, ignoreext = 0;
   opus_int32 *out32 = NULL;
   int ms = 0, streams = 0, coupled = 0; unsigned char mapping[MAX_CH]; int nmap = 0;
   struct sched sched[MAX_SCHED]; int nsched = 0;
   const char *pkt_path = NULL, *trace_path = NULL, *out_path = NULL;
   int i, err, count = 0, toggle = 0, lost = 0, lost_prev = 0, skip = 0;
   void *dec; FILE *fpkt, *ftr, *fout = NULL;
   unsigned char *data[2]; int len[2]; opus_uint32 enc_rng[2];
   opus_int16 *out; float *outf = NULL;

   lcg_state = 1;
   for (i = 0; i < argc; i++) {
      const char *a = argv[i];
#define NEXT (i+1 < argc ? argv[++i] : (dief("missing value for", a), (char*)0))
      if (!strcmp(a, "--rate")) rate = atoi(NEXT);
      else if (!strcmp(a, "--ch")) ch = atoi(NEXT);
      else if (!strcmp(a, "--lossperc")) lossperc = atoi(NEXT);
      else if (!strcmp(a, "--fec")) fec = 1;
      else if (!strcmp(a, "--gain")) gain = atoi(NEXT);
      else if (!strcmp(a, "--skip")) skip = atoi(NEXT);
      else if (!strcmp(a, "--float")) use_float = 1;
      else if (!strcmp(a, "--nophaseinv")) nophaseinv = 1;
      else if (!strcmp(a, "--pcm24")) pcm24 = 1;
      else if (!strcmp(a, "--ignore-ext")) ignoreext = 1;
      else if (!strcmp(a, "--projection")) { ms = 1; g_proj = 1; }
      else if (!strcmp(a, "--seed")) lcg_state = (uint32_t)strtoul(NEXT, NULL, 0);
      else if (!strcmp(a, "--sched")) nsched = parse_sched(NEXT, sched, rate);
      else if (!strcmp(a, "--streams")) { ms = 1; streams = atoi(NEXT); }
      else if (!strcmp(a, "--coupled")) { ms = 1; coupled = atoi(NEXT); }
      else if (!strcmp(a, "--mapping")) {
         char *v = NEXT, *t = strtok(v, ",");
         ms = 1;
         while (t) { if (nmap >= MAX_CH) die("mapping too long"); mapping[nmap++] = (unsigned char)atoi(t); t = strtok(NULL, ","); }
      }
      else if (a[0] == '-' && a[1] == '-') dief("unknown option", a);
      else if (!pkt_path) pkt_path = a;
      else if (!trace_path) trace_path = a;
      else if (!out_path) out_path = a;
      else dief("extra arg", a);
#undef NEXT
   }
   if (!pkt_path || !trace_path) die("usage: gen dec [opts] in.pkt out.trace [out.s16]");
   if (ch > MAX_CH) die("too many channels");
   fpkt = fopen(pkt_path, "rb"); if (!fpkt) dief("open", pkt_path);
   ftr = fopen(trace_path, "w"); if (!ftr) dief("open", trace_path);
   if (out_path) { fout = fopen(out_path, "wb"); if (!fout) dief("open", out_path); }

   if (g_proj) {
      int gain; unsigned char *matrix; opus_int32 msize;
      get_demixing_matrix(rate, ch, OPUS_APPLICATION_AUDIO, &matrix, &msize, &streams, &coupled, &gain);
      dec = opus_projection_decoder_create(rate, ch, streams, coupled, matrix, msize, &err);
      if (err != OPUS_OK) dief("projection dec create", opus_strerror(err));
      free(matrix);
   } else if (ms) {
      if (nmap != ch) die("mapping length must equal channels");
      dec = opus_multistream_decoder_create(rate, ch, streams, coupled, mapping, &err);
      if (err != OPUS_OK) dief("ms create", opus_strerror(err));
   } else {
      dec = opus_decoder_create(rate, ch, &err);
      if (err != OPUS_OK) dief("create", opus_strerror(err));
   }
#define SET(...) do { int r_ = DEC_CTL(dec, ms, __VA_ARGS__); if (r_ != OPUS_OK) { fprintf(stderr, "gen: dec ctl failed: %s\n", opus_strerror(r_)); exit(2);} } while (0)
#define GET_DUR(pv) SET(OPUS_GET_LAST_PACKET_DURATION(pv))
   if (gain) SET(OPUS_SET_GAIN(gain));
   if (nophaseinv) SET(OPUS_SET_PHASE_INVERSION_DISABLED(nophaseinv));
   if (ignoreext) SET(OPUS_SET_IGNORE_EXTENSIONS(ignoreext));
   {
      int v;
      SET(OPUS_GET_SAMPLE_RATE(&v)); fprintf(ftr, "H fs=%d", v);
      SET(OPUS_GET_GAIN(&v)); fprintf(ftr, " gain=%d", v);
      SET(OPUS_GET_PHASE_INVERSION_DISABLED(&v)); fprintf(ftr, " phaseinv=%d", v);
      if (!ms) { SET(OPUS_GET_IGNORE_EXTENSIONS(&v)); fprintf(ftr, " ignoreext=%d", v); } /* multistream ctl lacks it in 1.6 */
      fprintf(ftr, "\n");
   }
   data[0] = malloc(MAX_PACKET); data[1] = malloc(MAX_PACKET);
   len[0] = len[1] = 0; enc_rng[0] = enc_rng[1] = 0;
   out = malloc(sizeof(opus_int16)*MAX_FRAME*ch);
   if (use_float) outf = malloc(sizeof(float)*MAX_FRAME*ch);
   if (pcm24) out32 = malloc(sizeof(opus_int32)*MAX_FRAME*ch);

   for (;;) {
      uint32_t ulen, urng; int n, s_i, dur, pitch, dbw, decode_fec = 0; opus_uint32 drng;
      const unsigned char *pkt; int pktlen;
      if (!get_be32(fpkt, &ulen)) break;
      if (!get_be32(fpkt, &urng)) die("truncated packet file");
      if (ulen > MAX_PACKET) die("packet too large");
      len[toggle] = (int)ulen; enc_rng[toggle] = urng;
      if (ulen && fread(data[toggle], 1, ulen, fpkt) != ulen) die("truncated packet file");
      for (s_i = 0; s_i < nsched; s_i++) if (sched[s_i].frame == count) {
         if (!strcmp(sched[s_i].what, "gain")) SET(OPUS_SET_GAIN(sched[s_i].val));
         else if (!strcmp(sched[s_i].what, "reset")) SET(OPUS_RESET_STATE);
         else if (!strcmp(sched[s_i].what, "phaseinv")) SET(OPUS_SET_PHASE_INVERSION_DISABLED(sched[s_i].val));
         else dief("unknown dec sched key", sched[s_i].what);
      }
      lost = len[toggle] == 0 || (lossperc > 0 && (int)(lcg()%100) < lossperc);
      if (count < fec) { /* FEC: delay by one packet, exactly like opus_demo */
         fprintf(ftr, "D i=%d delayed\n", count);
         lost_prev = lost; count++; toggle = (toggle + fec) & 1; continue;
      }
      if (fec) {
         if (lost_prev) {
            GET_DUR(&dur);
            pkt = lost ? NULL : data[toggle]; pktlen = len[toggle]; n = dur; decode_fec = 1;
         } else {
            pkt = data[1-toggle]; pktlen = len[1-toggle]; n = MAX_FRAME;
            if (pktlen == 0) pkt = NULL;
         }
      } else {
         if (lost) { GET_DUR(&dur); n = dur; pkt = NULL; pktlen = 0; }
         else { n = MAX_FRAME; pkt = data[toggle]; pktlen = len[toggle]; }
      }
      if (pkt == NULL) pktlen = 0;
      if (pcm24) {
         if (g_proj) n = opus_projection_decode24((OpusProjectionDecoder*)dec, pkt, pktlen, out32, n, decode_fec);
         else if (ms) n = opus_multistream_decode24((OpusMSDecoder*)dec, pkt, pktlen, out32, n, decode_fec);
         else n = opus_decode24((OpusDecoder*)dec, pkt, pktlen, out32, n, decode_fec);
         if (n > 0) for (i = 0; i < n*ch; i++) out[i] = (opus_int16)(out32[i] >> 8);
      } else if (use_float) {
         if (g_proj) n = opus_projection_decode_float((OpusProjectionDecoder*)dec, pkt, pktlen, outf, n, decode_fec);
         else if (ms) n = opus_multistream_decode_float((OpusMSDecoder*)dec, pkt, pktlen, outf, n, decode_fec);
         else n = opus_decode_float((OpusDecoder*)dec, pkt, pktlen, outf, n, decode_fec);
         if (n > 0) for (i = 0; i < n*ch; i++) { float x = outf[i]*32768.f; int v = (int)(x < 0 ? x - 0.5f : x + 0.5f); if (v > 32767) v = 32767; if (v < -32768) v = -32768; out[i] = (opus_int16)v; }
      } else {
         if (g_proj) n = opus_projection_decode((OpusProjectionDecoder*)dec, pkt, pktlen, out, n, decode_fec);
         else if (ms) n = opus_multistream_decode((OpusMSDecoder*)dec, pkt, pktlen, out, n, decode_fec);
         else n = opus_decode((OpusDecoder*)dec, pkt, pktlen, out, n, decode_fec);
      }
      SET(OPUS_GET_FINAL_RANGE(&drng));
      SET(OPUS_GET_BANDWIDTH(&dbw));
      GET_DUR(&dur);
      pitch = 0;
      if (!ms) SET(OPUS_GET_PITCH(&pitch));
      if (n < 0) {
         fprintf(ftr, "D i=%d lost=%d fec=%d err=%d\n", count, lost, decode_fec, n);
      } else {
         fprintf(ftr, "D i=%d lost=%d fec=%d n=%d rng=%08x bw=%d dur=%d pitch=%d crc=%08x\n",
                 count, lost, decode_fec, n, (unsigned)drng, dbw, dur, pitch, pcm24 ? (unsigned)crc_s32(out32, n*ch) : (unsigned)crc_s16(out, n*ch));
         if (fout && n > skip) write_s16(fout, out + skip*ch, (n-skip)*ch);
         if (n < skip) skip -= n; else skip = 0;
      }
      lost_prev = lost; count++; toggle = (toggle + fec) & 1;
   }
#undef SET
#undef GET_DUR
   if (g_proj) opus_projection_decoder_destroy((OpusProjectionDecoder*)dec); else if (ms) opus_multistream_decoder_destroy((OpusMSDecoder*)dec); else opus_decoder_destroy((OpusDecoder*)dec);
   fclose(fpkt); fclose(ftr); if (fout) fclose(fout);
   free(data[0]); free(data[1]); free(out); free(outf); free(out32);
   return 0;
}

/* ---- info: packet parsing helpers ---------------------------------------- */
static int cmd_info(int argc, char **argv) {
   const char *pkt_path = NULL, *trace_path = NULL; int rate = 48000, i, count = 0;
   FILE *fpkt, *ftr; unsigned char data[MAX_PACKET]; OpusDecoder *dec;
   for (i = 0; i < argc; i++) {
      if (!strcmp(argv[i], "--rate") && i+1 < argc) rate = atoi(argv[++i]);
      else if (!pkt_path) pkt_path = argv[i]; else if (!trace_path) trace_path = argv[i]; else dief("extra arg", argv[i]);
   }
   if (!pkt_path || !trace_path) die("usage: gen info [--rate R] in.pkt out.trace");
   fpkt = fopen(pkt_path, "rb"); if (!fpkt) dief("open", pkt_path);
   ftr = fopen(trace_path, "w"); if (!ftr) dief("open", trace_path);
   dec = opus_decoder_create(rate, 1, NULL);
   for (;;) {
      uint32_t ulen, urng; int nf, bw, spf, nch, ns, dns, j; unsigned char toc; const unsigned char *frames[48]; opus_int16 sizes[48]; int payload_offset;
      if (!get_be32(fpkt, &ulen)) break;
      if (!get_be32(fpkt, &urng)) die("truncated");
      if (ulen > MAX_PACKET) die("too large");
      if (ulen && fread(data, 1, ulen, fpkt) != ulen) die("truncated");
      if (ulen == 0) { fprintf(ftr, "P i=%d empty\n", count++); continue; }
      nf = opus_packet_parse(data, ulen, &toc, frames, sizes, &payload_offset);
      bw = opus_packet_get_bandwidth(data);
      spf = opus_packet_get_samples_per_frame(data, rate);
      nch = opus_packet_get_nb_channels(data);
      ns = opus_packet_get_nb_samples(data, ulen, rate);
      dns = opus_decoder_get_nb_samples(dec, data, ulen);
      fprintf(ftr, "P i=%d len=%u toc=%02x nf=%d bw=%d spf=%d nch=%d ns=%d dns=%d lbrr=%d off=%d sizes=", count, (unsigned)ulen, toc, nf, bw, spf, nch, ns, dns, opus_packet_has_lbrr(data, ulen), nf > 0 ? payload_offset : -1);
      if (nf > 0) for (j = 0; j < nf; j++) fprintf(ftr, "%s%d@%d", j ? "," : "", sizes[j], (int)(frames[j]-data));
      fprintf(ftr, "\n");
      count++;
   }
   opus_decoder_destroy(dec);
   fclose(fpkt); fclose(ftr);
   return 0;
}

/* ---- repack: repacketizer ------------------------------------------------ */
/* --merge N : concatenate groups of N consecutive packets.
 * --split   : split every packet into single-frame packets via out_range.
 * Output packets keep rng=0 (encoder range is meaningless after repacketizing). */
static int cmd_repack(int argc, char **argv) {
   const char *in_path = NULL, *out_path = NULL, *trace_path = NULL; int merge = 1, split = 0, pad = 0, streams = 0, i, count = 0;
   FILE *fin, *fout, *ftr; unsigned char *bufs[48]; unsigned char outbuf[MAX_PACKET*48]; OpusRepacketizer *rp; int inrp = 0;
   for (i = 0; i < argc; i++) {
      if (!strcmp(argv[i], "--merge") && i+1 < argc) merge = atoi(argv[++i]);
      else if (!strcmp(argv[i], "--split")) split = 1;
      else if (!strcmp(argv[i], "--pad") && i+1 < argc) pad = atoi(argv[++i]);
      else if (!strcmp(argv[i], "--streams") && i+1 < argc) streams = atoi(argv[++i]);
      else if (!in_path) in_path = argv[i]; else if (!out_path) out_path = argv[i]; else if (!trace_path) trace_path = argv[i]; else dief("extra arg", argv[i]);
   }
   if (!in_path || !out_path || !trace_path) die("usage: gen repack [--merge N|--split|--pad N [--streams S]] in.pkt out.pkt out.trace");
   if (merge < 1 || merge > 48) die("merge out of range");
   fin = fopen(in_path, "rb"); if (!fin) dief("open", in_path);
   fout = fopen(out_path, "wb"); if (!fout) dief("open", out_path);
   ftr = fopen(trace_path, "w"); if (!ftr) dief("open", trace_path);
   for (i = 0; i < 48; i++) bufs[i] = malloc(MAX_PACKET);
   rp = opus_repacketizer_create();
   for (;;) {
      uint32_t ulen, urng; int r;
      if (!get_be32(fin, &ulen)) break;
      if (!get_be32(fin, &urng)) die("truncated");
      if (ulen > MAX_PACKET) die("too large");
      if (ulen && fread(bufs[inrp], 1, ulen, fin) != ulen) die("truncated");
      if (ulen == 0) { fprintf(ftr, "R i=%d empty\n", count++); continue; }
      if (pad) {
         /* opus_packet_pad / opus_multistream_packet_pad in place, then unpad a copy */
         opus_int32 newlen = (opus_int32)ulen + pad, ul;
         if (newlen > (opus_int32)sizeof(outbuf)) die("pad too large");
         memcpy(outbuf, bufs[0], ulen);
         if (streams > 0) r = opus_multistream_packet_pad(outbuf, ulen, newlen, streams);
         else r = opus_packet_pad(outbuf, ulen, newlen);
         fprintf(ftr, "R i=%d pad=%d", count, r);
         if (r == OPUS_OK) {
            unsigned char *tmp = malloc(newlen);
            memcpy(tmp, outbuf, newlen);
            if (streams > 0) ul = opus_multistream_packet_unpad(tmp, newlen, streams);
            else ul = opus_packet_unpad(tmp, newlen);
            fprintf(ftr, " unpad=%d", (int)ul);
            if (ul > 0) fprintf(ftr, " ucrc=%08x", (unsigned)crc_bytes(tmp, ul));
            free(tmp);
            put_be32(fout, (uint32_t)newlen); put_be32(fout, 0); if (fwrite(outbuf, 1, newlen, fout) != (size_t)newlen) die("write");
         }
         fprintf(ftr, "\n");
         count++;
         continue;
      }
      if (split) {
         int nf, j;
         opus_repacketizer_init(rp);
         r = opus_repacketizer_cat(rp, bufs[0], ulen);
         nf = opus_repacketizer_get_nb_frames(rp);
         fprintf(ftr, "R i=%d cat=%d nf=%d outs=", count, r, nf);
         if (r == OPUS_OK) for (j = 0; j < nf; j++) {
            opus_int32 ol = opus_repacketizer_out_range(rp, j, j+1, outbuf, sizeof(outbuf));
            fprintf(ftr, "%s%d", j ? "," : "", (int)ol);
            if (ol > 0) { put_be32(fout, (uint32_t)ol); put_be32(fout, 0); if (fwrite(outbuf, 1, ol, fout) != (size_t)ol) die("write"); }
         }
         fprintf(ftr, "\n");
         count++;
         continue;
      }
      if (inrp == 0) opus_repacketizer_init(rp);
      r = opus_repacketizer_cat(rp, bufs[inrp], ulen);
      fprintf(ftr, "R i=%d cat=%d nf=%d\n", count, r, opus_repacketizer_get_nb_frames(rp));
      if (r == OPUS_OK) inrp++;
      else { /* flush what we have and start over with this packet */
         opus_int32 ol = opus_repacketizer_out(rp, outbuf, sizeof(outbuf));
         fprintf(ftr, "O len=%d\n", (int)ol);
         if (ol > 0) { put_be32(fout, (uint32_t)ol); put_be32(fout, 0); if (fwrite(outbuf, 1, ol, fout) != (size_t)ol) die("write"); }
         opus_repacketizer_init(rp);
         /* the packet becomes frame 0: move it to slot 0 so that later packets cannot overwrite it */
         if (inrp != 0) memcpy(bufs[0], bufs[inrp], ulen);
         r = opus_repacketizer_cat(rp, bufs[0], ulen);
         fprintf(ftr, "R i=%d cat=%d nf=%d\n", count, r, opus_repacketizer_get_nb_frames(rp));
         inrp = r == OPUS_OK ? 1 : 0;
      }
      if (inrp == merge) {
         opus_int32 ol = opus_repacketizer_out(rp, outbuf, sizeof(outbuf));
         fprintf(ftr, "O len=%d\n", (int)ol);
         if (ol > 0) { put_be32(fout, (uint32_t)ol); put_be32(fout, 0); if (fwrite(outbuf, 1, ol, fout) != (size_t)ol) die("write"); }
         inrp = 0;
      }
      count++;
   }
   if (inrp > 0) {
      opus_int32 ol = opus_repacketizer_out(rp, outbuf, sizeof(outbuf));
      fprintf(ftr, "O len=%d\n", (int)ol);
      if (ol > 0) { put_be32(fout, (uint32_t)ol); put_be32(fout, 0); if (fwrite(outbuf, 1, ol, fout) != (size_t)ol) die("write"); }
   }
   opus_repacketizer_destroy(rp);
   for (i = 0; i < 48; i++) free(bufs[i]);
   fclose(fin); fclose(fout); fclose(ftr);
   return 0;
}

int main(int argc, char **argv) {
   crc_init();
   if (argc < 2) die("usage: gen enc|dec|info|repack|version ...");
   if (!strcmp(argv[1], "version")) { printf("%s\n", opus_get_version_string()); return 0; }
   if (!strcmp(argv[1], "enc")) return cmd_enc(argc-2, argv+2);
   if (!strcmp(argv[1], "dec")) return cmd_dec(argc-2, argv+2);
   if (!strcmp(argv[1], "info")) return cmd_info(argc-2, argv+2);
   if (!strcmp(argv[1], "repack")) return cmd_repack(argc-2, argv+2);
   dief("unknown command", argv[1]);
   return 2;
}
