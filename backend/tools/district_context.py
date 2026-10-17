"""Source-role adaptation and build-time district geometry, never address parsing."""

def is_district(entity):
    return entity['kind'] == 'district' or (
        entity['kind'] == 'admin' and entity['source'].startswith('KATO:')
        and entity['city_id'] and entity['parent_id'] == entity['city_id']
        and entity['name'].casefold().startswith('район '))


class DistrictGeometry:
    def __init__(self, context, areas, names):
        from shapely.strtree import STRtree
        self.context = context
        self.polygons = []
        self.unresolved = 0
        for tags, geometry in areas:
            if tags.get('boundary') != 'administrative':
                continue
            ids = {identity for identity in context.find(names(tags), '')
                   if is_district(context.entities[identity])}
            if not ids:
                continue
            point = geometry.representative_point()
            provinces = context.province_ids(point)
            city_hits = {context.cities[index][0] for index in context.city_tree.query(point, predicate='covered_by')}
            ids = {identity for identity in ids
                   if (not provinces or context.entities[identity]['province_id'] in provinces)
                   and (not context.entities[identity]['city_id'] or context.entities[identity]['city_id'] in city_hits)}
            if len(ids) == 1:
                self.polygons.append((next(iter(ids)), geometry))
            else:
                self.unresolved += 1
        self.tree = STRtree([geometry for _, geometry in self.polygons])

    def evidence(self, city_id, geometry):
        from shapely.ops import unary_union
        if geometry is None or geometry.is_empty or not geometry.is_valid:
            return [], False
        city = self.context.entities[city_id]
        hits = []
        for index in self.tree.query(geometry, predicate='intersects'):
            identity, polygon = self.polygons[index]
            entity = self.context.entities[identity]
            if entity['province_id'] != city['province_id']:
                continue
            if entity['city_id'] and entity['city_id'] != city_id:
                continue
            if not entity['city_id'] and identity != city['district_id']:
                continue
            hits.append((identity, polygon, bool(entity['city_id'])))
        # Urban subdivisions and enclosing rural districts are different levels.
        if any(urban for _, _, urban in hits):
            hits = [(identity, polygon, urban) for identity, polygon, urban in hits if urban]
        identities = sorted({identity for identity, _, _ in hits})
        complete = bool(hits) and unary_union([polygon for _, polygon, _ in hits]).covers(geometry)
        return identities, complete
