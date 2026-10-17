package app

import (
	"context"
	"errors"
	"os"
	"testing"

	"example.com/jana-locate-cli/internal/api"
	"github.com/jackc/pgx/v5"
)

func TestPostgresSnapshotPublicationAndStaleSource(t *testing.T) {
	dsn := os.Getenv("JANA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set JANA_TEST_DATABASE_URL to a disposable PostgreSQL database")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	defer conn.Close(ctx)
	schema := "test_" + id()
	if _, err = conn.Exec(ctx, "CREATE SCHEMA "+qi(schema)); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec(ctx, "DROP SCHEMA "+qi(schema)+" CASCADE")
	_, err = conn.Exec(ctx, "CREATE TABLE "+qi(schema, "source")+"(id bigint PRIMARY KEY,address text); INSERT INTO "+qi(schema, "source")+" VALUES(9223372036854775807,E'  raw\\nstring  '),(2,NULL),(3,'same'),(4,'same')")
	if err != nil {
		t.Fatal(err)
	}
	s := testStore(t)
	p := Postgres{DSN: dsn, APIURL: "http://normalizer.invalid"}
	src := api.Source{Schema: schema, Table: "source", Key: "id", Column: "address", TargetSchema: schema, TargetTable: "normalized"}
	run, err := p.Import(ctx, s, src)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := s.Rows(ctx, run.Id, 0, 100, false, true)
	for i, r := range rows {
		rows[i].Result = emptyResult(r.Position, r.RawAddress)
		rows[i].Result.Id = r.SourceId
	}
	if err = s.SaveBatch(ctx, run.Id, rows); err != nil {
		t.Fatal(err)
	}
	// Primary-key order puts the large ID last. Correct one record and publish all.
	rows, _ = s.Rows(ctx, run.Id, 0, 50, false, false)
	if _, err = s.Review(ctx, run.Id, rows[3].Position, rows[3].Revision, api.AddressFields{Apartment: ptr("77")}); err != nil {
		t.Fatal(err)
	}
	summary, err := s.Summary(ctx, run.Id)
	if err != nil || summary.Automatic != 0 || summary.Reviewed != 1 || summary.NeedsReview != 3 || summary.Pending != 0 {
		t.Fatalf("whole-run summary: %+v %v", summary, err)
	}
	run, _ = s.Get(ctx, run.Id)
	published, err := p.Publish(ctx, s, run.Id, run.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Publish(ctx, s, run.Id, published.Revision); err != nil {
		t.Fatal("repeat publish failed", err)
	}
	final, err := p.PublishedTable(ctx, s, run.Id, 0)
	if err != nil || final.Total != 4 || len(final.Rows) != 4 || final.Rows[3][1] != "9223372036854775807" || final.Rows[3][10] != "77" {
		t.Fatalf("final database table: %+v %v", final, err)
	}
	// The final screen must reflect committed database data, never a journal copy.
	if _, err = conn.Exec(ctx, "UPDATE "+qi(schema, "normalized")+" SET apartment='88' WHERE source_id=9223372036854775807"); err != nil {
		t.Fatal(err)
	}
	final, err = p.PublishedTable(ctx, s, run.Id, 3)
	if err != nil || len(final.Rows) != 1 || final.Rows[0][10] != "88" {
		t.Fatalf("database reread: %+v %v", final, err)
	}
	conn.Exec(ctx, "UPDATE "+qi(schema, "normalized")+" SET apartment='77' WHERE source_id=9223372036854775807")
	var count int
	var apartment, typ string
	if err = conn.QueryRow(ctx, "SELECT count(*) FROM "+qi(schema, "normalized")).Scan(&count); err != nil || count != 4 {
		t.Fatal("publication duplicates or missing rows", count, err)
	}
	if err = conn.QueryRow(ctx, "SELECT apartment,pg_typeof(source_id)::text FROM "+qi(schema, "normalized")+" WHERE source_id=9223372036854775807").Scan(&apartment, &typ); err != nil || apartment != "77" || typ != "bigint" {
		t.Fatal("PK type or saved edit lost", apartment, typ, err)
	}
	var raw *string
	if err = conn.QueryRow(ctx, "SELECT raw_address FROM "+qi(schema, "normalized")+" WHERE source_id=2").Scan(&raw); err != nil || raw != nil {
		t.Fatal("NULL source lost")
	}
	second, err := p.Import(ctx, s, src)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ = s.Rows(ctx, second.Id, 0, 100, false, true)
	for i, r := range rows {
		rows[i].Result = emptyResult(r.Position, r.RawAddress)
	}
	if err = s.SaveBatch(ctx, second.Id, rows); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, "UPDATE "+qi(schema, "source")+" SET address='changed' WHERE id=3"); err != nil {
		t.Fatal(err)
	}
	second, _ = s.Get(ctx, second.Id)
	if _, err = p.Publish(ctx, s, second.Id, second.Revision); !errors.Is(err, ErrSourceChanged) {
		t.Fatal("stale source published", err)
	}
	if err = conn.QueryRow(ctx, "SELECT count(*) FROM "+qi(schema, "normalized")).Scan(&count); err != nil || count != 4 {
		t.Fatal("failed publish changed prior results")
	}
	// Reject a composite key before importing any run.
	_, err = conn.Exec(ctx, "CREATE TABLE "+qi(schema, "composite")+"(id bigint,part int,address text,PRIMARY KEY(id,part))")
	if err != nil {
		t.Fatal(err)
	}
	src.Table = "composite"
	if _, err = p.Import(ctx, s, src); err == nil {
		t.Fatal("composite PK accepted")
	}
}

func TestDatabaseCatalogPrioritizesAddressNames(t *testing.T) {
	dsn := os.Getenv("JANA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx := context.Background()
	c, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	schema := "catalog_" + id()
	q := qi(schema)
	_, err = c.Exec(ctx, "CREATE SCHEMA "+q+";CREATE TABLE "+q+".orders(id bigint PRIMARY KEY,notes text,delivery_address text,amount numeric);CREATE TABLE "+q+".address_book(id uuid PRIMARY KEY,address text);CREATE TABLE "+q+".composite(id int,part int,address text,PRIMARY KEY(id,part))")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Exec(ctx, "DROP SCHEMA "+q+" CASCADE")
	_, err = (Postgres{DSN: dsn}).Check(ctx, api.Source{Schema: schema, Table: "orders", Key: "id", Column: "delivery_address", TargetSchema: schema, TargetTable: "address_book"})
	if err == nil || err.Error() != "Таблица результата уже существует и не принадлежит этому источнику Jana. Укажите другое имя таблицы результата" {
		t.Fatalf("target conflict must be actionable Russian text: %v", err)
	}
	catalog, err := (Postgres{DSN: dsn}).Catalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, table := range catalog.Tables {
		if table.Schema != schema {
			continue
		}
		names = append(names, table.Name)
		if table.Name == "orders" && (table.Key != "id" || len(table.Columns) != 2 || table.Columns[0] != "delivery_address") {
			t.Fatalf("wrong choices: %+v", table)
		}
	}
	if len(names) != 2 || names[0] != "address_book" || names[1] != "orders" {
		t.Fatalf("wrong table ordering/filter: %v", names)
	}
}
