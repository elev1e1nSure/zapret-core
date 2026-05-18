package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// KnowledgeEntry records a strategy that worked for a given ASN
type KnowledgeEntry struct {
	Vector   StrategyVector `json:"vector"`
	ASN      string         `json:"asn"`
	Score    float64        `json:"score"`
	Hits     int            `json:"hits"` // times confirmed working
	LastSeen time.Time      `json:"last_seen"`
}

// Knowledge is the full persistent store
type Knowledge struct {
	Entries []KnowledgeEntry `json:"entries"`
	path    string
}

// knowledgePath returns absolute path to data/knowledge.json
func knowledgePath() string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	return filepath.Join(cwd, "data", "knowledge.json")
}

// LoadKnowledge reads knowledge.json from disk
// Returns empty Knowledge (not an error) if file doesn't exist yet
func LoadKnowledge() (*Knowledge, error) {
	path := knowledgePath()

	k := &Knowledge{path: path}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return k, nil
	}
	if err != nil {
		return k, fmt.Errorf("read knowledge: %w", err)
	}

	if err := json.Unmarshal(data, k); err != nil {
		return k, fmt.Errorf("parse knowledge: %w", err)
	}

	logInfo("[knowledge] loaded %d entries from %s", len(k.Entries), path)
	return k, nil
}

// Save writes current state to disk
func (k *Knowledge) Save() error {
	if err := os.MkdirAll(filepath.Dir(k.path), 0755); err != nil {
		return fmt.Errorf("create knowledge dir: %w", err)
	}

	data, err := json.MarshalIndent(k, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal knowledge: %w", err)
	}

	if err := os.WriteFile(k.path, data, 0644); err != nil {
		return fmt.Errorf("write knowledge: %w", err)
	}
	return nil
}

// Record saves a working strategy result
// If same vector+ASN already exists — updates score and hits
func (k *Knowledge) Record(asn string, v StrategyVector, score float64) {
	for i, e := range k.Entries {
		if e.ASN == asn && vectorsEqual(e.Vector, v) {
			k.Entries[i].Score = (e.Score*float64(e.Hits) + score) / float64(e.Hits+1)
			k.Entries[i].Hits++
			k.Entries[i].LastSeen = time.Now()
			if err := k.Save(); err != nil {
				logWarn("Failed to save knowledge: %v", err)
			}
			return
		}
	}

	k.Entries = append(k.Entries, KnowledgeEntry{
		Vector:   v,
		ASN:      asn,
		Score:    score,
		Hits:     1,
		LastSeen: time.Now(),
	})
	if err := k.Save(); err != nil {
		logWarn("Failed to save knowledge: %v", err)
	}
}

// BestForASN returns top N strategies for a given ASN, sorted by score*hits
// Falls back to top strategies across all ASNs if none found for this ASN
func (k *Knowledge) BestForASN(asn string, n int) []StrategyVector {
	var candidates []KnowledgeEntry

	for _, e := range k.Entries {
		if e.ASN == asn {
			candidates = append(candidates, e)
		}
	}

	// Fallback: use all entries if no ASN-specific data
	if len(candidates) == 0 {
		candidates = append(candidates, k.Entries...)
	}

	// Sort by score * log(hits+1) — prefer both high score and strategies confirmed multiple times
	sort.Slice(candidates, func(i, j int) bool {
		wi := candidates[i].Score * float64(candidates[i].Hits+1)
		wj := candidates[j].Score * float64(candidates[j].Hits+1)
		return wi > wj
	})

	result := []StrategyVector{}
	for i, c := range candidates {
		if i >= n {
			break
		}
		result = append(result, c.Vector)
	}

	logInfo("[knowledge] %d candidates for ASN %s (total entries: %d)", len(result), asn, len(k.Entries))
	return result
}

// vectorsEqual compares two StrategyVectors field by field
func vectorsEqual(a, b StrategyVector) bool {
	if a.DesyncMethod != b.DesyncMethod ||
		a.RepeatsTCP != b.RepeatsTCP ||
		a.RepeatsUDP != b.RepeatsUDP ||
		a.Fooling != b.Fooling ||
		a.SplitPos != b.SplitPos ||
		a.TLSMode != b.TLSMode ||
		a.TLSMod != b.TLSMod ||
		a.SeqOvl != b.SeqOvl ||
		a.SeqOvlPattern != b.SeqOvlPattern ||
		a.HostFakeMod != b.HostFakeMod ||
		a.Cutoff != b.Cutoff ||
		a.BadseqIncrement != b.BadseqIncrement ||
		a.QuicBin != b.QuicBin ||
		a.AnyProtocol != b.AnyProtocol ||
		a.IPID != b.IPID {
		return false
	}

	if len(a.TLSFiles) != len(b.TLSFiles) {
		return false
	}
	for i := range a.TLSFiles {
		if a.TLSFiles[i] != b.TLSFiles[i] {
			return false
		}
	}

	return true
}

// ExportFormat wraps knowledge entries with metadata for export/import
type ExportFormat struct {
	ExportedAt string           `json:"exported_at"`
	EntryCount int              `json:"entry_count"`
	Entries    []KnowledgeEntry `json:"entries"`
}

// normalizeASN removes "AS" prefix if present for consistent comparison
func normalizeASN(asn string) string {
	if len(asn) > 2 && asn[:2] == "AS" {
		return asn[2:]
	}
	return asn
}

// RemoveForASN removes all entries matching the given ASN
// Returns the number of entries removed
func (k *Knowledge) RemoveForASN(asn string) int {
	count := 0
	var newEntries []KnowledgeEntry
	targetASN := normalizeASN(asn)
	for _, e := range k.Entries {
		if normalizeASN(e.ASN) != targetASN {
			newEntries = append(newEntries, e)
		} else {
			count++
		}
	}
	k.Entries = newEntries
	return count
}

// MergeEntry merges a single entry into knowledge
// Returns "added", "skipped", or "updated" based on comparison logic
// Comparison: Score * Hits wins; if equal, newer LastSeen wins
func (k *Knowledge) MergeEntry(entry KnowledgeEntry) string {
	importWeight := entry.Score * float64(entry.Hits)
	targetASN := normalizeASN(entry.ASN)

	for i, e := range k.Entries {
		if normalizeASN(e.ASN) == targetASN {
			existingWeight := e.Score * float64(e.Hits)

			// Compare using same formula as BestForASN
			if importWeight > existingWeight {
				// Replace with better entry
				k.Entries[i] = entry
				return "updated"
			} else if importWeight == existingWeight {
				// Tie-break by timestamp
				if entry.LastSeen.After(e.LastSeen) {
					k.Entries[i] = entry
					return "updated"
				}
				return "skipped"
			}
			return "skipped"
		}
	}

	// No entry for this ASN - add new
	k.Entries = append(k.Entries, entry)
	return "added"
}

// Export returns current knowledge in export format
func (k *Knowledge) Export() ExportFormat {
	return ExportFormat{
		ExportedAt: time.Now().Format(time.RFC3339),
		EntryCount: len(k.Entries),
		Entries:    k.Entries,
	}
}

// Import merges all entries from export format into knowledge
// Returns counts of added, skipped, and updated entries
func (k *Knowledge) Import(ef ExportFormat) (added, skipped, updated int) {
	for _, entry := range ef.Entries {
		result := k.MergeEntry(entry)
		switch result {
		case "added":
			added++
		case "skipped":
			skipped++
		case "updated":
			updated++
		}
	}
	return
}
