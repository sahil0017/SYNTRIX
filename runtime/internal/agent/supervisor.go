package agent

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/syntrix/runtime/internal/events"
	"github.com/syntrix/runtime/internal/intelligence"
	"github.com/syntrix/runtime/internal/llm"
	"github.com/syntrix/runtime/internal/research"
	"github.com/syntrix/runtime/internal/security"
	"github.com/syntrix/runtime/internal/store"
	"github.com/syntrix/runtime/internal/tools"
)

type WorkflowInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Steps       []string `json:"steps"`
}

func BuiltinWorkflows() []WorkflowInfo {
	return []WorkflowInfo{
		{
			ID:          "mac-health-audit",
			Name:        "Mac Health & Memory Audit",
			Description: "Collects live CPU/RAM/thermal/disk telemetry, runs optimization analysis, and proposes gated recommendations.",
			Steps:       []string{"Collect C17 Snapshot", "Analyze Memory Bottlenecks", "Policy Risk Validation"},
		},
		{
			ID:          "cache-storage-audit",
			Name:        "Cache & Storage Inspection",
			Description: "Inspects disk usage, ~/Library/Caches, and startup LaunchAgents read-only.",
			Steps:       []string{"Read Disk Metrics", "Scan ~/Library/Caches", "Inspect Startup Agents"},
		},
		{
			ID:          "dev-workspace-check",
			Name:        "Developer Workspace Index",
			Description: "Indexes local docs, PDFs, and code into SQLite + Vector RAG + Knowledge Graph.",
			Steps:       []string{"Walk Permitted Workspace", "Extract PDFs & Sanitize", "Build Vector Embeddings & Graph"},
		},
	}
}

type Supervisor struct {
	bus       *events.Bus
	registry  *tools.Registry
	store     *store.Store
	companion *Companion
	rag       *research.Engine
	llm       llm.Provider
}

func NewSupervisor(bus *events.Bus, reg *tools.Registry, st *store.Store, comp *Companion) *Supervisor {
	s := &Supervisor{
		bus:       bus,
		registry:  reg,
		store:     st,
		companion: comp,
		rag:       research.NewEngine(),
		llm:       llm.NewLocalOllama(""),
	}
	s.registerDefaultTools()
	return s
}

func (s *Supervisor) RAG() *research.Engine {
	return s.rag
}

func (s *Supervisor) registerDefaultTools() {
	if s.registry == nil {
		return
	}
	s.registry.Register(tools.Tool{
		Name:        "system.status",
		Description: "Collect live CPU, memory, thermal, disk, power, and process summary",
		Risk:        tools.RiskReadOnly,
		Handler: func(args map[string]string) (string, error) {
			return s.companion.toolStatus(), nil
		},
	})
	s.registry.Register(tools.Tool{
		Name:        "system.top_processes",
		Description: "List top CPU and memory consuming processes",
		Risk:        tools.RiskReadOnly,
		Handler: func(args map[string]string) (string, error) {
			return s.companion.toolTopProcesses(), nil
		},
	})
	s.registry.Register(tools.Tool{
		Name:        "optimization.scan",
		Description: "Run anomaly & memory optimization scan",
		Risk:        tools.RiskReadOnly,
		Handler: func(args map[string]string) (string, error) {
			return s.companion.toolScan(), nil
		},
	})
	s.registry.Register(tools.Tool{
		Name:        "storage.inspect_caches",
		Description: "Read-only inspection of ~/Library/Caches",
		Risk:        tools.RiskReadOnly,
		Handler: func(args map[string]string) (string, error) {
			return s.companion.toolInspectCaches(), nil
		},
	})
	s.registry.Register(tools.Tool{
		Name:        "hardening.startup_agents",
		Description: "List user and system LaunchAgents read-only",
		Risk:        tools.RiskReadOnly,
		Handler: func(args map[string]string) (string, error) {
			return s.InspectStartup(), nil
		},
	})
	s.registry.Register(tools.Tool{
		Name:        "hardening.duplicates",
		Description: "Find duplicate files in a directory using size + SHA-256",
		Risk:        tools.RiskReadOnly,
		Handler: func(args map[string]string) (string, error) {
			dir := args["dir"]
			if dir == "" {
				dir = "."
			}
			return s.FindDuplicates(dir)
		},
	})
	s.registry.Register(tools.Tool{
		Name:        "developer.inspect",
		Description: "DeveloperAgent: inspect git status and workspace layout",
		Risk:        tools.RiskReadOnly,
		Handler: func(args map[string]string) (string, error) {
			return s.DeveloperInspect(args["dir"]), nil
		},
	})
	s.registry.Register(tools.Tool{
		Name:        "learning.path",
		Description: "LearningAgent: generate adaptive learning path from indexed knowledge",
		Risk:        tools.RiskReadOnly,
		Handler: func(args map[string]string) (string, error) {
			return s.rag.BuildLearningPath(args["topic"]), nil
		},
	})
	s.registry.Register(tools.Tool{
		Name:        "research.index_dir",
		Description: "Index local markdown/code/PDF files into SQLite & Vector RAG",
		Risk:        tools.RiskLow,
		Handler: func(args map[string]string) (string, error) {
			dir := args["dir"]
			if dir == "" {
				dir = "."
			}
			count, err := s.IndexDirectory(dir)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Indexed %d files (including PDFs/docs/code) from %s into SQLite & Vector RAG.", count, dir), nil
		},
	})
	s.registry.Register(tools.Tool{
		Name:        "research.search",
		Description: "Semantic vector search + Knowledge Graph lookup",
		Risk:        tools.RiskReadOnly,
		Handler: func(args map[string]string) (string, error) {
			return s.SearchKnowledge(args["query"])
		},
	})
}

func (s *Supervisor) IndexDirectory(root string) (int, error) {
	if s.store == nil {
		return 0, fmt.Errorf("store unavailable")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return 0, err
	}

	allowedExt := map[string]bool{
		".md": true, ".txt": true, ".go": true, ".c": true, ".h": true, ".swift": true, ".pdf": true,
		".py": true, ".sh": true, ".json": true, ".yaml": true, ".yml": true,
	}
	indexed := 0
	redactions := 0
	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".build" || name == "build" || name == "node_modules" || name == ".venv" || name == "__pycache__" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if indexed >= 200 {
			return fs.SkipAll
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !allowedExt[ext] {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 5*1024*1024 {
			return nil
		}

		var rawText string
		if ext == ".pdf" {
			rawText, err = research.ExtractPDFText(path)
			if err != nil {
				return nil
			}
		} else {
			if info.Size() > 128*1024 {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			rawText = string(data)
		}

		sanitized, hits := security.SanitizeUntrustedText(rawText)
		redactions += hits

		if err := s.store.UpsertKnowledgeDoc(path, filepath.Base(path), sanitized); err == nil {
			s.rag.IngestDocument(path, filepath.Base(path), sanitized)
			indexed++
		}
		return nil
	})
	if s.store != nil {
		_ = s.store.AppendAuditLog("ResearchAgent", "index_directory", absRoot, "ok",
			fmt.Sprintf("indexed=%d sanitized_injections=%d", indexed, redactions))
	}
	if s.bus != nil {
		s.bus.Publish(events.Event{
			Type:    events.ActionExecuted,
			Source:  "ResearchAgent",
			Target:  "VectorRAG",
			Summary: fmt.Sprintf("Indexed %d docs/PDFs (%d injections sanitized)", indexed, redactions),
		})
	}
	return indexed, err
}

func (s *Supervisor) SearchKnowledge(query string) (string, error) {
	q := strings.TrimSpace(query)
	hits := s.rag.SemanticSearch(q, 5)
	edges := s.rag.KnowledgeGraphSummary(6)

	if s.store != nil {
		_ = s.store.AppendAuditLog("ResearchAgent", "semantic_search", q, "ok", fmt.Sprintf("hits=%d", len(hits)))
	}

	if len(hits) == 0 && s.store != nil {
		docs, err := s.store.SearchKnowledgeDocs(q, 5)
		if err == nil && len(docs) > 0 {
			var b strings.Builder
			b.WriteString(fmt.Sprintf("SQLite Knowledge Results for %q:\n", q))
			for i, d := range docs {
				clean := strings.ReplaceAll(strings.TrimSpace(d.Snippet), "\n", " ")
				if len(clean) > 120 {
					clean = clean[:120] + "..."
				}
				b.WriteString(fmt.Sprintf("%d. %s (%s)\n   %s\n", i+1, d.Title, d.Path, clean))
			}
			return strings.TrimSpace(b.String()), nil
		}
	}

	if len(hits) == 0 {
		return fmt.Sprintf("No indexed documents matched %q. Index a workspace directory or PDF folder first.", q), nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Semantic RAG Results for %q:\n", q))
	for i, h := range hits {
		clean := strings.ReplaceAll(strings.TrimSpace(h.Snippet), "\n", " ")
		if len(clean) > 130 {
			clean = clean[:130] + "..."
		}
		b.WriteString(fmt.Sprintf("%d. [%.2f] %s — %s\n", i+1, h.Score, h.Title, clean))
	}
	if len(edges) > 0 {
		b.WriteString("\nKnowledge Graph Links:\n")
		for _, ed := range edges {
			b.WriteString(fmt.Sprintf("• %s -[%s]-> %s\n", ed.From, ed.Kind, ed.To))
		}
	}

	// Optional local LLM synthesis if Ollama is running locally (fast 2s check).
	if s.llm != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if ans, err := s.llm.Complete(ctx, "Summarize the retrieved passages concisely.", b.String()); err == nil && strings.TrimSpace(ans) != "" {
			b.WriteString("\nLLM Synthesis (" + s.llm.Name() + "):\n" + strings.TrimSpace(ans))
		}
	}

	return strings.TrimSpace(b.String()), nil
}

func (s *Supervisor) InspectStartup() string {
	agents := intelligence.InspectStartupAgents()
	if s.store != nil {
		_ = s.store.AppendAuditLog("SystemAgent", "inspect_startup_agents", "LaunchAgents", "ok", fmt.Sprintf("count=%d", len(agents)))
	}
	if len(agents) == 0 {
		return "No user or system LaunchAgents found."
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Startup LaunchAgents (%d detected):\n", len(agents)))
	limit := minInt(10, len(agents))
	for i := 0; i < limit; i++ {
		b.WriteString(fmt.Sprintf("• [%s] %s\n", agents[i].Scope, agents[i].Name))
	}
	return strings.TrimSpace(b.String())
}

func (s *Supervisor) FindDuplicates(dir string) (string, error) {
	groups, err := intelligence.FindDuplicateFiles(dir, 4096, 8)
	if err != nil {
		return "", err
	}
	if s.store != nil {
		_ = s.store.AppendAuditLog("OptimizationAgent", "find_duplicates", dir, "ok", fmt.Sprintf("groups=%d", len(groups)))
	}
	if len(groups) == 0 {
		return fmt.Sprintf("No duplicate files (>= 4 KB) found in %s.", dir), nil
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Duplicate File Groups in %s:\n", dir))
	for i, g := range groups {
		b.WriteString(fmt.Sprintf("%d. %.1f KB x%d (%s):\n", i+1, float64(g.SizeBytes)/1024.0, len(g.Paths), g.Hash[:10]))
		for _, p := range g.Paths {
			b.WriteString("   - " + p + "\n")
		}
	}
	return strings.TrimSpace(b.String()), nil
}

func (s *Supervisor) DeveloperInspect(dir string) string {
	if dir == "" {
		dir = "."
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "status", "--short")
	out, err := cmd.Output()
	if s.store != nil {
		_ = s.store.AppendAuditLog("DeveloperAgent", "git_status", dir, "ok", "inspected workspace")
	}
	if err != nil {
		return fmt.Sprintf("DeveloperAgent: %s is not a git repository or git unavailable.", dir)
	}
	status := strings.TrimSpace(string(out))
	if status == "" {
		status = "clean working tree"
	}
	return fmt.Sprintf("DeveloperAgent [%s]:\n%s", dir, status)
}

func (s *Supervisor) RunWorkflow(workflowID string, workspaceDir string) (string, error) {
	if s.bus != nil {
		s.bus.Publish(events.Event{
			Type:    events.ActionExecutionRequested,
			Source:  "WorkflowAgent",
			Target:  "ToolRegistry",
			Summary: fmt.Sprintf("Starting workflow '%s'", workflowID),
		})
	}

	var out string
	switch workflowID {
	case "mac-health-audit":
		res1, _, err := s.registry.Execute("system.status", nil)
		if err != nil {
			return "", err
		}
		res2, _ := s.companion.HandlePrompt("/optimize-memory")
		out = res1 + "\n\n" + res2
	case "cache-storage-audit":
		res1, _, err := s.registry.Execute("storage.inspect_caches", nil)
		if err != nil {
			return "", err
		}
		res2, _, _ := s.registry.Execute("hardening.startup_agents", nil)
		out = res1 + "\n\n" + res2
	case "dev-workspace-check":
		res1, _, err := s.registry.Execute("research.index_dir", map[string]string{"dir": workspaceDir})
		if err != nil {
			return "", err
		}
		res2, _, _ := s.registry.Execute("developer.inspect", map[string]string{"dir": workspaceDir})
		out = res1 + "\n\n" + res2
	default:
		return "", fmt.Errorf("unknown workflow %q", workflowID)
	}

	if s.store != nil {
		_ = s.store.AppendAuditLog("WorkflowAgent", "run_workflow", workflowID, "ok", "completed workflow steps")
	}
	if s.bus != nil {
		s.bus.Publish(events.Event{
			Type:    events.ActionExecuted,
			Source:  "WorkflowAgent",
			Target:  "SwiftUI",
			Summary: fmt.Sprintf("Completed workflow '%s'", workflowID),
		})
	}
	return out, nil
}
