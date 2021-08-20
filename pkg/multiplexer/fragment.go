package multiplexer

import "context"

type FragmentContextKey struct{}

type FragmentRequest struct {
	Url         string
	Metadata    map[string]string
	timingLabel string
}

func FragmentFromContext(ctx context.Context) *FragmentRequest {
	if ctx == nil {
		return nil
	}

	if fragment := ctx.Value(FragmentContextKey{}); fragment != nil {
		fragment := fragment.(FragmentRequest)
		return &fragment
	}
	return nil
}
