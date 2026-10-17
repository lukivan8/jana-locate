"""Explicit online PREPARATION only. Runtime reads the bundled snapshot."""
import argparse,urllib.parse,urllib.request
from pathlib import Path
from prepare import KATO_URL
p=argparse.ArgumentParser();p.add_argument('--output',default='.artifacts/reference')
p.add_argument('--osm-source',choices=['kazakhstan','astana'],default='kazakhstan')
p.add_argument('--refresh',action='store_true');a=p.parse_args()
root=Path(a.output);root.mkdir(parents=True,exist_ok=True)
if a.osm_source=='kazakhstan':
    osm_url='https://download.geofabrik.de/asia/kazakhstan-latest.osm.pbf';osm_name='kazakhstan-latest.osm.pbf'
else:
    query=(Path(__file__).parent/'astana.overpassql').read_text()
    osm_url='https://overpass-api.de/api/interpreter?'+urllib.parse.urlencode({'data':query});osm_name='astana.osm.json'
for url,name in [(urllib.parse.quote(KATO_URL,safe=':/'),'kato.xlsx'),
                 (osm_url,osm_name)]:
    target=root/name
    if target.exists() and not a.refresh:
        print('Reusing preparation source:',name);continue
    temporary=target.with_suffix(target.suffix+'.download')
    try:
        request=urllib.request.Request(url,headers={'User-Agent':'JanaAddress-catalog-preparation/0.2'})
        with urllib.request.urlopen(request,timeout=180) as response,temporary.open('wb') as file:
            while chunk:=response.read(1024*1024):file.write(chunk)
        temporary.replace(target)
    finally:temporary.unlink(missing_ok=True)
    print('Downloaded preparation source:',name)
