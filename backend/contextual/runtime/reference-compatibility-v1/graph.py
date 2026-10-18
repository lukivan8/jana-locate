"""Positive metadata relations per parse/occurrence; never select or delete nodes."""
import copy
RELATIONS=[('city','province','province_id'),('district','province','province_id'),('district','city','city_id'),('street_name','city','city_id'),('living_area','city','city_id')]

def graph(record,entities):
 branches=[]
 for b in record['reference_branches']:
  groups=[];refs=b['reference']
  for source_field,target_field,parent_key in RELATIONS:
   for source in refs.get(source_field,[]):
    for target in refs.get(target_field,[]):
     target_ids=target['candidate_ids']
     # Unknown queries are explicit, not evidence that the source location is wrong.
     if not source['candidate_ids'] or not target_ids:
      groups.append({'relation':source_field+'→'+target_field,'source_span':[source['start'],source['end']],'target_span':[target['start'],target['end']],'state':'unresolved_query','source_ids':list(source['candidate_ids']),'target_ids':list(target_ids)})
      continue
     for identity in source['candidate_ids']:
      entity=entities.get(identity);parent=entity.get(parent_key) if entity else None
      compatible=[];conflicting=[];unknown=[]
      for other in target_ids:
       if not parent or parent not in entities or other not in entities:unknown.append(other)
       elif other==parent:compatible.append(other)
       else:conflicting.append(other)
      groups.append({'relation':source_field+'→'+target_field,'source_span':[source['start'],source['end']],'target_span':[target['start'],target['end']],'source_id':identity,'reference_key':parent_key,'reference_value':parent,'source':entity.get('source') if entity else None,'compatible_ids':compatible,'conflicting_ids':conflicting,'unknown_ids':unknown})
  branches.append({'branch':b['branch'],'relations':groups,'selected':False})
 return {'branches':branches,'selected_hypothesis':None,'scope':'pairwise snapshot metadata, conditional on extracted roles; not proof of complete address validity','absence_policy':'unresolved query is unknown; never excludes a settlement'}

def apply(record,entities):
 out=copy.deepcopy(record);out['reference_compatibility']=graph(record,entities);return out
