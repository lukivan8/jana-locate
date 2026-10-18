"""Reuse existing offline reference layers for each component/parse branch."""
import json,sys
from pathlib import Path
G=Path(__file__).resolve().parent.parent
sys.path[:0]=[str(G/'reference-candidates'),str(G/'reference-transliteration'),str(G/'settlement-alias-overlay-v1')]
from resolver import Resolver
from tiered_lookup import TieredLookup
from overlay import Overlay
class FullLookup:
 def __init__(self):
  self.legacy=Resolver([json.loads(s) for s in (G/'span-crf/reference-entities.jsonl').read_text().splitlines()])
  self.tiered=TieredLookup(G/'reference-transliteration/reference-index.sqlite',G/'reference-transliteration/preserve-reference-index.sqlite')
  self.overlay=Overlay(G/'settlement-alias-quality-v1/aliases-screened.jsonl')
 def close(self):self.tiered.close()
 def lookup(self,kind,value):
  tiers=self.tiered.lookup(kind,value);ids={c['entity_id'] for c in tiers['candidates']};legacy=[];alias=[];warnings=list(tiers['warnings'])
  if kind in ['city','province']:
   old=self.legacy.resolve({'warnings':[],'extracted_values':{kind:[value]}});ids.update(old['candidate_ids'][kind]);legacy=old['reference_evidence'][kind]
  if kind=='city':
   extra=self.overlay.lookup(kind,value,ids);ids.update(extra['candidate_ids']);alias=extra['evidence'];warnings+=extra['warnings']
  return {'surface':value,'kind':kind,'candidates':[{'entity_id':i} for i in sorted(ids)],'selected_id':None,'provenance':{'legacy':legacy,'tiered':tiers,'settlement_alias':alias},'warnings':sorted(set(warnings))}
