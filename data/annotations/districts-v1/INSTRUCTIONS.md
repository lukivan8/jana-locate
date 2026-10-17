# District supplemental synthetic corpus

Author 80 literal LLM-created and LLM-labelled cases per assigned shard, no
normalizer calls. Existing 2000-record corpus is immutable. Use local
reference-context.json for Kazakhstan district/province/city names and aliases.
It includes urban districts currently stored as raw kind=admin (Astana/Turan);
their semantic role in input is district. Dictionary kind is not a text label.
Addresses, street names, buildings and delivery details are synthetic; do not
assert real street-to-district or building-to-district relationships.

Each line of assigned *.cases.jsonl is a literal JSON object:
{"id":"district_001","family_id":"urban_01","raw_address":"Алматы, Алмалинский район, ул. Учебная, д 12","labels":{"c":"Алматы","d":"Алмалинский район","t":"ул.","s":"Учебная","h":"12"},"language":"ru","tags":["explicit_district"],"reference_ids":["kato:751110000"],"expected_input_issues":[]}

Use labels/codes/selector syntax from ../address-anomalies-v1/INSTRUCTIONS.md:
p province,c settlement,d district,v living area,t street type,s street name,
h house,a apartment,e entrance,f floor,l landmark,i delivery instruction,o other.
No invented absent components. Select exact literal text. Labels are span
extraction, not normalized canonical names. Ordinarily full district phrase
includes район/ауданы/tumani when present. City markers may be outside city span.
Optional review records genuinely ambiguous span/role boundaries. Referenced IDs
are dictionary context only, not mandatory unique identity targets.

Use 20 family groups with four examples each. Family variants MUST be kept
together in a future train/validation/test split. Vary names, order, punctuation,
abbreviations, slang/typos, case, joined markers, RU/KZ/UZ, transliteration and
mixed scripts. Original Kazakhstan names remain Kazakhstan names in Uzbek input.
Include >=8 Uzbek-language constructions (e.g. tumani, ko'chasi, uy, xonadon),
>=8 Kazakh, >=8 transliterated, >=8 typo/mixed variants per shard. Declare
language plus tags for combinations rather than assuming all Latin text is Uzbek.
At least 8 negative examples per shard with no address district span: names
only as street, landmark/business name, delivery note or wholly absent.
At least 8 conflict/ambiguity cases per shard. Tag conflict and list an explicit
expected_input_issues code (province_conflict, city_district_conflict,
multiple_districts, ambiguous_district_identity, ambiguous_city_identity etc).
Two same-name districts without context can have a clear d span while identity
is ambiguous. Prefer marking an expected ambiguity over choosing one identity.

Distinct tasks: extraction (these cases), reference enrichment (separate fixture
graph) and runtime normalizer evaluation (not run here). If no district appears
in input, labels.d is absent even when reference could enrich it. District name
inside a street or landmark must not be relabelled as district. A typo alone
need not imply ambiguity; synthetic corrupted forms are literal extraction labels.

References authorize geography only where explicit context is compatible;
conflicting synthetic pairs are allowed but must be flagged. Keep urban city
districts distinct from province-level rural districts and districts with same
name. LLM preferred interpretation cannot suppress normalizer ambiguity.
All examples are silver synthetic data; no human-language or address verification.
Scripts may serialize explicit authored cases and mechanically locate selected
spans, but must not infer semantic labels or run existing normalizer/libpostal.
