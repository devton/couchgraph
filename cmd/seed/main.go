// Command seed populates CouchDB with a rich IMDb-style movies dataset,
// Mango indexes, and MapReduce views using UUIDv7 IDs.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/devton/couchgraph/internal/config"
	"github.com/devton/couchgraph/internal/couch"
	"github.com/devton/couchgraph/internal/project"
)

type Movie struct {
	ID             string   `json:"_id"`
	Type           string   `json:"type"`
	Title          string   `json:"title"`
	Year           int      `json:"year"`
	DirectorID     string   `json:"director_id"`
	Director       string   `json:"director"`
	Genres         []string `json:"genres"`
	Rating         float64  `json:"rating"`
	Votes          int      `json:"votes"`
	RuntimeMinutes int      `json:"runtime_minutes"`
	Cast           []string `json:"cast"`
	Plot           string   `json:"plot"`
	BoxOfficeUSD   int64    `json:"box_office_usd"`
}

type Director struct {
	ID          string   `json:"_id"`
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	BirthYear   int      `json:"birth_year"`
	Nationality string   `json:"nationality"`
	KnownFor    []string `json:"known_for"`
}

func mustUUIDv7() string {
	id, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return id.String()
}

func main() {
	var targetDB, projectPath string
	flag.StringVar(&targetDB, "db", "couchgraph_movies", "Target database name for IMDb seed data")
	flag.StringVar(&projectPath, "project", "examples/movies/couchgraph.yaml", "couchgraph.yaml whose design documents are pushed (single source of truth for the views)")
	flag.Parse()

	// ── 1. Config ─────────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Override database with flag value if provided or default to couchgraph_movies
	if targetDB != "" {
		cfg.CouchDB.Database = targetDB
	}

	logger, _ := zap.NewDevelopment()
	defer logger.Sync() //nolint:errcheck

	logger.Info("Starting IMDb movies seeder...",
		zap.String("couchdb_url", cfg.CouchDB.URL),
		zap.String("database", cfg.CouchDB.Database),
	)

	// ── 2. CouchDB Connection ─────────────────────────────────────────────
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := couch.New(ctx, &cfg.CouchDB)
	if err != nil {
		logger.Fatal("failed to connect to CouchDB", zap.Error(err))
	}
	repo := couch.NewRepository(client)

	// Clean up previous seed documents to maintain clean relations
	if res, err := repo.Find(ctx, couch.FindOptions{
		Selector: map[string]any{"type": map[string]any{"$in": []string{"movie", "director"}}},
		Fields:   []string{"_id", "_rev"},
		Limit:    1000,
	}); err == nil && len(res.Docs) > 0 {
		var deleteDocs []map[string]any
		for _, d := range res.Docs {
			deleteDocs = append(deleteDocs, map[string]any{
				"_id":      d["_id"],
				"_rev":     d["_rev"],
				"_deleted": true,
			})
		}
		_, _ = repo.BulkDocs(ctx, deleteDocs)
	}

	// ── 3. Sync Mango Indexes & MapReduce Views from Project ───────────────
	logger.Info("Syncing design documents and indexes from project...", zap.String("project", projectPath))
	p, err := project.Load(projectPath)
	if err != nil {
		logger.Fatal("failed to load project", zap.String("path", projectPath), zap.Error(err))
	}
	indexNames := syncProjectIndexes(ctx, repo, p, logger)
	designIDs := syncProjectDesignDocs(ctx, repo, p, logger)

	// ── 5. Dataset Definition (IMDb Top Classics) ─────────────────────────
	nolanID := mustUUIDv7()
	tarantinoID := mustUUIDv7()
	miyazakiID := mustUUIDv7()
	scorseseID := mustUUIDv7()
	darabontID := mustUUIDv7()
	coppolaID := mustUUIDv7()
	wachowskiID := mustUUIDv7()
	bongID := mustUUIDv7()

	directors := []Director{
		{
			ID:          nolanID,
			Type:        "director",
			Name:        "Christopher Nolan",
			BirthYear:   1970,
			Nationality: "British-American",
			KnownFor:    []string{"Inception", "The Dark Knight", "Interstellar", "Oppenheimer"},
		},
		{
			ID:          tarantinoID,
			Type:        "director",
			Name:        "Quentin Tarantino",
			BirthYear:   1963,
			Nationality: "American",
			KnownFor:    []string{"Pulp Fiction", "Kill Bill", "Inglourious Basterds"},
		},
		{
			ID:          miyazakiID,
			Type:        "director",
			Name:        "Hayao Miyazaki",
			BirthYear:   1941,
			Nationality: "Japanese",
			KnownFor:    []string{"Spirited Away", "Princess Mononoke", "My Neighbor Totoro"},
		},
		{
			ID:          scorseseID,
			Type:        "director",
			Name:        "Martin Scorsese",
			BirthYear:   1942,
			Nationality: "American",
			KnownFor:    []string{"Goodfellas", "Taxi Driver", "The Wolf of Wall Street"},
		},
		{
			ID:          darabontID,
			Type:        "director",
			Name:        "Frank Darabont",
			BirthYear:   1959,
			Nationality: "French-American",
			KnownFor:    []string{"The Shawshank Redemption", "The Green Mile"},
		},
		{
			ID:          coppolaID,
			Type:        "director",
			Name:        "Francis Ford Coppola",
			BirthYear:   1939,
			Nationality: "American",
			KnownFor:    []string{"The Godfather", "Apocalypse Now"},
		},
		{
			ID:          wachowskiID,
			Type:        "director",
			Name:        "Lana & Lilly Wachowski",
			BirthYear:   1965,
			Nationality: "American",
			KnownFor:    []string{"The Matrix", "V for Vendetta", "Cloud Atlas"},
		},
		{
			ID:          bongID,
			Type:        "director",
			Name:        "Bong Joon Ho",
			BirthYear:   1969,
			Nationality: "South Korean",
			KnownFor:    []string{"Parasite", "Snowpiercer", "Memories of Murder"},
		},
	}

	movies := []Movie{
		{
			ID:             mustUUIDv7(),
			Type:           "movie",
			Title:          "The Shawshank Redemption",
			Year:           1994,
			DirectorID:     darabontID,
			Director:       "Frank Darabont",
			Genres:         []string{"Drama"},
			Rating:         9.3,
			Votes:          2800000,
			RuntimeMinutes: 142,
			Cast:           []string{"Tim Robbins", "Morgan Freeman", "Bob Gunton"},
			Plot:           "Over the course of several years, two convicts form a friendship, seeking solace and eventual redemption through basic compassion.",
			BoxOfficeUSD:   73300000,
		},
		{
			ID:             mustUUIDv7(),
			Type:           "movie",
			Title:          "The Godfather",
			Year:           1972,
			DirectorID:     coppolaID,
			Director:       "Francis Ford Coppola",
			Genres:         []string{"Crime", "Drama"},
			Rating:         9.2,
			Votes:          1950000,
			RuntimeMinutes: 175,
			Cast:           []string{"Marlon Brando", "Al Pacino", "James Caan", "Robert Duvall"},
			Plot:           "The aging patriarch of an organized crime dynasty transfers control of his clandestine empire to his reluctant son.",
			BoxOfficeUSD:   291000000,
		},
		{
			ID:             mustUUIDv7(),
			Type:           "movie",
			Title:          "The Dark Knight",
			Year:           2008,
			DirectorID:     nolanID,
			Director:       "Christopher Nolan",
			Genres:         []string{"Action", "Crime", "Drama", "Thriller"},
			Rating:         9.0,
			Votes:          2750000,
			RuntimeMinutes: 152,
			Cast:           []string{"Christian Bale", "Heath Ledger", "Aaron Eckhart", "Michael Caine"},
			Plot:           "When the menace known as the Joker wreaks havoc and chaos on the people of Gotham, Batman must accept one of the greatest psychological and physical tests.",
			BoxOfficeUSD:   1006000000,
		},
		{
			ID:             mustUUIDv7(),
			Type:           "movie",
			Title:          "Pulp Fiction",
			Year:           1994,
			DirectorID:     tarantinoID,
			Director:       "Quentin Tarantino",
			Genres:         []string{"Crime", "Drama"},
			Rating:         8.9,
			Votes:          2100000,
			RuntimeMinutes: 154,
			Cast:           []string{"John Travolta", "Uma Thurman", "Samuel L. Jackson", "Bruce Willis"},
			Plot:           "The lives of two mob hitmen, a boxer, a gangster and his wife, and a pair of diner bandits intertwine in four tales of violence and redemption.",
			BoxOfficeUSD:   213900000,
		},
		{
			ID:             mustUUIDv7(),
			Type:           "movie",
			Title:          "Inception",
			Year:           2010,
			DirectorID:     nolanID,
			Director:       "Christopher Nolan",
			Genres:         []string{"Action", "Adventure", "Sci-Fi", "Thriller"},
			Rating:         8.8,
			Votes:          2450000,
			RuntimeMinutes: 148,
			Cast:           []string{"Leonardo DiCaprio", "Joseph Gordon-Levitt", "Elliot Page", "Tom Hardy"},
			Plot:           "A thief who steals corporate secrets through the use of dream-sharing technology is given the inverse task of planting an idea into the mind of a C.E.O.",
			BoxOfficeUSD:   836800000,
		},
		{
			ID:             mustUUIDv7(),
			Type:           "movie",
			Title:          "Interstellar",
			Year:           2014,
			DirectorID:     nolanID,
			Director:       "Christopher Nolan",
			Genres:         []string{"Adventure", "Drama", "Sci-Fi"},
			Rating:         8.7,
			Votes:          1950000,
			RuntimeMinutes: 169,
			Cast:           []string{"Matthew McConaughey", "Anne Hathaway", "Jessica Chastain", "Michael Caine"},
			Plot:           "When Earth becomes uninhabitable in the future, a farmer and ex-NASA pilot, Joseph Cooper, is tasked to pilot a spacecraft along with a team of researchers to find a new planet for humans.",
			BoxOfficeUSD:   773800000,
		},
		{
			ID:             mustUUIDv7(),
			Type:           "movie",
			Title:          "The Matrix",
			Year:           1999,
			DirectorID:     wachowskiID,
			Director:       "Lana & Lilly Wachowski",
			Genres:         []string{"Action", "Sci-Fi"},
			Rating:         8.7,
			Votes:          1980000,
			RuntimeMinutes: 136,
			Cast:           []string{"Keanu Reeves", "Laurence Fishburne", "Carrie-Anne Moss", "Hugo Weaving"},
			Plot:           "When a beautiful stranger leads computer hacker Neo to a forbidding underworld, he discovers the shocking truth--the life he knows is the elaborate deception of an evil cyber-intelligence.",
			BoxOfficeUSD:   467200000,
		},
		{
			ID:             mustUUIDv7(),
			Type:           "movie",
			Title:          "Goodfellas",
			Year:           1990,
			DirectorID:     scorseseID,
			Director:       "Martin Scorsese",
			Genres:         []string{"Biography", "Crime", "Drama"},
			Rating:         8.7,
			Votes:          1200000,
			RuntimeMinutes: 145,
			Cast:           []string{"Robert De Niro", "Ray Liotta", "Joe Pesci", "Lorraine Bracco"},
			Plot:           "The story of Henry Hill and his life in the mafia, covering his relationship with his wife Karen and his mob partners Jimmy Conway and Tommy DeVito.",
			BoxOfficeUSD:   47100000,
		},
		{
			ID:             mustUUIDv7(),
			Type:           "movie",
			Title:          "Spirited Away",
			Year:           2001,
			DirectorID:     miyazakiID,
			Director:       "Hayao Miyazaki",
			Genres:         []string{"Animation", "Adventure", "Family", "Fantasy"},
			Rating:         8.6,
			Votes:          820000,
			RuntimeMinutes: 125,
			Cast:           []string{"Daveigh Chase", "Suzanne Pleshette", "Miyu Irino"},
			Plot:           "During her family's move to the suburbs, a sullen 10-year-old girl wanders into a world ruled by gods, witches, and spirits, a world where humans are changed into beasts.",
			BoxOfficeUSD:   395800000,
		},
		{
			ID:             mustUUIDv7(),
			Type:           "movie",
			Title:          "Parasite",
			Year:           2019,
			DirectorID:     bongID,
			Director:       "Bong Joon Ho",
			Genres:         []string{"Drama", "Thriller"},
			Rating:         8.5,
			Votes:          890000,
			RuntimeMinutes: 132,
			Cast:           []string{"Song Kang-ho", "Lee Sun-kyun", "Cho Yeo-jeong", "Choi Woo-shik"},
			Plot:           "Greed and class discrimination threaten the newly formed symbiotic relationship between the wealthy Park family and the destitute Kim clan.",
			BoxOfficeUSD:   263100000,
		},
	}

	// ── 6. Bulk Insert Documents ──────────────────────────────────────────
	logger.Info("Inserting documents via BulkDocs...")
	var rawDocs []map[string]any

	for _, m := range movies {
		b, _ := json.Marshal(m)
		var doc map[string]any
		_ = json.Unmarshal(b, &doc)
		rawDocs = append(rawDocs, doc)
	}

	for _, d := range directors {
		b, _ := json.Marshal(d)
		var doc map[string]any
		_ = json.Unmarshal(b, &doc)
		rawDocs = append(rawDocs, doc)
	}

	results, err := repo.BulkDocs(ctx, rawDocs)
	if err != nil {
		logger.Fatal("BulkDocs failed", zap.Error(err))
	}

	successCount := 0
	for _, res := range results {
		if ok, _ := res["ok"].(bool); ok {
			successCount++
		}
	}

	logger.Info("✅ Seeding completed successfully!",
		zap.Int("total_documents", len(rawDocs)),
		zap.Int("inserted", successCount),
		zap.String("database", cfg.CouchDB.Database),
	)

	fmt.Printf("\n🎬 IMDb Dataset seeded into CouchDB (%s)!\n", cfg.CouchDB.Database)
	fmt.Printf("   • %d Movies & %d Directors inserted with UUIDv7 IDs\n", len(movies), len(directors))
	fmt.Printf("   • Foreign keys (director_id) linked across documents\n")
	fmt.Printf("   • Mango Indexes synced from %s: %s\n", projectPath, strings.Join(indexNames, ", "))
	fmt.Printf("   • Design documents synced from %s: %s\n", projectPath, strings.Join(designIDs, ", "))
	fmt.Printf("   • Open http://localhost:%d to test with GraphQL Playground!\n\n", cfg.Server.Port)
}

func syncProjectIndexes(ctx context.Context, repo *couch.Repository, p *project.Project, logger *zap.Logger) []string {
	names := make([]string, 0, len(p.Indexes))
	for _, idx := range p.Indexes {
		changed, err := repo.SyncIndex(ctx, idx)
		if err != nil {
			logger.Fatal("failed to sync mango index", zap.String("name", idx.Name), zap.Error(err))
		}
		status := "unchanged"
		if changed {
			status = "created"
		}
		logger.Info("mango index synced", zap.String("name", idx.Name), zap.String("status", status))
		names = append(names, idx.Name)
	}
	return names
}

func syncProjectDesignDocs(ctx context.Context, store couch.Store, p *project.Project, logger *zap.Logger) []string {
	docs, err := p.LoadDesignDocs()
	if err != nil {
		logger.Fatal("failed to read design documents", zap.Error(err))
	}
	ids := make([]string, 0, len(docs))
	for _, d := range docs {
		id, _ := d.Doc["_id"].(string)
		changed, err := couch.SyncDesignDoc(ctx, store, d.Doc)
		if err != nil {
			logger.Fatal("failed to sync design document", zap.String("file", d.File), zap.Error(err))
		}
		status := "unchanged"
		if changed {
			status = "updated"
		}
		logger.Info("design document synced", zap.String("id", id), zap.String("file", d.File), zap.String("status", status))
		ids = append(ids, id)
	}
	return ids
}
