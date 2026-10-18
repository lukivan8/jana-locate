import torch
from alignment import LABELS

def context_features(x):
 x=x.float();zero=torch.zeros_like(x[:1])
 if not len(x):return x.new_empty((0,x.shape[1]*3))
 return torch.cat([x,torch.cat([zero,x[:-1]]),torch.cat([x[1:],zero])],dim=1)
def make_head(width=2328):
 return torch.nn.Sequential(torch.nn.LayerNorm(width),torch.nn.Linear(width,192),torch.nn.GELU(),torch.nn.Dropout(0.1),torch.nn.Linear(192,len(LABELS)))
