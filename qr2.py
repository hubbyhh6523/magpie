#!/usr/bin/env python3
"""Render the half-block QR art in a terminal log into a scannable PNG.

Terminal QR art drawn with half-block characters packs two module rows into
each text row:
    '█' (full block)  -> both rows light
    '▄' (lower half)  -> upper light, lower dark
    '▀' (upper half)  -> upper dark,  lower light
    ' ' (space)       -> both rows dark

The art already carries its own quiet zone, so nothing is trimmed here.
"""
import sys, zlib
from struct import pack

LOG = sys.argv[1] if len(sys.argv) > 1 else '/tmp/q5.log'
OUT = sys.argv[2] if len(sys.argv) > 2 else '/workspace/uuqr.png'

lines = open(LOG, encoding='utf-8', errors='replace').read().splitlines()
art = [l for l in lines if l.startswith('\u2588')]
if not art:
    sys.exit('no QR art found')

W = len(art[0])
rows = []
for line in art:
    top, bot = [], []
    for ch in line:
        if ch == '\u2588':
            top.append(0); bot.append(0)
        elif ch == '\u2584':
            top.append(0); bot.append(1)
        elif ch == '\u2580':
            top.append(1); bot.append(0)
        else:
            top.append(1); bot.append(1)
    rows.append(top)
    rows.append(bot)

H = len(rows)
print(f'grid {W}x{H}')

scale = 12
size = W * scale
canvas = bytearray(b'\xff' * (size * size))

for y, row in enumerate(rows):
    for x, val in enumerate(row):
        if val:
            base = y * scale * size
            start = x * scale
            for dy in range(scale):
                off = base + dy * size + start
                canvas[off:off + scale] = b'\x00' * scale

raw = b''.join(b'\x00' + bytes(canvas[y * size:(y + 1) * size]) for y in range(size))

def chunk(tag, data):
    return pack('>I', len(data)) + tag + data + pack('>I', zlib.crc32(tag + data) & 0xffffffff)

png = (b'\x89PNG\r\n\x1a\n'
       + chunk(b'IHDR', pack('>IIBBBBB', size, size, 8, 0, 0, 0, 0))
       + chunk(b'IDAT', zlib.compress(raw, 9))
       + chunk(b'IEND', b''))

open(OUT, 'wb').write(png)
print(f'wrote {OUT} {size}x{size}')