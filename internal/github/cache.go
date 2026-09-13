package github

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// DefaultCacheTTL is how long a cached response is served without contacting
// GitHub. After that the entry is revalidated with If-None-Match; a 304 reuses
// the stored body and, when authenticated, costs no rate limit.
const DefaultCacheTTL = time.Hour

type cacheEntry struct {
	ETag      string    `json:"etag"`
	Link      string    `json:"link,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
	Body      []byte    `json:"body"`
}

type diskCache struct {
	dir string
	ttl time.Duration
}

// DefaultCacheDir returns the per-user cache directory for worthy.
func DefaultCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "worthy"), nil
}

// WithCache stores 200 responses under dir and serves them for ttl before
// revalidating with the stored ETag. A zero or negative ttl always revalidates.
func WithCache(dir string, ttl time.Duration) Option {
	return func(c *Client) { c.cache = &diskCache{dir: dir, ttl: ttl} }
}

type forceRevalidateKey struct{}

// ForceRevalidate returns a context whose requests ignore cache freshness and
// always ask GitHub, still sending If-None-Match so unchanged data is cheap.
func ForceRevalidate(ctx context.Context) context.Context {
	return context.WithValue(ctx, forceRevalidateKey{}, true)
}

func forced(ctx context.Context) bool {
	v, _ := ctx.Value(forceRevalidateKey{}).(bool)
	return v
}

func (d *diskCache) file(path, accept string) string {
	sum := sha256.Sum256([]byte(accept + "\n" + path))
	return filepath.Join(d.dir, hex.EncodeToString(sum[:])+".json")
}

func (d *diskCache) load(path, accept string) (*cacheEntry, bool) {
	b, err := os.ReadFile(d.file(path, accept)) //nolint:gosec // path is cache dir + sha256 hex
	if err != nil {
		return nil, false
	}
	var e cacheEntry
	if json.Unmarshal(b, &e) != nil || e.ETag == "" {
		return nil, false
	}
	return &e, true
}

// store writes atomically (temp file + rename) so a concurrent reader never
// sees a partial entry. Failures are ignored: the cache is an optimisation.
func (d *diskCache) store(path, accept string, e *cacheEntry) {
	if err := os.MkdirAll(d.dir, 0o700); err != nil {
		return
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	dst := d.file(path, accept)
	tmp, err := os.CreateTemp(d.dir, "tmp-*")
	if err != nil {
		return
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Chmod(tmp.Name(), 0o600) != nil || os.Rename(tmp.Name(), dst) != nil {
		// Best effort: a leftover temp file is harmless and swept next store.
		_ = os.Remove(tmp.Name()) //nolint:errcheck // nothing useful to do on failure
	}
}

func (d *diskCache) fresh(e *cacheEntry) bool {
	return d.ttl > 0 && time.Since(e.FetchedAt) < d.ttl
}

func cachedHeader(e *cacheEntry) http.Header {
	h := http.Header{}
	h.Set("ETag", e.ETag)
	if e.Link != "" {
		h.Set("Link", e.Link)
	}
	return h
}
