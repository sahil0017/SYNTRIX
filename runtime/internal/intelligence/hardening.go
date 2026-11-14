package intelligence

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type StartupAgent struct {
	Name  string `json:"name"`
	Scope string `json:"scope"`
	Path  string `json:"path"`
}

type DuplicateGroup struct {
	Hash      string   `json:"hash"`
	SizeBytes int64    `json:"size_bytes"`
	Paths     []string `json:"paths"`
}

// InspectStartupAgents lists user and system LaunchAgents read-only.
func InspectStartupAgents() []StartupAgent {
	var agents []StartupAgent
	home, _ := os.UserHomeDir()
	dirs := []struct {
		scope string
		path  string
	}{
		{"user", filepath.Join(home, "Library", "LaunchAgents")},
		{"system", "/Library/LaunchAgents"},
	}

	for _, d := range dirs {
		entries, err := os.ReadDir(d.path)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".plist") {
				continue
			}
			agents = append(agents, StartupAgent{
				Name:  strings.TrimSuffix(e.Name(), ".plist"),
				Scope: d.scope,
				Path:  filepath.Join(d.path, e.Name()),
			})
		}
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })
	return agents
}

// FindDuplicateFiles finds duplicate files >= minBytes under root using size+SHA256.
func FindDuplicateFiles(root string, minBytes int64, maxGroups int) ([]DuplicateGroup, error) {
	if minBytes <= 0 {
		minBytes = 4096
	}
	if maxGroups <= 0 {
		maxGroups = 15
	}

	bySize := make(map[int64][]string)
	visited := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".build" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if visited >= 10000 {
			return fs.SkipAll
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() < minBytes {
			return nil
		}
		visited++
		bySize[info.Size()] = append(bySize[info.Size()], path)
		return nil
	})
	if err != nil {
		return nil, err
	}

	var groups []DuplicateGroup
	for sz, paths := range bySize {
		if len(paths) < 2 {
			continue
		}
		byHash := make(map[string][]string)
		for _, p := range paths {
			h, err := hashFileSHA256(p)
			if err != nil {
				continue
			}
			byHash[h] = append(byHash[h], p)
		}
		for h, hPaths := range byHash {
			if len(hPaths) >= 2 {
				groups = append(groups, DuplicateGroup{
					Hash:      h,
					SizeBytes: sz,
					Paths:     hPaths,
				})
			}
		}
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].SizeBytes*int64(len(groups[i].Paths)) > groups[j].SizeBytes*int64(len(groups[j].Paths))
	})
	if len(groups) > maxGroups {
		groups = groups[:maxGroups]
	}
	return groups, nil
}

func hashFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
