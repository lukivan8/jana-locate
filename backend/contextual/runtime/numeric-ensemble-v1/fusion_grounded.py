"""Experimental contextual native numeric-role evidence; no address/name rules."""
import copy,unicodedata,sys
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parent.parent/"integrated-candidate-v1"))
from diagnostics import numeric_evidence
ROLES={'house_number':'house','unit':'apartment','level':'floor','staircase':'entrance'}
def tokens(raw):
 out=[];start=None;kind=None
 for i,c in enumerate(raw+' '):
  new='d' if c.isdecimal() else 'a' if c.isalpha() else None
  if new!=kind or new is None:
   if start is not None:out.append((start,i,raw[start:i].casefold(),kind));start=None
   if new:start=i
  kind=new
 return out

def native_evidence(raw,components):
 source=tokens(raw);out=[]
 for component in components:
  if component['label'] not in ROLES:continue
  ts=tokens(component['value']);nums=[(i,t) for i,t in enumerate(ts) if t[3]=='d']
  # Restrict proposed overrides to a single standalone integer in a native
  # numeric component containing alphabetic context. All others stay unresolved.
  if len(nums)!=1 or not any(t[3]=='a' for t in ts):continue
  pattern=[t[2] for t in ts];matches=[i for i in range(len(source)-len(ts)+1) if [t[2] for t in source[i:i+len(ts)]]==pattern]
  if len(matches)!=1:continue
  i=matches[0];number=source[i+nums[0][0]];a,b=number[:2]
  context=raw[source[i][0]:source[i+len(ts)-1][1]]
  punctuation=lambda text: ''.join(c for c in text if unicodedata.category(c).startswith('P'))
  # Never ground a native phrase across punctuation that disappeared from it.
  # E.g. native 'floor 19' cannot justify source 'floor, 19 apartment'.
  if punctuation(context)!=punctuation(component['value']):continue
  if (a and raw[a-1].isalnum()) or (b<len(raw) and raw[b].isalnum()):continue
  out.append({'label':ROLES[component['label']],'start':a,'end':b,'text':raw[a:b],'native_component':component,'context_start':source[i][0],'context_end':source[i+len(ts)-1][1]})
 return out

def fuse(pred,components):
 out=copy.deepcopy(pred);ev=native_evidence(pred['raw_address'],components);out['native_numeric_evidence']=ev;changes=[];roles=set(ROLES.values())
 for e in ev:
  overlap=[s for s in out['spans'] if s['start']<e['end'] and e['start']<s['end']]
  if any(s['label'] not in roles or (s['start'],s['end'])!=(e['start'],e['end']) for s in overlap):continue
  if any(s['label']==e['label'] for s in overlap):continue
  before=[dict(s) for s in overlap];out['spans']=[s for s in out['spans'] if s not in overlap];span={k:e[k] for k in ['label','start','end','text']};out['spans'].append(span);changes.append({'before':before,'after':span,'context':e})
 out['spans'].sort(key=lambda s:(s['start'],s['end'],s['label']))
 for field in out['extracted_values']:
  vals=list(dict.fromkeys(s['text'] for s in out['spans'] if s['label']==field));out['extracted_values'][field]=vals;out[field]=vals[0] if len(vals)==1 else None
 out['numeric_role_changes']=changes
 # Keep the original assignments in changes[].before; never hide disagreement.
 stale={'multiple_extracted_'+r for r in roles}
 out['warnings']=[w for w in out.get('warnings',[]) if w not in stale and 'numeric_role_' not in w and w!='unresolved_numeric_span']
 out['warnings'] += ['multiple_extracted_'+r for r in roles if len(out['extracted_values'].get(r,[]))>1]
 if changes:out['warnings'].append('numeric_parser_disagreement')
 numeric=numeric_evidence(out);out['numeric_evidence']=numeric['numeric_spans'];out['warnings']+=numeric['warnings'];out['warnings']=sorted(set(out['warnings']))
 out['status']='experimental_candidate';out['confidence']=None
 return out
