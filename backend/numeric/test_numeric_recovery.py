import unittest
from numeric_recovery import tokenize,features,predict
class FeaturesTest(unittest.TestCase):
 def test_numeric_values_are_masked(self):
  self.assertEqual(features(tokenize('дом 12 кв34')),features(tokenize('дом 98 кв76')))
 def test_marker_attachment_is_observable(self):
  self.assertNotEqual(features(tokenize('2подъезд 3этаж')),features(tokenize('2 подъезд3 этаж')))
 def test_empty_input(self):
  self.assertEqual(predict(None,{'raw_address':''}),[])
 def test_unicode_source_offsets(self):
  raw='Қарағанды, кв12\n3қабат'
  for a,b,v in tokenize(raw):self.assertEqual(raw[a:b],v)
if __name__=='__main__':unittest.main()
