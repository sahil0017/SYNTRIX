package research

import (
	"fmt"
	"hash/fnv"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/syntrix/runtime/internal/security"
)

const EmbedDim = 256

type Chunk struct {
	DocPath string    `json:"doc_path"`
	Title   string    `json:"title"`
	Index   int       `json:"index"`
	Text    string    `json:"text"`
	Vector  []float32 `json:"-"`
}

type SearchHit struct {
	DocPath string  `json:"doc_path"`
	Title   string  `json:"title"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
}

type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type Engine struct {
	mu     sync.RWMutex
	chunks []Chunk
	edges  []GraphEdge
}

func NewEngine() *Engine {
	return &Engine{}
}

// IngestDocument sanitizes untrusted text, chunks it, embeds each chunk, and updates the knowledge graph.
func (e *Engine) IngestDocument(docPath, title, rawContent string) int {
	sanitized, _ := security.SanitizeUntrustedText(rawContent)
	parts := chunkText(sanitized, 500, 80)

	e.mu.Lock()
	defer e.mu.Unlock()

	// Remove previous chunks/edges for this doc
	filtered := e.chunks[:0]
	for _, c := range e.chunks {
		if c.DocPath != docPath {
			filtered = append(filtered, c)
		}
	}
	e.chunks = filtered

	filteredEdges := e.edges[:0]
	for _, ed := range e.edges {
		if ed.From != title {
			filteredEdges = append(filteredEdges, ed)
		}
	}
	e.edges = filteredEdges

	for i, part := range parts {
		e.chunks = append(e.chunks, Chunk{
			DocPath: docPath,
			Title:   title,
			Index:   i,
			Text:    part,
			Vector:  EmbedText(part),
		})
	}

	concepts := extractConcepts(sanitized, 12)
	for _, concept := range concepts {
		e.edges = append(e.edges, GraphEdge{
			From: title,
			To:   concept,
			Kind: "mentions",
		})
	}

	return len(parts)
}

// SemanticSearch ranks chunks by hybrid cosine similarity + lexical term match against query.
func (e *Engine) SemanticSearch(query string, topK int) []SearchHit {
	if topK <= 0 {
		topK = 5
	}
	qVec := EmbedText(query)
	terms := strings.Fields(strings.ToLower(query))

	e.mu.RLock()
	defer e.mu.RUnlock()

	hits := make([]SearchHit, 0, len(e.chunks))
	for _, c := range e.chunks {
		score := CosineSimilarity(qVec, c.Vector)
		if len(terms) > 0 {
			lowerText := strings.ToLower(c.Text)
			lowerTitle := strings.ToLower(c.Title)
			matched := 0
			for _, t := range terms {
				if len(t) < 2 {
					continue
				}
				if strings.Contains(lowerTitle, t) {
					score += 0.18
					matched++
				} else if strings.Contains(lowerText, t) {
					score += 0.08
					matched++
				}
			}
			if matched == len(terms) && len(terms) > 1 {
				score += 0.12
			}
		}
		if score > 0.05 {
			if score > 1.0 {
				score = 1.0
			}
			hits = append(hits, SearchHit{
				DocPath: c.DocPath,
				Title:   c.Title,
				Score:   score,
				Snippet: c.Text,
			})
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	// Stage-2 Reranker: Maximal Marginal Relevance (MMR) + exact-phrase boost to reduce duplicate chunks from same doc.
	lowerQ := strings.ToLower(strings.TrimSpace(query))
	reranked := make([]SearchHit, 0, topK)
	docSeen := make(map[string]int, len(hits))
	for _, h := range hits {
		score := h.Score
		if lowerQ != "" && strings.Contains(strings.ToLower(h.Snippet), lowerQ) {
			score = minFloat(1.0, score+0.10)
		}
		if cnt := docSeen[h.DocPath]; cnt > 0 {
			score *= 1.0 / (1.0 + 0.25*float64(cnt))
		}
		docSeen[h.DocPath]++
		h.Score = score
		reranked = append(reranked, h)
	}
	sort.Slice(reranked, func(i, j int) bool { return reranked[i].Score > reranked[j].Score })
	if len(reranked) > topK {
		reranked = reranked[:topK]
	}
	return reranked
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// Stats returns the number of distinct documents, vector chunks, and knowledge graph edges.
func (e *Engine) Stats() (docs int, chunks int, edges int) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	seen := make(map[string]struct{}, len(e.chunks))
	for _, c := range e.chunks {
		seen[c.DocPath] = struct{}{}
	}
	return len(seen), len(e.chunks), len(e.edges)
}

// TopConcepts returns the most frequently linked concepts in the knowledge graph.
func (e *Engine) TopConcepts(limit int) []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	counts := make(map[string]int)
	for _, ed := range e.edges {
		if ed.To != "" {
			counts[ed.To]++
		}
	}
	type kv struct {
		name  string
		count int
	}
	list := make([]kv, 0, len(counts))
	for k, v := range counts {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].count == list[j].count {
			return list[i].name < list[j].name
		}
		return list[i].count > list[j].count
	})
	if limit <= 0 || limit > len(list) {
		limit = len(list)
	}
	out := make([]string, limit)
	for i := 0; i < limit; i++ {
		out[i] = list[i].name
	}
	return out
}

func (e *Engine) KnowledgeGraphSummary(limit int) []GraphEdge {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if limit <= 0 || limit > len(e.edges) {
		limit = len(e.edges)
	}
	out := make([]GraphEdge, limit)
	copy(out, e.edges[:limit])
	return out
}

// BuildLearningPath creates a structured 4-stage learning path from indexed knowledge.
func (e *Engine) BuildLearningPath(topic string) string {
	hits := e.SemanticSearch(topic, 4)
	edges := e.KnowledgeGraphSummary(10)

	var concepts []string
	seen := make(map[string]bool)
	for _, ed := range edges {
		if !seen[ed.To] {
			seen[ed.To] = true
			concepts = append(concepts, ed.To)
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Adaptive Learning Path: %s\n", topic))
	b.WriteString("1. Foundations — Core terminology & architecture\n")
	if len(concepts) > 0 {
		b.WriteString("   Key concepts: " + strings.Join(concepts[:minInt(5, len(concepts))], ", ") + "\n")
	}
	b.WriteString("2. Deep Dive — Primary source passages\n")
	for i, h := range hits {
		snip := strings.ReplaceAll(strings.TrimSpace(h.Snippet), "\n", " ")
		if len(snip) > 100 {
			snip = snip[:100] + "..."
		}
		b.WriteString(fmt.Sprintf("   • [%s] (score=%.2f): %s\n", h.Title, h.Score, snip))
		if i >= 2 {
			break
		}
	}
	b.WriteString("3. Synthesis — Connect concepts across documents\n")
	b.WriteString("4. Practice — Run /top, /optimize-memory, or inspect source modules to verify understanding.")
	return strings.TrimSpace(b.String())
}

func EmbedText(text string) []float32 {
	vec := make([]float32, EmbedDim)
	lower := strings.ToLower(text)
	tokens := strings.Fields(lower)
	for _, tok := range tokens {
		addFeature(vec, tok, 1.5)
		if len(tok) >= 3 {
			for i := 0; i <= len(tok)-3; i++ {
				addFeature(vec, tok[i:i+3], 0.5)
			}
		}
	}
	normalize(vec)
	return vec
}

func addFeature(vec []float32, feat string, weight float32) {
	h := fnv.New32a()
	_, _ = h.Write([]byte(feat))
	idx := h.Sum32() % EmbedDim
	vec[idx] += weight
}

func normalize(vec []float32) {
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return
	}
	inv := float32(1.0 / math.Sqrt(sum))
	for i := range vec {
		vec[i] *= inv
	}
}

func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot
}

func chunkText(text string, size, overlap int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if len(text) <= size {
		return []string{text}
	}
	var chunks []string
	step := size - overlap
	if step <= 0 {
		step = size
	}
	for start := 0; start < len(text); start += step {
		end := start + size
		if end > len(text) {
			end = len(text)
		}
		chunks = append(chunks, text[start:end])
		if end == len(text) {
			break
		}
	}
	return chunks
}

var conceptRe = regexp.MustCompile(`\b[A-Z][A-Za-z0-9_]{3,}\b`)

func extractConcepts(text string, limit int) []string {
	matches := conceptRe.FindAllString(text, -1)
	counts := make(map[string]int)
	for _, m := range matches {
		counts[m]++
	}
	type kv struct {
		k string
		v int
	}
	var list []kv
	for k, v := range counts {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })
	if len(list) > limit {
		list = list[:limit]
	}
	out := make([]string, len(list))
	for i, item := range list {
		out[i] = item.k
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
