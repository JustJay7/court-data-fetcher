package cache

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/JustJay7/court-data-fetcher/internal/database"
)

type Cache interface {
	Get(key string) (*database.CaseInfo, bool)
	Set(key string, value *database.CaseInfo) error
	Delete(key string)
	Clear()
	Stats() CacheStats
}

type CacheStats struct {
	Hits       int64     `json:"hits"`
	Misses     int64     `json:"misses"`
	Size       int       `json:"size"`
	LastAccess time.Time `json:"last_access"`
}

type LRUCache struct {
	cache      *cache.Cache
	mu         sync.RWMutex
	stats      CacheStats
	maxSize    int
}

func NewCache(maxSize int, ttl time.Duration) Cache {
	return &LRUCache{
		cache:   cache.New(ttl, ttl*2),
		maxSize: maxSize,
		stats:   CacheStats{},
	}
}

func (c *LRUCache) Get(key string) (*database.CaseInfo, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stats.LastAccess = time.Now()

	if data, found := c.cache.Get(key); found {
		c.stats.Hits++
		if caseInfo, ok := data.(*database.CaseInfo); ok {
			return caseInfo, true
		}
	}

	c.stats.Misses++
	return nil, false
}

func (c *LRUCache) Set(key string, value *database.CaseInfo) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cache.ItemCount() >= c.maxSize {
		c.removeOldest()
	}

	c.cache.Set(key, value, cache.DefaultExpiration)
	return nil
}

func (c *LRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache.Delete(key)
}

func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache.Flush()
	c.stats = CacheStats{}
}

func (c *LRUCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	c.stats.Size = c.cache.ItemCount()
	return c.stats
}

func (c *LRUCache) removeOldest() {
	items := c.cache.Items()
	if len(items) == 0 {
		return
	}

	var oldestKey string
	var oldestTime time.Time

	for key, item := range items {
		if oldestTime.IsZero() || item.Expiration < oldestTime.Unix() {
			oldestKey = key
			oldestTime = time.Unix(item.Expiration, 0)
		}
	}

	if oldestKey != "" {
		c.cache.Delete(oldestKey)
	}
}

func GenerateCacheKey(caseType, caseNumber, filingYear string) string {
	return fmt.Sprintf("case:%s:%s:%s", caseType, caseNumber, filingYear)
}

func SerializeCaseInfo(info *database.CaseInfo) ([]byte, error) {
	return json.Marshal(info)
}

func DeserializeCaseInfo(data []byte) (*database.CaseInfo, error) {
	var info database.CaseInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}