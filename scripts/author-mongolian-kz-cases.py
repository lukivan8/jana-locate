#!/usr/bin/env python3
"""Compose declared synthetic KZ components in LLM-authored Mongolian patterns.

Labels come from explicitly typed components, never from parsing the rendered
text. This is synthetic authoring, not normalizer output or verified language.
"""
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "data/annotations/kazakhstan-multilingual-v1/shards/mn.cases.jsonl"

# Explicit LLM choices: source identity is dictionary context, not a unique target.
FAMILIES = [
    ("Алматы", "Алмалинский", ["kato:751110000"], "district"),
    ("Алматы", "Ауэзовский", ["kato:751310000"], "district"),
    ("Алматы", "Медеуский", ["kato:751710000"], "district"),
    ("Алматы", "Турксибский", ["kato:751910000"], "district"),
    ("Шымкент", "Абай", ["kato:791110000"], "district"),
    ("Шымкент", "Тұран", ["kato:791910000"], "district"),
    ("Астана", "Алматы", ["kato:711110000"], "district"),
    ("Астана", "Есіл", ["kato:711210000"], "district"),
    ("Астана", "Сарыарқа", ["kato:711310000"], "district"),
    ("Астана", "Сарайшық", ["kato:711610000"], "district"),
    ("Каскелен", "Қарасай", ["kato:195220100", "kato:195200000"], "rural"),
    ("Талгар", "Талғар", ["kato:196220100", "kato:196200000"], "rural"),
    ("Алматы", None, ["kato:750000000", "kato:751310000"], "negative_venue"),
    ("Астана", None, ["kato:710000000", "kato:711210000"], "negative_street"),
    ("Алматы", "Қаратау", ["kato:750000000", "kato:791710000"], "conflict"),
    ("Шымкент", "Алмалы", ["kato:790000000", "kato:751110000"], "conflict"),
    (None, "Абай", ["kato:103200000", "kato:353200000", "kato:791110000"], "homonym"),
    (None, "Есіл", ["kato:114800000", "kato:594200000", "kato:711210000"], "homonym"),
    ("Қарағанды", None, ["kato:154247300", "kato:351010000"], "city_homonym"),
    ("Семей", None, ["kato:101010000"], "no_district"),
]
CITY_LATIN = {"Алматы":"Almaty", "Шымкент":"Shymkent", "Астана":"Astana",
              "Каскелен":"Kaskelen", "Талгар":"Talgar", "Қарағанды":"Karagandy", "Семей":"Semey"}
DISTRICT_LATIN = {"Алмалинский":"Almalinskiy", "Ауэзовский":"Auezovskiy",
    "Медеуский":"Medeuskiy", "Турксибский":"Turksibskiy", "Абай":"Abay",
    "Тұран":"Turan", "Алматы":"Almaty", "Есіл":"Esil", "Сарыарқа":"Saryarqa",
    "Сарайшық":"Saraishyq", "Қарасай":"Karasay", "Талғар":"Talgar",
    "Қаратау":"Karatau", "Алмалы":"Almaly"}
STREETS = [
    ("Сургалтын Нарс", "Surgaltyn Nars"), ("Туршилтын Хус", "Turshiltyn Hus"),
    ("Нобай Улиас", "Nobai Ulias"), ("Сургалтын Алтан", "Surgaltyn Altan"),
    ("Туршилтын Мод", "Turshiltyn Mod"), ("Нобай Тал", "Nobai Tal"),
    ("Сургалтын Од", "Surgaltyn Od"), ("Туршилтын Сар", "Turshiltyn Sar"),
    ("Нобай Тэнгэр", "Nobai Tenger"), ("Сургалтын Гэрэл", "Surgaltyn Gerel"),
    ("Туршилтын Цэцэг", "Turshiltyn Tsetseg"), ("Нобай Навч", "Nobai Navch"),
    ("Сургалтын Уул", "Surgaltyn Uul"), ("Туршилтын Гол", "Turshiltyn Gol"),
    ("Нобай Салхи", "Nobai Salhi"), ("Сургалтын Хавар", "Surgaltyn Havar"),
    ("Туршилтын Өглөө", "Turshiltyn Ogl oo"), ("Нобай Үүл", "Nobai Uul"),
    ("Сургалтын Мөр", "Surgaltyn Mor"), ("Туршилтын Цас", "Turshiltyn Tsas"),
]
LANDMARK_NAMES = ["Гэрэл", "Лотос", "Байтерек", "Нар", "Весна", "Кітап", "Маяк", "Окуу"]


def render(number, family, variant, city, district, refs, kind):
    parts = []
    def text(value):
        parts.append((None, value))
    def field(label, value):
        parts.append((label, value))
    latin = variant == 3
    mixed = variant == 4 and family <= 8
    tags = ["native_language"] if not latin else ["transliteration"]
    language = "mn-Latn" if latin else "mixed" if mixed else "mn"
    if mixed:
        tags.append("typo_mixed")
    if variant == 1:
        field("co", "Казахстан"); text(", ")
    if city:
        field("c", CITY_LATIN[city] if latin else city)
        text(", " if kind in {"rural", "city_homonym"} else " hot, " if latin else " хот, ")
    if district:
        name = DISTRICT_LATIN[district] if latin else district
        suffix = " duureg" if latin else " дүүрэг"
        if mixed:
            suffix = " дүүрeг"  # Deliberate Latin e inside a Cyrillic word.
        field("d", name + suffix); text(", "); tags.append("explicit_district")
    else:
        tags.append("negative_district")
    street = "Esil" if kind == "negative_street" and latin else "Есіл" if kind == "negative_street" else STREETS[family - 1][1 if latin else 0]
    if mixed:
        street = street.replace("а", "a", 1)
    field("s", street); text(" "); field("t", "gudamj" if latin else "гудамж"); text(", ")
    field("h", str(70 + family)); text("-r bair, " if latin else "-р байр, ")
    field("a", str(3 + variant)); text(" toot" if latin else " тоот")
    if variant in [1, 2] or kind in {"negative_venue", "negative_street"}:
        text(", ")
        name = LANDMARK_NAMES[(family - 1) % len(LANDMARK_NAMES)]
        landmark = "«Auezovskiy rayon» delguuriin ard" if latin else "«Ауэзовский район» дэлгүүрийн ард" if kind == "negative_venue" else f"«{name}» кафены эсрэг талд" if variant == 1 else f"«{name}» дэлгүүрийн хажууд"
        if kind == "negative_street":
            landmark = "«Mongoliya» kafeny esreg tald" if latin else "«Монголия» кафены эсрэг талд"
            tags.append("foreign_name_in_landmark")
        field("l", landmark); tags.append("landmark")
    text("; ")
    instruction = "hurgeltiin omno zalgaarai" if latin else "хүргэлтийн өмнө залгаарай"
    if kind == "no_district":
        instruction = "«Bishkek» delguurtei anduurch bolohgui" if latin else "«Бишкек» дэлгүүртэй андуурч болохгүй"
        tags.append("foreign_name_in_instruction")
    field("i", instruction)
    issues = []
    if kind == "conflict":
        tags.append("conflict"); issues.append("city_district_conflict")
    elif kind == "homonym":
        tags.append("identity_ambiguous"); issues.append("ambiguous_district_identity")
    elif kind == "city_homonym":
        tags.append("identity_ambiguous"); issues.append("ambiguous_city_identity")
    if kind == "rural":
        tags.append("rural_district")
    raw = "".join(value for _, value in parts)
    labels, pos = {}, 0
    for label, value in parts:
        if label:
            starts, start = [], 0
            while True:
                match = raw.find(value, start)
                if match < 0: break
                starts.append(match); start = match + 1
            selector = {"text": value, "occurrence": starts.index(pos)}
            if label in labels:
                if not isinstance(labels[label], list): labels[label] = [labels[label]]
                labels[label].append(selector)
            else:
                labels[label] = selector
        pos += len(value)
    return dict(id=f"kzlang_mn_{number:03d}", family_id=f"mn_{family:02d}",
                geographic_scope="KZ", raw_address=raw, labels=labels, language=language,
                tags=tags, reference_ids=refs, expected_input_issues=issues,
                authoring_method="LLM-declared typed components composed mechanically; not parsed",
                linguistic_verification="not_human_verified")


def main():
    rows = []
    for family, values in enumerate(FAMILIES, 1):
        for variant in range(1, 5):
            rows.append(render(len(rows) + 1, family, variant, *values))
    OUT.write_text("".join(json.dumps(row, ensure_ascii=False) + "\n" for row in rows))


if __name__ == "__main__":
    main()
