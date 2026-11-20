package acquire

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ScotyLucas/kinora/internal/acquire/cardigann"
	"github.com/ScotyLucas/kinora/internal/acquire/sites"
)

// The definition catalogue: Kinora ships no site definitions, but the admin can browse the community
// collection Prowlarr keeps (github.com/Prowlarr/Indexers, several hundred sites in the Cardigann format)
// and add the ones they want with a click. The collection is fetched on demand as one archive and kept
// in memory for a while; KINORA_INDEXER_CATALOG points it elsewhere (a mirror, a fork, a local file server).

// DefaultCatalogURL is the archive of the Prowlarr definition repository.
const DefaultCatalogURL = "https://codeload.github.com/Prowlarr/Indexers/tar.gz/refs/heads/master"

const (
	catalogTTL     = 12 * time.Hour
	maxCatalogSize = 64 << 20
	maxDefSize     = 1 << 20
)

// CatalogEntry is one site of the catalogue.
type CatalogEntry struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Language    string   `json:"language"`
	Type        string   `json:"type"` // public | semi-private | private
	Links       []string `json:"links"`
	Login       bool     `json:"login"` // needs an account
	YAML        string   `json:"-"`
}

type catalog struct {
	mu   sync.Mutex
	at   time.Time
	src  string
	list []*CatalogEntry
	ids  map[string]*CatalogEntry
}

func catalogURL() string {
	if u := strings.TrimSpace(os.Getenv("KINORA_INDEXER_CATALOG")); u != "" {
		return u
	}
	return DefaultCatalogURL
}

// Catalog returns the catalogue, fetching it when it is older than half a day (or refresh is set).
func (m *Manager) Catalog(ctx context.Context, refresh bool) ([]*CatalogEntry, error) {
	c := &m.cat
	c.mu.Lock()
	defer c.mu.Unlock()
	src := catalogURL()
	if !refresh && c.list != nil && c.src == src && time.Since(c.at) < catalogTTL {
		return c.list, nil
	}
	list, err := fetchCatalog(ctx, src)
	if err != nil {
		if c.list != nil { // keep serving the older copy
			return c.list, nil
		}
		return nil, err
	}
	c.list, c.at, c.src = list, time.Now(), src
	c.ids = make(map[string]*CatalogEntry, len(list))
	for _, e := range list {
		c.ids[e.ID] = e
	}
	return list, nil
}

// CatalogEntry finds one site of the catalogue.
func (m *Manager) CatalogEntry(ctx context.Context, id string) (*CatalogEntry, error) {
	if _, err := m.Catalog(ctx, false); err != nil {
		return nil, err
	}
	m.cat.mu.Lock()
	defer m.cat.mu.Unlock()
	if e := m.cat.ids[id]; e != nil {
		return e, nil
	}
	return nil, errors.New("ez az oldal nincs a katalógusban")
}

var reDefDir = regexp.MustCompile(`(?:^|/)definitions/v(\d+)/([^/]+\.ya?ml)$`)

func fetchCatalog(ctx context.Context, src string) ([]*CatalogEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", src, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("a leírókatalógus nem érhető el: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("a leírókatalógus nem érhető el (HTTP %d)", resp.StatusCode)
	}
	return readCatalog(io.LimitReader(resp.Body, maxCatalogSize))
}

// readCatalog reads a .tar.gz of the repository. Prowlarr keeps one folder per format version
// (definitions/v10, v11, …); the newest well-filled one is used.
func readCatalog(r io.Reader) ([]*CatalogEntry, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, errors.New("a leírókatalógus nem .tar.gz archívum")
	}
	tr := tar.NewReader(gz)
	byVer := map[int]map[string][]byte{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("a leírókatalógus sérült: %w", err)
		}
		mm := reDefDir.FindStringSubmatch(h.Name)
		if h.Typeflag != tar.TypeReg || mm == nil || h.Size > maxDefSize {
			continue
		}
		v, _ := strconv.Atoi(mm[1])
		b, err := io.ReadAll(io.LimitReader(tr, maxDefSize))
		if err != nil {
			return nil, err
		}
		if byVer[v] == nil {
			byVer[v] = map[string][]byte{}
		}
		byVer[v][mm[2]] = b
	}
	best := -1
	for v, files := range byVer {
		if len(files) >= 20 && v > best {
			best = v
		}
	}
	if best < 0 {
		return nil, errors.New("a leírókatalógusban nincsenek leírók")
	}
	var out []*CatalogEntry
	for _, b := range byVer[best] {
		d, err := cardigann.Parse(b)
		if err != nil {
			continue
		}
		if _, builtin := sites.Builtin[d.ID]; builtin { // Kinora has its own (nCore)
			continue
		}
		out = append(out, &CatalogEntry{ID: d.ID, Name: d.Name, Description: d.Description, Language: d.Language,
			Type: d.Type, Links: d.Links, Login: d.Login != nil, YAML: string(b)})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}
