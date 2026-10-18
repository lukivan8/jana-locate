"""Typed offline candidates with explicit transform provenance; no forced resolution."""
import sqlite3,unicodedata

def key(s):return ' '.join(unicodedata.normalize('NFC',s).casefold().split())
def latin(s):
 letters=[c for c in s if c.isalpha()]
 return bool(letters) and all('LATIN' in unicodedata.name(c,'') for c in letters)

class TieredLookup:
 def __init__(self,ascii_index,preserved_index):
  self.ascii=sqlite3.connect('file:'+str(ascii_index)+'?mode=ro',uri=True,check_same_thread=False)
  self.preserved=sqlite3.connect('file:'+str(preserved_index)+'?mode=ro',uri=True,check_same_thread=False)
 def close(self):self.ascii.close();self.preserved.close()
 def lookup(self,kind,value):
  evidence={}
  def collect(db,tier,exact_only=False):
   sql='SELECT DISTINCT entity_id,source_alias FROM alias WHERE kind=? AND key=?'
   if exact_only:sql+=' AND mode="exact"'
   for identity,alias in db.execute(sql,(kind,key(value))):
    if not exact_only and not any('CYRILLIC' in unicodedata.name(c,'') for c in alias):continue
    evidence.setdefault(identity,[]).append({'tier':tier,'source_alias':alias,'key':key(value)})
  collect(self.ascii,'exact_alias',True)
  if latin(value):
   collect(self.preserved,'transliteration_preserved')
   collect(self.ascii,'transliteration_ascii_lossy')
  order={'exact_alias':0,'transliteration_preserved':1,'transliteration_ascii_lossy':2}
  candidates=[{'entity_id':identity,'best_tier':min((x['tier'] for x in ev),key=order.get),'evidence':ev} for identity,ev in sorted(evidence.items())]
  warnings=[]
  if len(candidates)>1:warnings.append('ambiguous_reference_'+kind)
  if any(x['best_tier']=='transliteration_ascii_lossy' for x in candidates):warnings.append('lossy_transliteration_candidate_'+kind)
  if not candidates:warnings.append('unresolved_reference_'+kind)
  return {'surface':value,'kind':kind,'candidates':candidates,'selected_id':None,'status':'candidate_only','warnings':warnings}
