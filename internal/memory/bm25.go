package memory

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Recency stage: freshly-updated nodes gain a bounded multiplicative boost
// that halves every recencyHalfLife and decays smoothly to nothing. The
// multiplier lives in (1.0, 1+recencyBoostMax], so freshness breaks ties but
// can never overturn a meaningful BM25 gap, and old canonical knowledge is
// never penalized below its base score.
const recencyBoostMax = 0.10
const recencyHalfLife = 30 * 24 * time.Hour

// BM25 over node title+content+tags. Self-contained copy of the scoring
// scheme proven in internal/agent/retrieval.go (k1=1.5, b=0.75) — not
// imported, so this package stays a leaf free of the agent dependency.

var memTokenRe = regexp.MustCompile(`[^a-z0-9]+`)

func memTokenize(text string) []string {
	parts := memTokenRe.Split(strings.ToLower(text), -1)
	out := parts[:0]
	for _, p := range parts {
		if len(p) > 0 {
			out = append(out, p)
		}
	}
	return out
}

func nodeIndexText(n *Node) string {
	return n.Title + "\n" + n.Content + "\n" + strings.Join(n.Tags, " ")
}

// rankNodes scores pool against query, applies mild priority and recency
// boosts (priority 5 is neutral; each step is ±5%), and returns the top
// limit results in descending score order. now is the view's reference time,
// so as-of queries rank recency relative to the reconstructed instant.
func rankNodes(query string, pool []*Node, limit int, now time.Time) []Scored {
	queryTerms := memTokenize(query)
	if len(queryTerms) == 0 || len(pool) == 0 {
		return nil
	}

	type doc struct {
		node     *Node
		termFreq map[string]int
		length   int
	}
	docs := make([]doc, 0, len(pool))
	df := make(map[string]int)
	totalLen := 0
	for _, n := range pool {
		tokens := memTokenize(nodeIndexText(n))
		tf := make(map[string]int, len(tokens))
		for _, t := range tokens {
			tf[t]++
		}
		for term := range tf {
			df[term]++
		}
		totalLen += len(tokens)
		docs = append(docs, doc{node: n, termFreq: tf, length: len(tokens)})
	}
	avgDL := float64(totalLen) / float64(len(docs))

	const k1 = 1.5
	const b = 0.75

	results := make([]Scored, 0, len(docs))
	for _, d := range docs {
		score := 0.0
		for _, term := range queryTerms {
			tf := float64(d.termFreq[term])
			if tf == 0 {
				continue
			}
			dfTerm := float64(df[term])
			idf := math.Log(1 + (float64(len(docs))-dfTerm+0.5)/(dfTerm+0.5))
			dl := float64(d.length)
			tfNorm := tf * (k1 + 1) / (tf + k1*(1-b+b*dl/avgDL))
			score += idf * tfNorm
		}
		if score == 0 {
			continue
		}
		score *= 1 + 0.05*float64(d.node.Priority-5)
		age := now.Sub(d.node.Updated)
		if age < 0 {
			age = 0
		}
		score *= 1 + recencyBoostMax*math.Exp(-math.Ln2*float64(age)/float64(recencyHalfLife))
		results = append(results, Scored{Node: *d.node, Score: score})
	}
	applyEdgeBoost(results)
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

// applyEdgeBoost rewards graph connectivity among the candidates: when two
// matches are linked, both endpoints gain edgeBoostFactor × link weight ×
// the other's base score. Bonuses are computed from a snapshot of the base
// scores so iteration order doesn't compound. Nodes outside the candidate
// set contribute nothing — a link is only evidence when both ends matched
// the query.
const edgeBoostFactor = 0.1

func applyEdgeBoost(results []Scored) {
	if len(results) < 2 {
		return
	}
	base := make(map[string]float64, len(results))
	for _, r := range results {
		base[r.ID] = r.Score
	}
	bonus := make(map[string]float64, len(results))
	for _, r := range results {
		for _, l := range r.Node.Links {
			other, ok := base[l.Target]
			if !ok || l.Target == r.ID {
				continue
			}
			bonus[r.ID] += edgeBoostFactor * l.Weight * other
			bonus[l.Target] += edgeBoostFactor * l.Weight * base[r.ID]
		}
	}
	for i := range results {
		results[i].Score += bonus[results[i].ID]
	}
}
