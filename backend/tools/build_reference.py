"""Offline JSONL -> read-only SQLite. No coordinates; no connection to confirmation overlay."""
import argparse, gzip, hashlib, json, os, sqlite3, tempfile
from pathlib import Path
from district_context import is_district

def build(source, output, manifest=None, addresses=None, districts=None):
    if addresses and not Path(addresses).exists(): raise FileNotFoundError(addresses)
    compressed = Path(source).with_name('addresses.jsonl.gz')
    addresses = Path(addresses) if addresses else compressed if compressed.exists() else Path(source).with_name('addresses.jsonl')
    has_addresses = addresses.exists()
    if districts and not Path(districts).exists(): raise FileNotFoundError(districts)
    candidate = Path(source).with_name('districts.jsonl.gz')
    districts = Path(districts) if districts else candidate if candidate.exists() else Path(source).with_name('districts.jsonl')
    has_districts = districts.exists()
    if has_districts and not has_addresses: raise ValueError('District schema requires address evidence table')
    if manifest:
        facts = json.loads(Path(manifest).read_text())
        for path, key in [(Path(source), 'jsonl_sha256'), (addresses, 'addresses_sha256'), (districts, 'districts_sha256')]:
            if key in facts:
                if not path.exists(): raise ValueError(f'Missing manifested source: {path}')
                with path.open('rb') as file: actual = hashlib.file_digest(file,'sha256').hexdigest()
                if actual != facts[key]: raise ValueError(f'Checksum mismatch: {path}')
    output = Path(output); output.parent.mkdir(parents=True, exist_ok=True)
    fd, temporary = tempfile.mkstemp(dir=output.parent, prefix='.reference-', suffix='.sqlite'); os.close(fd)
    db = None
    try:
        db = sqlite3.connect(temporary)
        db.executescript('''
        PRAGMA foreign_keys=ON;
        CREATE TABLE metadata(key TEXT PRIMARY KEY,value TEXT NOT NULL);
        CREATE TABLE entities(id TEXT PRIMARY KEY,kind TEXT NOT NULL,name TEXT NOT NULL,
          parent_id TEXT NOT NULL,city_id TEXT NOT NULL,province_id TEXT NOT NULL,
          district_id TEXT NOT NULL,street_type TEXT NOT NULL,source TEXT NOT NULL);
        CREATE TABLE aliases(entity_id TEXT REFERENCES entities(id),alias TEXT NOT NULL,PRIMARY KEY(entity_id,alias));
        CREATE INDEX alias_lookup ON aliases(alias);
        ''')
        db.execute('INSERT INTO metadata VALUES(?,?)', ('schema_version','3' if has_districts else '2' if has_addresses else '1'))
        if manifest: db.execute('INSERT INTO metadata VALUES(?,?)', ('manifest',Path(manifest).read_text()))
        count = 0
        entities = {}
        with open(source) as lines:
            for line in lines:
                e = json.loads(line)
                if e['kind'] not in {'province','city','district','living_area','street','admin'}: raise ValueError('Unknown entity kind')
                if any(k in e for k in ['coordinates','latitude','longitude']): raise ValueError('Administrative directory must not contain building coordinates')
                keys = ['id','kind','name','parent_id','city_id','province_id','district_id','street_type','source']
                entities[e['id']] = {key:e.get(key,'') for key in keys}
                db.execute('INSERT INTO entities VALUES(?,?,?,?,?,?,?,?,?)', [e.get(k,'') for k in keys])
                for alias in sorted({a.strip() for a in e.get('aliases',[]) + [e['name']] if a.strip()}):
                    db.execute('INSERT INTO aliases VALUES(?,?)',(e['id'],alias))
                count += 1
        if has_addresses:
            db.executescript('''CREATE TABLE addresses(id TEXT PRIMARY KEY,
              street_id TEXT NOT NULL REFERENCES entities(id), city_id TEXT NOT NULL REFERENCES entities(id),
              house TEXT NOT NULL, source TEXT NOT NULL);
              CREATE INDEX address_lookup ON addresses(street_id,house);''')
            stream = gzip.open(addresses,'rt',encoding='utf-8') if addresses.suffix=='.gz' else addresses.open()
            with stream as lines:
                for line in lines:
                    e = json.loads(line)
                    if set(e) != {'id','street_id','city_id','house','source'} or not all(isinstance(v,str) and v.strip() for v in e.values()):
                        raise ValueError('Address evidence requires nonempty component IDs, house and source; no coordinates')
                    if not any(c.isalnum() for c in e['house']) or len(e['house'].encode())>256:
                        raise ValueError('Address house must contain an identifier, at most 256 bytes')
                    db.execute('INSERT INTO addresses VALUES(?,?,?,?,?)', [e[k] for k in ['id','street_id','city_id','house','source']])
            bad = db.execute("""SELECT a.id FROM addresses a JOIN entities s ON s.id=a.street_id
              JOIN entities c ON c.id=a.city_id WHERE s.kind NOT IN ('street','living_area')
              OR c.kind!='city' OR s.city_id!=a.city_id LIMIT 1""").fetchone()
            if bad: raise ValueError(f'Address evidence has inconsistent hierarchy: {bad[0]}')
        if has_districts:
            db.executescript('''
              CREATE TABLE district_objects(object_kind TEXT NOT NULL CHECK(object_kind IN ('street','address')),
                object_id TEXT NOT NULL,city_id TEXT NOT NULL REFERENCES entities(id),
                complete INTEGER NOT NULL CHECK(complete IN (0,1)),source TEXT NOT NULL,
                PRIMARY KEY(object_kind,object_id));
              CREATE TABLE district_links(object_kind TEXT NOT NULL,object_id TEXT NOT NULL,
                district_id TEXT NOT NULL REFERENCES entities(id),PRIMARY KEY(object_kind,object_id,district_id),
                FOREIGN KEY(object_kind,object_id) REFERENCES district_objects(object_kind,object_id));
            ''')
            stream = gzip.open(districts,'rt',encoding='utf-8') if districts.suffix=='.gz' else districts.open()
            with stream as lines:
                for line in lines:
                    row=json.loads(line)
                    if set(row) != {'object_kind','object_id','city_id','district_ids','complete','source'}:
                        raise ValueError('Invalid district evidence fields; geometry/coordinates are forbidden')
                    if type(row['complete']) is not bool or not isinstance(row['district_ids'],list) or len(set(row['district_ids']))!=len(row['district_ids']):
                        raise ValueError('District evidence requires a boolean coverage and unique district IDs')
                    if not all(isinstance(row[key],str) and row[key].strip() for key in ['object_kind','object_id','city_id','source']):
                        raise ValueError('Invalid district identifiers/source')
                    city=entities.get(row['city_id'],{})
                    if city.get('kind')!='city': raise ValueError('Invalid district city context')
                    if row['object_kind']=='street':
                        obj=entities.get(row['object_id'],{})
                        valid=obj.get('kind') in {'street','living_area'} and obj['city_id']==city['id']
                    elif row['object_kind']=='address':
                        valid=db.execute('SELECT 1 FROM addresses WHERE id=? AND city_id=?',(row['object_id'],city['id'])).fetchone()
                    else: valid=False
                    if not valid: raise ValueError('Dangling or incompatible district object')
                    if row['complete'] and not row['district_ids']: raise ValueError('Complete coverage requires district candidates')
                    db.execute('INSERT INTO district_objects VALUES(?,?,?,?,?)',[row[key] for key in ['object_kind','object_id','city_id','complete','source']])
                    for identity in row['district_ids']:
                        entity=entities.get(identity,{})
                        if not entity or not is_district(entity) or entity['province_id']!=city['province_id'] or not (entity['city_id']==city['id'] or not entity['city_id'] and city['district_id']==identity):
                            raise ValueError('Incompatible district hierarchy')
                        db.execute('INSERT INTO district_links VALUES(?,?,?)',(row['object_kind'],row['object_id'],identity))
        for key in ['parent_id','city_id','province_id','district_id']:
            missing = db.execute(f"SELECT id,{key} FROM entities WHERE {key} != '' AND {key} NOT IN (SELECT id FROM entities)").fetchall()
            if missing: raise ValueError(f'Dangling {key}: {missing[:5]}')
        if manifest:
            if facts.get('entity_count',count)!=count: raise ValueError('Manifest entity count mismatch')
            if has_addresses and facts.get('address_count',db.execute('SELECT count(*) FROM addresses').fetchone()[0])!=db.execute('SELECT count(*) FROM addresses').fetchone()[0]:
                raise ValueError('Manifest address count mismatch')
            if has_districts and facts.get('district_object_count',db.execute('SELECT count(*) FROM district_objects').fetchone()[0])!=db.execute('SELECT count(*) FROM district_objects').fetchone()[0]:
                raise ValueError('Manifest district object count mismatch')
        db.commit(); assert db.execute('PRAGMA integrity_check').fetchone()[0] == 'ok'; db.close()
        os.chmod(temporary,0o444); os.replace(temporary,output)
        print(f'Built {count} real reference entities: {output}')
    finally:
        if db is not None: db.close()
        if os.path.exists(temporary): os.unlink(temporary)

if __name__ == '__main__':
    p=argparse.ArgumentParser();p.add_argument('--input',required=True);p.add_argument('--output',required=True);p.add_argument('--manifest');p.add_argument('--addresses');p.add_argument('--districts')
    a=p.parse_args();build(a.input,a.output,a.manifest,a.addresses,a.districts)
