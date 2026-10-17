// Package testutil supplies independent, synthetic reference fixtures.
package testutil

import (
	"database/sql"
	"github.com/lukivan8/jana-address/internal/reference"
	"path/filepath"
	"testing"
)

func Reference(t testing.TB) *reference.Directory {
	t.Helper()
	path := filepath.Join(t.TempDir(), "reference.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE metadata(key TEXT,value TEXT); INSERT INTO metadata VALUES('schema_version','1');
 CREATE TABLE entities(id TEXT,kind TEXT,name TEXT,parent_id TEXT,city_id TEXT,province_id TEXT,district_id TEXT,street_type TEXT,source TEXT);
 CREATE TABLE aliases(entity_id TEXT,alias TEXT);
 INSERT INTO entities VALUES
 ('p1','province','Карагандинская область','','','p1','','','test'),
 ('p2','province','Актюбинская область','','','p2','','','test'),
 ('c1','city','Караганда','p1','c1','p1','','','test'),
 ('c2','city','Караганда','p2','c2','p2','','','test'),
 ('other','city','Астана','p1','other','p1','','','test'),
 ('s1','street','Алалыкина','c1','c1','p1','','улица','test');
 INSERT INTO aliases VALUES('p1','Карагандинская область'),('p2','Актюбинская область'),
 ('c1','Караганда'),('c1','г.Караганда'),('c2','Караганда'),('c2','с.Караганда'),('other','Астана'),('s1','Алалыкина');`)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	d, err := reference.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
