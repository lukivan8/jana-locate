"""Uniform occurrence/branch lookup. No role assignment or identity selection."""
import copy,json,sys
from pathlib import Path
G=Path(__file__).resolve().parent.parent
sys.path[:0]=[str(G/'parse-branch-references-v1'),str(G/'district-role-lookup-v1')]
from full_lookup import FullLookup
from lookup import DistrictLookup
from branches import validate
FIELDS={'city':'city','province':'province','district':'district','living_area':'living_area','street_name':'street'}
MUTABLE={'street_type','street_name','living_area','house'}
class ReferenceStage:
 def __init__(self):
  self.base=FullLookup();self.district=DistrictLookup()
  self.entities={r['id']:r for r in map(json.loads,(G/'span-crf/reference-entities.jsonl').open())}
 def close(self):self.base.close();self.district.close()
 def query(self,field,surface):
  result=self.district.lookup(surface) if field=='district' else self.base.lookup(FIELDS[field],surface)
  ids=sorted({c['entity_id'] for c in result['candidates']})
  # Raw source warnings remain in source_evidence, never treated as final state.
  diagnostics=[]
  if not ids:diagnostics.append('unresolved_reference_'+field)
  if len(ids)>1:diagnostics.append('ambiguous_reference_'+field)
  tier=result.get('provenance',{}).get('tiered')
  tiers=[tier] if tier else result.get('source_queries',[])
  if any(c['entity_id'] in ids and c.get('best_tier')=='transliteration_ascii_lossy' for q in tiers for c in q.get('candidates',[])):
   diagnostics.append('lossy_transliteration_candidate_'+field)
  return {'surface':surface,'candidate_ids':ids,'selected_id':None,'diagnostics':diagnostics,'source_evidence':result}
 def apply(self,pred):
  raw=pred['raw_address'];validate(raw,pred['spans']);choices=[('current',pred['spans'])]
  alt=pred.get('microdistrict_alternative')
  if alt:
   sig=lambda spans:sorted((s['label'],s['start'],s['end'],s['text']) for s in spans)
   if sig(alt['proposal'])!=sig([s for s in pred['spans'] if s['label'] in MUTABLE]):raise ValueError('proposal mismatch')
   original=alt['before']+[s for s in pred['spans'] if s['label'] not in MUTABLE];validate(raw,original)
   choices=[('original',original),('proposal',pred['spans'])]
  branches=[];union={f:set() for f in FIELDS};diagnostics=[]
  for name,spans in choices:
   refs={f:[] for f in FIELDS}
   for s in spans:
    if s['label'] not in FIELDS:continue
    field=s['label'];q=self.query(field,s['text']);q['start']=s['start'];q['end']=s['end'];refs[field].append(q);union[field].update(q['candidate_ids'])
    diagnostics.extend({'branch':name,'field':field,'start':s['start'],'end':s['end'],'code':code} for code in q['diagnostics'])
   branches.append({'branch':name,'spans':copy.deepcopy(spans),'reference':refs,'selected':False})
  result=copy.deepcopy(pred)
  # Separate stage output; historical diagnostics retained rather than silently reinterpreted.
  result['prior_reference_warnings']=[w for w in pred.get('warnings',[]) if w.startswith(('unresolved_reference_','ambiguous_reference_','lossy_transliteration_candidate_'))]
  result['warnings']=[w for w in pred.get('warnings',[]) if w not in result['prior_reference_warnings']]
  result['reference_diagnostics']=diagnostics;result['reference_branches']=branches
  result['candidate_ids']={f:sorted(ids) for f,ids in union.items()};result['selected_ids']={f:None for f in FIELDS};result['selected_parse_branch']=None
  result['canonical_candidates']={f:[{'id':i,'name':self.entities[i]['name'],'source_kind':self.entities[i]['kind'],'province_id':self.entities[i]['province_id'],'city_id':self.entities[i]['city_id']} for i in ids] for f,ids in result['candidate_ids'].items()}
  result['warnings']=sorted(set(result['warnings'])|{d['code'] for d in diagnostics}|({'unresolved_parse_alternative'} if alt else set()))
  return result
