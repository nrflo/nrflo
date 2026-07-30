package tools_web

import "context"

// fallbackSearch chains two providers: the primary's result wins unless it
// errors or comes back empty — an empty set from a degraded meta-engine (all
// upstreams suspended) is indistinguishable from "no hits", so both cases
// fall through to the secondary. A secondary failure never masks the
// primary's error.
type fallbackSearch struct {
	primary   SearchProvider
	secondary SearchProvider
}

func (f *fallbackSearch) Name() string { return f.primary.Name() + "+" + f.secondary.Name() }

func (f *fallbackSearch) Search(ctx context.Context, query string, opts SearchOpts) ([]Result, error) {
	results, err := f.primary.Search(ctx, query, opts)
	if err == nil && len(results) > 0 {
		return results, nil
	}
	fbResults, fbErr := f.secondary.Search(ctx, query, opts)
	if fbErr == nil && len(fbResults) > 0 {
		return fbResults, nil
	}
	if err != nil {
		return nil, err
	}
	return results, nil
}
