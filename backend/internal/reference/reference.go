// Package reference reads the immutable administrative/street directory.
// It deliberately has no building coordinates or writable confirmation methods.
package reference

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"unicode"

	_ "modernc.org/sqlite"
)

type Entity struct {
	ID, Kind, Name, ParentID, CityID, ProvinceID, DistrictID, StreetType, Source string
	Aliases                                                                      []string
}
type Match struct {
	Entity     Entity
	Surface    string
	Start, End int // UTF-8 byte offsets in the supplied text.
}
type Directory struct {
	version         string
	entities        map[string]Entity
	aliases         map[string][]string
	latinAliases    map[string][]string
	maxWords        int
	addresses       map[string]map[string]districtEvidence
	streetDistricts map[string]districtEvidence
}

func Canon(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '/' && r != '-' }), " ")
}
func Open(path string) (*Directory, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	hash := sha256.New()
	_, err = io.Copy(hash, f)
	f.Close()
	if err != nil {
		return nil, err
	}
	fingerprint := fmt.Sprintf("sha256:%x", hash.Sum(nil))
	uri := url.URL{Scheme: "file", Path: path}
	q := uri.Query()
	q.Set("mode", "ro")
	q.Set("immutable", "1")
	uri.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var version string
	if err = db.QueryRow("SELECT value FROM metadata WHERE key='schema_version'").Scan(&version); err != nil {
		return nil, err
	}
	if version != "1" && version != "2" && version != "3" {
		return nil, fmt.Errorf("unsupported reference schema %q", version)
	}
	d := &Directory{version: fingerprint, entities: map[string]Entity{}, aliases: map[string][]string{}, latinAliases: map[string][]string{}, maxWords: 1, addresses: map[string]map[string]districtEvidence{}, streetDistricts: map[string]districtEvidence{}}
	rows, err := db.Query("SELECT id,kind,name,parent_id,city_id,province_id,district_id,street_type,source FROM entities ORDER BY id")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var e Entity
		if err = rows.Scan(&e.ID, &e.Kind, &e.Name, &e.ParentID, &e.CityID, &e.ProvinceID, &e.DistrictID, &e.StreetType, &e.Source); err != nil {
			rows.Close()
			return nil, err
		}
		d.entities[e.ID] = e
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	rows, err = db.Query("SELECT entity_id,alias FROM aliases ORDER BY entity_id,alias")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, alias string
		if err = rows.Scan(&id, &alias); err != nil {
			return nil, err
		}
		e, ok := d.entities[id]
		if !ok {
			return nil, fmt.Errorf("dangling alias %s", id)
		}
		e.Aliases = append(e.Aliases, alias)
		d.entities[id] = e
		key := Canon(alias)
		if key != "" {
			d.aliases[key] = append(d.aliases[key], id)
			if latin := LatinKey(key); latin != key {
				d.latinAliases[latin] = append(d.latinAliases[latin], id)
			}
			if n := len(strings.Fields(key)); n > d.maxWords {
				d.maxWords = n
			}
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	// Expand explicit source qualifiers, never infer a settlement's type from
	// its bare name or popularity. These are component aliases, not addresses.
	for id, e := range d.entities {
		if e.Kind != "city" {
			continue
		}
		for _, alias := range e.Aliases {
			words := strings.Fields(Canon(alias))
			if len(words) < 2 {
				continue
			}
			full := map[string]string{"г": "город"}[words[0]]
			if full == "" {
				continue
			}
			key := full + " " + strings.Join(words[1:], " ")
			d.aliases[key] = append(d.aliases[key], id)
			if latin := LatinKey(key); latin != key {
				d.latinAliases[latin] = append(d.latinAliases[latin], id)
			}
			if len(words) > d.maxWords {
				d.maxWords = len(words)
			}
		}
	}
	if version == "2" || version == "3" {
		query := "SELECT street_id,city_id,house,0,'' FROM addresses"
		if version == "3" {
			query = `SELECT a.street_id,a.city_id,a.house,COALESCE(o.complete,0),COALESCE(l.district_id,'')
 FROM addresses a LEFT JOIN district_objects o ON o.object_kind='address' AND o.object_id=a.id
 LEFT JOIN district_links l ON l.object_kind=o.object_kind AND l.object_id=o.object_id`
		}
		rows, err = db.Query(query)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var streetID, cityID, house, districtID string
			var complete bool
			if err = rows.Scan(&streetID, &cityID, &house, &complete, &districtID); err != nil {
				return nil, err
			}
			street, ok := d.entities[streetID]
			city, cityOK := d.entities[cityID]
			if !ok || !cityOK || city.Kind != "city" || street.CityID != cityID || (street.Kind != "street" && street.Kind != "living_area") || Canon(house) == "" {
				return nil, fmt.Errorf("invalid address hierarchy for %s", streetID)
			}
			if d.addresses[streetID] == nil {
				d.addresses[streetID] = map[string]districtEvidence{}
			}
			if districtID != "" && !d.validDistrict(cityID, districtID) {
				return nil, fmt.Errorf("invalid address district %s", districtID)
			}
			key := Canon(house)
			value, seen := d.addresses[streetID][key]
			if !seen {
				value.Complete = true
			}
			value.Complete = value.Complete && complete
			value.add(districtID)
			d.addresses[streetID][key] = value
		}
		if err = rows.Err(); err != nil {
			return nil, err
		}
		rows.Close()
	}
	if version == "3" {
		if err = d.loadDistricts(db); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func (d *Directory) Entity(id string) (Entity, bool) {
	if d == nil {
		return Entity{}, false
	}
	e, ok := d.entities[id]
	return e, ok
}
func (d *Directory) Exact(value, kind string) []Entity {
	out := []Entity{}
	if d == nil {
		return out
	}
	seen := map[string]bool{}
	for _, id := range d.aliases[Canon(value)] {
		e := d.entities[id]
		if (kind == "" || kind == e.Kind || kind == "district" && e.IsDistrict()) && !seen[id] {
			out = append(out, e)
			seen[id] = true
		}
	}
	return out
}

// LatinExact is opt-in component matching. It does not change geographic scans
// or the ordinary Exact contract used for identity and district reconciliation.
func (d *Directory) LatinExact(value, kind string) []Entity {
	out := d.Exact(value, kind)
	if d == nil {
		return out
	}
	seen := map[string]bool{}
	for _, e := range out {
		seen[e.ID] = true
	}
	for _, id := range d.latinAliases[Canon(value)] {
		e := d.entities[id]
		if !seen[id] && e.Kind == kind {
			out = append(out, e)
			seen[id] = true
		}
	}
	return out
}

func (d *Directory) Matches(text string) []Match {
	out := []Match{}
	seen := map[string]bool{}
	for _, match := range d.Mentions(text) {
		if !seen[match.Entity.ID] {
			out = append(out, match)
			seen[match.Entity.ID] = true
		}
	}
	return out
}

// Mentions retains repeated occurrences so a name in a street/venue can be
// excluded without deleting an independent administrative mention elsewhere.
func (d *Directory) Mentions(text string) []Match {
	out := []Match{}
	if d == nil {
		return out
	}
	// Token slices retain original case/spelling. Punctuation and bilingual slashes
	// between words are separators; a fractional numeric house stays untouched elsewhere.
	words := []string{}
	starts, ends := []int{}, []int{}
	start := -1
	for offset, char := range text {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || char == '-' {
			if start < 0 {
				start = offset
			}
		} else if start >= 0 {
			words = append(words, text[start:offset])
			starts = append(starts, start)
			ends = append(ends, offset)
			start = -1
		}
	}
	if start >= 0 {
		words = append(words, text[start:])
		starts = append(starts, start)
		ends = append(ends, len(text))
	}
	for i := range words {
		for n := 1; n <= d.maxWords && i+n <= len(words); n++ {
			surface := strings.Join(words[i:i+n], " ")
			for _, id := range d.aliases[Canon(surface)] {
				out = append(out, Match{Entity: d.entities[id], Surface: surface, Start: starts[i], End: ends[i+n-1]})
			}
		}
	}
	return out
}
func (d *Directory) Count() int {
	if d == nil {
		return 0
	}
	return len(d.entities)
}

// Version identifies the bytes of the immutable snapshot, including aliases and addresses.
func (d *Directory) Version() string {
	if d == nil {
		return "unavailable"
	}
	return d.version
}

// SettlementType uses explicit source qualifiers only; no inference from popularity.
func (e Entity) SettlementType() *string {
	types := map[string]bool{}
	for _, alias := range e.Aliases {
		a := strings.ToLower(strings.TrimSpace(alias))
		for prefix, kind := range map[string]string{"г.": "город", "город ": "город", "с.": "село", "село ": "село", "п.": "посёлок", "поселок ": "посёлок"} {
			if strings.HasPrefix(a, prefix) {
				types[kind] = true
			}
		}
	}
	if len(types) == 1 {
		for kind := range types {
			return &kind
		}
	}
	return nil
}
