#!/usr/bin/env python3
"""Render terminal half-block QR art to a scannable PNG.

Standard block semantics (a block character paints its cell(s)):
    '█' full block : top cell ink, bottom cell ink
    '▀' upper half : top cell ink, bottom cell blank
    '▄' lower half : top cell blank, bottom cell ink
    ' ' space      : both cells blank

Ink is foreground (dark) and blank is background (light), which is exactly the
polarity a QR decoder expects once we emit dark modules for ink.
"""
import sys, zlib
from struct import pack

LOG = sys.argv[1] if len(sys.argv) > 1 else '/tmp/q5.log'
OUT = sys.argv[2] if len(sys.argv) > 2 else '/workspace/uuqr.png'

lines = open(LOG, encoding='utf-8', errors='replace').read().splitlines()
art = [l for l in lines if l.startswith('\u2588')]
if not art:
    sys.exit('no QR art found')

# Rows made purely of full blocks are the art's own quiet-zone padding.
# Keep them: a QR needs a light margin, and here light == blank == no ink.
W = len(art[0])
rows = []
for line in art:
    top, bot = [], []
    for ch in line:
        if ch == '\u2588':
            top.append(1); bot.append(1)
        elif ch == '\u2580':
            top.append(1); bot.append(0)
        elif ch == '\u2584':
            top.append(0); bot.append(1)
        else:
            top.append(0); bot.append(0)
    rows.append(top)
    rows.append(bot)

# Trim to a square: a QR is always square, so drop excess rows from the bottom.
H = len(rows)
if H > W:
    rows = rows[:W]
elif W > H:
    sys.exit(f'art not square: {W}x{H}')

print(f'grid {W}x{len(rows)}')

scale = 12
size = W * scale
canvas = bytearray(b'\xff' * (size * size))

for y, row in enumerate(rows):
    for x, ink in enumerate(row):
        if ink:
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

from PIL import Image
from pyzbar.pyzbar import decode
res = decode(Image.open(OUT))
print(f'wrote {OUT} {size}x{size}')
print('DECODED:', [d.data.decode() for d in res] if res else 'NO DECODE')