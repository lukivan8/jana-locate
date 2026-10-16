"""Standard-library integration check; use only against a disposable catalog."""
import json, sys, urllib.request
api, web, phase = sys.argv[1:]
def request(path, body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(api + path, data=data, headers={'Content-Type': 'application/json'})
    with urllib.request.urlopen(req, timeout=310) as r:
        return json.load(r)
assert request('/readyz')['status'] == 'ok'
assert request('/openapi.json')['info']['title'] == 'Jana Address API'
with urllib.request.urlopen(web + '/config.js') as r:
    assert api.encode() in r.read()
with urllib.request.urlopen(web + '/review/deep-link') as r:
    assert b'<html' in r.read()
fields = dict.fromkeys(['province','city','district','living_area','street_type','street_name','house','apartment','entrance','floor','landmark'])
fields.update(province='Smoke', city='Smoke City', street_type='улица', street_name='Smoke Street', house='55/6')
if phase == 'write':
    saved = request('/v1/catalog/confirmations', {'fields': fields, 'coordinates': {'latitude': 43.24, 'longitude': 76.89}, 'expected_revision': 0})
    assert saved['revision'] == 1
result = request('/v1/normalize', {'records': [{'id': 'new-id', 'raw_address': 'Smoke City, улица Smoke Street 55/6 кв 99'}]})['results'][0]
assert result['status'] == 'matched', result
assert result['coordinates'] == {'latitude': 43.24, 'longitude': 76.89}, result
assert result['apartment'] == '99' and result['catalog_revision'] == 1
batch = request('/v1/normalize', {'records': [{'id': str(i), 'raw_address': 'неизвестно'} for i in range(1000)]})
assert len(batch['results']) == 1000
print('HTTP, static SPA, runtime URL, 1000-record batch and confirmation persistence passed:', phase)
