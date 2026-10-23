package secrets

import (
	"encoding/json"
	"sort"

	"github.com/dont-be-evil-company/withsecrets/internal/lib/cache"
)

type pathMappingKind string

const (
	pathMappingKindSecret pathMappingKind = "secret-path"
	pathMappingKindParam  pathMappingKind = "param-path"
)

// resolvedPathEntry is one environment variable produced by expanding a path mapping.
type resolvedPathEntry struct {
	EnvironmentVariable string
	Value               string
	SourceRef           string
}

type capturedPathMapping struct {
	overlay  map[string]resolvedPathEntry
	complete bool
}

type pathMappingIdentity struct {
	Kind       string   `json:"kind"`
	Provider   string   `json:"provider"`
	Project    string   `json:"project"`
	Region     string   `json:"region"`
	MappingKey string   `json:"mapping_key"`
	Paths      []string `json:"paths"`
}

func pathMappingCacheID(kind pathMappingKind, key fetchKey, mappingKey string, paths []string) string {
	ident := pathMappingIdentity{
		Kind:       string(kind),
		Provider:   key.provider,
		Project:    key.project,
		Region:     key.region,
		MappingKey: mappingKey,
		Paths:      append([]string(nil), paths...),
	}
	data, err := json.Marshal(ident)
	if err != nil {
		return string(kind) + "|" + key.provider + "|" + key.project + "|" + key.region + "|" + mappingKey
	}
	return string(data)
}

// resolvePathEntries derives environment variable names from provider secrets under basePath.
// When mappingKey is "*", each relative name is used as the env var directly;
// otherwise names are prefixed with mappingKey + "_".
func resolvePathEntries(mappingKey, basePath string, secrets map[string]string) []resolvedPathEntry {
	entries := make([]resolvedPathEntry, 0, len(secrets))
	for originalName, secretValue := range secrets {
		rel := relativeEnvVarName(basePath, originalName)
		envName := rel
		if mappingKey != "*" {
			envName = mappingKey + "_" + rel
		}
		entries = append(entries, resolvedPathEntry{
			EnvironmentVariable: envName,
			Value:               secretValue,
			SourceRef:           originalName,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].EnvironmentVariable != entries[j].EnvironmentVariable {
			return entries[i].EnvironmentVariable < entries[j].EnvironmentVariable
		}
		return entries[i].SourceRef < entries[j].SourceRef
	})
	return entries
}

func mergeResolvedPathEntries(allSecrets, sourceRefs map[string]string, entries []resolvedPathEntry) {
	for _, entry := range entries {
		allSecrets[entry.EnvironmentVariable] = entry.Value
		if sourceRefs != nil {
			sourceRefs[entry.EnvironmentVariable] = entry.SourceRef
		}
	}
}

func overlayPathEntries(overlay map[string]resolvedPathEntry, entries []resolvedPathEntry) {
	for _, entry := range entries {
		overlay[entry.EnvironmentVariable] = entry
	}
}

func capturedEntries(captured capturedPathMapping) []resolvedPathEntry {
	entries := make([]resolvedPathEntry, 0, len(captured.overlay))
	for _, entry := range captured.overlay {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].EnvironmentVariable < entries[j].EnvironmentVariable
	})
	return entries
}

func toCachedPathMapping(entries []resolvedPathEntry, allSecrets map[string]string) cache.CachedPathMapping {
	cached := make([]cache.CachedPathEntry, 0, len(entries))
	for _, entry := range entries {
		value := entry.Value
		if v, ok := allSecrets[entry.EnvironmentVariable]; ok {
			value = v
		}
		cached = append(cached, cache.CachedPathEntry{
			EnvironmentVariable: entry.EnvironmentVariable,
			SourceRef:           entry.SourceRef,
			Value:               value,
		})
	}
	return cache.CachedPathMapping{
		Version: cache.PathMappingVersion,
		Entries: cached,
	}
}

func applyCachedPathEntries(allSecrets, sourceRefs map[string]string, mapping cache.CachedPathMapping) {
	for _, entry := range mapping.Entries {
		allSecrets[entry.EnvironmentVariable] = entry.Value
		if sourceRefs != nil && entry.SourceRef != "" {
			sourceRefs[entry.EnvironmentVariable] = entry.SourceRef
		}
	}
}
