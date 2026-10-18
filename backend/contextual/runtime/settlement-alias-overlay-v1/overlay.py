"""Additive typed source-backed aliases. No raw-address lookup or identity selection."""
import json,unicodedata,collections
from pathlib import Path
def key(value):return ' '.join(unicodedata.normalize('NFC',value).casefold().split())
class Overlay:
 def __init__(self,path):
  self.index=collections.defaultdict(list)
  for r in map(json.loads,Path(path).read_text().splitlines()):
   if r['kind']!='city' or r.get('selected') is not False:raise ValueError('invalid alias evidence')
   self.index[key(r['alias'])].append(r)
 def lookup(self,kind,value,existing_ids=()):
  ev=self.index.get(key(value),[]) if kind=='city' else [];ids=set(existing_ids)|{r['entity_id'] for r in ev};warnings=[]
  if ev:warnings.append('source_backed_alias_candidate_'+kind)
  if len(ids)>1:warnings.append('ambiguous_reference_'+kind)
  return {'kind':kind,'surface':value,'candidate_ids':sorted(ids),'added_ids':sorted(ids-set(existing_ids)),'evidence':ev,'selected_id':None,'status':'candidate_only','warnings':warnings}
