package multiplexer

import "context"

type FragmentContextKey struct{}

type Fragment struct {
	Url         string
	Metadata    map[string]string
	timingLabel string
}

func FragmentFromContext(ctx context.Context) *Fragment {
	if ctx == nil {
		return nil
	}

	if fragment := ctx.Value(FragmentContextKey{}); fragment != nil {
		fragment := fragment.(Fragment)
		return &fragment
	}
	return nil
}
