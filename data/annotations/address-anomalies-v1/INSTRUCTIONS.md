# LLM annotation protocol v1

Annotate every assigned input by reading its text as an LLM. Do not run the
normalizer/libpostal or use regex, templates, generator inversions, or a parser
to decide semantic labels. Scripts may read/write/validate files and locate
positions of text that you have explicitly selected.

Input records and IDs must stay exact. `view` is a mechanically whitespace-folded
copy without zero-width characters, provided for selecting text; final offsets
are in the original raw string (Unicode code points, half-open).

Write one compact JSON object per record to your assigned `.labels.jsonl` file:

    {"id":"synthetic_037","labels":{"c":"Астана","t":"ул.","s":"Пробная","h":"19","l":"за рынком Весна","a":"34"}}

Codes: p province, c city/settlement, d administrative district, v living_area,
t street_type, s street_name, h house, a apartment, e entrance number,
f floor, l landmark, i delivery_instruction, o other non-address content.
Omit absent fields. Never invent a missing province/city/house from a reference.
Fields contain selected text, NOT corrected spelling or canonical values.
Separate type from name when unambiguous (including suffix types). Retain
typos/transliteration/wrong-layout text in the chosen field. Markers for numeric
fields are excluded; province can include 'область'. City wrappers can be omitted.
Numbered microdistrict/quarter used as the primary street address: t type,
s name/number. Additional named microdistrict alongside a street: v whole phrase.
Landmark includes relation and object ('за рынком Весна'); explicit 'ориентир:'
may be included. Distinguish directions such as 'вход со двора' as i, not landmark.
Venue with no explicit relation can still be l when used as an address landmark;
do not confuse a building/destination name with a city just because it is a city name.

Each label value may be a string or list of selected strings. To select a
particular occurrence, use {"text":"Алматы","occurrence":0} (zero-based occurrence
among literal matches in view). To label all repetitions of the same phrase,
use {"text":"ориентир: павильон у остановки","all":true}. Default is first
unoccupied exact match. More specific/long labels are allocated before short ones.
No overlaps allowed. Embedded text in glued words/numbers can be selected.

Add `review` with a concrete reason when interpretation is ambiguous/conflicting,
input is corrupted beyond safe interpretation, or it is non-address text. Label
all conflicting explicit provinces/cities rather than silently choosing one.
Missing fields alone do not require review. Use o for clear non-address messages.
For unresolvable field boundaries leave that fragment unassigned and explain review.
Do not use heuristic numeric confidence.

`reference_hint` describes the PUBLIC source before mutation, not truth for this
input. Use it only as dictionary context for spelling/role; missing fields stay
absent and altered numbers stay altered. Additional read-only dictionary:
backend/data/reference/entities.jsonl (KATO/OSM, Kazakhstan). Ambiguity of
settlement identity is distinct from identifying an explicit city text span.

Read every row, make explicit semantic selections per row, and write literal
annotation decisions in manageable batches. Do not programmatically generate
labels from input or propagate a template through rows. Mechanical expansion of
your explicit decisions into spans is done separately. Output is LLM-labelled
synthetic silver data, not human-verified ground truth.

## Preserve ambiguity when training the normalizer

The LLM may choose a preferred interpretation to complete an annotation. That
choice is a hypothesis, not proof that the deployed normalizer must select it.
Exposing unresolved ambiguity is correct normalizer behavior. Retain the reason
and alternatives when known; do not erase them because an LLM picked a span.

Separate span-role ambiguity from geographic identity ambiguity. `city` means
settlement, including a city, village or other locality. The text Караганда can
be confidently labelled city while still referring to multiple KATO entities
(a city and a village). Preserve those identities independently. Equal names
do not imply equal identity; one chosen entity cannot relabel all homonyms.

This dataset supervises text extraction, not unique geographic identification.
`annotation_status: resolved` closes a span annotation check only. `review: null`
and empty `input_issues` are not evidence that a geographic identity is unique:
identity ambiguity has not been exhaustively annotated in these 2000 records.
Do not export them as negative labels for runtime ambiguity diagnostics.

Never use reference_hint.city_id, source_base_id, mutation seed, generated source
house or the LLM's preferred locality as a mandatory unique identity target.
Disambiguation requires evidence in the actual input plus compatible reference
facts, or explicit operator selection. A familiar/populous city, model score or
missing alternative in an incomplete directory is insufficient on its own.

For ambiguous spans, a training adapter must preserve the ambiguity metadata
and mask competing selections or exclude that row from a single-answer loss.
Do not discard homonymous villages from reference candidates or evaluation.
Future data and evaluation must cover ambiguous bare names, explicit locality
types, useful province/street/house context, transliterations and cases whose
correct output is an ambiguity diagnostic rather than a unique locality.
Keep all mutations of one source base in a single train/validation/test partition
to avoid leakage, and test homonymous names across distinct settlement identities. Measure false
confident resolution and ambiguity preservation separately from span accuracy.
