from head_joint import context_features
from alignment import LABELS
from sequence import masks,viterbi
import sys,torch,numpy as np,os
from pathlib import Path
sys.path.insert(0,'/lab/goal/span-crf')
from model import decode,FIELDS
MODE=os.environ.get('SEQUENCE_MODE','bio')
if MODE=='markov':
 prior=np.load(Path(__file__).parent/'transitions.npz');START,TRANS=prior['start'],prior['trans']
elif MODE=='bio':START,TRANS=masks(LABELS)
else:raise ValueError('Unknown sequence mode')
def predict(row,features,net):
 with torch.inference_mode():
  logits=net(context_features(features['features'])).numpy() if features['tokens'] else []
 tags=[LABELS[i] for i in viterbi(logits,START,TRANS)]
 spans=decode(row['raw_address'],features['tokens'],tags);values={f:[] for f in FIELDS}
 for s in spans:
  if s['text'] not in values[s['label']]:values[s['label']].append(s['text'])
 result={f:(v[0] if len(v)==1 else None) for f,v in values.items()};result.update(id=row['id'],raw_address=row['raw_address'],spans=spans,extracted_values=values,status='not_evaluated',confidence=None,warnings=['extraction_only_identity_not_evaluated']+['multiple_extracted_'+f for f,v in values.items() if len(v)>1],missing_context_indices=features['missing_context_indices']);return result
