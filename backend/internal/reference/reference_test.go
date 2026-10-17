package reference

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestReadOnlyDirectoryResolvesAliasesAndRelations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reference with spaces.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE metadata(key TEXT PRIMARY KEY,value TEXT); INSERT INTO metadata VALUES('schema_version','1');
 CREATE TABLE entities(id TEXT PRIMARY KEY,kind TEXT,name TEXT,parent_id TEXT,city_id TEXT,province_id TEXT,district_id TEXT,street_type TEXT,source TEXT);
 CREATE TABLE aliases(entity_id TEXT,alias TEXT);
 INSERT INTO entities VALUES('city','city','Астана','','city','city','','','test');
 INSERT INTO entities VALUES('road','street','Туран','city','city','city','','проспект','test');
 INSERT INTO aliases VALUES('city','Астана'),('city','Astana'),('road','Туран'),('road','Turan');`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Count() != 2 || len(d.Exact("turan", "street")) != 1 {
		t.Fatal("aliases not loaded")
	}
	matches := d.Matches("Astana, Turan 55/6")
	if len(matches) != 2 {
		t.Fatalf("matches: %+v", matches)
	}
	road, _ := d.Entity("road")
	if road.CityID != "city" {
		t.Fatal("hierarchy lost")
	}
	missing := filepath.Join(t.TempDir(), "missing.sqlite")
	if _, err = Open(missing); err == nil {
		t.Fatal("missing reference silently created")
	}
}

func TestSettlementTypeRequiresConsistentSourceQualifiers(t *testing.T) {
	for _, tt := range []struct {
		aliases []string
		want    string
	}{
		{[]string{"Караганда", "г.Караганда"}, "город"},
		{[]string{"Караганда", "с.Караганда"}, "село"},
		{[]string{"Караганда"}, ""},
		{[]string{"г.Караганда", "с.Караганда"}, ""},
	} {
		if got := (Entity{Aliases: tt.aliases}).SettlementType(); (got == nil && tt.want != "") || (got != nil && *got != tt.want) {
			t.Fatalf("type from %v: %v", tt.aliases, got)
		}
	}
}
