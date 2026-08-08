package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/adapters/sqlite"
	"github.com/infopek/news-aggregator/internal/application"
	localranking "github.com/infopek/news-aggregator/internal/application/ranking"
	"github.com/infopek/news-aggregator/internal/domain"
)

type rankingGolden struct {
	AlgorithmVersion    string `json:"algorithmVersion"`
	TokenizationVersion string `json:"tokenizationVersion"`
	TokenizationCases   []struct {
		Input    string   `json:"input"`
		Expected []string `json:"expected"`
	} `json:"tokenizationCases"`
	Interests            []domain.WeightedInterest                               `json:"interests"`
	Articles             []struct{ ID, Title, Excerpt, Body, Permission string } `json:"articles"`
	ExpectedOrder        []string                                                `json:"expectedOrder"`
	ExpectedMatchedTerms map[string][]string                                     `json:"expectedMatchedTerms"`
	ExpectedCorpus       localranking.CorpusStats                                `json:"expectedCorpus"`
}

func TestRankingGoldenAndDatabaseReloadDeterminism(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "ranking", "golden.json"))
	must(t, err)
	var fixture rankingGolden
	must(t, json.Unmarshal(data, &fixture))
	if fixture.AlgorithmVersion != localranking.AlgorithmVersion || fixture.TokenizationVersion != localranking.TokenizationVersion {
		t.Fatal("fixture version drift")
	}
	for _, test := range fixture.TokenizationCases {
		if got := localranking.Tokenize(test.Input); !reflect.DeepEqual(got, test.Expected) {
			t.Fatalf("tokenize %q=%v want %v", test.Input, got, test.Expected)
		}
	}

	store, path := openStore(t)
	ctx := context.Background()
	source := feedSource("ranking-source", "https://example.com/ranking-feed")
	must(t, store.Sources().Save(ctx, source))
	for _, item := range fixture.Articles {
		article := domain.Article{ID: domain.ArticleID(item.ID), Fingerprint: "fp-" + item.ID, SourceID: source.ID, CanonicalURL: "https://example.com/" + item.ID, Title: item.Title, Excerpt: item.Excerpt, FullContent: item.Body, ContentPermission: domain.ContentPermission(item.Permission), FetchedAt: time.Unix(1, 0).UTC()}
		// A metadata-only fixture may include hostile residual body to prove the
		// scorer ignores it; persistence correctly refuses to store that field.
		if article.ContentPermission == domain.ContentMetadataOnly {
			article.FullContent = ""
		}
		_, err = store.Articles().Upsert(ctx, article)
		must(t, err)
	}
	stored, err := store.Articles().ListForRanking(ctx)
	must(t, err)
	if got := localranking.NewIndex(stored).Stats(); !reflect.DeepEqual(got, fixture.ExpectedCorpus) {
		t.Fatalf("corpus stats=%+v want %+v", got, fixture.ExpectedCorpus)
	}
	first := rankArticles(t, stored, fixture.Interests)
	must(t, store.Close())
	store = reopenStore(t, path)
	defer store.Close()
	second := rankStored(t, store, fixture.Interests)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("reload changed results\n%+v\n%+v", first, second)
	}

	sort.SliceStable(first, func(i, j int) bool {
		if first[i].Score == first[j].Score {
			return first[i].ArticleID < first[j].ArticleID
		}
		return first[i].Score > first[j].Score
	})
	gotOrder := make([]string, len(first))
	for i, result := range first {
		gotOrder[i] = string(result.ArticleID)
	}
	if !reflect.DeepEqual(gotOrder, fixture.ExpectedOrder) {
		t.Fatalf("golden order=%v want %v", gotOrder, fixture.ExpectedOrder)
	}
	for _, result := range first {
		got := result.Contributions[0].ReasonValues["matched_terms"]
		want := fixture.ExpectedMatchedTerms[string(result.ArticleID)]
		if got != joinTerms(want) {
			t.Fatalf("article %s matches=%q want %v", result.ArticleID, got, want)
		}
	}
}

func TestCombinedRankingPersistsReconciledComponentsAcrossReload(t *testing.T) {
	store, path := openStore(t)
	ctx := context.Background()
	source := feedSource("combined-ranking-source", "https://example.com/combined-feed")
	must(t, store.Sources().Save(ctx, source))
	calculated := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	article := domain.Article{ID: "combined-article", Fingerprint: "combined-fp", SourceID: source.ID, CanonicalURL: "https://example.com/combined", Title: "Combined", FetchedAt: calculated, ContentPermission: domain.ContentMetadataOnly}
	_, err := store.Articles().Upsert(ctx, article)
	must(t, err)
	configuration := domain.RankingConfiguration{
		Interest: domain.SignalWeight{Enabled: true, Weight: .8},
		Age:      domain.SignalWeight{Enabled: true, Weight: .1}, Gender: domain.SignalWeight{Enabled: true, Weight: .1},
		PerDemographicCap: .1, TotalDemographicCap: .2, NormalizationVersion: "v1",
	}
	input := localranking.AggregateInput{Configuration: configuration, CalculatedAt: calculated, Candidates: []localranking.Candidate{{
		ArticleID: article.ID,
		Signals:   []localranking.SignalResult{{Signal: domain.SignalInterest, Score: .75, ReasonCode: localranking.ReasonInterestMatch, ReasonValues: map[string]string{"matched_interests": "science"}}},
		Age:       localranking.SignalResult{Signal: domain.SignalAge, Score: 1, ReasonCode: localranking.ReasonAgeAdjustment, ReasonValues: map[string]string{"source": "explicit_profile"}},
	}}}
	service := localranking.RankingService{Repository: store.Rankings()}
	results, err := service.RankAndSave(ctx, input)
	must(t, err)
	if len(results) != 1 {
		t.Fatalf("results=%d", len(results))
	}
	must(t, store.Close())
	store = reopenStore(t, path)
	defer store.Close()
	persisted, err := store.Rankings().GetResult(ctx, article.ID)
	must(t, err)
	if !reflect.DeepEqual(persisted, results[0]) {
		t.Fatalf("reload changed result:\n%+v\n%+v", results[0], persisted)
	}
	sum := 0.0
	for _, contribution := range persisted.Contributions {
		sum += contribution.WeightedScore
	}
	if difference := sum - persisted.Score; difference > 1e-12 || difference < -1e-12 {
		t.Fatalf("persisted contributions do not reconcile: sum=%v score=%v", sum, persisted.Score)
	}
}

func TestRankingRepositoryRejectsNonReconcilingOrNonFiniteResults(t *testing.T) {
	store, _ := openStore(t)
	defer store.Close()
	ctx := context.Background()
	source := feedSource("ranking-validation-source", "https://example.com/validation-feed")
	must(t, store.Sources().Save(ctx, source))
	now := time.Unix(100, 0).UTC()
	article := domain.Article{ID: "ranking-validation-article", Fingerprint: "validation-fp", SourceID: source.ID, CanonicalURL: "https://example.com/validation", Title: "Validation", FetchedAt: now, ContentPermission: domain.ContentMetadataOnly}
	_, err := store.Articles().Upsert(ctx, article)
	must(t, err)
	valid := domain.RankingResult{ArticleID: article.ID, Score: .5, AlgorithmVersion: "test", CalculatedAt: now, Contributions: []domain.ScoreContribution{{Signal: domain.SignalInterest, RawScore: .5, Weight: 1, WeightedScore: .5, ReasonCode: "test", ReasonValues: map[string]string{}}}}
	tests := []domain.RankingResult{
		{ArticleID: article.ID, Score: .5, AlgorithmVersion: "test", CalculatedAt: now},
		func() domain.RankingResult { value := valid; value.Score = .4; return value }(),
		func() domain.RankingResult {
			value := valid
			value.Contributions = append([]domain.ScoreContribution(nil), valid.Contributions...)
			value.Contributions[0].WeightedScore = .4
			return value
		}(),
		func() domain.RankingResult { value := valid; value.Score = math.NaN(); return value }(),
	}
	for i, value := range tests {
		if err := store.Rankings().SaveResults(ctx, []domain.RankingResult{value}); !errors.Is(err, application.ErrInvalidInput) {
			t.Fatalf("case %d error=%v", i, err)
		}
	}
	valid.CalculatedAt = time.Time{}
	if err := store.Rankings().SaveResults(ctx, []domain.RankingResult{valid}); err != nil {
		t.Fatalf("RANK-001-compatible zero calculation time rejected: %v", err)
	}
}

func rankStored(t *testing.T, store *sqlite.Store, interests []domain.WeightedInterest) []domain.RankingResult {
	t.Helper()
	articles, err := store.Articles().ListForRanking(context.Background())
	must(t, err)
	return rankArticles(t, articles, interests)
}

func rankArticles(t *testing.T, articles []domain.Article, interests []domain.WeightedInterest) []domain.RankingResult {
	t.Helper()
	results, err := (localranking.Ranker{}).Rank(context.Background(), application.RankingInput{Articles: articles, Profile: domain.UserProfile{Interests: interests}, Configuration: domain.RankingConfiguration{TextSimilarity: domain.SignalWeight{Enabled: true, Weight: .8}}})
	must(t, err)
	return results
}

func joinTerms(terms []string) string {
	result := ""
	for i, term := range terms {
		if i > 0 {
			result += ","
		}
		result += term
	}
	return result
}
