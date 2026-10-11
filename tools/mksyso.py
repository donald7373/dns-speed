#!/usr/bin/env python3
"""Buduje plik .syso (zasoby Windows) z ikony .ico, bez zadnych zewnetrznych narzedzi.

Go automatycznie dolinkowuje pliki *_windows_amd64.syso do programu, dzieki czemu
ikona pojawia sie w Eksploratorze i na pasku zadan.

Uzycie: mksyso.py assets/icon.ico cmd/dns-speed-v1/rsrc_windows_amd64.syso
"""
import struct
import sys

RT_ICON, RT_GROUP_ICON = 3, 14
LANG_NEUTRAL = 0


def read_ico(path):
    data = open(path, "rb").read()
    reserved, kind, count = struct.unpack_from("<HHH", data, 0)
    assert reserved == 0 and kind == 1, "to nie jest plik .ico"
    imgs = []
    for i in range(count):
        w, h, colors, res, planes, bits, size, off = struct.unpack_from("<BBBBHHII", data, 6 + 16 * i)
        imgs.append((w, h, colors, planes, bits, data[off:off + size]))
    return imgs


def build_resources(imgs):
    """Zwraca {typ: {id: bajty}} dla ikon i grupy ikon."""
    icons = {}
    grp = struct.pack("<HHH", 0, 1, len(imgs))
    for i, (w, h, colors, planes, bits, blob) in enumerate(imgs, start=1):
        icons[i] = blob
        grp += struct.pack("<BBBBHHIH", w, h, colors, 0, planes, bits, len(blob), i)
    return {RT_ICON: icons, RT_GROUP_ICON: {1: grp}}


class Layout:
    """Uklada drzewo katalogow zasobow PE (typ -> id -> jezyk -> dane)."""

    def __init__(self, resources):
        self.res = resources
        self.types = sorted(resources)
        # rozmiary katalogow
        self.root_size = 16 + 8 * len(self.types)
        off = self.root_size
        self.type_dir_off = {}
        for t in self.types:
            self.type_dir_off[t] = off
            off += 16 + 8 * len(resources[t])
        self.id_dir_off = {}
        for t in self.types:
            for rid in sorted(resources[t]):
                self.id_dir_off[(t, rid)] = off
                off += 16 + 8  # jeden jezyk
        self.entry_off = {}
        for t in self.types:
            for rid in sorted(resources[t]):
                self.entry_off[(t, rid)] = off
                off += 16
        self.data_start = off
        self.data_off = {}
        for t in self.types:
            for rid in sorted(resources[t]):
                self.data_off[(t, rid)] = off
                off += (len(resources[t][rid]) + 3) & ~3
        self.total = off

    def build(self):
        buf = bytearray(self.total)
        relocs = []

        def dir_hdr(pos, n_id):
            struct.pack_into("<IIHHHH", buf, pos, 0, 0, 0, 0, 0, n_id)

        dir_hdr(0, len(self.types))
        for i, t in enumerate(self.types):
            struct.pack_into("<II", buf, 16 + 8 * i, t, 0x80000000 | self.type_dir_off[t])
        for t in self.types:
            base = self.type_dir_off[t]
            ids = sorted(self.res[t])
            dir_hdr(base, len(ids))
            for i, rid in enumerate(ids):
                struct.pack_into("<II", buf, base + 16 + 8 * i, rid, 0x80000000 | self.id_dir_off[(t, rid)])
        for t in self.types:
            for rid in sorted(self.res[t]):
                base = self.id_dir_off[(t, rid)]
                dir_hdr(base, 1)
                struct.pack_into("<II", buf, base + 16, LANG_NEUTRAL, self.entry_off[(t, rid)])
        for t in self.types:
            for rid in sorted(self.res[t]):
                blob = self.res[t][rid]
                eo = self.entry_off[(t, rid)]
                do = self.data_off[(t, rid)]
                # OffsetToData jest RVA: w pliku .syso zapisujemy offset w sekcji,
                # a linker dodaje adres sekcji dzieki relokacji ADDR32NB.
                struct.pack_into("<IIII", buf, eo, do, len(blob), 0, 0)
                relocs.append(eo)
                buf[do:do + len(blob)] = blob
        return bytes(buf), relocs


def coff(section, relocs):
    n_sections = 1
    hdr_size = 20 + 40 * n_sections
    raw_ptr = hdr_size
    reloc_ptr = raw_ptr + len(section)
    sym_ptr = reloc_ptr + 10 * len(relocs)
    file_hdr = struct.pack("<HHIIIHH", 0x8664, n_sections, 0, sym_ptr, 1, 0, 0)
    sec_hdr = struct.pack("<8sIIIIIIHHI", b".rsrc", 0, 0, len(section), raw_ptr, reloc_ptr, 0,
                          len(relocs), 0, 0xC0000040)
    rel = b"".join(struct.pack("<IIH", r, 0, 3) for r in relocs)  # 3 = IMAGE_REL_AMD64_ADDR32NB
    sym = struct.pack("<8sIhHBB", b".rsrc", 0, 1, 0, 3, 0)       # symbol sekcji (static)
    strtab = struct.pack("<I", 4)
    return file_hdr + sec_hdr + section + rel + sym + strtab


RT_MANIFEST = 24


def main():
    # Uzycie: mksyso.py ikona.ico wyjscie.syso [manifest.xml]
    ico, out = sys.argv[1], sys.argv[2]
    imgs = read_ico(ico)
    res = build_resources(imgs)
    extra = ""
    if len(sys.argv) > 3:
        # CREATEPROCESS_MANIFEST_RESOURCE_ID = 1
        res[RT_MANIFEST] = {1: open(sys.argv[3], "rb").read()}
        extra = ", manifest"
    section, relocs = Layout(res).build()
    with open(out, "wb") as f:
        f.write(coff(section, relocs))
    print(f"zapisano {out}: {len(imgs)} ikon{extra}, {len(section)} bajtow zasobow, {len(relocs)} relokacji")


if __name__ == "__main__":
    main()
