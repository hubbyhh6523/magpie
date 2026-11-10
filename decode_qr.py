#!/usr/bin/env python3
"""Decode the half-block QR art from a terminal log back into an image."""
import re, sys

LOG = '/tmp/q5.log'
OUT = '/workspace/uuqr.png'

lines = open(LOG, encoding='utf-8', errors='replace').read().splitlines()

# Keep only the full-block lines that make up the QR body.
art = [l for l in lines if l.startswith('\u2588') or l.startswith('\u2584') or l.startswith('\u2580')]
if not art:
    sys.exit('no QR art found')

# Each text row encodes two pixel rows: upper half-block = top, lower = bottom.
# Light modules are drawn as '█' with a border, so we must infer polarity from
# the quiet zone: the first/last rows are all '█' (the white margin).
width = len(art[0])
pixels = []          # list of rows, each a string of '0'/'1'

for row in art:
    top = []
    bot = []
    for ch in row:
        if ch == '\u2588':      # full block -> light module (white)
            top.append('0'); bot.append('0')
        elif ch == '\u2584':    # lower half -> top light, bottom dark
            top.append('0'); bot.append('1')
        elif ch == '\u2580':    # upper half -> top dark, bottom light
            top.append('1'); bot.append('0')
        elif ch == ' ':         # space -> dark module (black)
            top.append('1'); bot.append('1')
        else:
            top.append('0'); bot.append('0')
    pixels.append(''.join(top))
    pixels.append(''.join(bot))

# Drop the surrounding padding rows the terminal adds.
while pixels and set(pixels[0]) == {'0'}:
    pixels.pop(0)
while pixels and set(pixels[-1]) == {'0'}:
    pixels.pop()

w = len(pixels[0])
h = len(pixels)
print(f'grid {w}x{h}')

# Render as PNG with a generous module size + quiet zone.
scale = 10
quiet = 4
from struct import pack
import zlib

size = (w + quiet * 2) * scale
canvas = bytearray(b'\xff' * (size * size))  # white background

for y, row in enumerate(pixels):
    for x, px in enumerate(row):
        if px == '1':
            for dy in range(scale):
                base = ((y + quiet) * scale + dy) * size
                start = (x + quiet) * scale
                canvas[base + start: base + start + scale] = b'\x00' * scale

raw = b''.join(b'\x00' + bytes(canvas[y * size:(y + 1) * size]) for y in range(size))

def chunk(tag, data):
    return pack('>I', len(data)) + tag + data + pack('>I', zlib.crc32(tag + data) & 0xffffffff)

png = (b'\x89PNG\r\n\x1a\n'
       + chunk(b'IHDR', pack('>IIBBBBB', size, size, 8, 0, 0, 0, 0))
       + chunk(b'IDAT', zlib.compress(raw, 9))
       + chunk(b'IEND', b''))

open(OUT, 'wb').write(png)
print(f'wrote {OUT} {size}x{size}')