package resolver

import (
	"github.com/ton/couchgraph/internal/couch"
	"github.com/ton/couchgraph/internal/graph/model"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

// Resolver is the root resolver. It holds shared dependencies injected at startup.
type Resolver struct {
	// Repo provides all CouchDB operations.
	Repo *couch.Repository
}

// rawToDocument converts a raw CouchDB map into a GraphQL Document model.
func rawToDocument(raw map[string]any) *model.Document {
	if raw == nil {
		return nil
	}
	id, _ := raw["_id"].(string)
	rev, _ := raw["_rev"].(string)

	data := make(map[string]any, len(raw))
	for k, v := range raw {
		if k != "_id" && k != "_rev" {
			data[k] = v
		}
	}

	return &model.Document{
		ID:   id,
		Rev:  rev,
		Data: data,
	}
}

// mapToMovie converts a CouchDB map document to the typed Movie model.
func mapToMovie(doc map[string]any) *model.Movie {
	if doc == nil {
		return nil
	}
	id, _ := doc["_id"].(string)
	if id == "" {
		id, _ = doc["id"].(string)
	}
	docType, _ := doc["type"].(string)
	title, _ := doc["title"].(string)

	var year float64
	switch y := doc["year"].(type) {
	case float64:
		year = y
	case int:
		year = float64(y)
	case int64:
		year = float64(y)
	}

	rating, _ := doc["rating"].(float64)

	var votes float64
	switch v := doc["votes"].(type) {
	case float64:
		votes = v
	case int:
		votes = float64(v)
	case int64:
		votes = float64(v)
	}

	var runtime float64
	switch rt := doc["runtime_minutes"].(type) {
	case float64:
		runtime = rt
	case int:
		runtime = float64(rt)
	case int64:
		runtime = float64(rt)
	}

	plot, _ := doc["plot"].(string)

	var genres []string
	if rawGenres, ok := doc["genres"].([]any); ok {
		for _, g := range rawGenres {
			if s, ok := g.(string); ok {
				genres = append(genres, s)
			}
		}
	} else if rawGenres, ok := doc["genres"].([]string); ok {
		genres = rawGenres
	}

	var cast []string
	if rawCast, ok := doc["cast"].([]any); ok {
		for _, c := range rawCast {
			if s, ok := c.(string); ok {
				cast = append(cast, s)
			}
		}
	} else if rawCast, ok := doc["cast"].([]string); ok {
		cast = rawCast
	}

	var boxOfficeUsd *int
	switch bo := doc["box_office_usd"].(type) {
	case float64:
		if bo > 0 {
			v := int(bo)
			boxOfficeUsd = &v
		}
	case int64:
		if bo > 0 {
			v := int(bo)
			boxOfficeUsd = &v
		}
	case int:
		if bo > 0 {
			boxOfficeUsd = &bo
		}
	}

	var directorID *string
	if dID, ok := doc["director_id"].(string); ok && dID != "" {
		directorID = &dID
	}

	return &model.Movie{
		ID:             id,
		Type:           docType,
		Title:          title,
		Year:           int(year),
		Rating:         rating,
		Votes:          int(votes),
		RuntimeMinutes: int(runtime),
		Genres:         genres,
		Cast:           cast,
		Plot:           plot,
		BoxOfficeUsd:   boxOfficeUsd,
		DirectorID:     directorID,
	}
}

// mapToDirector converts a CouchDB map document to the typed Director model.
func mapToDirector(doc map[string]any) *model.Director {
	if doc == nil {
		return nil
	}
	id, _ := doc["_id"].(string)
	if id == "" {
		id, _ = doc["id"].(string)
	}
	docType, _ := doc["type"].(string)
	name, _ := doc["name"].(string)

	var birthYear float64
	switch by := doc["birth_year"].(type) {
	case float64:
		birthYear = by
	case int:
		birthYear = float64(by)
	case int64:
		birthYear = float64(by)
	}

	nationality, _ := doc["nationality"].(string)

	var knownFor []string
	if rawKF, ok := doc["known_for"].([]any); ok {
		for _, k := range rawKF {
			if s, ok := k.(string); ok {
				knownFor = append(knownFor, s)
			}
		}
	} else if rawKF, ok := doc["known_for"].([]string); ok {
		knownFor = rawKF
	}

	return &model.Director{
		ID:          id,
		Type:        docType,
		Name:        name,
		BirthYear:   int(birthYear),
		Nationality: nationality,
		KnownFor:    knownFor,
	}
}
