"""Source-grounded uncertainty; never assigns a number to a new role."""
import copy,sys
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parent.parent/'integrated-candidate-v1'))
from diagnostics import digit_runs
NUMERIC={'house','apartment','entrance','floor'}
def diagnose(pred):
 out=copy.deepcopy(pred);raw=pred['raw_address'];spans=pred['spans'];clusters=[]
 def owners(a,b):return [s for s in spans if s['start']<b and a<s['end']]
 def grounded(a,b):return any(e['start']==a and e['end']==b and e['label'] in NUMERIC for e in pred.get('native_numeric_evidence',[]))
 runs=digit_runs(raw)
 for (a,b),(c,d) in zip(runs,runs[1:]):
  gap=raw[b:c]
  if not gap or any(not(ch.isspace() or ch in ',;') for ch in gap):continue
  left,right=owners(a,b),owners(c,d)
  if any(s['label'] not in NUMERIC|{'other'} for s in left+right):continue
  # Adjacent letters or numeric suffixes are not a bare-number sequence.
  if (a and raw[a-1].isalnum()) or (d<len(raw) and raw[d].isalnum()):continue
  # A hyphen-linked lexical suffix/prefix can be an explicit role marker
  # in several input languages. This detector does not interpret that word.
  if d+1<len(raw) and raw[d] in '-‐‑' and raw[d+1].isalpha():continue
  if a>=2 and raw[a-1] in '-‐‑' and raw[a-2].isalpha():continue
  if grounded(a,b) and grounded(c,d):continue
  clusters.append({'start':a,'end':d,'text':raw[a:d],'reason':'adjacent_numbers_without_role_separator','numbers':[{'start':a,'end':b,'text':raw[a:b],'proposed_roles':sorted({s['label'] for s in left})},{'start':c,'end':d,'text':raw[c:d],'proposed_roles':sorted({s['label'] for s in right})}],'resolved':False})
 out['numeric_role_ambiguities']=clusters
 if clusters:out['warnings']=sorted(set(out.get('warnings',[]))|{'ambiguous_unmarked_numeric_roles'})
 return out
