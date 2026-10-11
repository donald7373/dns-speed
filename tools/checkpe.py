#!/usr/bin/env python3
"""Sprawdza zasoby ikon w gotowym pliku .exe (czyta drzewo zasobow PE)."""
import struct
import sys

data = open(sys.argv[1], "rb").read()
pe = struct.unpack_from("<I", data, 0x3C)[0]
assert data[pe:pe + 4] == b"PE\0\0", "to nie PE"
n_sec = struct.unpack_from("<H", data, pe + 6)[0]
opt_size = struct.unpack_from("<H", data, pe + 20)[0]
opt = pe + 24
magic = struct.unpack_from("<H", data, opt)[0]
dd = opt + (112 if magic == 0x20B else 96)
rsrc_rva, rsrc_size = struct.unpack_from("<II", data, dd + 2 * 8)
subsystem = struct.unpack_from("<H", data, opt + 68)[0]
print("podsystem:", {2: "GUI", 3: "konsola"}.get(subsystem, subsystem), "| katalog zasobow RVA=%#x rozmiar=%d" % (rsrc_rva, rsrc_size))
assert rsrc_rva, "brak zasobow w pliku!"

secs = []
for i in range(n_sec):
    o = opt + opt_size + 40 * i
    name = data[o:o + 8].rstrip(b"\0").decode()
    vsize, va, rawsize, rawptr = struct.unpack_from("<IIII", data, o + 8)
    secs.append((name, va, max(vsize, rawsize), rawptr))


def rva2off(rva):
    for name, va, size, ptr in secs:
        if va <= rva < va + size:
            return rva - va + ptr
    raise ValueError("RVA poza sekcjami: %#x" % rva)


base = rva2off(rsrc_rva)


def walk(off, depth, path, out):
    n_named, n_id = struct.unpack_from("<HH", data, base + off + 12)
    for i in range(n_named + n_id):
        name, ptr = struct.unpack_from("<II", data, base + off + 16 + 8 * i)
        if ptr & 0x80000000:
            walk(ptr & 0x7FFFFFFF, depth + 1, path + [name], out)
        else:
            rva, size, cp, _ = struct.unpack_from("<IIII", data, base + ptr)
            out.append((path + [name], rva, size))


leaves = []
walk(0, 0, [], leaves)
icons = [l for l in leaves if l[0][0] == 3]
groups = [l for l in leaves if l[0][0] == 14]
for path, rva, size in [l for l in leaves if l[0][0] == 24]:
    xml = data[rva2off(rva):rva2off(rva) + size].decode("utf-8", "replace")
    print("manifest id=%d: %d B | dpi=%s | common-controls v6=%s" % (
        path[1], size, "PerMonitorV2" in xml, "Common-Controls" in xml))
print("ikony RT_ICON:", len(icons), "| grupy RT_GROUP_ICON:", len(groups))
ok = True
for path, rva, size in icons:
    blob = data[rva2off(rva):rva2off(rva) + size]
    if blob[:8] == b"\x89PNG\r\n\x1a\n":
        w, h = struct.unpack(">II", blob[16:24])
        kind = "PNG"
    else:
        hs, w, h2 = struct.unpack_from("<Iii", blob, 0)
        h, kind = h2 // 2, "BMP"
        ok &= hs == 40
    print("  id=%d  %s %dx%d  %d B" % (path[1], kind, w, h, size))
for path, rva, size in groups:
    blob = data[rva2off(rva):rva2off(rva) + size]
    _, kind, count = struct.unpack_from("<HHH", blob, 0)
    ids = [struct.unpack_from("<H", blob, 6 + 14 * i + 12)[0] for i in range(count)]
    print("  grupa id=%d: %d wpisow -> ikony %s" % (path[1], count, ids))
    ok &= kind == 1 and count == len(icons)
print("WYNIK:", "OK" if ok else "BLAD")
sys.exit(0 if ok else 1)
