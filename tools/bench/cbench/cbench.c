/*
 * cbench: times upstream libopus on the same workload that tools/bench runs
 * through the Go port.  Linked statically against an out-of-tree build of the
 * submodule (never the host's libopus); see tools/bench/Makefile.
 *
 *   cbench enc|dec in.s16 rate channels app bitrate complexity frame_ms iters
 *
 * The whole input is loaded into memory first.  "enc" times encoding every
 * frame of the input; "dec" encodes once (untimed) and times decoding every
 * packet.  Each iteration creates a fresh encoder/decoder; the best (minimum)
 * wall time over all iterations is reported, together with a checksum of the
 * produced packets/PCM so the Go side can confirm it ran the same workload.
 *
 * Output (one line):  ns=<best> frames=<n> bytes=<packet bytes> sum=<fnv32a>
 */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include <time.h>
#include "opus.h"

#define MAX_PACKET 1500

static double now_ns(void) {
   struct timespec ts;
   clock_gettime(CLOCK_MONOTONIC, &ts);
   return ts.tv_sec*1e9 + ts.tv_nsec;
}

static uint32_t fnv(uint32_t h, const unsigned char *p, size_t n) {
   size_t i;
   for (i = 0; i < n; i++) { h ^= p[i]; h *= 16777619u; }
   return h;
}

static int parse_app(const char *s) {
   if (!strcmp(s, "voip")) return OPUS_APPLICATION_VOIP;
   if (!strcmp(s, "audio")) return OPUS_APPLICATION_AUDIO;
   if (!strcmp(s, "lowdelay")) return OPUS_APPLICATION_RESTRICTED_LOWDELAY;
   fprintf(stderr, "cbench: bad app %s\n", s); exit(2);
}

static OpusEncoder *mk_enc(int rate, int ch, int app, int bitrate, int cplx) {
   int err;
   OpusEncoder *enc = opus_encoder_create(rate, ch, app, &err);
   if (err != OPUS_OK) { fprintf(stderr, "cbench: %s\n", opus_strerror(err)); exit(2); }
   opus_encoder_ctl(enc, OPUS_SET_BITRATE(bitrate));
   opus_encoder_ctl(enc, OPUS_SET_COMPLEXITY(cplx));
   return enc;
}

int main(int argc, char **argv) {
   const char *mode, *path;
   int rate, ch, app, bitrate, cplx, iters, frame, nframes, i, it;
   double frame_ms, best = 1e30;
   FILE *f; long size; opus_int16 *pcm, *out;
   unsigned char *pkts; int *lens; long total_bytes = 0; uint32_t sum = 2166136261u;

   if (argc != 10) { fprintf(stderr, "usage: cbench enc|dec in.s16 rate ch app bitrate complexity frame_ms iters\n"); return 2; }
   mode = argv[1]; path = argv[2]; rate = atoi(argv[3]); ch = atoi(argv[4]); app = parse_app(argv[5]);
   bitrate = atoi(argv[6]); cplx = atoi(argv[7]); frame_ms = atof(argv[8]); iters = atoi(argv[9]);
   frame = (int)(rate*frame_ms/1000 + .5);

   f = fopen(path, "rb"); if (!f) { perror(path); return 2; }
   fseek(f, 0, SEEK_END); size = ftell(f); fseek(f, 0, SEEK_SET);
   nframes = (int)(size/2/ch/frame);
   pcm = malloc((size_t)nframes*frame*ch*sizeof(opus_int16));
   if (fread(pcm, sizeof(opus_int16), (size_t)nframes*frame*ch, f) != (size_t)nframes*frame*ch) { fprintf(stderr, "cbench: short read\n"); return 2; }
   fclose(f);
   pkts = malloc((size_t)nframes*MAX_PACKET); lens = malloc(sizeof(int)*nframes);
   out = malloc(sizeof(opus_int16)*5760*ch);

   if (!strcmp(mode, "enc")) {
      for (it = 0; it < iters; it++) {
         OpusEncoder *enc = mk_enc(rate, ch, app, bitrate, cplx);
         double t0 = now_ns(), t;
         for (i = 0; i < nframes; i++)
            lens[i] = opus_encode(enc, pcm + (size_t)i*frame*ch, frame, pkts + (size_t)i*MAX_PACKET, MAX_PACKET);
         t = now_ns() - t0;
         if (t < best) best = t;
         opus_encoder_destroy(enc);
      }
      for (i = 0; i < nframes; i++) { total_bytes += lens[i]; sum = fnv(sum, pkts + (size_t)i*MAX_PACKET, lens[i]); }
   } else if (!strcmp(mode, "dec")) {
      OpusEncoder *enc = mk_enc(rate, ch, app, bitrate, cplx);
      for (i = 0; i < nframes; i++) {
         lens[i] = opus_encode(enc, pcm + (size_t)i*frame*ch, frame, pkts + (size_t)i*MAX_PACKET, MAX_PACKET);
         total_bytes += lens[i];
      }
      opus_encoder_destroy(enc);
      for (it = 0; it < iters; it++) {
         int err; OpusDecoder *dec = opus_decoder_create(rate, ch, &err);
         double t0 = now_ns(), t;
         uint32_t s = 2166136261u;
         for (i = 0; i < nframes; i++) {
            int n = opus_decode(dec, pkts + (size_t)i*MAX_PACKET, lens[i], out, 5760, 0);
            if (it == 0 && n > 0) s = fnv(s, (const unsigned char*)out, (size_t)n*ch*2);
         }
         t = now_ns() - t0;
         if (t < best) best = t;
         if (it == 0) sum = s;
         opus_decoder_destroy(dec);
      }
   } else { fprintf(stderr, "cbench: bad mode %s\n", mode); return 2; }
   printf("ns=%.0f frames=%d bytes=%ld sum=%08x\n", best, nframes, total_bytes, (unsigned)sum);
   free(pcm); free(out); free(pkts); free(lens);
   return 0;
}
