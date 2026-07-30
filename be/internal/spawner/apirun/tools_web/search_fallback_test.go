package tools_web

import (
	"context"
	"errors"
	"testing"
)

type stubSearch struct {
	name    string
	results []Result
	err     error
	calls   int
}

func (s *stubSearch) Name() string { return s.name }
func (s *stubSearch) Search(context.Context, string, SearchOpts) ([]Result, error) {
	s.calls++
	return s.results, s.err
}

func TestFallbackSearch_PrimaryWinsWhenNonEmpty(t *testing.T) {
	primary := &stubSearch{name: "p", results: []Result{{URL: "https://a"}}}
	secondary := &stubSearch{name: "s", results: []Result{{URL: "https://b"}}}
	f := &fallbackSearch{primary: primary, secondary: secondary}

	got, err := f.Search(context.Background(), "q", SearchOpts{})
	if err != nil || len(got) != 1 || got[0].URL != "https://a" {
		t.Fatalf("got %v, %v; want primary result", got, err)
	}
	if secondary.calls != 0 {
		t.Fatal("secondary must not be called when primary succeeds")
	}
}

func TestFallbackSearch_EmptyPrimaryFallsThrough(t *testing.T) {
	primary := &stubSearch{name: "p"}
	secondary := &stubSearch{name: "s", results: []Result{{URL: "https://b"}}}
	f := &fallbackSearch{primary: primary, secondary: secondary}

	got, err := f.Search(context.Background(), "q", SearchOpts{})
	if err != nil || len(got) != 1 || got[0].URL != "https://b" {
		t.Fatalf("got %v, %v; want secondary result", got, err)
	}
}

func TestFallbackSearch_ErroredPrimaryFallsThrough(t *testing.T) {
	primary := &stubSearch{name: "p", err: errors.New("searxng down")}
	secondary := &stubSearch{name: "s", results: []Result{{URL: "https://b"}}}
	f := &fallbackSearch{primary: primary, secondary: secondary}

	got, err := f.Search(context.Background(), "q", SearchOpts{})
	if err != nil || len(got) != 1 || got[0].URL != "https://b" {
		t.Fatalf("got %v, %v; want secondary result", got, err)
	}
}

func TestFallbackSearch_SecondaryFailureKeepsPrimaryError(t *testing.T) {
	primaryErr := errors.New("searxng down")
	primary := &stubSearch{name: "p", err: primaryErr}
	secondary := &stubSearch{name: "s", err: errors.New("ddg captcha")}
	f := &fallbackSearch{primary: primary, secondary: secondary}

	_, err := f.Search(context.Background(), "q", SearchOpts{})
	if !errors.Is(err, primaryErr) {
		t.Fatalf("err = %v; want primary error", err)
	}
}

func TestFallbackSearch_BothEmptyIsEmptyNotError(t *testing.T) {
	f := &fallbackSearch{primary: &stubSearch{name: "p"}, secondary: &stubSearch{name: "s"}}
	got, err := f.Search(context.Background(), "q", SearchOpts{})
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v; want empty, nil", got, err)
	}
}

func TestResolverSearchProvider_DefaultChainsDDG(t *testing.T) {
	r := &Resolver{}
	p, err := r.SearchProvider()
	if err != nil {
		t.Fatalf("SearchProvider: %v", err)
	}
	if p.Name() != "searxng+ddg" {
		t.Fatalf("Name() = %q, want searxng+ddg", p.Name())
	}
}
