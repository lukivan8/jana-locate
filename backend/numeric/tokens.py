"""Unicode tokenization and BIO decoding used by the numeric CRF."""
import unicodedata
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

def decode(raw,tokens,tags):
 spans=[];active=None
 for (start,end,value),tag in zip(tokens,tags):
  if tag=='O':active=None;continue
  prefix,label=tag.split('-',1)
  if prefix=='B' or active is None or active['label']!=label:
   active={'label':label,'start':start,'end':end,'text':raw[start:end]};spans.append(active)
  else:active['end']=end;active['text']=raw[active['start']:end]
 return spans

