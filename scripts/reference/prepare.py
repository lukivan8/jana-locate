"""Build-time preparation from an official KATO workbook and an OSM Overpass JSON snapshot.
Raw sources are local inputs; this script never makes runtime network requests.
"""
import argparse, hashlib, json, re
from datetime import datetime, timezone
from pathlib import Path
import openpyxl

KATO_URL='https://stat.gov.kz/upload/iblock/a7d/eqogm92xc0udumlzfzffhcrutk0fjhb4/КАТО_18.09.2026.xlsx'
def strip_name(s):
    s=re.sub(r'^(?:г\.|с\.|п\.|город\s+|село\s+|пос[её]лок\s+)\s*','',s,flags=re.I)
    s=re.sub(r'\s+(?:қ\.|а\.|к\.)$','',s)
    return s.strip()
def prepare(kato,osm,output,manifest):
    wb=openpyxl.load_workbook(kato,read_only=True,data_only=True)
    rows=list(wb.active.iter_rows(values_only=True)); header=list(rows.pop(0))
    raw={str(r[0]).zfill(9):dict(zip(header,r)) for r in rows if r[0]}
    entities={}
    for code,r in raw.items():
        ru=str(r.get('rus_name') or '').strip();kk=str(r.get('kaz_name') or '').strip()
        republican=code.endswith('0000000') and bool(re.match(r'^(?:г\.|город)',ru,re.I))
        kind='city' if republican or re.match(r'^(?:г\.|с\.|п\.|город\s|село\s|пос[её]лок\s)',ru,re.I) else 'province' if code.endswith('0000000') else 'district' if ru.endswith('район') else 'admin'
        parent=next((p for p in [code[:6]+'000',code[:4]+'00000',code[:2]+'0000000'] if p!=code and p in raw),'')
        name=re.sub(r'\s+район$','',strip_name(ru)) if kind=='district' else strip_name(ru)
        entities['kato:'+code]={'id':'kato:'+code,'kind':kind,'name':name,'parent_id':'kato:'+parent if parent else '',
            'city_id':'','province_id':'kato:'+code[:2]+'0000000','district_id':'','street_type':'','source':'KATO:18.09.2026',
            'aliases':sorted(set([ru,kk,strip_name(ru),strip_name(kk),name]))}
    for e in entities.values():
        cur=e;seen=set()
        while cur and cur['id'] not in seen:
            seen.add(cur['id'])
            if cur['kind']=='city' and not e['city_id']:e['city_id']=cur['id']
            if cur['kind']=='district' and not e['district_id']:e['district_id']=cur['id']
            cur=entities.get(cur['parent_id'])
    astana=next(e for e in entities.values() if e['kind']=='city' and e['name']=='Астана' and e['parent_id']=='')
    street_prefix=re.compile(r'^(улица|проспект|переулок|проезд|бульвар|шоссе)\s+|\s+(көшесі|даңғылы)$',re.I)
    osm_data=json.loads(Path(osm).read_text())
    if not osm_data.get('elements'):raise ValueError('Empty OSM snapshot; do not publish an apparently complete directory')
    if 'remark' in osm_data:raise ValueError('Overpass returned an incomplete/error snapshot: '+osm_data['remark'])
    for item in osm_data['elements']:
        tags=item.get('tags',{});ru=tags.get('name:ru',tags.get('name',''));name=street_prefix.sub('',ru).strip()
        if not name:continue
        kind='street' if 'highway' in tags else 'living_area'
        typ=next((v for k,v in [('проспект','проспект'),('переулок','переулок'),('бульвар','бульвар'),('шоссе','шоссе'),('улица','улица')] if k in ru.lower()),'') if kind=='street' else ''
        aliases=[]
        for key,value in tags.items():
            if key in {'name','name:ru','name:kk','name:en','alt_name','old_name','short_name'}:
                for alias in value.split(';'):aliases.extend([alias,street_prefix.sub('',alias).strip()])
        # Multiple OSM road segments with the same name are one logical street in
        # this explicitly city-scoped snapshot; their source snapshot remains identified by its hash in the manifest.
        identity='osm:astana:'+kind+':'+hashlib.sha256((typ+'|'+name.casefold()).encode()).hexdigest()[:20]
        if identity in entities:
            entities[identity]['aliases']=sorted(set(entities[identity]['aliases']+aliases))
        else:
            entities[identity]={'id':identity,'kind':kind,'name':name,'parent_id':astana['id'],'city_id':astana['id'],
                'province_id':astana['province_id'],'district_id':'','street_type':typ,'source':'OSM:Astana-bbox','aliases':sorted(set(aliases))}
    output=Path(output);output.parent.mkdir(parents=True,exist_ok=True)
    with output.open('w') as f:
        for e in sorted(entities.values(),key=lambda e:e['id']):f.write(json.dumps(e,ensure_ascii=False,separators=(',',':'))+'\n')
    facts={'prepared_at':datetime.now(timezone.utc).isoformat(),'kato':{'url':KATO_URL,'sha256':hashlib.sha256(Path(kato).read_bytes()).hexdigest(),'rows':len(raw)},
        'osm':{'url':'https://overpass-api.de/api/interpreter','bbox':[51.00,71.20,51.25,71.65], 'scope':'Intersection of Astana administrative area (name:ru=Астана, admin_level=4) and bbox; NOT nationwide street coverage',
            'timestamp':osm_data.get('osm3s',{}).get('timestamp_osm_base'),'sha256':hashlib.sha256(Path(osm).read_bytes()).hexdigest(),'objects':len(osm_data['elements']),
            'attribution':'© OpenStreetMap contributors','license':'ODbL-1.0','license_url':'https://www.openstreetmap.org/copyright'},
        'entity_count':len(entities),'jsonl_sha256':hashlib.sha256(output.read_bytes()).hexdigest(),'building_coordinates':False}
    Path(manifest).write_text(json.dumps(facts,ensure_ascii=False,indent=2)+'\n')
    print('Prepared reference:',len(entities),'entities')
if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('--kato',required=True);p.add_argument('--osm',required=True);p.add_argument('--output',required=True);p.add_argument('--manifest',required=True)
    a=p.parse_args();prepare(a.kato,a.osm,a.output,a.manifest)
