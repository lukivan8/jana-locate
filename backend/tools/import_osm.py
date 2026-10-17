"""Prepare nationwide OSM components/address evidence from a local PBF.

Preparation only. Geometry resolves administrative context but is never exported
as building coordinates. Unresolved identities are counted and omitted.
"""
import argparse
from collections import Counter, defaultdict
from contextlib import ExitStack
from datetime import datetime, timezone
import hashlib
import gzip
import io
import json
from pathlib import Path
import re

import osmium
from shapely import from_wkb
from shapely.geometry import Point
from shapely.strtree import STRtree
from district_context import DistrictGeometry


def canon(value):
    return ' '.join(re.sub(r'[^\w/\-]+', ' ', value.casefold()).split())


def locality(value):
    return re.sub(r'\s+(?:қ\.|а\.|к\.)$', '', re.sub(
        r'^(?:г\.|с\.|п\.|город\s+|село\s+|пос[её]лок\s+)\s*', '', value, flags=re.I)).strip()


STREET = re.compile(r'^(улица|ул\.?|проспект|пр\.?|пр-т|переулок|пер\.?|проезд|бульвар|шоссе)\s+|\s+(көшесі|даңғылы)$', re.I)
TYPES = {'улица':'улица','ул':'улица','проспект':'проспект','пр':'проспект','пр-т':'проспект',
         'переулок':'переулок','пер':'переулок','проезд':'проезд','бульвар':'бульвар','шоссе':'шоссе',
         'көшесі':'улица','даңғылы':'проспект'}


def names(tags):
    out = []
    for key in ['name:ru','name','name:kk','name:en','alt_name','old_name','short_name']:
        out.extend(v.strip() for v in tags.get(key, '').split(';') if v.strip())
    return list(dict.fromkeys(out))


def street_type(value):
    m = STREET.search(value)
    return TYPES.get((m.group(1) or m.group(2)).lower().rstrip('.'), '') if m else ''


def usable_house(value):
    return bool(value) and len(value.encode()) <= 256 and any(c.isalnum() for c in value)


class Context:
    def __init__(self, entities, areas):
        self.entities = entities
        self.aliases = defaultdict(set)
        for entity in entities.values():
            for value in [entity['name'], *entity['aliases']]:
                self.aliases[canon(locality(value))].add(entity['id'])
        self.provinces = []
        for tags, geometry in areas:
            if tags.get('admin_level') != '4':
                continue
            ids = self.find(names(tags), 'province')
            if len(ids) == 1:
                self.provinces.append((next(iter(ids)), geometry))
        self.province_tree = STRtree([g for _, g in self.provinces])
        self.cities = []
        for tags, geometry in areas:
            if tags.get('place') not in {'city','town','village','hamlet'} and tags.get('admin_level') not in {'4','6','8'}:
                continue
            ids = self.find(names(tags), 'city')
            roots = {i for i in ids if entities[i]['parent_id']=='' and entities[i]['province_id']==i}
            if tags.get('admin_level')=='4' and len(roots)==1:
                ids=roots
            provinces = self.province_ids(geometry.representative_point())
            if provinces:
                ids = {i for i in ids if entities[i]['province_id'] in provinces or entities[i]['parent_id']==''}
            if len(ids) == 1:
                self.cities.append((next(iter(ids)), geometry))
        self.city_tree = STRtree([g for _, g in self.cities])

    def find(self, values, kind):
        return {i for value in values for i in self.aliases[canon(locality(value))]
                if not kind or self.entities[i]['kind'] == kind}

    def province_ids(self, point):
        return {self.provinces[i][0] for i in self.province_tree.query(point, predicate='covered_by')}

    def city(self, point, tags):
        hits = [self.cities[i] for i in self.city_tree.query(point, predicate='covered_by')]
        if tags.get('addr:city'):
            explicit = self.find([tags['addr:city']], 'city')
            provinces = self.province_ids(point)
            if not provinces and not hits:
                return None
            if provinces:
                explicit = {i for i in explicit if self.entities[i]['province_id'] in provinces or
                            self.entities[i]['parent_id']=='' and i in {j for j,_ in hits}}
            if hits:
                explicit &= {i for i, _ in hits}
            return next(iter(explicit)) if len(explicit) == 1 else None
        # Nested settlement polygons can refine an enclosing administration.
        # Overlapping unrelated identities remain unresolved.
        specific = [(i,g) for i,g in hits if not any(i != j and g.contains(h) for j,h in hits)]
        ids = {i for i,_ in specific}
        return next(iter(ids)) if len(ids) == 1 else None


def digest(path):
    with Path(path).open('rb') as file:
        return hashlib.file_digest(file, 'sha256').hexdigest()


def prepare(base, pbf, output, addresses, manifest, districts=None):
    original = [json.loads(line) for line in Path(base).read_text().splitlines() if line.strip()]
    entities = {e['id']: e for e in original if e['source'].startswith('KATO:')}
    if not entities:
        raise ValueError('A prepared KATO base is required')
    kato_count = len(entities)
    base_hash = digest(base)
    factory = osmium.geom.WKBFactory()
    stats = Counter()
    areas = []
    class Boundaries(osmium.SimpleHandler):
        def area(self, area):
            tags = dict(area.tags)
            if not names(tags) or not (tags.get('boundary') == 'administrative' or tags.get('place') in {'city','town','village','hamlet'}):
                return
            try:
                geometry = from_wkb(bytes.fromhex(factory.create_multipolygon(area)))
                if not geometry.is_valid or geometry.is_empty:
                    stats['invalid_boundaries'] += 1
                    return
                areas.append((tags, geometry))
            except (RuntimeError, ValueError):
                stats['invalid_boundaries'] += 1
    Boundaries().apply_file(str(pbf), locations=True, filters=[osmium.filter.KeyFilter('boundary','place')])
    context = Context(entities, areas)
    district_context = DistrictGeometry(context, areas, names)
    if not context.cities:
        raise ValueError('No OSM settlement boundaries linked unambiguously to KATO')
    print(f'Linked {len(context.cities)} settlement polygons; reading streets/addresses', flush=True)
    facts = {}
    address_districts = {}
    road_districts = {}
    incomplete_road_names = set()

    def block_road(tags):
        if tags.get('highway'):
            incomplete_road_names.update(canon(STREET.sub('', value)) for value in names(tags))

    def district_row(kind, identity, city_id, geometry):
        ids, complete = district_context.evidence(city_id, geometry)
        return dict(object_kind=kind,object_id=identity,city_id=city_id,district_ids=ids,
                    complete=bool(complete),source='OSM:Kazakhstan-PBF:district-geometry')

    def add_name(city_id, values, kind):
        name = STREET.sub('', values[0]).strip() if kind == 'street' else values[0].strip()
        if not name:
            return None
        typ = street_type(values[0]) if kind == 'street' else ''
        identity = 'osm:' + city_id + ':' + kind + ':' + hashlib.sha256((typ+'|'+canon(name)).encode()).hexdigest()[:20]
        aliases = {v for value in values for v in [value, STREET.sub('', value).strip()] if v}
        if identity in entities:
            entities[identity]['aliases'] = sorted(set(entities[identity]['aliases']) | aliases)
        else:
            city = entities[city_id]
            entities[identity] = dict(id=identity, kind=kind, name=name, parent_id=city_id, city_id=city_id,
                                      province_id=city['province_id'], district_id='', street_type=typ,
                                      source='OSM:Kazakhstan-PBF', aliases=sorted(aliases))
        return identity

    def consume(tags, point, osm_id, geometry=None):
        address_name = tags.get('addr:street') or tags.get('addr:place')
        road = bool(tags.get('highway')) and bool(names(tags))
        residential = bool(names(tags)) and (tags.get('place') in {'suburb','neighbourhood','quarter'} or bool(tags.get('residential')))
        if not (address_name or road or residential):
            return
        stats['eligible_objects'] += 1
        city_id = context.city(point, tags)
        if not city_id:
            block_road(tags)
            stats['unresolved_city_objects'] += 1
            return
        if road or residential:
            identity = add_name(city_id, names(tags), 'street' if road else 'living_area')
            if road and identity:
                row=district_row('street',identity,city_id,geometry)
                previous=road_districts.get(identity)
                if previous:
                    row['district_ids']=sorted(set(previous['district_ids'])|set(row['district_ids']))
                    row['complete']=row['complete'] and previous['complete']
                road_districts[identity]=row
        if address_name:
            kind = 'street' if tags.get('addr:street') else 'living_area'
            street_id = add_name(city_id, [address_name], kind)
            house = tags.get('addr:housenumber', '').strip()
            # Keep identifiers exactly; never interpolate ranges or strip fractions.
            if house and not usable_house(house):
                stats['invalid_house_objects'] += 1
            if street_id and usable_house(house):
                facts[osm_id] = dict(id='osm:'+osm_id, street_id=street_id, city_id=city_id,
                                     house=house, source='OSM:Kazakhstan-PBF')
                address_districts[osm_id]=district_row('address','osm:'+osm_id,city_id,geometry if geometry is not None else point)
                stats['address_objects'] += 1
    class Objects(osmium.SimpleHandler):
        def node(self, node):
            if not (node.tags.get('addr:street') or node.tags.get('addr:place') or
                    node.tags.get('place') in {'suburb','neighbourhood','quarter'} or
                    node.tags.get('residential') and node.tags.get('name')):
                return
            tags = dict(node.tags)
            if node.location.valid():
                point=Point(node.location.lon,node.location.lat)
                consume(tags, point, 'node:'+str(node.id), point)
        def way(self, way):
            tags = dict(way.tags)
            if not (tags.get('highway') and names(tags) or tags.get('addr:street') or tags.get('addr:place') or tags.get('place') in {'suburb','neighbourhood','quarter'} or tags.get('residential') and names(tags)):
                return
            try:
                geometry = from_wkb(bytes.fromhex(factory.create_linestring(way)))
                district_geometry=geometry
                if tags.get('addr:housenumber') and not tags.get('highway') and geometry.is_ring:
                    from shapely.geometry import Polygon
                    district_geometry=Polygon(geometry.coords)
                consume(tags, geometry.interpolate(.5, normalized=True), 'way:'+str(way.id), district_geometry)
            except (RuntimeError, ValueError):
                block_road(tags)
                stats['invalid_object_geometries'] += 1
    Objects().apply_file(str(pbf), locations=True, filters=[osmium.filter.KeyFilter('highway','addr:street','addr:place','place','residential')])
    if not facts:
        raise ValueError('No address evidence; refusing to publish an empty import')
    # Address tags often omit a road type or use a bilingual alias. Merge only
    # into a single positively identified typed street in that same settlement.
    typed = defaultdict(set)
    for entity in entities.values():
        if entity['kind'] == 'street' and entity['street_type']:
            for alias in entity['aliases']:
                typed[(entity['city_id'], canon(STREET.sub('', alias)))].add(entity['id'])
    remap = {}
    for identity, entity in list(entities.items()):
        if entity['kind'] != 'street' or entity['street_type']:
            continue
        matches = {i for alias in entity['aliases'] for i in typed[(entity['city_id'],canon(STREET.sub('',alias)))]}
        if len(matches) == 1:
            target = next(iter(matches))
            entities[target]['aliases'] = sorted(set(entities[target]['aliases']) | set(entity['aliases']))
            remap[identity] = target
            del entities[identity]
    for fact in facts.values():
        fact['street_id'] = remap.get(fact['street_id'], fact['street_id'])
    merged_roads={}
    for identity,row in road_districts.items():
        identity=remap.get(identity,identity);row['object_id']=identity
        old=merged_roads.get(identity)
        if old:
            row['district_ids']=sorted(set(row['district_ids'])|set(old['district_ids']))
            row['complete']=row['complete'] and old['complete']
        if any(canon(STREET.sub('',alias)) in incomplete_road_names for alias in entities[identity]['aliases']):
            row['complete']=False
        merged_roads[identity]=row
    district_rows=[row for row in [*merged_roads.values(),*address_districts.values()] if row['district_ids']]
    districts=Path(districts) if districts else Path(output).with_name('districts.jsonl.gz')
    for target, rows in [(output, entities.values()), (addresses, facts.values()),(districts,district_rows)]:
        path = Path(target); path.parent.mkdir(parents=True, exist_ok=True)
        temporary = path.with_suffix(path.suffix+'.tmp')
        with ExitStack() as stack:
            if path.suffix=='.gz':
                raw=stack.enter_context(temporary.open('wb'))
                packed=stack.enter_context(gzip.GzipFile(filename='',mode='wb',fileobj=raw,mtime=0))
                file=stack.enter_context(io.TextIOWrapper(packed,encoding='utf-8'))
            else:
                file=stack.enter_context(temporary.open('w',encoding='utf-8'))
            for row in sorted(rows, key=lambda e:e.get('id',e.get('object_kind','')+e.get('object_id',''))):
                file.write(json.dumps(row, ensure_ascii=False, separators=(',',':'))+'\n')
        temporary.replace(path)
    with osmium.io.Reader(str(pbf)) as reader:
        timestamp = reader.header().get('osmosis_replication_timestamp')
    counts = Counter(e['kind'] for e in entities.values())
    report = dict(prepared_at=datetime.now(timezone.utc).isoformat(),
                  kato=dict(source='retained from prepared KATO base', rows=kato_count, base_sha256=base_hash),
                  osm=dict(url='https://download.geofabrik.de/asia/kazakhstan-latest.osm.pbf',
                           scope='Kazakhstan extract; only settlements linked unambiguously to KATO are included; coverage is partial',
                           timestamp=timestamp, sha256=digest(pbf), bytes=Path(pbf).stat().st_size,
                           attribution='© OpenStreetMap contributors', license='ODbL-1.0', license_url='https://www.openstreetmap.org/copyright',
                           settlement_polygons=len(context.cities), cities_with_streets=len({e['city_id'] for e in entities.values() if e['kind']=='street'}),
                           statistics=dict(stats)),
                  entity_count=len(entities), entity_kinds=dict(counts), jsonl_sha256=digest(output),
                  address_count=len(facts), addresses_sha256=digest(addresses), building_coordinates=False,
                  districts_sha256=digest(districts),district_object_count=len(district_rows),
                  district_geometry=dict(linked_polygons=len(district_context.polygons),unresolved_polygons=district_context.unresolved,
                    address_objects=sum(row['object_kind']=='address' for row in district_rows),
                    street_objects=sum(row['object_kind']=='street' for row in district_rows),
                    complete_objects=sum(row['complete'] for row in district_rows),
                    ambiguous_objects=sum(len(row['district_ids'])>1 for row in district_rows)))
    Path(manifest).write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')
    print(json.dumps({k:report[k] for k in ['entity_count','entity_kinds','address_count']}, ensure_ascii=False))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(__doc__)
    parser.add_argument('--base', required=True)
    parser.add_argument('--pbf', required=True)
    parser.add_argument('--output', required=True)
    parser.add_argument('--addresses', required=True)
    parser.add_argument('--manifest', required=True)
    parser.add_argument('--districts')
    args = parser.parse_args()
    prepare(args.base, args.pbf, args.output, args.addresses, args.manifest,args.districts)
