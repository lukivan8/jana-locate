"""Bridge fusion output to pre-reference arbitration and rebuild numeric owners."""
import copy,sys
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parent.parent/'integrated-candidate-v1'))
from diagnostics import numeric_evidence
from combine import combine

def apply_after_fusion(base,proposal):
 b=copy.deepcopy(base);p=copy.deepcopy(proposal)
 b.pop('numeric_evidence',None);p.pop('numeric_evidence',None)
 out=combine(b,p)
 if 'microdistrict_alternative' not in out:return copy.deepcopy(base)
 old=set(numeric_evidence(base)['warnings']);out['warnings']=[w for w in out['warnings'] if w not in old]
 n=numeric_evidence(out);out['numeric_evidence']=n['numeric_spans'];out['warnings']=sorted(set(out['warnings'])|set(n['warnings']))
 return out
