"""Presentation-only district type removal. No geographical identity selection."""
from copy import deepcopy
# Explicit administrative type words, not address-specific corrections.
MARKERS=frozenset(['район','района','р-н','ауданы','аудан','tumani','tuman','району','ноҳияи','нохияи','дүүрэг','district'])
def district_name(value):
    if not isinstance(value,str) or not value.strip():return value
    tokens=value.split()
    def marker(t):return t.lower().strip('.,:') in MARKERS
    # Interior words are retained; a marker alone is not a district name.
    left=0;right=len(tokens)
    while left<right and marker(tokens[left]):left+=1
    while right>left and marker(tokens[right-1]):right-=1
    if left==right:return None
    if left==0 and right==len(tokens):return value
    # Preserve exact spelling and internal whitespace of remaining name.
    start=0
    for t in tokens[:left]:start=value.index(t,start)+len(t)
    start=value.index(tokens[left],start)
    end=start
    for t in tokens[left:right]:end=value.index(t,end)+len(t)
    return value[start:end]
def apply(pred):
    out=deepcopy(pred);old=out.get('district');new=district_name(old)
    if new!=old:
        out['district']=new
        out['presentation_evidence']={**out.get('presentation_evidence',{}),'district':{'source_value':old,'output_value':new,'operation':'strip_explicit_boundary_type','identity_selected':False}}
        if new is None:out['warnings']=sorted(set(out.get('warnings',[])+['district_name_missing_after_type']))
    return out
