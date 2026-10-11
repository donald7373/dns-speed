#!/usr/bin/env python3
"""Nakłada tekst na obraz podgladu okna (ksztalty rysuje Go, tekst robi PIL).

Uzycie: overlay.py plik.png   (obok musi lezec plik.json z lista napisow)
Do podgladu uzywamy czcionki Inter, ktora jest nieco szersza niz Segoe UI,
wiec jesli tekst miesci sie tutaj, w Windows tez sie zmiesci.
"""
import json
import sys

from PIL import Image, ImageDraw, ImageFont

DIR = "/usr/share/fonts/opentype/inter/"
# numery jak w canvas.go: Title, H2, Body, BodyB, Small, SmallB, Tiny, Num
FILES = ["Inter-Bold.otf", "Inter-SemiBold.otf", "Inter-Regular.otf", "Inter-SemiBold.otf",
         "Inter-Regular.otf", "Inter-SemiBold.otf", "Inter-Regular.otf", "Inter-Regular.otf"]
PX = [40, 24, 15, 15, 13, 13, 12, 13]

AL_C, AL_R, VMID, WRAP, ELL = 1, 2, 4, 8, 16


def main():
    path = sys.argv[1]
    data = json.load(open(path[:-4] + ".json"))
    scale = data["scale"]
    im = Image.open(path).convert("RGB")
    d = ImageDraw.Draw(im)
    cache = {}

    def font(i):
        if i not in cache:
            cache[i] = ImageFont.truetype(DIR + FILES[i], max(6, round(PX[i] * scale * 0.93)))
        return cache[i]

    def fit(text, f, maxw):
        if d.textlength(text, font=f) <= maxw or maxw <= 0:
            return text
        while text and d.textlength(text + "…", font=f) > maxw:
            text = text[:-1]
        return text + "…"

    clipped = []
    for op in data["ops"]:
        f = font(op["f"])
        s, fl = op["s"], op["fl"]
        x, y, w, h = (op[k] * scale for k in ("x", "y", "w", "h"))
        col = tuple(op["c"])
        if fl & WRAP:
            words, lines, cur = s.split(), [], ""
            for wd in words:
                t = (cur + " " + wd).strip()
                if d.textlength(t, font=f) <= w or not cur:
                    cur = t
                else:
                    lines.append(cur)
                    cur = wd
            lines.append(cur)
            lh = f.size * 1.35
            maxl = max(1, int(h // lh))
            if len(lines) > maxl:
                lines = lines[:maxl]
                lines[-1] = fit(lines[-1] + " …", f, w)
                clipped.append(s)
            for i, ln in enumerate(lines):
                d.text((x, y + i * lh), ln, font=f, fill=col, anchor="la")
            continue
        shown = fit(s, f, w) if fl & ELL else s
        if shown != s:
            clipped.append(s)
        horiz = "m" if fl & AL_C == AL_C else ("r" if fl & AL_R else "l")
        vert = "m" if fl & VMID else "a"
        ax = x + (w / 2 if horiz == "m" else (w if horiz == "r" else 0))
        ay = y + (h / 2 if vert == "m" else 0)
        d.text((ax, ay), shown, font=f, fill=col, anchor=horiz + vert)
    out = path[:-4] + "-final.png"
    im.save(out)
    print(out, "| obcięte:", clipped if clipped else "brak")


if __name__ == "__main__":
    main()
