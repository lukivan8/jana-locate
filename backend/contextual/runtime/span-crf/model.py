"""Unicode token BIO CRF. No address-specific regex or dictionary of answers."""
import unicodedata
FIELDS=['province','city','district','living_area','street_type','street_name','house','apartment','entrance','floor','landmark','delivery_instruction','country','other']
def tokenize(raw):
 result=[];start=None;last=None
 for i,c in enumerate(raw):
  cat=unicodedata.category(c)
  group='L' if cat[0] in 'LM' else 'N' if cat[0]=='N' else last if cat=='Cf' and start is not None else 'P'
  if c.isspace():
   if start is not None:result.append((start,i,raw[start:i]))
   start=None;last=None;continue
  if start is not None and (group!=last or group=='P'):
   result.append((start,i,raw[start:i]));start=None
  if start is None:start=i
  last=group
 if start is not None:result.append((start,len(raw),raw[start:]))
 return result

def features(tokens):
 out=[]
 for i,(start,end,value) in enumerate(tokens):
  lower=value.casefold();shape=''.join('D' if c.isdigit() else 'U' if c.isupper() else 'l' if c.isalpha() else c for c in value)
  f={'bias':1.0,'word':lower,'shape':shape[:20],'upper':value.isupper(),'title':value.istitle(),'digit':value.isdigit(),'length':str(min(len(value),20)),'bos':i==0,'eos':i==len(tokens)-1}
  for n in range(1,5):f['prefix'+str(n)]=lower[:n];f['suffix'+str(n)]=lower[-n:]
  for delta in [-2,-1,1,2]:
   j=i+delta
   if 0<=j<len(tokens):
    v=tokens[j][2];f['word'+str(delta)]=v.casefold();f['digit'+str(delta)]=v.isdigit();f['suffix3:'+str(delta)]=v.casefold()[-3:]
  if i:f['space_before']=start>tokens[i-1][1]
  if i+1<len(tokens):f['space_after']=tokens[i+1][0]>end
  out.append(f)
 return out

def gold_labels(raw,spans,tokens):
 tags=[];last=None
 for start,end,value in tokens:
  overlaps=[(i,s) for i,s in enumerate(spans) if s['start']<end and s['end']>start]
  if not overlaps:tags.append('O');last=None;continue
  if len(overlaps)!=1 or not overlaps[0][1]['start']<=start<end<=overlaps[0][1]['end']:raise ValueError('Token crosses annotation boundary')
  index,span=overlaps[0]
  if span['label'] not in FIELDS:raise ValueError('Unknown annotation label')
  tags.append(('I-' if last==index else 'B-')+span['label']);last=index
 return tags

def decode(raw,tokens,tags):
 spans=[];active=None
 for (start,end,value),tag in zip(tokens,tags):
  if tag=='O':active=None;continue
  prefix,label=tag.split('-',1)
  if prefix=='B' or active is None or active['label']!=label:
   active={'label':label,'start':start,'end':end,'text':raw[start:end]};spans.append(active)
  else:active['end']=end;active['text']=raw[active['start']:end]
 return spans

def predict(tagger,row,feature_fn=features):
 raw=row['raw_address'];tokens=tokenize(raw);spans=decode(raw,tokens,tagger.tag(feature_fn(tokens))) if tokens else []
 values={f:[] for f in FIELDS}
 for s in spans:
  if s['text'] not in values[s['label']]:values[s['label']].append(s['text'])
 result={f:(v[0] if len(v)==1 else None) for f,v in values.items()}
 result.update(id=row['id'],raw_address=raw,spans=spans,extracted_values=values,status='not_evaluated',confidence=None,warnings=['extraction_only_identity_not_evaluated']+['multiple_extracted_'+f for f,v in values.items() if len(v)>1])
 return result
