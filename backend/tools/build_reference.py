"""Offline JSONL -> read-only SQLite. No coordinates; no connection to confirmation overlay."""
import argparse, json, os, sqlite3, tempfile
from pathlib import Path

def build(source, output, manifest=None):
    output = Path(output); output.parent.mkdir(parents=True, exist_ok=True)
    fd, temporary = tempfile.mkstemp(dir=output.parent, prefix='.reference-', suffix='.sqlite'); os.close(fd)
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
        db.execute('INSERT INTO metadata VALUES(?,?)', ('schema_version','1'))
        if manifest: db.execute('INSERT INTO metadata VALUES(?,?)', ('manifest',Path(manifest).read_text()))
        count = 0
        with open(source) as lines:
            for line in lines:
                e = json.loads(line)
                if e['kind'] not in {'province','city','district','living_area','street','admin'}: raise ValueError('Unknown entity kind')
                if any(k in e for k in ['coordinates','latitude','longitude']): raise ValueError('Administrative directory must not contain building coordinates')
                keys = ['id','kind','name','parent_id','city_id','province_id','district_id','street_type','source']
                db.execute('INSERT INTO entities VALUES(?,?,?,?,?,?,?,?,?)', [e.get(k,'') for k in keys])
                for alias in sorted(set(e.get('aliases',[]) + [e['name']])):
                    if alias.strip(): db.execute('INSERT INTO aliases VALUES(?,?)',(e['id'],alias.strip()))
                count += 1
        for key in ['parent_id','city_id','province_id','district_id']:
            missing = db.execute(f"SELECT id,{key} FROM entities WHERE {key} != '' AND {key} NOT IN (SELECT id FROM entities)").fetchall()
            if missing: raise ValueError(f'Dangling {key}: {missing[:5]}')
        db.commit(); assert db.execute('PRAGMA integrity_check').fetchone()[0] == 'ok'; db.close()
        os.chmod(temporary,0o444); os.replace(temporary,output)
        print(f'Built {count} real reference entities: {output}')
    finally:
        if os.path.exists(temporary): os.unlink(temporary)

if __name__ == '__main__':
    p=argparse.ArgumentParser();p.add_argument('--input',required=True);p.add_argument('--output',required=True);p.add_argument('--manifest')
    a=p.parse_args();build(a.input,a.output,a.manifest)
