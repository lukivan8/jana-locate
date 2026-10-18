import torch
from head_recurrent import SequenceHead
from head_wide import make_head as make_local,context_features as local_features

def context_features(x):return x.float()
class JointHead(torch.nn.Module):
 def __init__(self,width=776):
  super().__init__();self.local=make_local(width*5);self.sequence=SequenceHead(width)
 def forward(self,x,lengths=None):
  single=x.ndim==2
  if single:x=x.unsqueeze(0)
  if lengths is None:lengths=torch.full((len(x),),x.shape[1],dtype=torch.long)
  # Slice before local context construction: neighbouring padding must never enter windows.
  local=[self.local(local_features(row[:int(n)])) for row,n in zip(x,lengths)]
  logits=torch.nn.utils.rnn.pad_sequence(local,batch_first=True)
  if logits.shape[1]<x.shape[1]:logits=torch.nn.functional.pad(logits,(0,0,0,x.shape[1]-logits.shape[1]))
  logits=logits+self.sequence(x,lengths)
  return logits[0] if single else logits

def make_head(width=776):return JointHead(width)
