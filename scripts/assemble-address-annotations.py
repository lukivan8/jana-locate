#!/usr/bin/env python3
"""Validate/assemble explicit LLM selections. No semantic parsing or normalization."""
import argparse
import collections
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "data/annotations/address-anomalies-v1"
LABELS = dict(p="province", c="city", d="district", v="living_area",
              t="street_type", s="street_name", h="house", a="apartment",
              e="entrance", f="floor", l="landmark", i="delivery_instruction",
              o="other")
INVISIBLE = "\u200b\u200c\u200d\ufeff"


def view_mapping(raw):
    chars, starts, ends = [], [], []
    whitespace = None
    for index, char in enumerate(raw):
        if char in INVISIBLE:
            continue
        if char.isspace():
            if whitespace is None:
                whitespace = index
            continue
        if whitespace is not None and chars:
            chars.append(" ")
            starts.append(whitespace)
            ends.append(index)
        whitespace = None
        chars.append(char)
        starts.append(index)
        ends.append(index + 1)
    return "".join(chars), starts, ends


def matches(view, text):
    found, pos = [], 0
    while True:
        start = view.find(text, pos)
        if start < 0:
            return found
        found.append((start, start + len(text)))
        pos = start + 1


def assemble(record, decision, audit=None):
    if set(decision) - {"id", "labels", "review"}:
        raise ValueError("unknown decision keys")
    if audit is not None:
        if audit["id"] != decision["id"] or audit["original_review"] != decision.get("review"):
            raise ValueError("review audit no longer matches original decision")
        if audit["annotation_status"] not in {"resolved", "needs_review"}:
            raise ValueError("invalid annotation audit status")
        decision = {**decision, "labels": {**decision["labels"], **audit.get("labels_patch", {})}}
        if audit["annotation_status"] == "resolved":
            decision.pop("review", None)
    view, starts, ends = view_mapping(record["raw_address"])
    selections = []
    for code, values in decision["labels"].items():
        if code not in LABELS:
            raise ValueError(f"unknown label {code}")
        if not isinstance(values, list):
            values = [values]
        for value in values:
            selector = {"text": value} if isinstance(value, str) else value
            if not isinstance(selector, dict) or set(selector) - {"text", "all", "occurrence"}:
                raise ValueError(f"invalid selector {selector!r}")
            text = selector["text"]
            if not isinstance(text, str) or not text:
                raise ValueError("empty/non-string selection")
            selections.append((code, selector))
    # Locate longer explicit selections first to avoid short strings inside them.
    selections.sort(key=lambda item: -len(item[1]["text"]))
    occupied, spans = [], []
    for code, selector in selections:
        options = matches(view, selector["text"])
        if "occurrence" in selector:
            index = selector["occurrence"]
            if not isinstance(index, int) or index < 0 or index >= len(options):
                raise ValueError(f"invalid occurrence: {code} {selector!r}")
            options = [options[index]]
        free = [(s, e) for s, e in options if not any(s < b and e > a for a, b in occupied)]
        if not free:
            raise ValueError(f"selection missing/overlaps: {code} {selector!r}")
        for start, end in free if selector.get("all") else free[:1]:
            occupied.append((start, end))
            raw_start, raw_end = starts[start], ends[end - 1]
            spans.append(dict(label=LABELS[code], start=raw_start, end=raw_end,
                              text=record["raw_address"][raw_start:raw_end]))
    spans.sort(key=lambda item: item["start"])
    for left, right in zip(spans, spans[1:]):
        if left["end"] > right["start"]:
            raise ValueError("overlapping raw spans")
    fields = {label: [] for label in LABELS.values()}
    for span in spans:
        fields[span["label"]].append(span["text"])
    return dict(id=record["id"], raw_address=record["raw_address"], spans=spans,
                fields={key: value or None for key, value in fields.items()},
                review=decision.get("review"), annotation_source="llm",
                annotation_status=audit["annotation_status"] if audit else ("needs_review" if decision.get("review") else "unflagged"),
                review_category=audit["category"] if audit else None,
                input_issues=audit.get("input_issues", []) if audit else [],
                human_verified=False, synthetic=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--shard", choices=list("abcd"))
    args = parser.parse_args()
    dataset_path = ROOT / "frontend/src/data/address-anomalies.json"
    dataset = json.loads(dataset_path.read_text())
    manifest = json.loads((OUT / "source-manifest.json").read_text())
    assert hashlib.sha256(dataset_path.read_bytes()).hexdigest() == manifest["source_sha256"], "source changed"
    inputs = dataset["records"]
    audit_path = OUT / "review-audit.json"
    audit = {}
    if audit_path.exists():
        entries = json.loads(audit_path.read_text())["entries"]
        audit = {entry["id"]: entry for entry in entries}
        assert len(audit) == len(entries), "duplicate review audit IDs"
    names = [args.shard] if args.shard else list("abcd")
    decisions, errors = {}, []
    for name in names:
        path = OUT / "shards" / f"{name}.labels.jsonl"
        expected = [json.loads(line)["id"] for line in (OUT / "shards" / f"{name}.input.jsonl").read_text().splitlines()]
        shard_ids = []
        for index, line in enumerate(path.read_text().splitlines(), 1):
            try:
                decision = json.loads(line)
                record_id = decision["id"]
                if record_id in decisions or record_id not in expected:
                    raise ValueError(f"duplicate/out-of-shard id: {record_id}")
                decisions[record_id] = decision
                shard_ids.append(record_id)
            except (ValueError, KeyError, TypeError) as exc:
                errors.append(f"{path.name}:{index}: {exc}")
        if shard_ids != expected:
            missing = sorted(set(expected) - set(shard_ids))
            errors.append(f"{name}: expected {len(expected)} records in input order, got {len(shard_ids)}; missing={missing[:10]}")
    if audit:
        audited_ids = set(audit) & set(decisions)
        reviewed_ids = {key for key, decision in decisions.items() if decision.get("review")}
        if audited_ids != reviewed_ids or (not args.shard and set(audit) != reviewed_ids):
            errors.append("review audit must cover exactly every original review")
    assembled = []
    for record in inputs:
        if record["id"] not in decisions:
            continue
        try:
            assembled.append(assemble(record, decisions[record["id"]], audit.get(record["id"])))
        except (ValueError, KeyError, TypeError) as exc:
            errors.append(f"{record['id']}: {exc}")
    if errors:
        for error in errors:
            print(error)
        raise SystemExit(1)
    print(f"Validated {len(assembled)} LLM annotation records; no normalization executed.")
    if args.shard:
        return
    assert len(assembled) == len(inputs) == 2000
    (OUT / "annotations.jsonl").write_text("".join(json.dumps(row, ensure_ascii=False) + "\n" for row in assembled))
    counts = collections.Counter(span["label"] for row in assembled for span in row["spans"])
    reviews = [dict(id=row["id"], reason=row["review"]) for row in assembled if row["review"]]
    issue_counts = collections.Counter(issue["code"] for row in assembled for issue in row["input_issues"])
    report = dict(records=len(assembled), spans=sum(counts.values()), spans_by_label=dict(sorted(counts.items())),
                  records_with_landmark=sum(row["fields"]["landmark"] is not None for row in assembled),
                  review_count=len(reviews), review_records=reviews,
                  original_review_count=sum(bool(row.get("review")) for row in decisions.values()),
                  annotation_status_counts=dict(collections.Counter(row["annotation_status"] for row in assembled)),
                  resolved_review_count=sum(row["annotation_status"] == "resolved" for row in assembled),
                  records_with_input_issues=sum(bool(row["input_issues"]) for row in assembled),
                  input_issues_by_code=dict(sorted(issue_counts.items())),
                  checks=["exact source SHA-256", "all IDs exactly once in input order", "exact raw input preservation",
                          "selected text exists", "non-overlapping raw spans", "Unicode code-point half-open offsets",
                          "audit covers all original reviews and preserves their reasons"],
                  normalization_executed=False, semantic_accuracy_measured=False, human_verified=False)
    (OUT / "validation-report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: value for key, value in report.items() if key != "review_records"}, ensure_ascii=False))


if __name__ == "__main__":
    main()
