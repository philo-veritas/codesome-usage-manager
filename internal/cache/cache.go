package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const CacheTTLSeconds = 120

type CacheEntry struct {
	Timestamp float64                `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

type CacheFile map[string]CacheEntry

// GetCacheFilePath returns the path to .usage_cache.json in project root
func GetCacheFilePath() string {
	// Try to find cache file in current directory or parent directory
	paths := []string{
		".usage_cache.json",
		"../.usage_cache.json",
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			absPath, _ := filepath.Abs(path)
			return absPath
		}
	}

	// Default to current directory
	absPath, _ := filepath.Abs(".usage_cache.json")
	return absPath
}

// LoadCachedData loads cached data for a given key
func LoadCachedData(key string) (map[string]interface{}, error) {
	cacheFilePath := GetCacheFilePath()

	data, err := os.ReadFile(cacheFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var cacheFile CacheFile
	if err := json.Unmarshal(data, &cacheFile); err != nil {
		return nil, nil
	}

	entry, exists := cacheFile[key]
	if !exists {
		return nil, nil
	}

	// Check if cache is still valid
	now := float64(time.Now().Unix()) + float64(time.Now().Nanosecond())/1e9
	if (now - entry.Timestamp) >= CacheTTLSeconds {
		return nil, nil
	}

	return entry.Data, nil
}

// SaveCache saves data to cache
func SaveCache(key string, data map[string]interface{}) error {
	cacheFilePath := GetCacheFilePath()

	var cacheFile CacheFile

	// Load existing cache
	existingData, err := os.ReadFile(cacheFilePath)
	if err == nil {
		json.Unmarshal(existingData, &cacheFile)
	}

	if cacheFile == nil {
		cacheFile = make(CacheFile)
	}

	// Save new entry
	now := float64(time.Now().Unix()) + float64(time.Now().Nanosecond())/1e9
	cacheFile[key] = CacheEntry{
		Timestamp: now,
		Data:      data,
	}

	// Write to file
	jsonData, err := json.MarshalIndent(cacheFile, "", "    ")
	if err != nil {
		return err
	}

	return os.WriteFile(cacheFilePath, jsonData, 0644)
}

// LoadCacheTimestamp loads the timestamp of a cache entry without TTL check.
// Returns the timestamp and whether the entry exists.
func LoadCacheTimestamp(key string) (time.Time, bool) {
	data, err := os.ReadFile(GetCacheFilePath())
	if err != nil {
		return time.Time{}, false
	}
	var cacheFile CacheFile
	if err := json.Unmarshal(data, &cacheFile); err != nil {
		return time.Time{}, false
	}
	entry, exists := cacheFile[key]
	if !exists {
		return time.Time{}, false
	}
	sec := int64(entry.Timestamp)
	nsec := int64((entry.Timestamp - float64(sec)) * 1e9)
	return time.Unix(sec, nsec), true
}

// DeleteCacheKey removes a specific key from the cache file.
func DeleteCacheKey(key string) error {
	cacheFilePath := GetCacheFilePath()

	data, err := os.ReadFile(cacheFilePath)
	if err != nil {
		return nil // no cache file, nothing to delete
	}

	var cacheFile CacheFile
	if err := json.Unmarshal(data, &cacheFile); err != nil {
		return nil
	}

	delete(cacheFile, key)

	jsonData, err := json.MarshalIndent(cacheFile, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(cacheFilePath, jsonData, 0644)
}
