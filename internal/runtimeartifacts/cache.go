// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/internal/directorymutex"
	"github.com/exasol/exasol-personal/internal/launcherpaths"
	"go.yaml.in/yaml/v3"
)

const (
	runtimeArtifactsDirName  = "runtime-artifacts"
	artifactsDirName         = "artifacts"
	downloadsDirName         = "downloads"
	cacheIndexFileName       = "index.json"
	cacheConfigFileName      = "runtime-artifacts.yaml"
	cacheIndexVersion        = 1
	defaultRetentionDays     = 30
	automaticCleanupInterval = 24 * time.Hour
	integrityStatusOK        = "ok"
	integrityStatusMissing   = "missing"
	integrityStatusMismatch  = "mismatch"
	integrityStatusReadError = "read_error"
)

var (
	ErrInvalidCacheConfig = errors.New("invalid runtime artifact cache configuration")
	ErrCacheLocked        = errors.New("runtime artifact cache is locked")
)

type clocker interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now().UTC()
}

type CacheConfig struct {
	//nolint:tagliatelle // YAML config uses snake_case field names.
	RetentionDays int `json:"retentionDays" yaml:"retention_days"`
}

type Cache struct {
	root       string
	configPath string
	clock      clocker
}

type CacheEntryInfo struct {
	ID           string    `json:"id"`
	ResourceID   string    `json:"resourceId"`
	Platform     string    `json:"platform"`
	URL          string    `json:"url"`
	Sha256       string    `json:"sha256"`
	ArtifactPath string    `json:"artifactPath"`
	ResolvedPath string    `json:"resolvedPath"`
	CreatedAt    time.Time `json:"createdAt"`
	LastUsedAt   time.Time `json:"lastUsedAt"`
	SizeBytes    int64     `json:"sizeBytes"`
}

type CleanupMode string

const (
	CleanupModeStale            CleanupMode = "stale"
	CleanupModeInvalid          CleanupMode = "invalid"
	CleanupModeAll              CleanupMode = "all"
	CleanupModePartialDownloads CleanupMode = "partial-downloads"
)

type CleanOptions struct {
	Mode   CleanupMode
	DryRun bool
}

type CleanSummary struct {
	Mode           CleanupMode      `json:"mode"`
	DryRun         bool             `json:"dryRun"`
	RemovedEntries int              `json:"removedEntries"`
	RemovedBytes   int64            `json:"removedBytes"`
	InvalidEntries int              `json:"invalidEntries,omitempty"`
	Entries        []CacheEntryInfo `json:"entries"`
}

type CacheLockStatus struct {
	CacheExists bool   `json:"cacheExists"`
	Locked      bool   `json:"locked"`
	Mode        string `json:"mode,omitempty"`
	SharedCount int    `json:"sharedCount,omitempty"`
	MarkerPath  string `json:"markerPath,omitempty"`
	Error       string `json:"error,omitempty"`
}

type cleanupPlan struct {
	mode           CleanupMode
	dryRun         bool
	removedBytes   int64
	invalidEntries int
	candidates     []cleanupCandidate
}

type cleanupCandidate struct {
	id    string
	entry cacheIndexEntry
	info  CacheEntryInfo
}

type partialDownloadPlan struct {
	removedBytes int64
	candidates   []partialDownloadCandidate
}

type partialDownloadCandidate struct {
	path string
}

func DefaultCacheRoot() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache directory: %w", err)
	}

	return filepath.Join(launcherpaths.DirPath(cacheDir), runtimeArtifactsDirName), nil
}

func DefaultConfigPath() (string, error) {
	return launcherpaths.ResourceConfigFilePath()
}

// LegacyConfigPath exists only so an existing file there can be migrated
// to DefaultConfigPath.
func LegacyConfigPath() (string, error) {
	rootDir, err := launcherpaths.RootDirPath()
	if err != nil {
		return "", err
	}

	return filepath.Join(rootDir, cacheConfigFileName), nil
}

func NewDefaultCache() (*Cache, error) {
	root, err := DefaultCacheRoot()
	if err != nil {
		return nil, err
	}
	configPath, err := DefaultConfigPath()
	if err != nil {
		return nil, err
	}

	return NewCache(root, configPath), nil
}

func NewCache(root, configPath string) *Cache {
	return newCacheWithClock(root, configPath, systemClock{})
}

func newCacheWithClock(root, configPath string, clk clocker) *Cache {
	return &Cache{root: filepath.Clean(root), configPath: filepath.Clean(configPath), clock: clk}
}

func (c *Cache) Root() string {
	return c.root
}

func (c *Cache) ConfigPath() string {
	return c.configPath
}

func (c *Cache) IndexPath() string {
	return filepath.Join(c.root, cacheIndexFileName)
}

func DefaultCacheConfig() CacheConfig {
	return CacheConfig{RetentionDays: defaultRetentionDays}
}

func LoadCacheConfig(path string) (CacheConfig, bool, error) {
	cfg := DefaultCacheConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, false, nil
		}

		return cfg, false, fmt.Errorf("read cache config %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, true, fmt.Errorf("parse cache config %s: %w", path, err)
	}
	if err := validateCacheConfig(cfg); err != nil {
		return cfg, true, err
	}

	return cfg, true, nil
}

func EnsureCacheConfig(path string) (CacheConfig, error) {
	cfg, exists, err := LoadCacheConfig(path)
	if err != nil || exists {
		return cfg, err
	}

	return cfg, writeCacheConfig(path, cfg)
}

func writeCacheConfig(path string, cfg CacheConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("create directory for cache config %s: %w", path, err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode cache config %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, filePerm); err != nil {
		return fmt.Errorf("write cache config %s: %w", path, err)
	}

	return nil
}

func validateCacheConfig(cfg CacheConfig) error {
	if cfg.RetentionDays <= 0 {
		return fmt.Errorf("%w: retention_days must be positive", ErrInvalidCacheConfig)
	}

	return nil
}

func (c *Cache) List(ctx context.Context) ([]CacheEntryInfo, error) {
	var entries []CacheEntryInfo
	err := c.withExclusiveLock(ctx, func() error {
		if _, err := EnsureCacheConfig(c.configPath); err != nil {
			return err
		}
		index, _, err := c.readIndex()
		if err != nil {
			return err
		}
		entries = c.listEntries(index)

		return nil
	})

	return entries, err
}

func (c *Cache) Clean(ctx context.Context, opts CleanOptions) (CleanSummary, error) {
	mode, err := normalizeCleanupMode(opts.Mode)
	if err != nil {
		return CleanSummary{}, err
	}
	opts.Mode = mode

	var summary CleanSummary
	err = c.withExclusiveLock(ctx, func() error {
		cfg, err := EnsureCacheConfig(c.configPath)
		if err != nil {
			return err
		}
		if opts.Mode == CleanupModePartialDownloads {
			summary, err = c.cleanPartialDownloads(opts.DryRun)

			return err
		}

		summary, err = c.cleanIndexedEntries(cfg, opts)

		return err
	})

	return summary, err
}

func (c *Cache) Unlock() error {
	return c.clearLock()
}

//nolint:revive // dryRun mirrors the command-level --dry-run flag.
func (c *Cache) cleanPartialDownloads(dryRun bool) (CleanSummary, error) {
	plan, err := c.planPartialDownloadCleanup()
	summary := plan.summary(dryRun)
	if err != nil || dryRun {
		return summary, err
	}

	return summary, removePartialDownloads(plan)
}

func (c *Cache) cleanIndexedEntries(cfg CacheConfig, opts CleanOptions) (CleanSummary, error) {
	index, _, err := c.readIndex()
	if err != nil {
		// If we can't read the cache index, we treat it as empty so we can
		// still wipe the cache.
		index = emptyCacheIndex()
	}
	plan, err := c.planCleanup(index, cfg, opts)
	summary := plan.summary()
	if err == nil && !opts.DryRun {
		if opts.Mode == CleanupModeAll {
			err = c.wipeCacheContents(&index)
		} else {
			err = c.removeEntries(&index, plan)
		}
	}
	if err != nil || opts.DryRun {
		return summary, err
	}
	index.LastCleanup = c.clock.Now().UTC()

	return summary, c.writeIndex(index)
}

func (c *Cache) listEntries(index cacheIndex) []CacheEntryInfo {
	entries := make([]CacheEntryInfo, 0, len(index.Entries))
	for _, entryID := range sortedEntryIDs(index) {
		info, err := c.entryInfo(entryID, index.Entries[entryID])
		if err == nil {
			entries = append(entries, info)
		}
	}

	return entries
}

func (c *Cache) entryInfo(entryID string, entry cacheIndexEntry) (CacheEntryInfo, error) {
	if _, err := c.pathFromRelative(entry.EntryPath, "entryPath"); err != nil {
		return CacheEntryInfo{}, err
	}
	if _, err := c.pathFromRelative(entry.ArtifactPath, "artifactPath"); err != nil {
		return CacheEntryInfo{}, err
	}
	if _, err := c.pathFromRelative(entry.ResolvedPath, "resolvedPath"); err != nil {
		return CacheEntryInfo{}, err
	}

	return CacheEntryInfo{
		ID:           entryID,
		ResourceID:   entry.ResourceID,
		Platform:     entry.Platform,
		URL:          entry.URL,
		Sha256:       entry.Sha256,
		ArtifactPath: entry.ArtifactPath,
		ResolvedPath: entry.ResolvedPath,
		CreatedAt:    entry.CreatedAt,
		LastUsedAt:   entry.LastUsedAt,
		SizeBytes:    entry.SizeBytes,
	}, nil
}

func (c *Cache) planCleanup(
	index cacheIndex,
	cfg CacheConfig,
	opts CleanOptions,
) (cleanupPlan, error) {
	plan := cleanupPlan{mode: opts.Mode, dryRun: opts.DryRun}
	now := c.clock.Now()
	for _, entryID := range sortedEntryIDs(index) {
		entry := index.Entries[entryID]
		invalid := false
		var remove bool
		switch opts.Mode {
		case CleanupModeAll:
			remove = true
		case CleanupModeStale:
			remove = isEntryStale(entry, cfg, now)
		case CleanupModeInvalid:
			check := c.checkIntegrity(entry)
			invalid = check.Status != integrityStatusOK
			remove = invalid
		case CleanupModePartialDownloads:
			remove = false
		default:
			remove = isEntryStale(entry, cfg, now)
		}
		if !remove {
			continue
		}
		info, err := c.entryInfo(entryID, entry)
		if err != nil {
			return plan, err
		}
		size, err := directorySize(c.absolutePath(entry.EntryPath))
		if err == nil {
			info.SizeBytes = size
		}
		plan.removedBytes += info.SizeBytes
		if invalid {
			plan.invalidEntries++
		}
		plan.candidates = append(plan.candidates, cleanupCandidate{
			id:    entryID,
			entry: entry,
			info:  info,
		})
	}

	return plan, nil
}

func (p cleanupPlan) summary() CleanSummary {
	entries := make([]CacheEntryInfo, 0, len(p.candidates))
	for _, candidate := range p.candidates {
		entries = append(entries, candidate.info)
	}

	return CleanSummary{
		Mode:           p.mode,
		DryRun:         p.dryRun,
		RemovedEntries: len(p.candidates),
		RemovedBytes:   p.removedBytes,
		InvalidEntries: p.invalidEntries,
		Entries:        entries,
	}
}

func normalizeCleanupMode(mode CleanupMode) (CleanupMode, error) {
	if mode == "" {
		return CleanupModeStale, nil
	}
	switch mode {
	case CleanupModeStale, CleanupModeInvalid, CleanupModeAll, CleanupModePartialDownloads:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported cache cleanup mode %q", mode)
	}
}

func isEntryStale(entry cacheIndexEntry, cfg CacheConfig, now time.Time) bool {
	if entry.LastUsedAt.IsZero() {
		return true
	}
	cutoff := now.Add(-time.Duration(cfg.RetentionDays) * 24 * time.Hour)

	return entry.LastUsedAt.Before(cutoff)
}

func (c *Cache) cleanupStaleIfDue(index *cacheIndex) error {
	if !index.LastCleanup.IsZero() &&
		c.clock.Now().Sub(index.LastCleanup) < automaticCleanupInterval {
		return nil
	}
	cfg, _, err := LoadCacheConfig(c.configPath)
	if err != nil {
		return err
	}
	plan, err := c.planCleanup(*index, cfg, CleanOptions{Mode: CleanupModeStale})
	if err != nil {
		return err
	}
	if err := c.removeEntries(index, plan); err != nil {
		return err
	}
	index.LastCleanup = c.clock.Now().UTC()

	return nil
}

func (c *Cache) removeEntries(index *cacheIndex, plan cleanupPlan) error {
	for _, candidate := range plan.candidates {
		if err := os.RemoveAll(c.absolutePath(candidate.entry.EntryPath)); err != nil {
			return err
		}
		delete(index.Entries, candidate.id)
	}

	return nil
}

func (p partialDownloadPlan) summary(dryRun bool) CleanSummary {
	return CleanSummary{
		Mode:           CleanupModePartialDownloads,
		DryRun:         dryRun,
		RemovedEntries: len(p.candidates),
		RemovedBytes:   p.removedBytes,
	}
}

func removePartialDownloads(plan partialDownloadPlan) error {
	for _, candidate := range plan.candidates {
		if err := os.RemoveAll(candidate.path); err != nil {
			return err
		}
	}

	return nil
}

func (c *Cache) planPartialDownloadCleanup() (partialDownloadPlan, error) {
	plan := partialDownloadPlan{}
	root := c.downloadsRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return plan, nil
		}

		return plan, err
	}
	slices.SortFunc(entries, func(left, right os.DirEntry) int {
		return strings.Compare(left.Name(), right.Name())
	})

	plan.candidates = make([]partialDownloadCandidate, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		size, err := directorySize(path)
		if err != nil {
			return partialDownloadPlan{}, err
		}
		plan.removedBytes += size
		plan.candidates = append(plan.candidates, partialDownloadCandidate{path: path})
	}

	return plan, nil
}

// wipeCacheContents removes cache contents, including unindexed files, while
// preserving lock state for the running operation and resetting artifact
// metadata.
func (c *Cache) wipeCacheContents(index *cacheIndex) error {
	entries, err := os.ReadDir(c.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			index.Entries = map[string]cacheIndexEntry{}
			return nil
		}

		return err
	}

	for _, entry := range entries {
		if directorymutex.IsMarkerName(entry.Name()) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(c.root, entry.Name())); err != nil {
			return err
		}
	}
	index.Entries = map[string]cacheIndexEntry{}

	return nil
}

func sortedEntryIDs(index cacheIndex) []string {
	keys := make([]string, 0, len(index.Entries))
	for key := range index.Entries {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	return keys
}

func directorySize(path string) (int64, error) {
	var size int64
	err := filepath.WalkDir(path, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		size += info.Size()

		return nil
	})

	return size, err
}
