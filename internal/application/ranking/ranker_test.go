package ranking

import (
	"context"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

func TestImplementationHasNoNetworkOrProcessImports(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	files, err := filepath.Glob(filepath.Join(filepath.Dir(current), "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Imports {
			name := strings.Trim(spec.Path.Value, `"`)
			if name == "net" || strings.HasPrefix(name, "net/") || name == "os/exec" {
				t.Fatalf("forbidden runtime capability %q in %s", name, filepath.Base(path))
			}
		}
	}
	if _, err := os.Stat(current); err != nil {
		t.Fatal(err)
	}
}

func TestTokenizeUnicodeCaseMalformedAndBounds(t *testing.T) {
	malformed := string([]byte{'A', 0xff, 'B'})
	if got, want := Tokenize("ÁRVÍZTŰRŐ 42 cats-cat"), []string{"árvíztűrő", "42", "cats", "cat"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Tokenize()=%q want %q", got, want)
	}
	if got, want := Tokenize(malformed), []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("malformed Tokenize()=%q want %q", got, want)
	}
	if got := Tokenize(strings.Repeat("x ", MaxTokensPerField+10)); len(got) != MaxTokensPerField {
		t.Fatalf("bounded tokens=%d", len(got))
	}
}

func TestRankGoldenOrderingReasonsAndPermission(t *testing.T) {
	input := fixtureInput()
	got, err := (Ranker{}).Rank(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !(got[0].Score > got[1].Score && got[1].Score > got[2].Score) {
		t.Fatalf("scores=%+v", got)
	}
	if got[0].Contributions[0].ReasonValues["matched_terms"] != "climate,science" {
		t.Fatalf("reasons=%v", got[0].Contributions[0].ReasonValues)
	}
	// Forbidden content contains both query terms but must never contribute.
	if got[2].Score != 0 || got[2].Contributions[0].ReasonValues["matched_terms"] != "" {
		t.Fatalf("metadata-only body affected score: %+v", got[2])
	}
}

func TestDeterministicAcrossCorpusOrderAndRepeatedBuilds(t *testing.T) {
	input := fixtureInput()
	want, _ := (Ranker{}).Rank(context.Background(), input)
	for i := 0; i < 50; i++ {
		input.Articles[0], input.Articles[2] = input.Articles[2], input.Articles[0]
		got, err := (Ranker{}).Rank(context.Background(), input)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d differs: %v\n%+v\n%+v", i, err, got, want)
		}
	}
}

func TestEmptyDisabledAndCommonTermsAreFiniteBounded(t *testing.T) {
	cases := []application.RankingInput{{}, fixtureInput(), {Articles: []domain.Article{{ID: "one", Title: "common common", ContentPermission: domain.ContentMetadataOnly}}, Profile: domain.UserProfile{Interests: []domain.WeightedInterest{{Name: "common", Weight: 1}}}, Configuration: domain.RankingConfiguration{TextSimilarity: domain.SignalWeight{Enabled: true, Weight: 1}}}}
	cases[1].Profile.Interests = nil
	for i, input := range cases {
		got, err := (Ranker{}).Rank(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		for _, result := range got {
			if math.IsNaN(result.Score) || math.IsInf(result.Score, 0) || result.Score < 0 || result.Score > 1 {
				t.Fatalf("case %d invalid score %v", i, result.Score)
			}
		}
	}
}

func FuzzRankFiniteBounded(f *testing.F) {
	f.Add("science", "SCIENCE science", "excerpt", "body", true, 1.0)
	f.Add(string([]byte{0xff}), "", "", string([]byte{0xfe}), false, 0.5)
	f.Fuzz(func(t *testing.T, interest, title, excerpt, body string, permitted bool, weight float64) {
		permission := domain.ContentMetadataOnly
		if permitted {
			permission = domain.ContentFullAllowed
		}
		input := application.RankingInput{Articles: []domain.Article{{ID: "a", Title: title, Excerpt: excerpt, FullContent: body, ContentPermission: permission}}, Profile: domain.UserProfile{Interests: []domain.WeightedInterest{{Name: interest, Weight: 1}}}, Configuration: domain.RankingConfiguration{TextSimilarity: domain.SignalWeight{Enabled: true, Weight: weight}}}
		got, err := (Ranker{}).Rank(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || math.IsNaN(got[0].Score) || math.IsInf(got[0].Score, 0) || got[0].Score < 0 || got[0].Score > 1 {
			t.Fatalf("invalid result %+v", got)
		}
	})
}

func fixtureInput() application.RankingInput {
	return application.RankingInput{
		Articles: []domain.Article{
			{ID: "a", Title: "Climate science", Excerpt: "science", ContentPermission: domain.ContentMetadataOnly},
			{ID: "b", Title: "Daily report", Excerpt: "climate", ContentPermission: domain.ContentMetadataOnly},
			{ID: "c", Title: "Sports", FullContent: "climate science", ContentPermission: domain.ContentMetadataOnly},
		},
		Profile:       domain.UserProfile{Interests: []domain.WeightedInterest{{Name: "science", Weight: 1}, {Name: "climate", Weight: .5}}},
		Configuration: domain.RankingConfiguration{TextSimilarity: domain.SignalWeight{Enabled: true, Weight: .8}},
	}
}
