"""Preparation tests; run in the optional reference tooling environment."""
import json
import gzip
from pathlib import Path
import sqlite3
import tempfile
import unittest

from shapely.geometry import Point, box, LineString
from import_osm import Context, prepare
from build_reference import build
from district_context import DistrictGeometry, is_district


def entity(identity, kind, name, province='', city=''):
    return dict(id=identity, kind=kind, name=name, aliases=[name], parent_id=province if kind=='city' else '',
                province_id=province, city_id=city, district_id='', street_type='', source='KATO:test')


class PreparationTests(unittest.TestCase):
    def test_district_geometry_uses_full_objects_and_keeps_boundaries(self):
        city=entity('c','city','Город','p','c')
        districts=[entity('north','district','Север','p','c'),entity('south','district','Юг','p','c')]
        rows=[entity('p','province','Область'),city,*districts]
        areas=[({'name':'Область','admin_level':'4'},box(-1,-1,21,11)),
               ({'name':'Город','admin_level':'6'},box(0,0,20,10)),
               ({'name':'Север','boundary':'administrative','admin_level':'8'},box(0,0,10,10)),
               ({'name':'Юг','boundary':'administrative','admin_level':'8'},box(10,0,20,10))]
        context=Context({row['id']:row for row in rows},areas)
        geometry=DistrictGeometry(context,areas,lambda tags:[tags['name']])
        self.assertEqual(geometry.evidence('c',Point(5,5)),(['north'],True))
        self.assertEqual(geometry.evidence('c',Point(10,5)),(['north','south'],True))
        self.assertEqual(geometry.evidence('c',LineString([(5,5),(15,5)])),(['north','south'],True))
        self.assertEqual(geometry.evidence('c',box(9,4,11,6)),(['north','south'],True))
        self.assertEqual(geometry.evidence('c',LineString([(5,5),(25,5)])),(['north','south'],False))
        self.assertEqual(geometry.evidence('c',Point(30,5)),([],False))

    def test_urban_kato_admin_adaptation_does_not_promote_councils(self):
        district=entity('d','admin','район Алматы','c','c');district['parent_id']='c'
        self.assertTrue(is_district(district))
        council={**district,'name':'Алматы сельский округ'}
        self.assertFalse(is_district(council))
        district['source']='OSM:test'
        self.assertFalse(is_district(district))

    def test_schema_three_district_validation_and_atomicity(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);source=root/'entities.jsonl';facts=root/'addresses.jsonl';districts=root/'districts.jsonl';output=root/'reference.sqlite'
            city=entity('c','city','Город','p','c')
            district=entity('d','admin','район Учебный','p','c');district['parent_id']='c'
            street=entity('s','street','Учебная','p','c')
            rows=[entity('p','province','Область'),city,district,street]
            source.write_text(''.join(json.dumps(row)+'\n' for row in rows))
            facts.write_text(json.dumps(dict(id='a',street_id='s',city_id='c',house='10/1',source='synthetic'))+'\n')
            row=dict(object_kind='address',object_id='a',city_id='c',district_ids=['d'],complete=True,source='synthetic geometry')
            districts.write_text(json.dumps(row)+'\n');build(source,output)
            with sqlite3.connect(output) as db:
                self.assertEqual(db.execute("SELECT value FROM metadata WHERE key='schema_version'").fetchone()[0],'3')
                self.assertEqual(db.execute('SELECT district_id FROM district_links').fetchone()[0],'d')
            before=output.read_bytes()
            for bad in [{**row,'district_ids':['missing']},{**row,'object_id':'missing'},{**row,'latitude':1},
                        {**row,'complete':1},{**row,'district_ids':[]},{**row,'district_ids':['d','d']}]:
                districts.write_text(json.dumps(bad)+'\n')
                with self.assertRaises((ValueError,sqlite3.IntegrityError)):build(source,output)
                self.assertEqual(output.read_bytes(),before)

    def test_republican_city_boundary_is_not_a_regional_village_homonym(self):
        province=entity('p','province','Область')
        city=entity('capital','city','Алматы','capital','capital');city['parent_id']=''
        village=entity('village','city','Алматы','p','village')
        context=Context({e['id']:e for e in [province,city,village]},[
            ({'name':'Область','admin_level':'4'},box(0,0,10,10)),
            ({'name':'Алматы','admin_level':'4'},box(1,1,3,3))])
        self.assertEqual(context.city(Point(2,2),{}),'capital')
        self.assertEqual(context.city(Point(2,2),{'addr:city':'Алматы'}),'capital')

    def test_import_keeps_address_identity_without_exporting_geometry(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);base=root/'base.jsonl';osm=root/'sample.osm'
            base.write_text(''.join(json.dumps(row)+'\n' for row in [entity('p','province','Область'),entity('c','city','Город','p','c')]))
            osm.write_text('''<osm version="0.6" generator="test">
              <node id="1" lat="0" lon="0"/><node id="2" lat="0" lon="5"/>
              <node id="3" lat="5" lon="5"/><node id="4" lat="5" lon="0"/>
              <node id="5" lat="1" lon="1"/><node id="6" lat="2" lon="2"/>
              <node id="7" lat="1.1" lon="1.1"><tag k="addr:city" v="Город"/>
                <tag k="addr:street" v="Новая"/><tag k="addr:housenumber" v="12/1а"/></node>
              <node id="8" lat="1.2" lon="1.2"><tag k="place" v="quarter"/></node>
              <way id="10"><nd ref="1"/><nd ref="2"/><nd ref="3"/><nd ref="4"/><nd ref="1"/>
                <tag k="boundary" v="administrative"/><tag k="admin_level" v="6"/><tag k="name" v="Город"/></way>
              <way id="11"><nd ref="5"/><nd ref="6"/><tag k="highway" v="residential"/>
                <tag k="name" v="улица Новая"/></way></osm>''')
            output=root/'entities.jsonl';addresses=root/'addresses.jsonl.gz';manifest=root/'manifest.json'
            prepare(base,osm,output,addresses,manifest)
            rows=[json.loads(line) for line in output.read_text().splitlines()]
            streets=[row for row in rows if row['kind']=='street']
            self.assertEqual(len(streets),1)
            fact=json.loads(gzip.decompress(addresses.read_bytes()));self.assertEqual(fact['street_id'],streets[0]['id'])
            self.assertEqual(fact['house'],'12/1а');self.assertNotIn('coordinates',fact)
            self.assertEqual(json.loads(manifest.read_text())['kato']['rows'],2)
            build(output,root/'reference.sqlite',manifest)

    def test_spatial_context_disambiguates_homonyms_and_rejects_conflicting_city(self):
        rows=[entity('p1','province','Первая область'),entity('p2','province','Вторая область'),
              entity('c1','city','Караганда','p1','c1'),entity('c2','city','Караганда','p2','c2')]
        context=Context({e['id']:e for e in rows},[
            ({'name':'Первая область','admin_level':'4'},box(0,0,10,10)),
            ({'name':'Вторая область','admin_level':'4'},box(20,20,30,30)),
            ({'name':'Караганда','admin_level':'6'},box(1,1,3,3)),
            ({'name':'Караганда','admin_level':'6'},box(21,21,23,23))])
        self.assertEqual(context.city(Point(2,2),{}),'c1')
        self.assertEqual(context.city(Point(22,22),{'addr:city':'Караганда'}),'c2')
        self.assertIsNone(context.city(Point(2,2),{'addr:city':'Другой город'}))
        self.assertIsNone(context.city(Point(40,40),{'addr:city':'Караганда'}))

    def test_address_hierarchy_and_atomic_failure(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);source=root/'entities.jsonl';addresses=root/'addresses.jsonl';output=root/'reference.sqlite'
            rows=[entity('c1','city','Первый','','c1'),entity('c2','city','Второй','','c2'),entity('s','street','Новая','','c1')]
            rows[-1]['aliases']=['Новая',' Новая ']
            source.write_text(''.join(json.dumps(row)+'\n' for row in rows))
            fact=dict(id='osm:node:1',street_id='s',city_id='c1',house='12/1а',source='OSM:test')
            addresses.write_text(json.dumps(fact)+'\n');build(source,output)
            with sqlite3.connect(output) as db:
                self.assertEqual(db.execute("SELECT value FROM metadata WHERE key='schema_version'").fetchone()[0],'2')
                self.assertEqual(db.execute('SELECT house FROM addresses').fetchone()[0],'12/1а')
            before=output.read_bytes();fact['city_id']='c2';addresses.write_text(json.dumps(fact)+'\n')
            with self.assertRaises(ValueError):build(source,output)
            fact.pop('latitude',None);fact['house']='№.';addresses.write_text(json.dumps(fact)+'\n')
            with self.assertRaises(ValueError):build(source,output)
            self.assertEqual(output.read_bytes(),before)
            fact['city_id']='c1';fact['latitude']=50;addresses.write_text(json.dumps(fact)+'\n')
            with self.assertRaises(ValueError):build(source,output)


if __name__=='__main__': unittest.main()
