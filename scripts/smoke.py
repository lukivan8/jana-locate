"""Standard-library integration check; use only against a disposable catalog."""
import json, os, subprocess, sys, time, urllib.request
api, web, phase = sys.argv[1:]
def fetch(url, body=None):
    data = None if body is None else json.dumps(body).encode()
    if container := os.environ.get('SMOKE_CONTAINER'):
        # Probe from the existing frontend: no host port or extra runtime image.
        command = ['docker', 'exec', container, 'wget', '-qO-', '-T', '310']
        if data is not None:
            command += ['--header=Content-Type: application/json', '--post-data=' + data.decode()]
        return subprocess.check_output(command + [url], timeout=315)
    req = urllib.request.Request(url, data=data, headers={'Content-Type': 'application/json'})
    with urllib.request.urlopen(req, timeout=310) as response:
        return response.read()
def request(path, body=None):
    return json.loads(fetch(api + path, body))
assert request('/readyz')['status'] == 'ok'
assert request('/openapi.json')['info']['title'] == 'Jana Address API'
reference_results = request('/v1/normalize', {'records': [
    {'id': 'reference-positive', 'raw_address': 'г.Караганда, Алалыкина 12'},
    {'id': 'reference-missing-house', 'raw_address': 'Караганда, Алалыкина 999999'},
    {'id': 'reference-missing-city', 'raw_address': 'Алалыкина 12'},
    {'id': 'reference-homonym-with-house', 'raw_address': 'Караганда, Алалыкина 12'},
]})['results']
assert all('gliner_unavailable' not in result['warnings'] and 'libpostal_unavailable' not in result['warnings'] for result in reference_results), reference_results
resolved = reference_results[0]
assert resolved['city'] == 'Караганда' and resolved['province'] == 'Карагандинская область', resolved
assert resolved['street_name'] == 'Алалыкина' and resolved['street_type'] == 'улица', resolved
assert resolved['coordinates'] is None and 'ambiguous_city_reference' not in resolved['warnings'], resolved
assert reference_results[1]['province'] is None and 'ambiguous_city_reference' in reference_results[1]['warnings'], reference_results[1]
assert reference_results[2]['city'] is None and reference_results[2]['coordinates'] is None, reference_results[2]
assert reference_results[3]['status'] == 'needs_review' and reference_results[3]['province'] is None and reference_results[3]['district'] is None and 'ambiguous_city_reference' in reference_results[3]['warnings'], reference_results[3]
assert os.environ.get('SMOKE_EXPECTED_API_URL', api).encode() in fetch(web + '/config.js')
assert b'<html' in fetch(web + '/review/deep-link')
fields = dict.fromkeys(['province','city','district','living_area','street_type','street_name','house','apartment','entrance','floor','landmark'])
fields.update(province='Smoke', city='Smoke City', street_type='улица', street_name='Smoke Street', house='55/6')
if phase == 'write':
    saved = request('/v1/catalog/confirmations', {'fields': fields, 'coordinates': {'latitude': 43.24, 'longitude': 76.89}, 'expected_revision': 0})
    assert saved['revision'] == 1
result = request('/v1/normalize', {'records': [{'id': 'new-id', 'raw_address': 'Smoke City, улица Smoke Street 55/6 кв 99'}]})['results'][0]
assert result['status'] == 'needs_review', result
assert result['coordinates'] is None and result['catalog_id'] is None, result
assert result['apartment'] == '99' and result['catalog_revision'] is None
assert 'coordinates_unknown' not in result['warnings']
# The legacy catalog persists independently and never supplies normalization defaults.
assert any(e['fields']['city'] == 'Smoke City' and e['revision'] == 1 for e in request('/v1/catalog')['entries'])
batch = request('/v1/normalize', {'records': [{'id': str(i), 'raw_address': 'неизвестно'} for i in range(1000)]})
assert len(batch['results']) == 1000
assert all('gliner_unavailable' not in result['warnings'] for result in batch['results'])
print('HTTP, static SPA, runtime URL, 1000-record batch and confirmation persistence passed:', phase)

# The job journal shares the disposable volume, but is separate from building defaults.
if phase == 'write':
    job = request('/v1/normalization-jobs', [{'id': 'smoke-job', 'raw_address': 'unknown'}, {'id': 'smoke-job', 'raw_address': 'Караганда, Алалыкина 999999'}])
    for _ in range(120):
        job = request('/v1/normalization-jobs/' + job['id'])
        if job['status'] == 'completed':
            break
        assert job['status'] != 'failed', job
        time.sleep(1)
    assert job['status'] == 'completed', job
    reviewed = dict(fields, apartment='77', entrance='3')
    job = request('/v1/normalization-jobs/' + job['id'] + '/records/0/review', {'expected_revision': job['revision'], 'fields': reviewed})
    assert job['reviewed_count'] == 1
    path = '/v1/normalization-jobs/' + job['id'] + '/records/1'
    candidates = request(path + '/candidates')
    assert len(candidates['candidates']) == 2, candidates
    city = next(c for c in candidates['candidates'] if c['entity_id'] == 'kato:351010000')
    assert city['settlement_type'] == 'город', city
    job = request(path + '/selections', {'expected_revision': candidates['revision'], 'reference_version': candidates['reference_version'], 'entity_id': city['entity_id']})
    assert job['results'][1]['province'] == 'Карагандинская область'
    assert job['results'][1]['coordinates'] is None
    assert job['reviewed'] == [True, False]
else:
    saved = request('/v1/normalization-jobs')['jobs']
    assert len(saved) == 1, saved
    job = request('/v1/normalization-jobs/' + saved[0]['id'])
    assert job['results'][0]['apartment'] == '77'
    assert job['results'][0]['entrance'] == '3'
    assert job['results'][0]['raw_address'] == 'unknown'
    assert job['reviewed'] == [True, False]
    assert job['results'][1]['province'] == 'Карагандинская область'
    assert job['results'][1]['coordinates'] is None
    assert job['results'][1]['id'] == 'smoke-job'
    assert job['city_selections'][0]['entity_id'] == 'kato:351010000'
    candidates = request('/v1/normalization-jobs/' + job['id'] + '/records/1/candidates')
    assert candidates['reference_version'] == job['city_selections'][0]['reference_version']
    assert job['results'][0]['coordinates'] is None
print('Durable job, delivery correction and city selection persistence passed:', phase)
