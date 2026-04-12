package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const cacheFile = "cache.json"

// Cache is an in-memory SHA256 file cache backed by a JSON file on disk.
// It is safe for concurrent read access but writes must go through Mark/Save.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]string // filepath → sha256 hex
	dir     string
}

// Load reads the cache from dir/cache.json. If the file does not exist, an
// empty cache is returned. dir is created if it does not exist.
func Load(dir string) (*Cache, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating cache dir %s: %w", dir, err)
	}

	c := &Cache{
		entries: make(map[string]string),
		dir:     dir,
	}

	data, err := os.ReadFile(filepath.Join(dir, cacheFile))
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading cache: %w", err)
	}

	if err := json.Unmarshal(data, &c.entries); err != nil {
		// Corrupt cache — start fresh
		c.entries = make(map[string]string)
	}
	return c, nil
}

// Save writes the current cache entries to dir/cache.json.
func (c *Cache) Save() error {
	c.mu.RLock()
	data, err := json.MarshalIndent(c.entries, "", "  ")
	c.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.dir, cacheFile), data, 0644)
}

// IsCached returns true if path has been processed with the given sha256 hash.
func (c *Cache) IsCached(path, sha string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.entries[path] == sha
}

// Mark records that path has been successfully processed with sha.
func (c *Cache) Mark(path, sha string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[path] = sha
}

// SHA256File computes the SHA256 hex digest of the file at path.
func SHA256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
