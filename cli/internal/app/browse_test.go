package app

import (
	"context"
	"testing"

	"example.com/jana-locate-cli/internal/api"
)

func TestBrowseUnicodeLiteralSearchAndStablePages(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	run := testImport(t, s, "http://normalizer.invalid", []sourceRow{{"1", ptr("АЛМАТЫ, дом 1")}, {"2", ptr("алматы, дом 2")}, {"3", ptr("Караганда, дом 3")}, {"4", ptr("100%_literal")}})
	rows, _ := s.Rows(ctx, run.Id, 0, 100, false, true)
	for i := range rows {
		rows[i].Result = emptyResult(i, rows[i].RawAddress)
		rows[i].Result.City = ptr("Караганда")
		rows[i].Result.Province = ptr("Область")
		if i == 0 {
			rows[i].Result.Status = api.Matched
		}
	}
	if err := s.SaveBatch(ctx, run.Id, rows); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Get(ctx, run.Id)
	for _, query := range []string{"аЛмАтЫ", "АЛМАТЫ"} {
		found, total, err := s.Browse(ctx, run.Id, BrowseOptions{Limit: 1, Offset: 1, Query: query})
		if err != nil || total != 2 || len(found) != 1 || found[0].SourceId != "2" {
			t.Fatalf("Unicode page: %+v %d %v", found, total, err)
		}
	}
	found, total, err := s.Browse(ctx, run.Id, BrowseOptions{Limit: 20, Query: "Алматы", ReviewOnly: true})
	if err != nil || total != 1 || found[0].SourceId != "2" {
		t.Fatalf("filter: %+v %d %v", found, total, err)
	}
	_, total, err = s.Browse(ctx, run.Id, BrowseOptions{Limit: 20, Query: "карагандА"})
	if err != nil || total != 4 {
		t.Fatalf("region search: %d %v", total, err)
	}
	for _, query := range []string{"%_", "%' OR 1=1 --"} {
		_, total, err = s.Browse(ctx, run.Id, BrowseOptions{Limit: 20, Query: query})
		want := 0
		if query == "%_" {
			want = 1
		}
		if err != nil || total != want {
			t.Fatalf("literal query %q: %d %v", query, total, err)
		}
	}
	after, _ := s.Get(ctx, run.Id)
	if before.Revision != after.Revision {
		t.Fatal("browse modified the journal")
	}
}
