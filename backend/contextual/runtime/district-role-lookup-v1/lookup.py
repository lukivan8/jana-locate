import json,sys,unicodedata
from pathlib import Path
P=Path(__file__).resolve().parent;G=P.parent
sys.path.insert(0,str(G/'reference-transliteration'))
from tiered_lookup import TieredLookup
key=lambda s:' '.join(unicodedata.normalize('NFC',s).casefold().split())
class DistrictLookup:
 def __init__(self):
  self.roles={r['id']:r for r in map(json.loads,(P/'roles.jsonl').open())};self.aliases={}
  for r in self.roles.values():
   for a in r['aliases']:self.aliases.setdefault(key(a['surface']),[]).append((r['id'],a))
  self.tiered=TieredLookup(G/'reference-transliteration/reference-index.sqlite',G/'reference-transliteration/preserve-reference-index.sqlite')
 def close(self):self.tiered.close()
 def lookup(self,value):
  sources=[self.tiered.lookup(k,value) for k in ['district','admin']];ev={}
  for source in sources:
   for c in source['candidates']:
    if c['entity_id'] in self.roles:ev.setdefault(c['entity_id'],[]).append({'source_kind':source['kind'],'match':c})
  for identity,alias in self.aliases.get(key(value),[]):ev.setdefault(identity,[]).append({'source_kind':'role_overlay','match':alias})
  candidates=[{'entity_id':i,'role_evidence':self.roles[i]['role_evidence'],'source_kind':self.roles[i]['source_kind'],'evidence':e} for i,e in sorted(ev.items())]
  warnings=[]
  if not candidates:warnings.append('unresolved_reference_district')
  if len(candidates)>1:warnings.append('ambiguous_reference_district')
  return {'surface':value,'kind':'district','candidates':candidates,'selected_id':None,'warnings':warnings,'source_queries':sources,'containment_inferred':False}
