package spec_import

import "context"

// MarkdownAdapter passes the raw body through verbatim.
type MarkdownAdapter struct{}

func (m *MarkdownAdapter) Source() Source { return SourceMarkdown }

func (m *MarkdownAdapter) Fetch(_ context.Context, in Input) (FetchedSpec, error) {
	return FetchedSpec{RawText: in.Body}, nil
}
