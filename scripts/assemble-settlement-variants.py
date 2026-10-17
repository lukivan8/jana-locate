#!/usr/bin/env python3
"""Serialize LLM-authored components and exact selections; never run a parser."""
import collections
import hashlib
import importlib.util
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'data/annotations/settlement-variants-v1'
spec = importlib.util.spec_from_file_location('spans', ROOT / 'scripts/assemble-address-annotations.py')
spans = importlib.util.module_from_spec(spec)
spec.loader.exec_module(spans)

# Each prefix, city surface and language/tag is an explicit author decision.
# The scaffold only concatenates these typed components; it does not parse text.
FAMILIES = [
 ('karaganda_alalykina_12', 'Караганда', 'Алалыкина', '12', [
  ('г ', 'Караганда', 'ru', 'short_marker'),
  ('город ', 'Караганда', 'ru', 'full_marker'),
  ('', 'Karaganda', 'mixed', 'transliteration'),
  ('', 'Караганды', 'mixed', 'kazakh_variant'),
  ('г.', 'Караганда', 'ru', 'joined_marker'),
  ('gorod ', 'Karaganda', 'ru-Latn', 'transliterated_marker'),
  ('', 'Қарағанды', 'mixed', 'kazakh_letters'),
  ('', 'Карагaнда', 'mixed', 'mixed_script_inside_word')]),
 ('almaty_maketnaya_21', 'Алматы', 'Макетная', '21', [
  ('город ', 'Алматы', 'ru', 'full_marker'), ('г.', 'Алматы', 'ru', 'joined_marker'),
  ('', 'Almaty', 'mixed', 'transliteration'), ('', 'Almatı', 'mixed', 'latin_variant'),
  ('', 'АЛМАТЫ', 'ru', 'uppercase'), ('', 'Алмaты', 'mixed', 'mixed_script_inside_word'),
  ('', 'Алмты', 'ru', 'missing_letter'), ('gorod ', 'Almaty', 'ru-Latn', 'transliterated_marker')]),
 ('astana_maketnaya_22', 'Астана', 'Макетная', '22', [
  ('город ', 'Астана', 'ru', 'full_marker'), ('г ', 'Астана', 'ru', 'short_marker'),
  ('', 'Astana', 'mixed', 'transliteration'), ('', 'ASTANA', 'mixed', 'uppercase'),
  ('', 'Астaна', 'mixed', 'mixed_script_inside_word'), ('', 'Астна', 'ru', 'missing_letter'),
  ('gorod ', 'Astana', 'ru-Latn', 'transliterated_marker'), ('', 'Астнаа', 'ru', 'transposition')]),
 ('shymkent_maketnaya_23', 'Шымкент', 'Макетная', '23', [
  ('город ', 'Шымкент', 'ru', 'full_marker'), ('г.', 'Шымкент', 'ru', 'joined_marker'),
  ('', 'Shymkent', 'mixed', 'transliteration'), ('', 'Shimkent', 'mixed', 'latin_variant'),
  ('', 'ШЫМКЕНТ', 'ru', 'uppercase'), ('', 'Шымкeнт', 'mixed', 'mixed_script_inside_word'),
  ('', 'Шымкнт', 'ru', 'missing_letter'), ('gorod ', 'Shymkent', 'ru-Latn', 'transliterated_marker')]),
 ('aktobe_maketnaya_24', 'Актобе', 'Макетная', '24', [
  ('город ', 'Актобе', 'ru', 'full_marker'), ('г.', 'Актобе', 'ru', 'joined_marker'),
  ('', 'Aktobe', 'mixed', 'transliteration'), ('', 'Ақтөбе', 'mixed', 'kazakh_letters'),
  ('', 'Aqtobe', 'mixed', 'latin_variant'), ('', 'Актoбе', 'mixed', 'mixed_script_inside_word'),
  ('', 'Актбе', 'ru', 'missing_letter'), ('gorod ', 'Aktobe', 'ru-Latn', 'transliterated_marker')]),
 ('kostanay_maketnaya_25', 'Костанай', 'Макетная', '25', [
  ('город ', 'Костанай', 'ru', 'full_marker'), ('г ', 'Костанай', 'ru', 'short_marker'),
  ('', 'Kostanay', 'mixed', 'transliteration'), ('', 'Kostanai', 'mixed', 'latin_variant'),
  ('', 'Қостанай', 'mixed', 'kazakh_letters'), ('', 'Костaнай', 'mixed', 'mixed_script_inside_word'),
  ('', 'Костнай', 'ru', 'missing_letter'), ('gorod ', 'Kostanay', 'ru-Latn', 'transliterated_marker')]),
]
NEGATIVES = [
 ('кафе «Karaganda»', 'l', 'foreign_script_venue'),
 ('магазин «город Караганда»', 'l', 'city_marker_in_venue'),
 ('напротив кафе «Караганды»', 'l', 'city_alias_in_landmark'),
 ('у вывески «Aktobe»', 'l', 'city_homonym_in_landmark'),
 ('не в Караганду, доставить в Астану', 'i', 'negated_destination'),
 ('позвонить после отправки машины в Karaganda', 'i', 'city_in_instruction'),
 ('вход со стороны кафе «город Алматы»', 'i', 'city_marker_in_instruction'),
 ('за магазином «Shymkent»', 'l', 'transliterated_city_in_landmark'),
]


def main():
    reference_path = ROOT / 'backend/data/reference/entities.jsonl'
    entities = list(map(json.loads, reference_path.read_text().splitlines()))
    refs = {name: [e['id'] for e in entities if e['kind'] == 'city' and e['name'] == name]
            for name in ['Караганда','Алматы','Астана','Шымкент','Актобе','Костанай']}
    assert all(refs.values())
    cases = []
    def add(raw, labels, family, canonical, language, tags, ambiguity=False):
        cases.append(dict(id=f'settlement_variant_{len(cases)+1:03d}', raw_address=raw,
                          labels=labels, family_id=family, language=language, tags=tags,
                          expected_canonical_city=canonical,
                          canonical_city_is_annotation_hypothesis=True,
                          reference_ids=refs[canonical], reference_ids_are_targets=False,
                          identity_policy='preserve_compatible_candidates',
                          expected_input_issues=['ambiguous_city_identity'] if ambiguity else [],
                          geographic_scope='KZ'))
    for family, canonical, street, house, variants in FAMILIES:
        for prefix, surface, language, tag in variants:
            add(f'{prefix}{surface}, улица {street} дом {house}',
                {'c':surface,'t':'улица','s':street,'h':house}, family, canonical, language,
                [tag, 'settlement_marker_excluded_from_city_span'])
    for surface, canonical in [('Караганда','Караганда'),('Karaganda','Караганда'),
                               ('Караганды','Караганда'),('Qaragandy','Караганда'),
                               ('Актобе','Актобе'),('Aktobe','Актобе'),
                               ('Aktöbe','Актобе'),('Ақтөбе','Актобе')]:
        add(surface, {'c':surface}, 'bare_'+canonical, canonical, 'mixed',
            ['missing_address_context','settlement_homonyms'], ambiguity=True)
    for phrase, label, tag in NEGATIVES:
        add('Астана, улица Макетная дом 30, '+phrase,
            {'c':'Астана','t':'улица','s':'Макетная','h':'30',label:phrase},
            'astana_landmark_instruction_30', 'Астана', 'mixed', [tag,'negative_city_mention'])
    for index, confidence in enumerate([0.93,0.88,0.88,0.93]):
        cases[index]['user_observation'] = dict(reported_confidence=confidence,
            source='user text and screenshot', is_training_target=False,
            reproduced=False)
    # Explicitly authored Kazakh street-marker and word-order variants.
    kazakh_cases = [
        ('Қарағанды, Алалыкин кош дом 12', 'Қарағанды', 'Алалыкин', 'кош', 'mixed', 'user_report'),
        ('Қарағанды, Алалыкин көшесі, 12 үй', 'Қарағанды', 'Алалыкин', 'көшесі', 'kk', 'postposed_house_marker'),
        ('Қарағанды қаласы, Алалыкин көш., 12-үй', 'Қарағанды', 'Алалыкин', 'көш.', 'kk', 'hyphenated_house_marker'),
        ('Қарағанды қ., Алалыкин көшесі, үй 12', 'Қарағанды', 'Алалыкин', 'көшесі', 'kk', 'preposed_house_marker'),
        ('Караганды, Алалыкин кош., дом12', 'Караганды', 'Алалыкин', 'кош.', 'mixed', 'joined_house_marker'),
        ('Қарағанды Алалыкин көшесі 12 үй', 'Қарағанды', 'Алалыкин', 'көшесі', 'kk', 'no_punctuation'),
        ('Алалыкин көшесі, 12 үй, Қарағанды', 'Қарағанды', 'Алалыкин', 'көшесі', 'kk', 'city_last'),
        ('Qaragandy, Alalykin koshesi, 12 ui', 'Qaragandy', 'Alalykin', 'koshesi', 'kk-Latn', 'transliteration'),
        ('Karaganda, Алалыкин көш дом 12', 'Karaganda', 'Алалыкин', 'көш', 'mixed', 'mixed_scripts'),
        ('Қарағанды, Алалыкина көшесі, 12 үй', 'Қарағанды', 'Алалыкина', 'көшесі', 'mixed', 'street_name_variant'),
        ('ҚАРАҒАНДЫ, АЛАЛЫКИН КӨШЕСІ, 12 ҮЙ', 'ҚАРАҒАНДЫ', 'АЛАЛЫКИН', 'КӨШЕСІ', 'kk', 'uppercase'),
        ('Қарағанды, Алалыкин кошеси, дом 12', 'Қарағанды', 'Алалыкин', 'кошеси', 'mixed', 'kazakh_letters_omitted'),
    ]
    for raw, city, street, street_type, language, tag in kazakh_cases:
        add(raw, {'c': city, 's': street, 't': street_type, 'h': '12'},
            'karaganda_alalykina_12', 'Караганда', language,
            ['kazakh_street_marker', 'street_type_after_name', tag])
        cases[-1]['expected_canonical_street_type'] = 'улица'
        cases[-1]['street_identity_resolution'] = 'not_performed'
        cases[-1]['interpretation_note'] = 'Author-selected street marker; spelling variant is not a verified reference alias.'
    cases[64]['user_observation'] = dict(reported_confidence=0.68,
        source='user text and screenshot', is_training_target=False, reproduced=False,
        reported_issue='city_missing_and_city_and_street_type_absorbed_into_street_name')
    annotations = []
    for case in cases:
        row = spans.assemble({k:case[k] for k in ['id','raw_address']},
                             {k:case[k] for k in ['id','labels']})
        row.update({k:v for k,v in case.items() if k not in ['id','raw_address','labels']})
        assert row['fields']['city'] == [case['labels']['c']]
        assert all(row['raw_address'][s['start']:s['end']] == s['text'] for s in row['spans'])
        annotations.append(row)
    assert len(cases)==len({c['raw_address'] for c in cases})==76
    assert len(refs['Караганда'])>=2 and len(refs['Актобе'])>=2
    inputs=[{k:c[k] for k in ['id','raw_address']} for c in cases]
    OUT.mkdir(parents=True, exist_ok=True)
    for name, rows in [('cases.jsonl',cases),('annotations.jsonl',annotations),('inputs.jsonl',inputs)]:
        (OUT/name).write_text(''.join(json.dumps(row,ensure_ascii=False)+'\n' for row in rows))
    (OUT/'inputs.json').write_text(json.dumps(inputs,ensure_ascii=False,indent=2)+'\n')
    report=dict(records=76, address_variants=60, kazakh_street_marker_cases=12, ambiguity_cases=8, negative_city_mentions=8,
                families=len({c['family_id'] for c in cases}),
                tags=dict(collections.Counter(t for c in cases for t in c['tags'])),
                normalization_executed=False, human_verified=False, model_training_executed=False,
                expected_confidence=None, geographic_scope='KZ')
    (OUT/'coverage-report.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    manifest=dict(reference_source=str(reference_path.relative_to(ROOT)),
                  reference_sha256=hashlib.sha256(reference_path.read_bytes()).hexdigest(),
                  annotation_source='explicit LLM-authored components and selections',
                  canonical_names='hypotheses separate from literal spans; not unique entity IDs',
                  preserved_corpora={name:hashlib.sha256((ROOT/'data/annotations'/name/'annotations.jsonl').read_bytes()).hexdigest()
                    for name in ['address-anomalies-v1','districts-v1','kazakhstan-multilingual-v1']})
    (OUT/'source-manifest.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(report,ensure_ascii=False))

if __name__=='__main__': main()
