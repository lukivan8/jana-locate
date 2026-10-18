"""Compact numeric CRF inference. No address lookup or geographic resolution."""
import sys
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parent.parent/'span-crf'))
from model import tokenize,decode
ROLES={'house','apartment','entrance','floor'}
def surface(v):
 return ''.join('#' if c.isdecimal() else c.casefold() for c in v)
def features(tokens):
 out=[]
 for i,(a,b,v) in enumerate(tokens):
  f={'bias':1.,'word':surface(v),'digit':v.isdecimal(),'alpha':v.isalpha()}
  for delta in [-3,-2,-1,0,1,2,3]:
   j=i+delta
   if 0<=j<len(tokens):
    x,y,w=tokens[j];key=str(delta)
    f['w'+key]=surface(w);f['boundary_word'+key]=surface(w)+'|'+str(min(x-tokens[j-1][1],2) if j else -1);f['adjacent_word'+key]=surface(w)+'|'+str(x==b if delta==1 else y==a if delta==-1 else False);f['suffix'+key]=surface(w)[-4:];f['digit'+key]=w.isdecimal()
    if j:f['gap'+key]=min(x-tokens[j-1][1],2)
   else:f['edge'+str(delta)]=True
  out.append(f)
 return out
def predict(tagger,row):
 raw=row['raw_address'];ts=tokenize(raw)
 if not ts:return []
 tags=tagger.tag(features(ts));ss=decode(raw,ts,tags)
 for s in ss:
  s['posterior']=min(tagger.marginal(tags[i],i) for i,(a,b,_) in enumerate(ts) if s['start']<=a<b<=s['end'])
 return ss
