"""Offline alias candidate generation on extracted spans, never on benchmark IDs."""
import copy
import unicodedata
from collections import defaultdict
from text_unidecode import unidecode

def exact(s):
 return ' '.join(unicodedata.normalize('NFC',s).casefold().split())
def folded(s):
 return ' '.join(''.join(c if c.isalnum() else ' ' for c in unidecode(exact(s))).split())

class Resolver:
 def __init__(self,entities):
  self.entities={e['id']:e for e in entities}
  self.index=defaultdict(set)
  for e in self.entities.values():
   if e['kind'] not in ('city','province'):continue
   for alias in [e['name'],*e.get('aliases',[])]:
    for mode,fn in [('exact',exact),('transliteration',folded)]:
     key=fn(alias)
     if key:self.index[e['kind'],mode,key].add(e['id'])
 def resolve(self,pred):
  out=copy.deepcopy(pred);out['candidate_ids']={};out['selected_ids']={};out['reference_evidence']={}
  for field in ('city','province'):
   values=pred.get('extracted_values',{}).get(field,[])
   evidence=[];candidates=set();matched=[]
   for value in values:
    found=set()
    for mode,fn in [('exact',exact),('transliteration',folded)]:
     ids=self.index[field,mode,fn(value)]
     for entity_id in sorted(ids):evidence.append({'surface':value,'mode':mode,'entity_id':entity_id})
     found.update(ids)
    matched.append(found);candidates.update(found)
   out['candidate_ids'][field]=sorted(candidates);out['reference_evidence'][field]=evidence
   # Union preserves competing interpretations. An unmatched second span prevents selection.
   chosen=next(iter(candidates)) if len(candidates)==1 and matched and all(matched) else None
   out['selected_ids'][field]=chosen
   out[field]=self.entities[chosen]['name'] if chosen else None
   if len(candidates)>1:out['warnings'].append('ambiguous_reference_'+field)
   if values and any(not ids for ids in matched):out['warnings'].append('unresolved_reference_'+field)
  # No claim of address-wide correctness, confidence calibration or exhaustive fuzzy recall.
  out['status']='not_evaluated';out['confidence']=None
  return out
