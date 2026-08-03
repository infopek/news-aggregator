// Package ranking implements deterministic, local-only text similarity.
//
// TokenizationVersion lowercases Unicode with Go's locale-independent case
// mapping, then extracts contiguous Unicode letter and number runs. It does not
// perform stemming, stop-word removal, language detection, or Unicode
// normalization; canonically equivalent composed and decomposed input can
// therefore tokenize differently. Invalid UTF-8 bytes are separators.
package ranking

import (
	"context"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

const (
	AlgorithmVersion    = "bm25f-local-v1"
	TokenizationVersion = "unicode-letters-numbers-lower-v1"
	TitleWeight         = 3.0
	ExcerptWeight       = 2.0
	ContentWeight       = 1.0
	MaxFieldRunes       = 100_000
	MaxTokensPerField   = 50_000
	bm25K1              = 1.2
	bm25B               = 0.75
)

type document struct {
	id     domain.ArticleID
	terms  map[string]float64
	length float64
}

// Index is a replaceable, immutable corpus snapshot. It contains only derived
// statistics and can always be rebuilt from locally stored permitted fields.
type Index struct {
	documents []document
	df        map[string]int
	average   float64
}

// CorpusStats is a read-only, machine-readable view of derived index state.
type CorpusStats struct {
	DocumentCount     int
	AverageLength     float64
	DocumentFrequency map[string]int
}

// Stats returns a copy; callers cannot mutate the index through it.
func (index *Index) Stats() CorpusStats {
	frequency := make(map[string]int, len(index.df))
	for term, count := range index.df {
		frequency[term] = count
	}
	return CorpusStats{DocumentCount: len(index.documents), AverageLength: index.average, DocumentFrequency: frequency}
}

// NewIndex builds corpus statistics from title, excerpt, and permitted body.
// FullContent is ignored unless the article explicitly permits full content.
func NewIndex(articles []domain.Article) *Index {
	index := &Index{df: make(map[string]int)}
	ordered := append([]domain.Article(nil), articles...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	for _, article := range ordered {
		terms := make(map[string]float64)
		addTerms(terms, Tokenize(article.Title), TitleWeight)
		addTerms(terms, Tokenize(article.Excerpt), ExcerptWeight)
		if article.ContentPermission == domain.ContentFullAllowed {
			addTerms(terms, Tokenize(article.FullContent), ContentWeight)
		}
		length := 0.0
		for term, frequency := range terms {
			length += frequency
			if frequency > 0 {
				index.df[term]++
			}
		}
		index.documents = append(index.documents, document{id: article.ID, terms: terms, length: length})
		index.average += length
	}
	if len(index.documents) > 0 {
		index.average /= float64(len(index.documents))
	}
	return index
}

func addTerms(target map[string]float64, tokens []string, weight float64) {
	for _, token := range tokens {
		target[token] += weight
	}
}

// Tokenize applies the documented bounded tokenizer. Very long fields are
// truncated deterministically by rune and token count.
func Tokenize(text string) []string {
	tokens := make([]string, 0)
	var current []rune
	seenRunes := 0
	flush := func() {
		if len(current) > 0 && len(tokens) < MaxTokensPerField {
			tokens = append(tokens, string(current))
		}
		current = current[:0]
	}
	for len(text) > 0 && seenRunes < MaxFieldRunes && len(tokens) < MaxTokensPerField {
		r, size := utf8.DecodeRuneInString(text)
		text = text[size:]
		seenRunes++
		if r == utf8.RuneError && size == 1 {
			flush()
			continue
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			current = append(current, unicode.ToLower(r))
		} else {
			flush()
		}
	}
	flush()
	return tokens
}

// Ranker implements application.Ranker without network or process capabilities.
type Ranker struct{}

var _ application.Ranker = Ranker{}

func (Ranker) Rank(ctx context.Context, input application.RankingInput) ([]domain.RankingResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	index := NewIndex(input.Articles)
	query := queryTerms(input.Profile.Interests)
	results := make([]domain.RankingResult, 0, len(index.documents))
	weight := input.Configuration.TextSimilarity.Weight
	if !input.Configuration.TextSimilarity.Enabled || !finiteUnit(weight) {
		weight = 0
	}
	for _, doc := range index.documents {
		raw, matches := index.score(doc, query)
		weighted := clamp(raw * weight)
		values := map[string]string{
			"algorithm_version":    AlgorithmVersion,
			"tokenization_version": TokenizationVersion,
			"matched_terms":        strings.Join(matches, ","),
			"title_weight":         strconv.FormatFloat(TitleWeight, 'f', 1, 64),
			"excerpt_weight":       strconv.FormatFloat(ExcerptWeight, 'f', 1, 64),
			"content_weight":       strconv.FormatFloat(ContentWeight, 'f', 1, 64),
		}
		results = append(results, domain.RankingResult{
			ArticleID:        doc.id,
			Score:            weighted,
			Contributions:    []domain.ScoreContribution{{Signal: domain.SignalTextSimilarity, RawScore: raw, Weight: weight, WeightedScore: weighted, ReasonCode: "local_text_match", ReasonValues: values}},
			AlgorithmVersion: AlgorithmVersion,
		})
	}
	return results, nil
}

func queryTerms(interests []domain.WeightedInterest) map[string]float64 {
	query := make(map[string]float64)
	for _, interest := range interests {
		if !finiteUnit(interest.Weight) {
			continue
		}
		for _, token := range Tokenize(interest.Name) {
			query[token] += interest.Weight
		}
	}
	return query
}

func (index *Index) score(doc document, query map[string]float64) (float64, []string) {
	if len(index.documents) == 0 || len(query) == 0 || doc.length == 0 || index.average == 0 {
		return 0, nil
	}
	var score float64
	matches := make([]string, 0)
	queryKeys := make([]string, 0, len(query))
	for term := range query {
		queryKeys = append(queryKeys, term)
	}
	sort.Strings(queryKeys)
	for _, term := range queryKeys {
		queryWeight := query[term]
		tf := doc.terms[term]
		if tf == 0 || queryWeight == 0 {
			continue
		}
		df := float64(index.df[term])
		idf := math.Log(1 + (float64(len(index.documents))-df+0.5)/(df+0.5))
		norm := tf + bm25K1*(1-bm25B+bm25B*doc.length/index.average)
		if norm > 0 {
			score += queryWeight * idf * (tf * (bm25K1 + 1) / norm)
			matches = append(matches, term)
		}
	}
	// Saturation maps any finite non-negative BM25 value into [0,1).
	if score <= 0 || math.IsNaN(score) || math.IsInf(score, 0) {
		return 0, matches
	}
	return clamp(score / (1 + score)), matches
}

func finiteUnit(value float64) bool {
	return value >= 0 && value <= 1 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func clamp(value float64) float64 {
	if math.IsNaN(value) || value <= 0 {
		return 0
	}
	if math.IsInf(value, 0) || value >= 1 {
		return 1
	}
	return value
}
