package store

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"
)

// ItemPool is a thread-safe in-memory pool of ordered item lists keyed by
// "language:level:topic". Each key holds a growing slice of JSON-encoded lists.
// Handlers append freshly-generated lists; users consume them sequentially by index
// tracked in their StudentProfile.
//
// If savePath is set, the pool is persisted to disk as JSON on every Append and
// can be reloaded on startup via Load(). This survives container restarts when the
// file lives on a mounted Docker volume.
type ItemPool struct {
	mu       sync.RWMutex
	lists    map[string][]json.RawMessage
	savePath string
}

// NewItemPool creates a pool. savePath may be empty to disable persistence.
func NewItemPool(savePath string) *ItemPool {
	return &ItemPool{
		lists:    make(map[string][]json.RawMessage),
		savePath: savePath,
	}
}

// Load reads the pool from savePath if the file exists. Safe to call even if
// the file does not exist yet (first run).
func (p *ItemPool) Load() {
	if p.savePath == "" {
		return
	}
	data, err := os.ReadFile(p.savePath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("pool: load %s: %v", p.savePath, err)
		}
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := json.Unmarshal(data, &p.lists); err != nil {
		log.Printf("pool: parse %s: %v", p.savePath, err)
	}
}

// Key returns the canonical cache key for a (language, level, topic) triple.
func (p *ItemPool) Key(language string, level int, topic string) string {
	return fmt.Sprintf("%s:%d:%s", language, level, topic)
}

// Len returns the number of cached lists for key.
func (p *ItemPool) Len(key string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.lists[key])
}

// Get returns the list at idx for key, or (nil, false) if out of range.
func (p *ItemPool) Get(key string, idx int) (json.RawMessage, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	lists := p.lists[key]
	if idx < 0 || idx >= len(lists) {
		return nil, false
	}
	return lists[idx], true
}

// Append adds a JSON-encoded list to the end of the pool for key and persists
// the pool to disk if a savePath was configured.
func (p *ItemPool) Append(key string, data json.RawMessage) {
	p.mu.Lock()
	p.lists[key] = append(p.lists[key], data)
	p.mu.Unlock()
	p.save()
}

// AllRaw returns copies of all raw list blobs for key.
// Used to build exclusion lists before generating new content.
func (p *ItemPool) AllRaw(key string) []json.RawMessage {
	p.mu.RLock()
	defer p.mu.RUnlock()
	src := p.lists[key]
	if len(src) == 0 {
		return nil
	}
	out := make([]json.RawMessage, len(src))
	copy(out, src)
	return out
}

// save writes the pool to disk atomically (temp file + rename).
func (p *ItemPool) save() {
	if p.savePath == "" {
		return
	}
	p.mu.RLock()
	data, err := json.Marshal(p.lists)
	p.mu.RUnlock()
	if err != nil {
		log.Printf("pool: marshal %s: %v", p.savePath, err)
		return
	}
	tmp := p.savePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		log.Printf("pool: write %s: %v", tmp, err)
		return
	}
	if err := os.Rename(tmp, p.savePath); err != nil {
		log.Printf("pool: rename %s: %v", tmp, err)
	}
}

// Shuffle randomizes a slice in-place using Fisher-Yates.
func Shuffle[T any](s []T) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := len(s) - 1; i > 0; i-- {
		j := r.Intn(i + 1)
		s[i], s[j] = s[j], s[i]
	}
}
