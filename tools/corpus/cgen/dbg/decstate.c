/* Differential debug harness: decodes a packet file like `gen dec` (no FEC)
 * and prints a per-frame dump of SILK decoder channel-0 state. */
#include "config.h"
#include "../../../../third_party/opus/src/opus_decoder.c"
#include "../../../../third_party/opus/silk/structs.h"
typedef struct { silk_decoder_state channel_state[ DECODER_NUM_CHANNELS ]; stereo_dec_state sStereo; opus_int nChannelsAPI; opus_int nChannelsInternal; opus_int prev_decode_only_middle; } silk_decoder;
#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>

static uint32_t lcg_state = 1;
static uint32_t lcg(void) { lcg_state = lcg_state*1664525u + 1013904223u; return lcg_state >> 8; }
static int get_be32(FILE *f, uint32_t *v) { unsigned char b[4]; if (fread(b,1,4,f)!=4) return 0; *v=((uint32_t)b[0]<<24)|((uint32_t)b[1]<<16)|((uint32_t)b[2]<<8)|b[3]; return 1; }
static int64_t h16(const opus_int16 *p, int n) { int64_t h = 0; int i; for (i=0;i<n;i++) h = h*31 + p[i]; return h; }
static int64_t h32(const opus_int32 *p, int n) { int64_t h = 0; int i; for (i=0;i<n;i++) h = h*31 + p[i]; return h; }

int main(int argc, char **argv) {
   int rate = atoi(argv[2]), ch = atoi(argv[3]), lossperc = atoi(argv[4]); lcg_state = (uint32_t)strtoul(argv[5], NULL, 0);
   FILE *f = fopen(argv[1], "rb"); int err; int count = 0;
   OpusDecoder *st = opus_decoder_create(rate, ch, &err);
   silk_decoder *sd = (silk_decoder*)((char*)st + st->silk_dec_offset);
   unsigned char data[1500]; opus_int16 out[5760*8];
   for (;;) {
      uint32_t ulen, urng; int n, lost; opus_int32 dur;
      if (!get_be32(f, &ulen)) break; get_be32(f, &urng);
      if (ulen && fread(data, 1, ulen, f) != ulen) break;
      lost = ulen == 0 || (lossperc > 0 && (int)(lcg()%100) < lossperc);
      if (lost) { opus_decoder_ctl(st, OPUS_GET_LAST_PACKET_DURATION(&dur)); n = opus_decode(st, NULL, 0, out, dur, 0); }
      else n = opus_decode(st, data, ulen, out, 5760, 0);
      {
         silk_decoder_state *s = &sd->channel_state[0];
         printf("F %d lost=%d n=%d mode=%d pcm=%lld | lossCnt=%d sig=%d lagPrev=%d lgi=%d ffar=%d fs=%d nsf=%d sfl=%d exc=%lld sLPC=%lld outBuf=%lld NLSF=%lld pg=%d | PLC pitch=%d ltp=%lld lpc=%lld lfl=%d seed=%d rs=%d ce=%d ces=%d pls=%d pg0=%d pg1=%d | CNG g=%d seed=%d syn=%lld nlsf=%lld exc=%lld\n",
            count, lost, n, st->mode, (long long)h16(out, n > 0 ? n*ch : 0),
            s->lossCnt, s->prevSignalType, s->lagPrev, s->LastGainIndex, s->first_frame_after_reset, s->fs_kHz, s->nb_subfr, s->subfr_length,
            (long long)h32(s->exc_Q14, MAX_FRAME_LENGTH), (long long)h32(s->sLPC_Q14_buf, MAX_LPC_ORDER), (long long)h16(s->outBuf, MAX_FRAME_LENGTH + 2*MAX_SUB_FRAME_LENGTH), (long long)h16(s->prevNLSF_Q15, MAX_LPC_ORDER), s->prev_gain_Q16,
            s->sPLC.pitchL_Q8, (long long)h16(s->sPLC.LTPCoef_Q14, LTP_ORDER), (long long)h16(s->sPLC.prevLPC_Q12, MAX_LPC_ORDER), s->sPLC.last_frame_lost, s->sPLC.rand_seed, s->sPLC.randScale_Q14, s->sPLC.conc_energy, s->sPLC.conc_energy_shift, s->sPLC.prevLTP_scale_Q14, s->sPLC.prevGain_Q16[0], s->sPLC.prevGain_Q16[1],
            s->sCNG.CNG_smth_Gain_Q16, s->sCNG.rand_seed, (long long)h32(s->sCNG.CNG_synth_state, MAX_LPC_ORDER), (long long)h16(s->sCNG.CNG_smth_NLSF_Q15, MAX_LPC_ORDER), (long long)h32(s->sCNG.CNG_exc_buf_Q14, MAX_FRAME_LENGTH));
      }
      count++;
   }
   return 0;
}
