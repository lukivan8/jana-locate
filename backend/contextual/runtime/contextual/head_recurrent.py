import torch
from alignment import LABELS
from torch.nn.utils.rnn import pack_padded_sequence,pad_packed_sequence

def context_features(x):return x.float()
class SequenceHead(torch.nn.Module):
 def __init__(self,width=776):
  super().__init__();self.norm=torch.nn.LayerNorm(width);self.project=torch.nn.Linear(width,128);self.gru=torch.nn.GRU(128,96,batch_first=True,bidirectional=True);self.drop=torch.nn.Dropout(.1);self.output=torch.nn.Linear(192,len(LABELS))
 def forward(self,x,lengths=None):
  single=x.ndim==2
  if single:x=x.unsqueeze(0)
  if lengths is None:lengths=torch.full((len(x),),x.shape[1],dtype=torch.long)
  x=torch.nn.functional.gelu(self.project(self.norm(x)))
  packed=pack_padded_sequence(x,lengths.cpu(),batch_first=True,enforce_sorted=False)
  y,_=self.gru(packed);y,_=pad_packed_sequence(y,batch_first=True,total_length=x.shape[1]);y=self.output(self.drop(y))
  return y[0] if single else y

def make_head(width=776):return SequenceHead(width)
