"""Offline diagnostic of bounded preprocessing hypotheses; never a production parser.

Only evaluates road/house extraction. Candidate recall is an oracle upper bound,
not accuracy of an implemented selector. Input and output may contain private data.
"""
import argparse
import ctypes as C
import json
import re
import time
from pathlib import Path


class Options(C.Structure):
    _fields_ = [('language', C.c_char_p), ('country', C.c_char_p)]


class Response(C.Structure):
    _fields_ = [('num_components', C.c_size_t),
                ('components', C.POINTER(C.c_char_p)), ('labels', C.POINTER(C.c_char_p))]


class NormalizeOptions(C.Structure):
    # ABI of the prepared libpostal header; use its matching native library.
    _fields_ = [('languages', C.POINTER(C.c_char_p)), ('num_languages', C.c_size_t),
                ('address_components', C.c_uint16)] + [(name, C.c_bool) for name in (
                    'latin_ascii transliterate strip_accents decompose lowercase trim_string '
                    'drop_parentheticals replace_numeric_hyphens delete_numeric_hyphens '
                    'split_alpha_from_numeric replace_word_hyphens delete_word_hyphens '
                    'delete_final_periods delete_acronym_periods drop_english_possessives '
                    'delete_apostrophes expand_numex roman_numerals').split()]


def working(text):
    return ' '.join(re.sub('[\u200b\u200c\u200d\ufeff]', '', text).split())


def hypotheses(text):
    spaced = re.sub(r'(?<=[^\W\d_])(?=\d)', ' ', text)
    spaced = re.sub(r'(\d)(?=(?:дом|квартал|микрорайон|квартира|подъезд|подьезд|этаж)\b)', r'\1 ', spaced, flags=re.I)
    markers = r'квартира|квартиры|кв\.?|kv|flat|подъезд|подьезд|подезд|этаж|эт\.?'
    # Both orientations are hypotheses: "9 кв 1" and "5 подъезд 239 кв"
    # require different choices. Never replace the original with one blindly.
    forward = r'(?<!\w)(?:' + markers + r')\s*\d+(?!\w)'
    reverse = r'(?<![\w/])\d+\s*(?:' + markers + r')(?!\w)'
    def core_with(patterns):
        core = spaced
        for pattern in patterns:
            core = re.sub(pattern, ' ', core, flags=re.I)
        return re.sub(r'(?<!\w)(?:магазин|школа|аптека|Qazpost)\b[^,;]*', ' ', core, flags=re.I)
    core = core_with([reverse, forward])
    forward_core = core_with([forward, reverse])
    bilingual = re.sub(r'(?:город\s+)?([^,/\d]+?)/[^,]+?\s+қаласы(?=,|$)', r'\1', core, flags=re.I)
    return {'original': text, 'spaced': working(spaced),
            'address_core_reverse': working(core), 'address_core_forward': working(forward_core),
            'bilingual_core': working(bilingual)}


def canon(text):
    return ' '.join(re.sub(r'[^\w/\-]+', ' ', text.lower()).split())


def road(text):
    text = re.sub(r'^(?:улица|ул\.?|проспект|пр\.?|пр-т|переулок|пер\.?|микрорайон|мкр\.?)\s*\.?\s*', '', text, flags=re.I)
    text = re.sub(r'\s+(?:көшесі|даңғылы|квартал|микрорайон)$', '', text, flags=re.I)
    return canon(text)


def main():
    parser = argparse.ArgumentParser(__doc__)
    parser.add_argument('--library', required=True)
    parser.add_argument('--data', required=True)
    parser.add_argument('--cases', required=True)
    parser.add_argument('--output', required=True)
    parser.add_argument('--expansions', action='store_true')
    args = parser.parse_args()
    lib = C.CDLL(args.library)
    for name in ['libpostal_setup_datadir', 'libpostal_setup_parser_datadir']:
        fn = getattr(lib, name); fn.argtypes = [C.c_char_p]; fn.restype = C.c_bool
        if not fn(args.data.encode()): raise RuntimeError(name + ' failed')
    lib.libpostal_parse_address.argtypes = [C.c_char_p, Options]
    lib.libpostal_parse_address.restype = C.POINTER(Response)
    lib.libpostal_address_parser_response_destroy.argtypes = [C.POINTER(Response)]
    cases = json.loads(Path(args.cases).read_text())
    measurements = {}; rows = []; started = time.monotonic()
    for case in cases:
        choices = []
        for variant, text in hypotheses(working(case['raw'])).items():
            for language in [None, 'ru', 'kk']:
                name = variant + '/' + (language or 'auto')
                response = lib.libpostal_parse_address(text.encode(), Options(language.encode() if language else None, b'kz')) if text else None
                parts = []
                if response:
                    try:
                        parts = [(response.contents.labels[i].decode(), response.contents.components[i].decode())
                                 for i in range(response.contents.num_components)]
                    finally:
                        lib.libpostal_address_parser_response_destroy(response)
                values = {'street_name': [road(v) for k,v in parts if k == 'road'],
                          'house': [canon(v) for k,v in parts if k == 'house_number']}
                fields = {f: ((road(case['want'][f]) if f == 'street_name' else canon(case['want'][f])) in values[f])
                          for f in values if case['want'].get(f)}
                measurement = measurements.setdefault(name, {'correct': 0, 'expected': 0})
                measurement['correct'] += sum(fields.values()); measurement['expected'] += len(fields)
                choices.append({'variant': name, 'text': text, 'parts': parts, 'correct': fields})
        rows.append({'name': case['name'], 'choices': choices})
    # An oracle chooses the right candidate per field; a real ranker must earn this.
    eligible = correct = 0
    for case, row in zip(cases, rows):
        for field in ['street_name', 'house']:
            if case['want'].get(field):
                eligible += 1
                correct += any(choice['correct'].get(field, False) for choice in row['choices'])
    report = {'scope': 'Raw libpostal road/house candidate coverage; no production reconciliation, no null-safety metric.',
              'seconds_excluding_startup': time.monotonic()-started, 'cases': len(cases),
              'measurements': measurements, 'oracle_candidate_coverage': {'correct': correct, 'expected': eligible}, 'rows': rows}
    if args.expansions:
        lib.libpostal_get_default_options.restype = NormalizeOptions
        lib.libpostal_expand_address.argtypes = [C.c_char_p, NormalizeOptions, C.POINTER(C.c_size_t)]
        lib.libpostal_expand_address.restype = C.POINTER(C.c_char_p)
        lib.libpostal_expansion_array_destroy.argtypes = [C.POINTER(C.c_char_p), C.c_size_t]
        languages = (C.c_char_p * 1)(b'ru')
        options = lib.libpostal_get_default_options()
        options.languages = languages; options.num_languages = 1
        options.replace_numeric_hyphens = False; options.delete_numeric_hyphens = False
        options.expand_numex = False; options.roman_numerals = False
        expansions = []
        for text in ['ул. Курильская', 'Тұран даңғылы', 'мкр. Аксай 3а', 'Назарбаева8', '12а', '55/6']:
            count = C.c_size_t()
            result = lib.libpostal_expand_address(text.encode(), options, C.byref(count))
            try:
                expansions.append({'input': text, 'total': count.value,
                                   'first_12': [result[i].decode() for i in range(min(count.value, 12))]})
            finally:
                if result: lib.libpostal_expansion_array_destroy(result, count.value)
        report['expansions_ru'] = expansions
    Path(args.output).write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')
    print(json.dumps({key: value for key,value in report.items() if key!='rows'}, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
