#!/usr/bin/env python3
"""Prepare public OSM-based fixture seeds, never organizer/overlay data.

This is a preparation tool. The independent frontend builds from its own
checked-in seed file and never needs the backend checkout or SQLite.
"""
import argparse
from collections import Counter, defaultdict
import gzip
import hashlib
import heapq
import json
from pathlib import Path


def rank(value):
    return int.from_bytes(hashlib.sha256(('jana-fixtures-v1:' + value).encode()).digest()[:8], 'big')


def sample(entities, addresses, manifest, count=920):
    facts = json.loads(Path(manifest).read_text())
    for path, key in [(entities, 'jsonl_sha256'), (addresses, 'addresses_sha256')]:
        with open(path, 'rb') as stream:
            if hashlib.file_digest(stream, 'sha256').hexdigest() != facts[key]:
                raise ValueError(f'Source checksum mismatch: {path}')
    directory = {e['id']: e for e in map(json.loads, Path(entities).read_text().splitlines())}
    buckets = defaultdict(list)
    counts = Counter()
    with gzip.open(addresses, 'rt', encoding='utf-8') as stream:
        for line in stream:
            a = json.loads(line)
            street, city = directory[a['street_id']], directory[a['city_id']]
            if street['kind'] != 'street' or not city.get('province_id'):
                continue
            counts[city['id']] += 1
            priority = rank(a['id'])
            bucket = buckets[city['id']]
            item = (-priority, a['id'], a)
            if len(bucket) < 6:
                heapq.heappush(bucket, item)
            elif priority < -bucket[0][0]:
                heapq.heapreplace(bucket, item)
    # Include the largest address inventories, then spread across settlements.
    major = [city for city, _ in counts.most_common(30)]
    cities = major + sorted(set(buckets) - set(major), key=rank)
    ordered = {city: sorted(buckets[city], key=lambda item: (-item[0], item[1])) for city in cities}
    chosen, seen = [], set()
    for offset in range(6):
        for city in cities:
            if offset >= len(ordered[city]):
                continue
            a = ordered[city][offset][2]
            key = (a['city_id'], a['street_id'], a['house'])
            if key not in seen:
                seen.add(key)
                chosen.append(a)
            if len(chosen) == count:
                break
        if len(chosen) == count:
            break
    if len(chosen) != count:
        raise ValueError('Insufficient public address evidence for fixture seeds')
    bases = []
    for i, a in enumerate(chosen):
        street, city = directory[a['street_id']], directory[a['city_id']]
        province = directory[city['province_id']]
        # A source alias is optional; do not invent a translated settlement name.
        kk = next((alias for alias in city.get('aliases', []) if any(c in alias for c in 'әғқңөұүһіӘҒҚҢӨҰҮҺІ') and not alias.endswith((' қ.', ' а.'))), city['name'])
        _, kind, identifier = a['id'].split(':')
        bases.append({
            'id': f'osm_base_{i+1:04d}', 'osm_id': a['id'],
            'source_url': f'https://www.openstreetmap.org/{kind}/{identifier}',
            'city_id': city['id'], 'street_id': street['id'],
            'province': province['name'], 'city': city['name'], 'city_alias': kk,
            'street_type': street.get('street_type', ''), 'street_name': street['name'], 'house': a['house'],
        })
    return {
        'description': 'Public source components for synthetic mutations, not expected normalization answers or verified delivery addresses.',
        'attribution': '© OpenStreetMap contributors', 'license': 'ODbL-1.0',
        'license_url': 'https://www.openstreetmap.org/copyright',
        'source_url': facts['osm']['url'], 'source_timestamp': facts['osm']['timestamp'],
        'source_sha256': facts['osm']['sha256'],
        'entities_sha256': facts['jsonl_sha256'], 'addresses_sha256': facts['addresses_sha256'],
        'selection': 'SHA-256 ranked address samples; largest 30 settlement inventories first, then settlement-balanced sampling. Not representative of traffic frequency.',
        'settlement_count': len({b['city_id'] for b in bases}), 'bases': bases,
    }


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--entities', required=True)
    p.add_argument('--addresses', required=True)
    p.add_argument('--manifest', required=True)
    p.add_argument('--output', required=True)
    a = p.parse_args()
    result = sample(a.entities, a.addresses, a.manifest)
    Path(a.output).write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(f"{len(result['bases'])} public source addresses across {result['settlement_count']} settlements")
