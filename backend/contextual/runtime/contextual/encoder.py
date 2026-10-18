import sys,torch,unicodedata
from pathlib import Path
from transformers import AutoModel,AutoTokenizer
sys.path.insert(0,str(Path(__file__).parent.parent/'span-crf'))
from model import tokenize
class Encoder:
 def __init__(self,assets):
  self.tokenizer=AutoTokenizer.from_pretrained(assets,local_files_only=True)
  self.model=AutoModel.from_pretrained(assets,local_files_only=True).eval()
 def encode(self,raws):
  words=[tokenize(raw) for raw in raws];chunks=[];width=self.model.config.hidden_size
  for sample,tokens in enumerate(words):
   if not tokens:continue
   enc=self.tokenizer([w[2] for w in tokens],is_split_into_words=True,max_length=256,truncation=True,stride=32,return_overflowing_tokens=True)
   for j,ids in enumerate(enc['input_ids']):chunks.append((sample,enc.word_ids(j),{'input_ids':ids,'attention_mask':enc['attention_mask'][j]}))
  sums=[torch.zeros((len(w),width)) for w in words];counts=[torch.zeros(len(w)) for w in words];chunks.sort(key=lambda c:len(c[2]['input_ids']))
  with torch.inference_mode():
   for start in range(0,len(chunks),8):
    batch=chunks[start:start+8];encoded=self.tokenizer.pad([c[2] for c in batch],return_tensors='pt');hidden=self.model(**encoded).last_hidden_state
    for j,(sample,word_ids,_) in enumerate(batch):
     positions=[i for i,w in enumerate(word_ids) if w is not None];ids=torch.tensor([word_ids[i] for i in positions],dtype=torch.long)
     if positions:sums[sample].index_add_(0,ids,hidden[j,positions]);counts[sample].index_add_(0,ids,torch.ones(len(ids)))
  outputs=[]
  for tokens,total,count in zip(words,sums,counts):
   context=total/count.clamp_min(1).unsqueeze(1)
   # Missing context is observable; original token boundaries and text are retained.
   shape=torch.tensor([[float(count[i]>0),float(v.isdigit()),float(v.isalpha()),float(v.isupper()),float(v.istitle()),min(len(v),30)/30,float(all(unicodedata.category(c)[0]=='P' for c in v)),float(all(unicodedata.category(c)[0]=='C' for c in v))] for i,(_,_,v) in enumerate(tokens)]).reshape(len(tokens),8)
   outputs.append({'tokens':tokens,'features':torch.cat([context,shape],dim=1),'missing_context_indices':[i for i,n in enumerate(count) if n==0]})
  return outputs
