// Package reference reads the immutable administrative/street directory.
// It deliberately has no building coordinates or writable confirmation methods.
package reference

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"unicode"

	_ "modernc.org/sqlite"
)

type Entity struct {
	ID, Kind, Name, ParentID, CityID, ProvinceID, DistrictID, StreetType, Source string
	Aliases                                                                      []string
}
type Match struct {
	Entity  Entity
	Surface string
}
type Directory struct {
	entities map[string]Entity
	aliases  map[string][]string
	maxWords int
}

func Canon(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '/' && r != '-' }), " ")
}
func Open(path string) (*Directory, error) {
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
	if version != "1" {
		return nil, fmt.Errorf("unsupported reference schema %q", version)
	}
	d := &Directory{entities: map[string]Entity{}, aliases: map[string][]string{}, maxWords: 1}
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
			if n := len(strings.Fields(key)); n > d.maxWords {
				d.maxWords = n
			}
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
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
		if (kind == "" || kind == e.Kind) && !seen[id] {
			out = append(out, e)
			seen[id] = true
		}
	}
	return out
}
func (d *Directory) Matches(text string) []Match {
	out := []Match{}
	if d == nil {
		return out
	}
	// Token slices retain original case/spelling. Punctuation and bilingual slashes
	// between words are separators; a fractional numeric house stays untouched elsewhere.
	words := strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' })
	seen := map[string]bool{}
	for i := range words {
		for n := 1; n <= d.maxWords && i+n <= len(words); n++ {
			surface := strings.Join(words[i:i+n], " ")
			for _, id := range d.aliases[Canon(surface)] {
				if !seen[id] {
					out = append(out, Match{Entity: d.entities[id], Surface: surface})
					seen[id] = true
				}
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
