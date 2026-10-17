"""Exercise only the disposable synthetic CLI Compose workspace."""
import base64
import json
import os
import sys
import time
import urllib.request
base = os.environ['JANA_TEST_WEB_URL']
auth = base64.b64encode(('jana:' + os.environ['JANA_WEB_TOKEN']).encode()).decode() if os.environ.get('JANA_WEB_TOKEN') else None
def request(path, body=None):
    headers = {'Content-Type': 'application/json'}
    if auth: headers['Authorization'] = 'Basic ' + auth
    req = urllib.request.Request(base + path, data=None if body is None else json.dumps(body).encode(), headers=headers)
    with urllib.request.urlopen(req, timeout=310) as response:
        return json.load(response)
if sys.argv[1] == 'write':
    run = request('/v1/runs', {'schema': 'public', 'table': 'orders', 'key': 'id', 'column': 'address', 'target_schema': 'public', 'target_table': 'orders_jana_addresses'})
    for _ in range(120):
        run = request('/v1/runs/' + run['id'])
        if run['status'] == 'review':
            break
        assert run['status'] != 'failed', run['error']
        time.sleep(1)
    assert run['status'] == 'review'
    rows = request('/v1/runs/' + run['id'] + '/rows')['rows']
    row = next(r for r in rows if r['source_id'] == '9223372036854775807')
    fields = {k: row['result'][k] for k in ['province','city','district','living_area','street_type','street_name','house','apartment','entrance','floor','landmark']}
    fields.update(apartment='88', entrance='3')
    request(f"/v1/runs/{run['id']}/rows/{row['position']}/review", {'expected_revision': row['revision'], 'fields': fields})
    print(json.dumps({'id': run['id']}))
else:
    run_id = json.load(sys.stdin)['id']
    run = request('/v1/runs/' + run_id)
    rows = request('/v1/runs/' + run_id + '/export')['rows']
    row = next(r for r in rows if r['source_id'] == '9223372036854775807')
    assert row['result']['apartment'] == '88' and row['result']['entrance'] == '3'
    assert next(r for r in rows if r['source_id'] == '3')['raw_address'] is None
    assert row['raw_address'] == '  Астана, Туран 55/6, кв 77\nвход со двора  '
    published = request('/v1/runs/' + run_id + '/publish', {'expected_revision': run['revision']})
    assert published['status'] == 'published'
    again = request('/v1/runs/' + run_id + '/publish', {'expected_revision': published['revision']})
    assert again['revision'] == published['revision']
    print('Restart, export, NULL, int64, exact raw and repeat publication: OK')
