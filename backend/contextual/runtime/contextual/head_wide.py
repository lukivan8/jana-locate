import torch
from alignment import LABELS

def context_features(x):
 x=x.float();n=len(x);width=x.shape[1]
 if not n:return x.new_empty((0,width*5))
 padded=torch.cat([x.new_zeros((2,width)),x,x.new_zeros((2,width))])
 return torch.cat([padded[2+d:2+d+n] for d in [-2,-1,0,1,2]],dim=1)
def make_head(width=3880):
 return torch.nn.Sequential(torch.nn.LayerNorm(width),torch.nn.Linear(width,192),torch.nn.GELU(),torch.nn.Dropout(0.1),torch.nn.Linear(192,len(LABELS)))
