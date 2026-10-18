"""Generic BIO-constrained Viterbi; optional training-only transition prior."""
import numpy as np

def masks(labels):
 start=np.zeros(len(labels));trans=np.zeros((len(labels),len(labels)))
 for j,label in enumerate(labels):
  if label.startswith('I-'):
   start[j]=-np.inf
   for i,previous in enumerate(labels):
    if previous not in ('B-'+label[2:],'I-'+label[2:]):trans[i,j]=-np.inf
 return start,trans

def viterbi(emissions,start,trans):
 if len(emissions)==0:return []
 score=np.asarray(emissions[0],dtype=np.float64)+start;back=[]
 for row in emissions[1:]:
  scores=score[:,None]+trans;indices=scores.argmax(0);back.append(indices);score=scores[indices,np.arange(len(score))]+row
 last=int(score.argmax());path=[last]
 for indices in reversed(back):last=int(indices[last]);path.append(last)
 return list(reversed(path))
