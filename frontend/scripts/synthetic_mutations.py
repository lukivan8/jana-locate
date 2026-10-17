"""Deterministic input mutations, intentionally without expected outputs."""
import random

EXTRA_CATEGORIES = [
    ('transliteration', 'Транслитерация названий', 'checkpoint'),
    ('mixed_script', 'Смешение кириллицы и латиницы', 'checkpoint'),
    ('keyboard', 'Ошибочная раскладка клавиатуры', 'checkpoint'),
]
LETTERS = dict(zip('абвгдеёжзийклмнопрстуфхцчшщъыьэюяәғқңөұүһі',
                   ['a','b','v','g','d','e','yo','zh','z','i','y','k','l','m','n','o','p','r','s','t','u','f','kh','ts','ch','sh','sch','','y','','e','yu','ya','a','gh','q','ng','o','u','u','h','i']))
LOOKALIKES = dict(zip('аАеЕоОрРсСуУхХкКмМтТвВнНіІ', 'aAeEoOpPcCyYxXkKmMtTbBhHiI'))
KEYBOARD = dict(zip('йцукенгшщзхъфывапролджэячсмитьбю', 'qwertyuiop[]asdfghjkl;\'zxcvbnm,.'))
PROFILES = [(tag,) for tag in [
    'joined','house_suffix','latin','microdistrict','landmark','bilingual','administrative',
    'typos','invisible','missing','order','conflict','homoglyph','long',
    'transliteration','mixed_script','keyboard',
]] + [
    ('transliteration','joined','latin'), ('mixed_script','typos'),
    ('homoglyph','joined','invisible'), ('keyboard','joined'),
    ('typos','house_suffix','latin'), ('bilingual','joined','landmark'),
    ('missing','order'), ('conflict','typos'),
    ('transliteration','typos','invisible'), ('microdistrict','joined','latin'),
    ('administrative','homoglyph','invisible'), ('mixed_script','joined','landmark','latin'),
    ('bilingual','typos','house_suffix'), ('order','joined','invisible'),
    ('keyboard','typos','latin'), ('conflict','transliteration','joined'),
]


def transliterate(text):
    return ''.join(LETTERS.get(c.lower(), c).capitalize() if c.isupper() else LETTERS.get(c, c) for c in text)


def typo(text, rng):
    positions = [i for i, c in enumerate(text) if c.isalpha()]
    if not positions:
        return text + 'а'
    i = rng.choice(positions)
    if len(positions) < 4 or rng.randrange(3) == 0:
        return text[:i] + text[i] + text[i:]
    if rng.randrange(2) and i+1 < len(text) and text[i+1].isalpha():
        return text[:i] + text[i+1] + text[i] + text[i+2:]
    return text[:i] + text[i+1:]


def render(base, tags, seed, provinces):
    rng = random.Random(seed)
    city, province, name, kind, house = [base[k] for k in ['city','province','street_name','street_type','house']]
    apartment = str(1 + seed % 260)
    if 'house_suffix' in tags:
        house += ['/2', 'а', '/1б'][seed % 3]
    if 'typos' in tags:
        name = typo(name, rng)
    if 'homoglyph' in tags:
        positions = [i for i, c in enumerate(name) if c in LOOKALIKES]
        if positions:
            i = rng.choice(positions)
            name = name[:i] + LOOKALIKES[name[i]] + name[i+1:]
        else:
            city = ''.join(LOOKALIKES.get(c, c) for c in city)
    if 'transliteration' in tags:
        city, province, name, kind = map(transliterate, [city, province, name, kind])
    elif 'mixed_script' in tags:
        if seed % 2:
            roman = transliterate(name)
            if roman != name:
                name = roman
            else:
                city = transliterate(city)
        else:
            city = transliterate(city)
    if 'keyboard' in tags:
        previous_name = name
        name = ''.join(KEYBOARD.get(c.lower(), c).upper() if c.isupper() else KEYBOARD.get(c, c) for c in name)
        if name == previous_name:
            city = ''.join(KEYBOARD.get(c.lower(), c).upper() if c.isupper() else KEYBOARD.get(c, c) for c in city)
    if 'bilingual' in tags:
        city = f"город {base['city']}/{base['city_alias']} қаласы"
    if 'administrative' in tags:
        city += ', ' + city
    street = f'{kind} {name}'.strip()
    if 'joined' in tags:
        # Explicitly include street+house without a separating space.
        street = name + house
    else:
        street += f', дом {house}'
    parts = [province, city, street, f'{"flat" if "latin" in tags else "кв"} {apartment}']
    if 'microdistrict' in tags:
        parts.insert(2, f'микрорайон Тестовый {seed % 7 + 1}')
    if 'missing' in tags:
        parts = [street, parts[-1]] if seed % 2 else [city, name]
    if 'order' in tags:
        parts = list(reversed(parts))
    if 'landmark' in tags:
        parts.append(['магазин Береке', 'аптека Алматы', 'Qazpost Солнечный', 'школа Пришахтинск'][seed % 4])
    if 'conflict' in tags:
        other = [p for p in provinces if p != base['province']]
        parts.insert(0, other[seed % len(other)])
    if 'long' in tags:
        parts.extend(['ориентир: павильон у остановки; вход со двора'] * (8 + seed % 8))
    raw = ', '.join(parts)
    if 'joined' in tags:
        raw = raw.replace('кв ', 'кв').replace('flat ', 'flat')
    if 'typos' in tags:
        raw = '  ' + raw.replace(', ', ',   ').replace('дом ', 'д ') + '  '
    if 'invisible' in tags:
        raw = '\ufeff' + raw.replace(', ', ',\u00a0', 1)
        at = raw.find(name)
        if at >= 0 and len(name) > 2:
            raw = raw[:at+2] + '\u200b' + raw[at+2:]
        raw = raw.replace(', ', ',\n', 1)
    if 'transliteration' in tags:
        raw = transliterate(raw)
    return raw


def expand(records, scenarios, source, target=2000):
    bases = source['bases']
    provinces = sorted({base['province'] for base in bases})
    seen = {record['raw_address'] for record in records}
    original_count = len(records)
    for offset in range(target-original_count):
        base = bases[offset // 2]
        tags = PROFILES[offset % len(PROFILES)]
        seed = 2026101600 + offset
        raw = render(base, tags, seed, provinces)
        # Deterministic retries retain the intended mutation profile.
        while raw in seen:
            seed += 100000
            raw = render(base, tags, seed, provinces)
            if seed > 2030000000:
                raise ValueError('Could not produce a distinct mutation')
        if len(raw.encode()) > 4096:
            raise ValueError('Fixture exceeds API raw-address limit')
        seen.add(raw)
        identifier = f'synthetic_{len(records)+1:03d}'
        records.append({'id': identifier, 'raw_address': raw})
        scenarios.append({
            'id': identifier, 'anomalies': list(tags), 'source_base_id': base['id'], 'mutation_seed': seed, 'source_url': base['source_url'],
            'note': 'Основа — публичная запись OSM. Искусственные изменения: ' + ', '.join(tags) + '. Детали доставки, изменённые номера и дополнительные территории вымышлены; это не эталон ответа.',
        })
