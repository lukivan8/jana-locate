"""Explicit online PREPARATION only. Runtime reads the bundled snapshot."""
import argparse,urllib.parse,urllib.request
from pathlib import Path
from prepare import KATO_URL
p=argparse.ArgumentParser();p.add_argument('--output',default='.artifacts/reference');a=p.parse_args()
root=Path(a.output);root.mkdir(parents=True,exist_ok=True)
query=(Path(__file__).parent/'astana.overpassql').read_text()
for url,name in [(urllib.parse.quote(KATO_URL,safe=':/'),'kato.xlsx'),
                 ('https://overpass-api.de/api/interpreter?'+urllib.parse.urlencode({'data':query}),'astana.osm.json')]:
    request=urllib.request.Request(url,headers={'User-Agent':'JanaAddress-catalog-preparation/0.1'})
    with urllib.request.urlopen(request,timeout=120) as response:(root/name).write_bytes(response.read())
    print('Downloaded preparation source:',name)
