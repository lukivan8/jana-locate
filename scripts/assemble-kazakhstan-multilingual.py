#!/usr/bin/env python3
"""Assemble explicit synthetic multilingual KZ labels, without normalization."""
import collections
import hashlib
import importlib.util
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "data/annotations/kazakhstan-multilingual-v1"
spec = importlib.util.spec_from_file_location("span_assembly", ROOT / "scripts/assemble-address-annotations.py")
span_assembly = importlib.util.module_from_spec(spec)
spec.loader.exec_module(span_assembly)
span_assembly.LABELS["co"] = "country"  # Extraction dataset only, not the HTTP API.


def main():
    manifest = json.loads((OUT / "source-manifest.json").read_text())
    for source in manifest["preserved_corpora"] + [manifest["geographic_reference"]]:
        assert hashlib.sha256((ROOT / source["path"]).read_bytes()).hexdigest() == source["sha256"], source["path"]
    entities = {entity["id"]: entity for entity in map(json.loads, (ROOT / manifest["geographic_reference"]["path"]).read_text().splitlines())}
    annotations, inputs, counts, families = [], [], {}, set()
    for group in manifest["language_groups"]:
        rows = [json.loads(line) for line in (OUT / "shards" / f"{group}.cases.jsonl").read_text().splitlines()]
        assert [row["id"] for row in rows] == [f"kzlang_{group}_{n:03d}" for n in range(1, 81)], group
        family_counts = collections.Counter(row["family_id"] for row in rows)
        assert len(family_counts) == 20 and set(family_counts.values()) == {4}, group
        assert not (set(family_counts) & families), "family shared between shards"
        families.update(family_counts)
        for row in rows:
            assert row["geographic_scope"] == "KZ", "foreign destination scope"
            assert row["reference_ids"] and all(identity in entities and identity.startswith("kato:") for identity in row["reference_ids"]), row["id"]
            record = {key: row[key] for key in ["id", "raw_address"]}
            decision = {key: row[key] for key in ["id", "labels"]}
            if row.get("review"):
                decision["review"] = row["review"]
            annotated = span_assembly.assemble(record, decision)
            if "negative_district" in row["tags"]:
                assert annotated["fields"]["district"] is None, row["id"]
            for key in ["family_id", "language", "geographic_scope", "tags", "reference_ids", "expected_input_issues", "origin", "authoring_method", "linguistic_verification"]:
                if key in row:
                    annotated[key] = row[key]
            annotated["reference_ids_are_targets"] = False
            annotated["geographic_identity_resolution"] = "not_performed"
            inputs.append(record); annotations.append(annotated)
        group_rows = annotations[-80:]
        count = dict(records=80, explicit_district=sum(bool(r["fields"]["district"]) for r in group_rows),
                     negative_district=sum("negative_district" in r["tags"] for r in group_rows),
                     landmark_records=sum(bool(r["fields"]["landmark"]) for r in group_rows),
                     diagnostic_records=sum(bool(r["expected_input_issues"]) for r in group_rows),
                     languages=dict(collections.Counter(r["language"] for r in group_rows)),
                     tags=dict(collections.Counter(t for r in group_rows for t in r["tags"])))
        if group != "base":
            assert count["explicit_district"] >= 24 and count["negative_district"] >= 8, (group, count)
            assert count["landmark_records"] >= 12 and count["diagnostic_records"] >= 8, (group, count)
            assert count["tags"].get("transliteration", 0) >= 12 and count["tags"].get("typo_mixed", 0) >= 8, (group, count)
            native = sum(number for language, number in count["languages"].items() if language == group or language.startswith(group + "-Cyrl") or language.startswith(group + "-Latn") and group == "uz")
            assert native >= 40, (group, "native construction quota", count)
        counts[group] = count
    assert len(inputs) == len({row["raw_address"] for row in inputs}) == 400
    for name, rows in [("inputs.jsonl", inputs), ("annotations.jsonl", annotations)]:
        (OUT / name).write_text("".join(json.dumps(row, ensure_ascii=False) + "\n" for row in rows))
    (OUT / "inputs.json").write_text(json.dumps(inputs, ensure_ascii=False, indent=2) + "\n")
    report = dict(records=400, families=len(families), geographic_scope="KZ", groups=counts,
                  explicit_district_records=sum(bool(r["fields"]["district"]) for r in annotations),
                  negative_district_records=sum("negative_district" in r["tags"] for r in annotations),
                  landmark_records=sum(bool(r["fields"]["landmark"]) for r in annotations),
                  diagnostic_records=sum(bool(r["expected_input_issues"]) for r in annotations),
                  languages=dict(sorted(collections.Counter(r["language"] for r in annotations).items())),
                  checks=["400 unique raw strings", "all geography scope KZ", "only Kazakhstan KATO reference IDs",
                          "all original corpus hashes unchanged", "exact nonoverlapping raw spans",
                          "language/negative/landmark/diagnostic coverage quotas", "100 four-variant families"],
                  normalization_executed=False, human_verified=False, geographic_identity_resolved=False,
                  country_field_scope="extraction dataset only; HTTP API unchanged")
    (OUT / "coverage-report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: value for key, value in report.items() if key != "groups"}, ensure_ascii=False))


if __name__ == "__main__":
    main()
