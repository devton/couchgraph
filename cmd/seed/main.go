// Command seed populates CouchDB with a rich IMDb-style movies dataset,
// Mango indexes, and MapReduce views using UUIDv7 IDs.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ton/couchgraph/internal/config"
	"github.com/ton/couchgraph/internal/couch"
)

type Movie struct {
	ID             string   `json:"_id"`
	Type           string   `json:"type"`
	Title          string   `json:"title"`
	Year           int      `json:"year"`
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
	var targetDB string
	flag.StringVar(&targetDB, "db", "couchgraph_movies", "Target database name for IMDb seed data")
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

	// ── 3. Create Mango Indexes ───────────────────────────────────────────
	logger.Info("Creating Mango indexes for movies...")
	createIndex(ctx, cfg, "idx_movies_genre_rating", []string{"type", "genres", "rating", "year"})
	createIndex(ctx, cfg, "idx_movies_director_year", []string{"type", "director", "year"})

	// ── 4. Create MapReduce Views ─────────────────────────────────────────
	logger.Info("Creating MapReduce design document (_design/movies)...")
	createDesignDoc(ctx, cfg)

	// ── 5. Dataset Definition (IMDb Top Classics) ─────────────────────────
	movies := []Movie{
		{
			ID:             mustUUIDv7(),
			Type:           "movie",
			Title:          "The Shawshank Redemption",
			Year:           1994,
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
			Director:       "Lana Wachowski, Lilly Wachowski",
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

	directors := []Director{
		{
			ID:          mustUUIDv7(),
			Type:        "director",
			Name:        "Christopher Nolan",
			BirthYear:   1970,
			Nationality: "British-American",
			KnownFor:    []string{"Inception", "The Dark Knight", "Interstellar", "Oppenheimer"},
		},
		{
			ID:          mustUUIDv7(),
			Type:        "director",
			Name:        "Quentin Tarantino",
			BirthYear:   1963,
			Nationality: "American",
			KnownFor:    []string{"Pulp Fiction", "Kill Bill", "Inglourious Basterds"},
		},
		{
			ID:          mustUUIDv7(),
			Type:        "director",
			Name:        "Hayao Miyazaki",
			BirthYear:   1941,
			Nationality: "Japanese",
			KnownFor:    []string{"Spirited Away", "Princess Mononoke", "My Neighbor Totoro"},
		},
		{
			ID:          mustUUIDv7(),
			Type:        "director",
			Name:        "Martin Scorsese",
			BirthYear:   1942,
			Nationality: "American",
			KnownFor:    []string{"Goodfellas", "Taxi Driver", "The Wolf of Wall Street"},
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
	fmt.Printf("   • Mango Indexes created: idx_movies_genre_rating, idx_movies_director_year\n")
	fmt.Printf("   • MapReduce Views created: _design/movies/_view/by_genre, _design/movies/_view/by_year, _design/movies/_view/top_rated\n")
	fmt.Printf("   • Open http://localhost:%d to test with GraphQL Playground!\n\n", cfg.Server.Port)
}

func createIndex(ctx context.Context, cfg *config.Config, indexName string, fields []string) {
	url := fmt.Sprintf("%s/%s/_index", strings.TrimSuffix(cfg.CouchDB.URL, "/"), cfg.CouchDB.Database)
	payload := map[string]any{
		"index": map[string]any{
			"fields": fields,
		},
		"name": indexName,
		"type": "json",
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	if cfg.CouchDB.User != "" && cfg.CouchDB.Password != "" {
		req.SetBasicAuth(cfg.CouchDB.User, cfg.CouchDB.Password)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("⚠️ Warning creating index %s: %v\n", indexName, err)
		return
	}
	defer resp.Body.Close()
}

func createDesignDoc(ctx context.Context, cfg *config.Config) {
	url := fmt.Sprintf("%s/%s/_design/movies", strings.TrimSuffix(cfg.CouchDB.URL, "/"), cfg.CouchDB.Database)

	// Fetch existing _rev if design doc already exists
	var rev string
	getReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if cfg.CouchDB.User != "" && cfg.CouchDB.Password != "" {
		getReq.SetBasicAuth(cfg.CouchDB.User, cfg.CouchDB.Password)
	}
	if getResp, err := http.DefaultClient.Do(getReq); err == nil {
		if getResp.StatusCode == http.StatusOK {
			var existing map[string]any
			if err := json.NewDecoder(getResp.Body).Decode(&existing); err == nil {
				if r, ok := existing["_rev"].(string); ok {
					rev = r
				}
			}
		}
		getResp.Body.Close()
	}

	ddoc := map[string]any{
		"views": map[string]any{
			"by_genre": map[string]string{
				"map": `function (doc) {
					if (doc.type === "movie" && Array.isArray(doc.genres)) {
						doc.genres.forEach(function (g) {
							emit(g, { title: doc.title, year: doc.year, rating: doc.rating, director: doc.director });
						});
					}
				}`,
				"reduce": "_count",
			},
			"by_year": map[string]string{
				"map": `function (doc) {
					if (doc.type === "movie" && doc.year) {
						emit(doc.year, { title: doc.title, rating: doc.rating, director: doc.director });
					}
				}`,
				"reduce": "_count",
			},
			"top_rated": map[string]string{
				"map": `function (doc) {
					if (doc.type === "movie" && doc.rating) {
						emit(doc.rating, { title: doc.title, year: doc.year, director: doc.director });
					}
				}`,
				"reduce": "_count",
			},
			"ratings_stats": map[string]string{
				"map": `function (doc) {
					if (doc.type === "movie" && Array.isArray(doc.genres) && typeof doc.rating === "number") {
						doc.genres.forEach(function (g) {
							emit(g, doc.rating);
						});
					}
				}`,
				"reduce": "_stats",
			},
			"box_office_by_genre": map[string]string{
				"map": `function (doc) {
					if (doc.type === "movie" && Array.isArray(doc.genres) && typeof doc.box_office_usd === "number") {
						doc.genres.forEach(function (g) {
							emit(g, doc.box_office_usd);
						});
					}
				}`,
				"reduce": "_stats",
			},
		},
	}
	if rev != "" {
		ddoc["_rev"] = rev
	}
	body, _ := json.Marshal(ddoc)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, url, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	if cfg.CouchDB.User != "" && cfg.CouchDB.Password != "" {
		req.SetBasicAuth(cfg.CouchDB.User, cfg.CouchDB.Password)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("⚠️ Warning creating design doc: %v\n", err)
		return
	}
	defer resp.Body.Close()
}
