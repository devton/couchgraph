package resolver

import (
	"context"

	"github.com/ton/couchgraph/internal/couch"
	"github.com/ton/couchgraph/internal/graph/generated"
	"github.com/ton/couchgraph/internal/graph/model"
)

// Movies is the resolver for the movies field on Director (1:N relationship).
func (r *directorResolver) Movies(ctx context.Context, obj *model.Director) ([]*model.Movie, error) {
	falseVal := false
	res, err := r.Repo.QueryView(ctx, couch.ViewOptions{
		DesignDoc: "movies",
		ViewName:  "by_director_id",
		Key:       obj.ID,
		Reduce:    &falseVal,
	})
	if err != nil {
		return nil, err
	}

	movies := make([]*model.Movie, 0, len(res.Rows))
	for _, row := range res.Rows {
		if m, ok := row.Value.(map[string]any); ok {
			movies = append(movies, mapToMovie(m))
		}
	}
	return movies, nil
}

// Director is the resolver for the director field on Movie (1:1 relationship via DataLoader).
func (r *movieResolver) Director(ctx context.Context, obj *model.Movie) (*model.Director, error) {
	if obj.DirectorID == nil || *obj.DirectorID == "" {
		return nil, nil
	}

	loader := couch.NewLoader(r.Repo)
	doc, err := loader.LoadAndWait(ctx, *obj.DirectorID)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, nil
	}

	return mapToDirector(doc), nil
}

// Movie is the resolver for the movie field on Query.
func (r *queryResolver) Movie(ctx context.Context, id string) (*model.Movie, error) {
	loader := couch.NewLoader(r.Repo)
	doc, err := loader.LoadAndWait(ctx, id)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, nil
	}

	return mapToMovie(doc), nil
}

// Movies is the resolver for the movies collection field on Query.
func (r *queryResolver) Movies(ctx context.Context, genre *string, minRating *float64, limit *int) ([]*model.Movie, error) {
	selector := map[string]any{"type": "movie"}
	if genre != nil && *genre != "" {
		selector["genres"] = map[string]any{"$in": []string{*genre}}
	}
	if minRating != nil {
		selector["rating"] = map[string]any{"$gte": *minRating}
	}

	findOpts := couch.FindOptions{
		Selector: selector,
	}
	if limit != nil && *limit > 0 {
		findOpts.Limit = *limit
	}

	res, err := r.Repo.Find(ctx, findOpts)
	if err != nil {
		return nil, err
	}

	movies := make([]*model.Movie, 0, len(res.Docs))
	for _, doc := range res.Docs {
		movies = append(movies, mapToMovie(doc))
	}
	return movies, nil
}

// Director is the resolver for the director field on Query.
func (r *queryResolver) Director(ctx context.Context, id string) (*model.Director, error) {
	loader := couch.NewLoader(r.Repo)
	doc, err := loader.LoadAndWait(ctx, id)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, nil
	}

	return mapToDirector(doc), nil
}

// Directors is the resolver for the directors collection field on Query.
func (r *queryResolver) Directors(ctx context.Context) ([]*model.Director, error) {
	falseVal := false
	res, err := r.Repo.QueryView(ctx, couch.ViewOptions{
		DesignDoc: "movies",
		ViewName:  "all_directors",
		Reduce:    &falseVal,
	})
	if err != nil {
		return nil, err
	}

	directors := make([]*model.Director, 0, len(res.Rows))
	for _, row := range res.Rows {
		if d, ok := row.Value.(map[string]any); ok {
			directors = append(directors, mapToDirector(d))
		}
	}
	return directors, nil
}

// Director returns generated.DirectorResolver implementation.
func (r *Resolver) Director() generated.DirectorResolver { return &directorResolver{r} }

// Movie returns generated.MovieResolver implementation.
func (r *Resolver) Movie() generated.MovieResolver { return &movieResolver{r} }

type (
	directorResolver struct{ *Resolver }
	movieResolver    struct{ *Resolver }
)

// ─── Helpers ────────────────────────────────────────────────────────────────

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
