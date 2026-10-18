"""Offset-safe supervision utilities for a future contextual tagger."""
FIELDS=['province','city','district','living_area','street_type','street_name','house','apartment','entrance','floor','landmark','delivery_instruction','country','other']
LABELS=['O']+[prefix+'-'+field for field in FIELDS for prefix in ['B','I']]

def trim(raw,start,end):
 if not 0<=start<=end<=len(raw):raise ValueError('offset outside original text')
 while start<end and raw[start].isspace():start+=1
 while end>start and raw[end-1].isspace():end-=1
 return start,end

def supervise(raw,offsets,spans):
 result=[];previous=None
 for start,end in offsets:
  start,end=trim(raw,start,end)
  if start==end:result.append(-100);continue
  overlaps=[(i,s) for i,s in enumerate(spans) if s['start']<end and s['end']>start]
  if not overlaps:result.append(0);previous=None;continue
  if len(overlaps)!=1:raise ValueError('subword spans multiple gold entities')
  i,s=overlaps[0]
  if not s['start']<=start<end<=s['end']:raise ValueError('subword crosses a gold boundary')
  label=('I-' if previous==i else 'B-')+s['label'];result.append(LABELS.index(label));previous=i
 return result

def decode(raw,offsets,labels):
 spans=[];active=None
 for (start,end),label in zip(offsets,labels):
  start,end=trim(raw,start,end)
  if start==end:continue
  if label=='O':active=None;continue
  prefix,field=label.split('-',1)
  if prefix=='B' or active is None or active['label']!=field:
   active={'label':field,'start':start,'end':end,'text':raw[start:end]};spans.append(active)
  else:
   active['end']=max(active['end'],end);active['text']=raw[active['start']:active['end']]
 return spans
