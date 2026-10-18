"""Pre-reference role arbitration; source spans and alternative retained."""
import copy,unicodedata
ALLOWED={'street_type','street_name','living_area','house'}
MARKERS={'микрорайон','мкр','мкрн','ықшамаудан','ықшамауданы','шағынаудан','шағынауданы','mikrorayon','mikrorajon','yqshamaudan','yqshamaudany'}
def signature(spans):return sorted((s['label'],s['start'],s['end']) for s in spans)
def lexical(raw,spans):return {i for s in spans for i in range(s['start'],s['end']) if unicodedata.category(raw[i])[0] in 'LNM'}
def marker_positions(raw):
 result=set();start=None
 for i,c in enumerate(raw+' '):
  if c.isalpha():
   if start is None:start=i
  elif start is not None:
   if raw[start:i].casefold() in MARKERS:result.update(range(start,i))
   start=None
 return result
def combine(base,proposal):
 if (base['id'],base['raw_address'])!=(proposal['id'],proposal['raw_address']):raise ValueError('input mismatch')
 if any(k in base for k in ['candidate_ids','numeric_evidence']):raise ValueError('must run before reference and numeric evidence assembly')
 out=copy.deepcopy(base);raw=base['raw_address']
 protected=lambda p:[s for s in p['spans'] if s['label'] not in ALLOWED]
 if signature(protected(base))!=signature(protected(proposal)):return out
 old=[s for s in base['spans'] if s['label'] in ALLOWED];new=[s for s in proposal['spans'] if s['label'] in ALLOWED]
 if signature(old)==signature(new):return out
 markers=marker_positions(raw)
 if not any(s['label'] in {'street_type','living_area'} and markers.intersection(range(s['start'],s['end'])) for s in new):return out
 was=lexical(raw,old);now=lexical(raw,new)
 if not was<=now or not (now-was)<=markers:return out
 out['spans']=copy.deepcopy(sorted(new+protected(base),key=lambda s:(s['start'],s['end'])))
 out['microdistrict_alternative']={'before':copy.deepcopy(old),'proposal':copy.deepcopy(new),'source':'microdistrict-contrast-v1','identity_selected':False}
 for field in ALLOWED:
  vs=list(dict.fromkeys(s['text'] for s in out['spans'] if s['label']==field));out['extracted_values'][field]=vs;out[field]=vs[0] if len(vs)==1 else None
 out['warnings']=[w for w in out.get('warnings',[]) if w not in {'multiple_extracted_'+f for f in ALLOWED}]
 out['warnings']=sorted(set(out['warnings'])|{'microdistrict_model_disagreement'}|{'multiple_extracted_'+f for f in ALLOWED if len(out['extracted_values'][f])>1})
 out['confidence']=None;out['status']='experimental_candidate'
 return out
