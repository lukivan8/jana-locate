package normalize

import (
	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/pkg/api"
	"testing"
)

func TestExplicitComponentsIndependentFamilies(t *testing.T) {
	for _, raw := range []string{
		"Тараз, Неизвестная 73; 41 пәтер; 2 кіреберіс; 4 қабат",
		"Тараз, Неизвестная 73, квартира 41, 2 подъезд, 4 этаж",
		"Тараз, Неизвестная 73, kvartira 41, podezd 2, etazh 4",
		"Тараз, Неизвестная 73, 41 xonadon, 2 kirish, 4 qavat",
		"Тараз, Неизвестная 73, пәтер 41, 2 кіреберс, 4-қабат",
		"Тараз, Неизвестная 73, кв41, 2pod, 4etaj",
	} {
		t.Run(raw, func(t *testing.T) {
			b := api.Result{RawAddress: raw, House: str("73"), Confidence: .8, Status: api.NeedsReview}
			r := supplementExplicitComponents(b)
			if catalog.Text(r.Apartment) != "41" || catalog.Text(r.Entrance) != "2" || catalog.Text(r.Floor) != "4" || catalog.Text(r.House) != "73" {
				t.Fatalf("unexpected fields: %+v", resultValues(r))
			}
			if r.Confidence != b.Confidence || r.Status != b.Status || r.RawAddress != raw {
				t.Fatal("baseline evidence changed")
			}
		})
	}
}
func TestExplicitComponentsRejectProseAndNegativeOwnership(t *testing.T) {
	for _, raw := range []string{"магазин Квартира 24", "позвонить после 18:00", "доставка на этаж 4", "улица 8 Марта", "floor -1", "-1 этаж", "кафе Подъезд 5", "20 кв 4", "рядом со школой 17"} {
		r := supplementExplicitComponents(api.Result{RawAddress: raw})
		if r.House != nil || r.Apartment != nil || r.Entrance != nil || r.Floor != nil {
			t.Fatalf("prose became numbers: %s %+v", raw, resultValues(r))
		}
	}
	for _, field := range []string{"street", "landmark"} {
		b := api.Result{RawAddress: "Тараз, этаж 4"}
		if field == "street" {
			b.StreetName = str("этаж 4")
		} else {
			b.Landmark = str("этаж 4")
		}
		if r := supplementExplicitComponents(b); r.Floor != nil {
			t.Fatalf("protected %s mined", field)
		}
	}
}
func TestExplicitComponentsAbstainOnCompetingFacts(t *testing.T) {
	r := supplementExplicitComponents(api.Result{RawAddress: "Тараз, этаж 2, этаж 4", Status: api.Matched})
	if r.Floor != nil || !hasWarning(r.Warnings, "conflicting_explicit_floor") || r.Status != api.NeedsReview {
		t.Fatalf("conflict hidden: %+v", r)
	}
	r = supplementExplicitComponents(api.Result{RawAddress: "Тараз, квартира 41", Apartment: str("99")})
	if catalog.Text(r.Apartment) != "99" {
		t.Fatal("existing value overwritten")
	}
}
func TestExplicitHouseCorpusSeparation(t *testing.T) {
	for _, marker := range []string{"корп", "корп.", "корпус", "korpus"} {
		raw := "Тараз, Неизвестная 73 " + marker + " 2, квартира 41"
		r := supplementExplicitComponents(api.Result{RawAddress: raw, House: str("2")})
		if catalog.Text(r.House) != "73" {
			t.Fatalf("house lost: %+v", r)
		}
		r = supplementExplicitComponents(api.Result{RawAddress: raw, House: str("99")})
		if catalog.Text(r.House) != "99" {
			t.Fatal("unrelated house overwritten")
		}
	}
}
