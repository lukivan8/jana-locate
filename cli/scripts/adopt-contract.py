#!/usr/bin/env python3
"""Explicitly adopt a supplied normalization OpenAPI release; no runtime coupling."""
import json
import pathlib
import sys
root = pathlib.Path(__file__).resolve().parents[1]
source = pathlib.Path(sys.argv[1])
upstream = json.loads(source.read_text())
local_path = root / 'api/openapi.json'
local = json.loads(local_path.read_text())
for name in ('Result', 'InputRecord', 'AddressFields', 'NullableCoordinates', 'CityCandidate', 'CityCandidates', 'CitySelection', 'CitySelectionRequest', 'RecordCityCandidatesRequest', 'RecordCityCandidates', 'RecordCitySelectionRequest'):
    local['components']['schemas'][name] = upstream['components']['schemas'][name]
(root / 'api/normalization.openapi.json').write_text(source.read_text())
local_path.write_text(json.dumps(local, ensure_ascii=False, indent=2) + '\n')
