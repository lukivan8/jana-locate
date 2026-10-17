# Kazakhstan addresses in different user languages

ALL addresses refer to Kazakhstan. The user explicitly corrected the scope:
the five-country business context concerns users/languages, NOT destination
countries. No Kyrgyz/Uzbek/Tajik/Mongolian settlements or administrative divisions
as destination. Use ONLY local Kazakhstan KATO/OSM dictionary context from
../districts-v1/reference-context.json and backend/data/reference/entities.jsonl.
Preserve real compatible province/district/settlement context. No foreign gazetteer.

Author80 literal LLM cases for assigned language group,20families×4. IDs:
kzlang_ky_001..080,kzlang_uz_001..080,kzlang_tg_001..080,kzlang_mn_001..080.
Root supplies80 base RU/KK/UZ variants. Every case geographic_scope='KZ'.

Row schema:
{"id":"kzlang_ky_001","family_id":"ky_01","geographic_scope":"KZ","raw_address":"Алматы, Алмалы району, Окуу көчөсү, 12 үй","labels":{"c":"Алматы","d":"Алмалы району","s":"Окуу","t":"көчөсү","h":"12"},"language":"ky","tags":["native_language","explicit_district"],"reference_ids":["kato:751110000"],"expected_input_issues":[]}

Use previous span codes/selector syntax: p province,c settlement,d district,
v living_area,t street_type,s street_name,h house,a apartment,e entrance,f floor,
l whole relation+landmark,i delivery_instruction,o other. Optional co country
ONLY for a literal Kazakhstan name actually present; no inferred country span.
Do not introduce other new fields. Retain exact strings, typos, translit, numbers.

Native language modifies address vocabulary, e.g. Kazakhstan names followed by
району/көчөсү/үй (Kyrgyz), tumani/ko'chasi/uy (Uzbek), ноҳия/кӯча/хона (Tajik),
дүүрэг/гудамж/байр (Mongolian). Kazakhstan oblast can be described locally;
foreign vocabulary does NOT create foreign administrative geography. Mongolia's
aimag/soum/khoroo hierarchy is not Kazakhstan's hierarchy. Use generic district
word only for actual KATO district, never import foreign locality types.

Per80: >=40 native-language constructions,>=12 transliteration variants,>=8 mixed
script/typo variants,>=8 district-negative examples (district name only street,
venue or delivery note OR absent),>=8 conflict/ambiguous cases with explicit
expected_input_issues codes,>=12 landmarks,>=24 explicit district examples.
Uzbek must include both Latin and Cyrillic; Kyrgyz/Tajik special Cyrillic letters;
Mongolian Cyrillic and Latin translit. Russian/English code-switching allowed.
Include some foreign COUNTRY/CITY names only inside quoted shops/landmarks or
negated delivery notes; mark their role l/i/s, never destination country/city.

Preserve all compatible homonymous Kazakhstan settlement/district identities.
LLM preference and source IDs cannot suppress normalizer ambiguity. Negative
examples have no district span. Country is external dataset scope, NOT predicted
from language. Existing2000 and240 inputs/annotations stay unchanged.

Addresses/street/building/delivery details synthetic, no verified coordinates or
street→district claims. Native-language quality not human-verified. Every author
must read authored rows and explicitly choose labels; scripts may serialize
literal decisions and locate spans but not infer semantic labels or run normalizer.
Only assigned cases file edits; foreign-country draft is discarded by root.
