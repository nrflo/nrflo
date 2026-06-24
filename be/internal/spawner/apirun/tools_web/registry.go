package tools_web

import "fmt"

// Name-keyed provider registries. Implementations register themselves in an
// init() so adding a provider is a new file, never a call-site edit.
type (
	searchCtor func(*Resolver) SearchProvider
	fetchCtor  func(*Resolver) FetchProvider
)

var (
	searchProviders = map[string]searchCtor{}
	fetchProviders  = map[string]fetchCtor{}
)

func registerSearch(name string, c searchCtor) { searchProviders[name] = c }
func registerFetch(name string, c fetchCtor)   { fetchProviders[name] = c }

func resolveSearch(name string, r *Resolver) (SearchProvider, error) {
	c, ok := searchProviders[name]
	if !ok {
		return nil, fmt.Errorf("unknown web_search_provider %q", name)
	}
	return c(r), nil
}

func resolveFetch(name string, r *Resolver) (FetchProvider, error) {
	c, ok := fetchProviders[name]
	if !ok {
		return nil, fmt.Errorf("unknown web_fetch_provider %q", name)
	}
	return c(r), nil
}
