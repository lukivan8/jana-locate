"""Conserve numeric source evidence without guessing apartment/building roles."""
def digit_runs(raw):
 runs=[];start=None
 for i,c in enumerate(raw+' '):
  if c.isdecimal():
   if start is None:start=i
  elif start is not None:runs.append((start,i));start=None
 return runs

def numeric_evidence(pred):
 raw=pred['raw_address'];spans=pred['spans'];runs=digit_runs(raw);evidence=[];warnings=[]
 for start,end in runs:
  owners=[{'label':s['label'],'start':s['start'],'end':s['end'],'text':s['text']} for s in spans if s['start']<=start and s['end']>=end]
  unresolved=not owners or all(s['label']=='other' for s in owners)
  if unresolved:warnings.append('unresolved_numeric_span')
  evidence.append({'start':start,'end':end,'text':raw[start:end],'owners':owners,'unresolved':unresolved})
 for label in ['house','apartment','entrance','floor']:
  ss=[s for s in spans if s['label']==label]
  if len(ss)>1:warnings.append('multiple_numeric_role_'+label)
  for s in ss:
   local=[(a,b) for a,b in runs if a>=s['start'] and b<=s['end']]
   # Slash/hyphen compounds remain whole. Space or words between numbers are
   # evidence of a composite/unclear role, not permission to split into fields.
   if any(raw[b:a].strip() not in ['/', '-'] for (_,b),(a,_) in zip(local,local[1:])):warnings.append('compound_numeric_role_'+label)
 return {'numeric_spans':evidence,'warnings':sorted(set(warnings))}
