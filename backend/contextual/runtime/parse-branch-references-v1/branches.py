"""Reference candidates for explicit alternative parses, never a forced identity."""
from copy import deepcopy
FIELDS={'city':'city','province':'province','street_name':'street','living_area':'living_area'}
MUTABLE={'street_type','street_name','living_area','house'}
def validate(raw,spans):
 last=0
 for s in sorted(spans,key=lambda s:(s['start'],s['end'])):
  a,b=s['start'],s['end']
  if not isinstance(a,int) or not isinstance(b,int) or not 0<=a<b<=len(raw) or a<last or raw[a:b]!=s['text']:raise ValueError('invalid source span')
  last=b

def collect(pred,lookup):
 raw=pred['raw_address'];current=pred['spans'];validate(raw,current)
 choices=[('current',current)]
 alt=pred.get('microdistrict_alternative')
 if alt:
  old=alt['before'];proposal=alt['proposal']
  if any(s['label'] not in MUTABLE for s in old+proposal):raise ValueError('protected role in alternative')
  sig=lambda ss:sorted((s['label'],s['start'],s['end'],s['text']) for s in ss)
  if sig(proposal)!=sig([s for s in current if s['label'] in MUTABLE]):raise ValueError('proposal differs from current extraction')
  original=old+[s for s in current if s['label'] not in MUTABLE];validate(raw,original)
  choices=[('original',original),('proposal',current)]
 branches=[];union={f:set() for f in FIELDS}
 for name,spans in choices:
  values={}
  for s in sorted(spans,key=lambda s:s['start']):
   vs=values.setdefault(s['label'],[])
   if s['text'] not in vs:vs.append(s['text'])
  refs={}
  for field,kind in FIELDS.items():
   ev=[];ids=set()
   for value in values.get(field,[]):
    result=deepcopy(lookup(kind,value));ev.append({'surface':value,'kind':kind,'evidence':result})
    ids.update(c['entity_id'] for c in result['candidates'])
   refs[field]={'candidate_ids':sorted(ids),'queries':ev};union[field].update(ids)
  branches.append({'branch':name,'values':values,'reference':refs,'selected':False})
 return {'parse_branches':branches,'candidate_ids':{f:sorted(ids) for f,ids in union.items()},'selected_branch':None,'selected_identity':None,'warnings':['unresolved_parse_alternative'] if alt else []}
