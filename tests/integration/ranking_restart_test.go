package integration_test

import (
	"context"
	"encoding/json"
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
