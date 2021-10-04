package multiplexer

import "context"

type RequestableContextKey struct{}

type Requestable interface {
	URL() string
	Metadata() map[string]string
}

func RequestableFromContext(ctx context.Context) Requestable {
	if ctx == nil {
		return nil
	}

	if requestable := ctx.Value(RequestableContextKey{}); requestable != nil {
		requestable := requestable.(Requestable)
		return requestable
	}
	return nil
}
