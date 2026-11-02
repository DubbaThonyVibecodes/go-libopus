#!/usr/bin/env python3
"""Generate Go stub files (one per upstream silk .c file) with the agreed Go
signatures of every non-static SILK function, derived from the headers.

Usage: silkstubs.py <upstream silk dir> <out go dir>
"""
import os, re, sys, glob

SRC, OUT = sys.argv[1], sys.argv[2]
HEADERS = ['main.h', 'fixed/main_FIX.h', 'SigProc_FIX.h', 'API.h', 'PLC.h', 'resampler_private.h']

TYPEMAP = {
    'opus_int': 'int', 'opus_int8': 'int8', 'opus_int16': 'int16', 'opus_int32': 'int32',
    'opus_int64': 'int64', 'opus_uint8': 'uint8', 'opus_uint16': 'uint16', 'opus_uint32': 'uint32',
    'silk_encoder_state': 'EncoderState', 'silk_encoder_state_FIX': 'EncoderStateFIX',
    'silk_encoder_state_Fxx': 'EncoderStateFIX', 'silk_encoder_control_FIX': 'EncoderControlFIX',
    'silk_decoder_state': 'DecoderState', 'silk_decoder_control': 'DecoderControl',
    'silk_nsq_state': 'NsqState', 'silk_VAD_state': 'VADState', 'silk_LP_state': 'LPState',
    'silk_NLSF_CB_struct': 'NLSFCBStruct', 'stereo_enc_state': 'StereoEncState',
    'stereo_dec_state': 'StereoDecState', 'SideInfoIndices': 'SideInfoIndices',
    'silk_resampler_state_struct': 'ResamplerStateStruct', 'silk_EncControlStruct': 'EncControlStruct',
    'silk_DecControlStruct': 'DecControlStruct', 'ec_enc': 'celt.EcEnc', 'ec_dec': 'celt.EcDec',
    'silk_PLC_struct': 'PLCStruct', 'silk_CNG_struct': 'CNGStruct',
}
STRUCTS = {v for k, v in TYPEMAP.items() if k.startswith(('silk_', 'stereo', 'Side', 'ec_')) and k not in ('silk_float',)}
# pointer params that are scalar out/in-out values rather than arrays
SCALAR_OUT = {'pN', 'scale', 'prev_ind', 'res_nrg', 'res_nrg_Q', 'ratio_Q14', 'lagIndex', 'contourIndex',
              'LTPCorr_Q15', 'energy', 'shift', 'rate_dist_Q14', 'ind', 'periodicity_index',
              'LTPredCodGain_Q7', 'lz', 'frac_Q7', 'decode_only_mid', 'nSamplesOut', 'nBytesOut',
              'encSizeBytes', 'decSizeBytes', 'pnBytesOut', 'mid_only_flag', 'rshifts'}
RECEIVER_TYPES = {'EncoderState', 'EncoderStateFIX', 'DecoderState', 'ResamplerStateStruct',
                  'StereoEncState', 'StereoDecState', 'VADState', 'LPState', 'Encoder', 'Decoder'}

def goname(n):
    if n.startswith('silk_'):
        n = n[5:]
    parts = n.split('_')
    return ''.join(p[:1].upper() + p[1:] for p in parts if p)

def strip_comments(s):
    s = re.sub(r'/\*.*?\*/', '', s, flags=re.S)
    s = re.sub(r'#if 0.*?#endif', '', s, flags=re.S)
    return s

def parse_param(p, fname):
    p = p.strip()
    if p == 'void' or not p:
        return None
    p = p.replace('const ', '').replace('silk_DWORD_ALIGN ', '')
    m = re.match(r'^([\w ]+?)\s*(\**)\s*(\w+)\s*((?:\[[^\]]*\])*)$', p.strip())
    if not m:
        m2 = re.match(r'^(\w+)\s*\*\s*const\s+(\w+)$', p)  # "const opus_int16 *const inVec1"
        if m2:
            ctype, stars, name, dims = m2.group(1), '*', m2.group(2), ''
        else:
            raise SystemExit(f'cannot parse param {p!r} in {fname}')
    else:
        ctype, stars, name, dims = m.groups()
    ctype = ctype.strip()
    if ctype == 'void' and stars:
        # void *encState / decState
        return (name, '*Encoder' if 'enc' in name else '*Decoder', True)
    base = TYPEMAP[ctype]
    dimlist = re.findall(r'\[([^\]]*)\]', dims)
    if base in STRUCTS:
        if dimlist:
            return (name, '[]' + base, False)
        return (name, '*' + base, True)
    if len(dimlist) >= 2:
        return (name, '*[' + ']['.join(d.strip() for d in dimlist) + ']' + base, False)
    if stars or dimlist:
        if name in SCALAR_OUT and not dimlist:
            return (name, '*' + base, False)
        return (name, '[]' + base, False)
    return (name, base, False)

protos = {}
for h in HEADERS:
    text = strip_comments(open(os.path.join(SRC, h)).read())
    for m in re.finditer(r'(?:^|\n)\s*(?:static\s+inline\s+)?(opus_int32|opus_int|void|opus_int16|opus_int64|opus_int8)\s+(silk_\w+|check_control_input)\s*\(([^;{]*?)\)\s*;', text):
        ret, name, params = m.groups()
        if name in protos:
            continue
        plist = [parse_param(p, name) for p in params.split(',')]
        plist = [p for p in plist if p]
        protos[name] = (ret, plist)

# which .c file defines which function
defs = {}
for c in glob.glob(os.path.join(SRC, '*.c')) + glob.glob(os.path.join(SRC, 'fixed', '*.c')):
    text = strip_comments(open(c).read())
    for m in re.finditer(r'(?:^|\n)(?:opus_int32|opus_int|void|opus_int16|opus_int64|opus_int8)\s+(silk_\w+|check_control_input)\s*\(', text):
        if m.group(1) in protos:
            defs.setdefault(os.path.basename(c), []).append(m.group(1))

missing = [n for n in protos if not any(n in v for v in defs.values())]
sys.stderr.write('no definition found (header-only or unused): ' + ', '.join(missing) + '\n')

for cfile, names in sorted(defs.items()):
    gofile = os.path.join(OUT, cfile[:-2] + '.go')
    lines = ['package silk', '', f'// C: silk/{cfile} — STUB, to be replaced by the port.', '']
    uses_celt = False
    for n in names:
        ret, plist = protos[n]
        recv = ''
        params = plist
        if plist and plist[0][2] and plist[0][1].lstrip('*') in RECEIVER_TYPES:
            recv = f'({plist[0][0]} {plist[0][1]}) '
            params = plist[1:]
        ps = ', '.join(f'{pn} {pt}' for pn, pt, _ in params)
        if 'celt.' in ps:
            uses_celt = True
        rt = '' if ret == 'void' else ' ' + TYPEMAP[ret]
        lines.append(f'// C: {ret} {n}(...)')
        lines.append(f'func {recv}{goname(n)}({ps}){rt} {{')
        lines.append(f'\tpanic("TODO: port silk/{cfile}:{n}")')
        lines.append('}')
        lines.append('')
    if uses_celt:
        lines.insert(2, 'import "github.com/DubbaThonyVibecodes/go-libopus/celt"\n')
    open(gofile, 'w').write('\n'.join(lines))
    print(gofile, names)
