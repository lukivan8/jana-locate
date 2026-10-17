#!/usr/bin/env python3
"""Generate synthetic input cases; no organizer data or expected API answers."""
import itertools
import json
from pathlib import Path
from synthetic_mutations import EXTRA_CATEGORIES, expand

CATEGORIES = [
    ("joined", "Слитное написание", "presentation"),
    ("house_suffix", "Дроби и литеры дома", "presentation"),
    ("latin", "Латинские маркеры", "presentation"),
    ("microdistrict", "Микрорайон или дом", "presentation"),
    ("landmark", "Ориентиры в строке", "presentation"),
    ("bilingual", "Двуязычная склейка", "presentation"),
    ("administrative", "Повтор города / район вместо города", "presentation"),
    ("typos", "Опечатки и пробелы", "presentation"),
    ("invisible", "Невидимые символы", "presentation"),
    ("missing", "Отсутствующие компоненты", "requirements"),
    ("order", "Перестановка и числа без маркеров", "hypothesis"),
    ("conflict", "Конфликт географии", "hypothesis"),
    ("homoglyph", "Похожие буквы разных алфавитов", "hypothesis"),
    ("unparseable", "Пустой / неадресный ввод", "hypothesis"),
    ("long", "Длинные строки", "hypothesis"),
]

SINGLES = {
    "joined": ["г. Алматы, Рассветная8, кв 21", "г. Астана, 14квартал 7дом 36квартира", "г. Актобе, ул. Пробная, 18дом 3подъезд", "г. Караганда, ул. Рассветная, 9дом 6этаж", "г. Костанай, 21микрорайон 4дом", "г. Павлодар, ул.Пробная17 кв32", "г. Семей, 8дом ул. Рассветная 12квартира", "г. Тараз, ул. Пробная, дом25 подъезд2 этаж7"],
    "house_suffix": [f"г. Алматы, ул. Пробная, д. {house}, кв. {20+i}" for i, house in enumerate(["7/2", "41/3", "63/8", "18а", "24б", "9А", "12/4б", "102в"])],
    "latin": [f"г. Астана, ул. Рассветная, д. {10+i}, {marker}" for i, marker in enumerate(["flat 23", "kv 48", "apt 106", "apartment 7", "KV 31", "Flat 205", "kv. 16", "apt. 83"])],
    "microdistrict": ["г. Алматы, Аксай 4 микрорайон, дом 18, кв 29", "г. Астана, Нурсити 3 микрорайон, дом 16, подъезд 2, кв 45", "г. Караганда, 12 микрорайон, дом 7", "г. Актобе, микрорайон 8, дом 22, кв 11", "г. Тараз, микрорайон Самал 2, дом 9", "г. Павлодар, 5 микрорайон, дом 5, кв 5", "г. Семей, микрорайон 11, дом 11, подъезд 1", "г. Костанай, микрорайон Северный 4, дом 2, кв 32"],
    "landmark": [f"г. Астана, ул. Пробная, д. {15+i}, {landmark}, кв {30+i}" for i, landmark in enumerate(["магазин Рассвет", "Qazpost Северный", "рядом с аптекой Лотос", "вход со стороны школы", "за рынком Весна", "напротив кафе Луч", "у остановки Парк", "магазин Маяк во дворе"])],
    "bilingual": [f"город {ru}/{kk} қаласы, ул. Пробная, дом {20+i}, кв {41+i}" for i, (ru, kk) in enumerate([("Алматы", "Алматы"), ("Астана", "Астана"), ("Актобе", "Ақтөбе"), ("Караганда", "Қарағанды"), ("Костанай", "Қостанай"), ("Павлодар", "Павлодар"), ("Семей", "Семей"), ("Тараз", "Тараз")])],
    "administrative": ["г. Алматы, Алматы, ул. Пробная, дом 19", "Астана, г. Астана, ул. Рассветная, дом 28", "г. Актобе, ул. Пробная, дом 31, Актобе", "Караганда, Караганда, ул. Рассветная, дом 42", "Ауэзовский район, ул. Пробная, дом 23", "Илийский район, ул. Рассветная, дом 16", "Турксибский район, ул. Пробная, дом 34", "Алматинская область, Карасайский район, ул. Рассветная, дом 11"],
    "typos": ["г. Алматы, Ул .Пробная, дом 8, кв 12", "г. Астана, ул. Рассветная, дом 17, подьезд 3", "  г. Актобе,   ул. Пробная,  дом 21, кв 6  ", "г. Караганда,, ул.. Рассветная, дом 33", "г. Тараз, улица Рассветнная, дом 24", "г. Семей, ул. пробная, дом 8, кв.  19", "г. Павлодар, ул . Рассветная, дом 14, этажж 5", "г. Костанай, ул. Пробная , дом 31 ,кв 7"],
    "invisible": ["г. Алматы, ул.\u00a0Пробная, дом 8, кв 21", "г. Астана, ул. Рассве\u200bтная, дом 19", "\ufeffг. Актобе, ул. Пробная, дом 7", "г. Караганда,\tул. Рассветная, дом 16", "г. Тараз, ул. Пробная,\nдом 12, кв 44", "г. Семей, ул. Рассветная,\r\nдом 21", "г. Павлодар, ул. Пробная, дом\u2060 18", "г. Костанай, ул. Рассветная,\u202fдом 27"],
    "missing": ["Рассветная 17", "г. Алматы, ул. Пробная", "ул. Рассветная, дом 23, кв 6", "г. Астана, микрорайон Самал 4, дом 9", "г. Актобе, ЖК Тестовый, ул. Пробная, дом 16"],
    "order": ["кв 27, дом 14, ул. Пробная, г. Алматы", "3 подъезд, 6 этаж, 19 квартира, дом 31, ул. Рассветная, Астана", "Алматы Пробная 18 23", "дом 7, Актобе, Рассветная, кв 29", "Караганда, 36, Пробная, 12"],
    "conflict": ["г. Алматы, г. Астана, ул. Пробная, дом 17", "Актюбинская область, г. Алматы, ул. Рассветная, дом 21", "г. Астана, Ауэзовский район, ул. Пробная, дом 13", "г. Караганда, г. Семей, ул. Рассветная, дом 34", "Павлодарская область, г. Тараз, ул. Пробная, дом 29"],
    "homoglyph": ["г. Алматы, ул. Рaссветная, дом 17", "г. Астана, ул. Прoбная, дом 23", "г. Актобе, ул. Рассветная, дом 12a", "г. Караганда, ул. Пpобная, дом 14", "г. Семей, ул. Раcсветная, дом 9"],
    "unparseable": ["", "   ", "уточню позже", "позвоните перед доставкой", "???"],
    "long": [f"г. Алматы, ул. Пробная, дом {20+i}, кв {30+i}, " + ("ориентир тестовый павильон; " * (12+i*8)) for i in range(5)],
}

NOTES = {
    "joined": "Граница слова и числа; маркер может стоять после номера.",
    "house_suffix": "Дробь и литера — часть строкового номера дома.",
    "latin": "Латинский маркер квартиры среди кириллицы; apt/apartment — расширенные варианты гипотезы.",
    "microdistrict": "Отличить номер микрорайона от номера дома и квартиры.",
    "landmark": "Ориентир не должен поглощать название улицы и номер дома.",
    "bilingual": "Два написания одного города через слэш, а не два города.",
    "administrative": "Повтор города либо район без явно указанного населённого пункта.",
    "typos": "Распознать маркеры; написание названия улицы сохранить как в исходнике.",
    "invisible": "Невидимые символы сохранены в raw_address; конкретные кодовые точки — наша гипотеза.",
    "missing": "Неизвестные поля — null; микрорайон/ЖК различать по наличию отдельной улицы.",
    "order": "Позиция и числа без маркеров могут не давать однозначного разбора.",
    "conflict": "Противоречащие административные сведения требуют проверки; не выбирать произвольно.",
    "homoglyph": "Похожие кириллические и латинские буквы различны по кодовым точкам.",
    "unparseable": "Валидная входная строка без надёжного адреса; результат нельзя пропускать.",
    "long": "Повторяющийся ориентир; длина остаётся в лимите API 4096 UTF-8 байт.",
}

records, scenarios = [], []


def add(raw, tags, note):
    identifier = f"synthetic_{len(records)+1:03d}"
    records.append({"id": identifier, "raw_address": raw})
    scenarios.append({"id": identifier, "anomalies": tags, "note": note})


def combination(tags, variant):
    """Compose independently visible manifestations of each tagged category."""
    city = "город Актобе/Ақтөбе қаласы" if "bilingual" in tags else "г. Актобе"
    if "administrative" in tags:
        city += ", Актобе"
    street = "микрорайон Самал 4" if "microdistrict" in tags else "ул. Рассветная"
    house = ["17/2", "24б", "31/4а", "42/7", "53в"][variant % 5] if "house_suffix" in tags else str(17+variant)
    apartment = f"kv {41+variant}" if "latin" in tags else f"кв {41+variant}"
    delivery = f"дом {house}, подъезд 3, этаж 6, {apartment}"
    if "joined" in tags:
        street = street.replace("Самал 4", "Самал4").replace("Рассветная", "Рассветная8")
        delivery = delivery.replace("дом ", "дом").replace("подъезд 3", "3подъезд")
    if "typos" in tags:
        street = street.replace("ул.", "Ул .").replace("микрорайон", "мкрн.")
        delivery = delivery.replace("подъезд", "подьезд")
    parts = [city, street, delivery]
    if "landmark" in tags:
        parts.insert(2, "магазин Тестовый маяк")
    raw = ", ".join(parts)
    if "typos" in tags:
        raw = "  " + raw.replace(", ", ",   ") + "  "
    if "invisible" in tags:
        raw = "\ufeff" + raw.replace("дом", "\u200bдом", 1).replace(",", ",\u00a0", 1)
    return raw


for category, _, _ in CATEGORIES:
    for raw in SINGLES[category]:
        add(raw, [category], NOTES[category])

primary = [c[0] for c in CATEGORIES[:9]]
for index, pair in enumerate(itertools.combinations(primary, 2)):
    add(combination(pair, index), list(pair), "Комбинация двух групп. " + " ".join(NOTES[tag] for tag in pair))
for index in range(18):
    tags = [primary[index % 9], primary[(index+2) % 9], primary[(index+5) % 9]]
    add(combination(tags, index), tags, "Комбинация трёх групп; проверить границы компонентов и сохранение исходной строки.")
for index in range(5):
    add(combination(primary, index), primary, "Все девять групп одновременно; синтетический стресс-сценарий.")

source = json.loads((Path(__file__).resolve().parents[1] / 'src/data/address-bases.json').read_text())
legacy_count = len(records)
expand(records, scenarios, source)
CATEGORIES += EXTRA_CATEGORIES

payload = {
    "schema_version": 2,
    "source": {key: value for key, value in source.items() if key != "bases"},
    "base_count": len(source["bases"]),
    "source_based_count": len(records)-legacy_count,
    "handcrafted_count": legacy_count,
    "synthetic": True,
    "description": "2000 входных сценариев без эталонной разметки: 161 придуманный пример и 1839 искусственно изменённых вариантов публичных адресных записей OSM. Не train.json и не база проверенных адресов доставки.",
    "categories": [{"id": identifier, "label": label, "source": source} for identifier, label, source in CATEGORIES],
    "records": records,
    "scenarios": scenarios,
}
target = Path(__file__).resolve().parents[1] / "src/data/address-anomalies.json"
target.parent.mkdir(parents=True, exist_ok=True)
target.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(f"{len(records)} synthetic cases → {target}")

for index in range(2):
    part = target.with_name(f'address-anomalies-part-{index+1}.json')
    part.write_text(json.dumps(records[index*1000:(index+1)*1000], ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
