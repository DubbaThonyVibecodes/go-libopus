#!/usr/bin/env python3
"""Regenerate celt/static_modes_fixed_tables.go from celt/static_modes_fixed.h
(libopus 1.6: the header carries #ifdef ENABLE_QEXT alternatives; this keeps
the !ENABLE_QEXT branches, i.e. the configuration we port)."""
import re, sys
src, out = sys.argv[1], sys.argv[2]
lines = open(src).read().split('\n')
# Minimal preprocessor for ENABLE_QEXT only (other conditionals are kept verbatim).
kept = []
stack = []  # each entry: (is_qext_cond, currently_active)
def active():
    return all(a for _, a in stack)
for ln in lines:
    s = ln.strip()
    m = re.match(r'#\s*(ifdef|ifndef)\s+(\w+)', s)
    if m:
        if m.group(2) == 'ENABLE_QEXT':
            stack.append((True, m.group(1) == 'ifndef'))
            continue
        stack.append((False, True))
        if active(): kept.append(ln)
        continue
    if re.match(r'#\s*if\b', s):
        stack.append((False, True))
        if active(): kept.append(ln)
        continue
    if re.match(r'#\s*else\b', s):
        q, a = stack[-1]
        if q:
            stack[-1] = (True, not a)
            continue
        if active(): kept.append(ln)
        continue
    if re.match(r'#\s*endif\b', s):
        q, a = stack.pop()
        if q: continue
        if active(): kept.append(ln)
        continue
    if active(): kept.append(ln)
text = '\n'.join(kept)
text = re.sub(r'/\*.*?\*/', '', text, flags=re.S)
TYPES = {'opus_val16': 'int16', 'celt_coef': 'int16', 'opus_int16': 'int16', 'unsigned char': 'uint8', 'kiss_twiddle_cpx': 'kiss_twiddle_cpx'}
body = []
seen = set()
for m in re.finditer(r'static const (opus_val16|celt_coef|opus_int16|unsigned char|kiss_twiddle_cpx) (\w+)\[(\d+)\] = \{(.*?)\};', text, flags=re.S):
    ctype, name, n, init = m.groups()
    if name in seen: continue
    seen.add(name)
    gotype = TYPES[ctype]
    if ctype == 'kiss_twiddle_cpx':
        items = re.findall(r'\{\s*(-?\d+)\s*,\s*(-?\d+)\s*\}', init)
        assert len(items) == int(n), (name, len(items))
        items = ['{%s, %s}' % it for it in items]
        per = 8
    else:
        items = re.findall(r'-?\d+', init)
        assert len(items) == int(n), (name, len(items))
        per = 16
    lines = ['\t' + ', '.join(items[i:i+per]) + ',' for i in range(0, len(items), per)]
    body.append(f'var {name} = [{n}]{gotype}{{\n' + '\n'.join(lines) + '\n}\n')
header = 'package celt\n\n// Code generated from upstream celt/static_modes_fixed.h (dump_modes 48000 960 96000 1920, !ENABLE_QEXT branches); DO NOT EDIT.\n//\n// C: celt/static_modes_fixed.h — static definitions for the pre-defined mode.\n\n'
open(out, 'w').write(header + '\n'.join(body))
for m in re.finditer(r'static const kiss_fft_state (\w+) = \{(.*?)\};', text, flags=re.S):
    print(m.group(1), re.sub(r'\s+', ' ', m.group(2)))
for m in re.finditer(r'static const CELTMode (\w+) = \{(.*?)\};', text, flags=re.S):
    print(m.group(1), re.sub(r'\s+', ' ', m.group(2)))
