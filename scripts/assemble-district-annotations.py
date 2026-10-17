#!/usr/bin/env python3
"""Assemble explicit LLM district examples; no semantic inference/normalizer."""
import collections
import hashlib
import importlib.util
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "data/annotations/districts-v1"
spec = importlib.util.spec_from_file_location("span_assembly", ROOT / "scripts/assemble-address-annotations.py")
span_assembly = importlib.util.module_from_spec(spec)
spec.loader.exec_module(span_assembly)


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    manifest = json.loads((OUT / "source-manifest.json").read_text())
    original = manifest["original_corpus"]
    assert digest(ROOT / original["inputs"]) == original["inputs_sha256"], "original inputs changed"
    assert digest(ROOT / original["annotations"]) == original["annotations_sha256"], "original annotations changed"
    assert digest(ROOT / manifest["reference_source"]) == manifest["reference_source_sha256"], "reference changed"
    reference = json.loads((OUT / "reference-context.json").read_text())
    reference_ids = {item["entity"]["id"] for item in reference["entries"]}
    for item in reference["entries"]:
        for key in ["city", "province"]:
            if item.get(key):
                reference_ids.add(item[key]["id"])
        reference_ids.update(entity["id"] for entity in item["sample_settlements"])
    inputs, annotations, families = [], [], collections.Counter()
    languages, tags, label_counts, issues = (collections.Counter() for _ in range(4))
    shard_counts = {}
    for shard_index, shard in enumerate("abc"):
        rows = [json.loads(line) for line in (OUT / "shards" / f"{shard}.cases.jsonl").read_text().splitlines()]
        expected = [f"district_{n:03d}" for n in range(1 + 80 * shard_index, 81 + 80 * shard_index)]
        assert [row["id"] for row in rows] == expected, f"{shard}: expected 80 rows in order"
        shard_counts[shard] = len(rows)
        shard_families = collections.Counter(row["family_id"] for row in rows)
        assert len(shard_families) == 20 and set(shard_families.values()) == {4}, f"{shard}: need 20 families of 4"
        assert not (set(families) & set(shard_families)), "family shared across shards"
        families.update(shard_families)
        for row in rows:
            assert isinstance(row["language"], str) and row["language"]
            assert isinstance(row["tags"], list) and len(row["tags"]) == len(set(row["tags"]))
            assert isinstance(row["expected_input_issues"], list)
            assert set(row["reference_ids"]) <= reference_ids, f"{row['id']}: unknown reference ID"
            record = {key: row[key] for key in ["id", "raw_address"]}
            decision = {key: row[key] for key in ["id", "labels"]}
            if row.get("review"):
                decision["review"] = row["review"]
            annotated = span_assembly.assemble(record, decision)
            if "negative_district" in row["tags"]:
                assert annotated["fields"]["district"] is None, f"{row['id']}: negative district has d span"
            if "conflict" in row["tags"]:
                assert row["expected_input_issues"], f"{row['id']}: conflict without diagnostic"
            for key in ["family_id", "language", "tags", "reference_ids", "expected_input_issues"]:
                annotated[key] = row[key]
            annotated["reference_ids_are_targets"] = False
            annotated["geographic_identity_resolution"] = "not_performed"
            inputs.append(record)
            annotations.append(annotated)
            languages[row["language"]] += 1
            tags.update(row["tags"])
            issues.update(row["expected_input_issues"])
            label_counts.update(span["label"] for span in annotated["spans"])
    assert len(inputs) == len({row["raw_address"] for row in inputs}) == 240, "duplicate inputs"
    fixtures = json.loads((OUT / "reference-enrichment-fixtures.json").read_text())
    graph = fixtures["reference"]
    city_ids = {entity["id"] for entity in graph["cities"]}
    district_ids = {entity["id"] for entity in graph["districts"]}
    street_ids = {entity["id"] for entity in graph["streets"]}
    for case in fixtures["cases"]:
        assert set(case["query"]["city_candidates"]) <= city_ids
        assert set(case["query"]["street_candidates"]) <= street_ids
        assert set(case["query"]["explicit_district_ids"]) <= district_ids
        assert set(case["expected"]["district_candidates"]) <= district_ids
    assert len({case["id"] for case in fixtures["cases"]}) == len(fixtures["cases"]) == 16
    for name, rows in [("inputs.jsonl", inputs), ("annotations.jsonl", annotations)]:
        (OUT / name).write_text("".join(json.dumps(row, ensure_ascii=False) + "\n" for row in rows))
    (OUT / "inputs.json").write_text(json.dumps(inputs, ensure_ascii=False, indent=2) + "\n")
    report = dict(records=len(inputs), families=len(families), shard_counts=shard_counts,
                  reference_enrichment_specification_cases=len(fixtures["cases"]),
                  reference_enrichment_runtime_evaluated=False,
                  explicit_district_records=sum(bool(row["fields"]["district"]) for row in annotations),
                  negative_district_records=tags["negative_district"],
                  records_with_expected_diagnostics=sum(bool(row["expected_input_issues"]) for row in annotations),
                  district_reference_entities=len({identity for row in annotations for identity in row["reference_ids"] if identity in {e['entity']['id'] for e in reference['entries']}}),
                  languages=dict(sorted(languages.items())), tags=dict(sorted(tags.items())),
                  expected_input_issues=dict(sorted(issues.items())), spans_by_label=dict(sorted(label_counts.items())),
                  original_corpus_preserved=True, normalization_executed=False,
                  semantic_accuracy_measured=False, human_verified=False,
                  checks=["240 unique input strings", "explicit selections grounded in raw text",
                          "non-overlapping code-point spans", "dictionary context IDs exist",
                          "negative cases have no district span", "60 families of four variants",
                          "original 2000 inputs and annotations SHA-256 unchanged"])
    (OUT / "coverage-report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(report, ensure_ascii=False))


if __name__ == "__main__":
    main()
