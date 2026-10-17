package reference

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// IsDistrict adapts explicit KATO urban district records without changing their
// original kind. Rural councils and other admin entities are not districts.
func (e Entity) IsDistrict() bool {
	return e.Kind == "district" || e.Kind == "admin" && strings.HasPrefix(e.Source, "KATO:") &&
		e.CityID != "" && e.ParentID == e.CityID && strings.HasPrefix(Canon(e.Name), "район ")
}

type districtEvidence struct {
	IDs      []string
	Complete bool
}

func (v *districtEvidence) add(id string) {
	if id == "" {
		return
	}
	for _, old := range v.IDs {
		if old == id {
			return
		}
	}
	v.IDs = append(v.IDs, id)
}

// DistrictEvidence represents identities, not a vote for the most frequent name.
// Complete describes the observed object's coverage, not nationwide completeness.
type DistrictEvidence struct {
	Candidates []Entity
	Complete   bool
	Source     string
}

func (d *Directory) validDistrict(cityID, districtID string) bool {
	district, ok := d.Entity(districtID)
	city, cityOK := d.Entity(cityID)
	return ok && cityOK && city.Kind == "city" && district.IsDistrict() &&
		(district.CityID == cityID || district.CityID == "" && city.DistrictID == districtID) && district.ProvinceID == city.ProvinceID
}

// DistrictContainsSettlement is positive administrative hierarchy evidence.
func (d *Directory) DistrictContainsSettlement(cityID, districtID string) bool {
	return d.validDistrict(cityID, districtID)
}

func (d *Directory) loadDistricts(db *sql.DB) error {
	var invalid int
	err := db.QueryRow(`SELECT COUNT(*) FROM district_objects o
 LEFT JOIN addresses a ON o.object_kind='address' AND a.id=o.object_id AND a.city_id=o.city_id
 WHERE (o.object_kind='address' AND a.id IS NULL)
 OR (o.complete=1 AND NOT EXISTS(SELECT 1 FROM district_links l WHERE l.object_kind=o.object_kind AND l.object_id=o.object_id))`).Scan(&invalid)
	if err != nil {
		return err
	}
	if invalid > 0 {
		return fmt.Errorf("invalid district coverage objects")
	}
	err = db.QueryRow(`SELECT COUNT(*) FROM district_links l LEFT JOIN district_objects o
 ON o.object_kind=l.object_kind AND o.object_id=l.object_id WHERE o.object_id IS NULL`).Scan(&invalid)
	if err != nil {
		return err
	}
	if invalid > 0 {
		return fmt.Errorf("dangling district coverage links")
	}
	// Reject dangling/incompatible relations even when supplied by a snapshot
	// producer other than our builder. No per-request database access remains.
	rows, err := db.Query(`SELECT o.object_kind,o.object_id,o.city_id,o.complete,COALESCE(l.district_id,'')
 FROM district_objects o LEFT JOIN district_links l ON l.object_kind=o.object_kind AND l.object_id=o.object_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, id, cityID, districtID string
		var complete bool
		if err = rows.Scan(&kind, &id, &cityID, &complete, &districtID); err != nil {
			return err
		}
		if districtID != "" && !d.validDistrict(cityID, districtID) {
			return fmt.Errorf("invalid district relation %s/%s", id, districtID)
		}
		if kind == "address" {
			continue
		}
		street, ok := d.Entity(id)
		if kind != "street" || !ok || (street.Kind != "street" && street.Kind != "living_area") || street.CityID != cityID {
			return fmt.Errorf("invalid district object %s/%s", kind, id)
		}
		v := d.streetDistricts[id]
		v.Complete = complete
		v.add(districtID)
		d.streetDistricts[id] = v
	}
	return rows.Err()
}

// DistrictsForAddress requires an already resolved settlement identity. Exact
// house evidence wins over whole-street geometry; missing houses are never
// interpolated. A partial sample of houses cannot establish a street's district.
func (d *Directory) DistrictsForAddress(cityID, street, house, streetType string) DistrictEvidence {
	out := DistrictEvidence{}
	if d == nil || cityID == "" {
		return out
	}
	matches := []Entity{}
	for _, entity := range d.Exact(street, "") {
		if entity.CityID == cityID && (entity.Kind == "street" || entity.Kind == "living_area") &&
			(streetType == "" || entity.StreetType == "" || Canon(streetType) == Canon(entity.StreetType)) {
			matches = append(matches, entity)
		}
	}
	addressFound := false
	for _, entity := range matches {
		if _, ok := d.addresses[entity.ID][Canon(house)]; house != "" && ok {
			addressFound = true
		}
	}
	combined := districtEvidence{Complete: true}
	for _, entity := range matches {
		value := d.streetDistricts[entity.ID]
		if addressFound {
			var ok bool
			value, ok = d.addresses[entity.ID][Canon(house)]
			if !ok {
				value = districtEvidence{}
			} // An unresolved same-name street remains an alternative.
		}
		combined.Complete = combined.Complete && value.Complete
		for _, id := range value.IDs {
			combined.add(id)
		}
	}
	if len(combined.IDs) > 0 {
		if !combined.Complete && len(combined.IDs) == 1 {
			if city, ok := d.Entity(cityID); ok && city.DistrictID == combined.IDs[0] && d.validDistrict(cityID, city.DistrictID) {
				return DistrictEvidence{Candidates: []Entity{d.entities[city.DistrictID]}, Complete: true, Source: "kato"}
			}
		}
		out.Complete = combined.Complete
		out.Source = "street"
		if addressFound {
			out.Source = "address"
		}
		sort.Strings(combined.IDs)
		for _, id := range combined.IDs {
			out.Candidates = append(out.Candidates, d.entities[id])
		}
		return out
	}
	// KATO settlement hierarchy is authoritative even without street coverage.
	if city, ok := d.Entity(cityID); ok && city.DistrictID != "" && d.validDistrict(cityID, city.DistrictID) {
		out.Candidates = []Entity{d.entities[city.DistrictID]}
		out.Complete = true
		out.Source = "kato"
	}
	return out
}
