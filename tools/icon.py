#!/usr/bin/env python3
"""Rysuje ikone programu i zapisuje ja jako assets/icon.ico (+ podglad PNG).

Motyw: trzy tory wyscigu (paski) na kobaltowym tle, zloty pasek = zwyciezca.
Male rozmiary (<=64 px) zapisujemy jako klasyczne BMP 32-bit, duze jako PNG.
"""
import io
import os
import struct

from PIL import Image, ImageDraw

HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(HERE, "..", "assets")
SS = 4            # supersampling
BASE = 1024
SIZES = [16, 24, 32, 48, 64, 128, 256]

COBALT_TOP = (52, 104, 255)
COBALT_BOT = (22, 52, 176)
GOLD = (255, 196, 20)
WHITE = (255, 255, 255)


def gradient(size, top, bot):
    mask = Image.linear_gradient("L").resize((size, size))
    return Image.composite(Image.new("RGB", (size, size), bot),
                           Image.new("RGB", (size, size), top), mask)


def master():
    n = BASE * SS
    k = n / 1024.0
    img = Image.new("RGBA", (n, n), (0, 0, 0, 0))

    # tlo: zaokraglony kwadrat z pionowym gradientem
    bg = gradient(n, COBALT_TOP, COBALT_BOT).convert("RGBA")
    m = Image.new("L", (n, n), 0)
    ImageDraw.Draw(m).rounded_rectangle([int(24 * k), int(24 * k), int(1000 * k), int(1000 * k)],
                                        radius=int(228 * k), fill=255)
    img.paste(bg, (0, 0), m)

    d = ImageDraw.Draw(img)

    # linia startu
    x0 = int(214 * k)
    d.rounded_rectangle([x0 - int(18 * k), int(236 * k), x0, int(788 * k)],
                        radius=int(9 * k), fill=WHITE + (235,))

    # trzy tory: zloty zwyciezca, dwa biale
    bars = [
        (254, 640, GOLD + (255,)),
        (418, 500, WHITE + (240,)),
        (582, 340, WHITE + (170,)),
    ]
    h = 128
    for y, length, col in bars:
        d.rounded_rectangle([x0 + int(26 * k), int(y * k), x0 + int((26 + length) * k), int((y + h) * k)],
                            radius=int(h / 2 * k), fill=col)

    return img.resize((BASE, BASE), Image.LANCZOS)


def bmp_entry(im):
    s = im.width
    px = im.convert("RGBA")
    rows = []
    for y in range(s - 1, -1, -1):  # BMP jest zapisany od dolu
        row = bytearray()
        for x in range(s):
            r, g, b, a = px.getpixel((x, y))
            row += bytes((b, g, r, a))
        rows.append(bytes(row))
    xor = b"".join(rows)
    and_row = ((s + 31) // 32) * 4
    andmask = bytes(and_row * s)
    hdr = struct.pack("<IiiHHIIiiII", 40, s, s * 2, 1, 32, 0, len(xor) + len(andmask), 0, 0, 0, 0)
    return hdr + xor + andmask


def png_entry(im):
    buf = io.BytesIO()
    im.save(buf, "PNG", optimize=True)
    return buf.getvalue()


def build():
    os.makedirs(OUT, exist_ok=True)
    big = master()
    big.save(os.path.join(OUT, "icon-1024.png"))

    entries = []
    for s in SIZES:
        im = big.resize((s, s), Image.LANCZOS)
        data = png_entry(im) if s >= 128 else bmp_entry(im)
        entries.append((s, data))

    head = struct.pack("<HHH", 0, 1, len(entries))
    offset = 6 + 16 * len(entries)
    dirs, blobs = b"", b""
    for s, data in entries:
        dirs += struct.pack("<BBBBHHII", 0 if s >= 256 else s, 0 if s >= 256 else s, 0, 0, 1, 32, len(data), offset)
        blobs += data
        offset += len(data)
    with open(os.path.join(OUT, "icon.ico"), "wb") as f:
        f.write(head + dirs + blobs)

    # arkusz podgladu: wszystkie rozmiary powiekszone (nearest), zeby ocenic male
    sheet = Image.new("RGBA", (1100, 420), (237, 243, 249, 255))
    x = 20
    for s in SIZES:
        im = big.resize((s, s), Image.LANCZOS)
        shown = im.resize((min(256, s * (256 // s if s < 128 else 1)),) * 2, Image.NEAREST) if s < 128 else im
        sheet.paste(shown, (x, 20), shown)
        sheet.paste(im, (x, 300), im)  # w rzeczywistym rozmiarze
        x += shown.width + 24
    sheet.save(os.path.join(OUT, "icon-preview.png"))
    print("OK:", [(s, len(d)) for s, d in entries])


if __name__ == "__main__":
    build()
