#!/usr/bin/env python3
"""Builds internal/gazetteer/townalts.tsv.gz from GeoNames cities15000.txt:
for every town of 15k+ people, its Latin-script alternate names (English,
local and Esperanto forms such as München / Munich / Munkeno), so feeds that
name places in words (Eventa Servo: "…, Parizo, FR") can be placed.

  curl -O https://download.geonames.org/export/dump/cities15000.zip
  unzip cities15000.zip && python3 tools/townalts.py cities15000.txt
"""
import gzip, sys, unicodedata

def latin(s):
    return all(not ch.isalpha() or 'LATIN' in unicodedata.name(ch, '') for ch in s)

def fold(s):
    s = unicodedata.normalize('NFD', s.lower())
    return ''.join(c for c in s if unicodedata.category(c) != 'Mn' and c.isalnum())

lines = []
for line in open(sys.argv[1], encoding='utf8'):
    f = line.rstrip('\n').split('\t')
    name, ascii_, alts, lat, lon, cc, pop = f[1], f[2], f[3], f[4], f[5], f[8], f[14]
    seen, keep = {fold(name)}, []
    for a in [ascii_] + alts.split(','):
        a = a.strip()
        if not a or len(a) > 40 or not latin(a) or any(c.isdigit() for c in a):
            continue
        k = fold(a)
        if k and k not in seen:
            seen.add(k)
            keep.append(a)
    lines.append(f"{name}\t{cc}\t{float(lat):.3f}\t{float(lon):.3f}\t{pop or 0}\t{'|'.join(keep)}\n")
with gzip.open('internal/gazetteer/townalts.tsv.gz', 'wt', encoding='utf8', compresslevel=9) as out:
    out.writelines(lines)
print(len(lines), 'towns')
