package notifier

import (
	"context"
	"reflect"
	"sync"
)

// Function signature used for Around subscriptions
type AroundHandler = func(context.Context, func(ctx context.Context))

// Function signature used for On subscriptions
type OnHandler = func(context.Context)

// The Notifier interface is used to define Notifiers.
type Notifier interface {
	Around(name interface{}, handler AroundHandler)
	On(name interface{}, handler OnHandler)
	RemoveOn(name interface{}, handler OnHandler)
	RemoveAround(name interface{}, handler AroundHandler)
	Emit(name interface{}, ctx context.Context, f func(context.Context))
}

type nullNotifier struct{}

var _ Notifier = (*nullNotifier)(nil)

// NullNotifier satisfies the Notifiable interface but does not persist Around
// or On subscriptions.
var NullNotifier = &nullNotifier{}

func (n *nullNotifier) Around(name interface{}, handler AroundHandler)       {}
func (n *nullNotifier) On(name interface{}, handler OnHandler)               {}
func (n *nullNotifier) RemoveOn(name interface{}, handler OnHandler)         {}
func (n *nullNotifier) RemoveAround(name interface{}, handler AroundHandler) {}
func (n *nullNotifier) Emit(name interface{}, ctx context.Context, handler func(context.Context)) {
	handler(ctx)
}

// DefaultNotifier exposes hooks to subscribe and emit notifications that pass a
// context.Context value allowing for easy implementation of custom logging,
// observability, and other use-cases.
type DefaultNotifier struct {
	aroundSubscriptions map[interface{}][]AroundHandler
	onSubscriptions     map[interface{}][]OnHandler

	mu sync.Mutex
}

var _ Notifier = (*DefaultNotifier)(nil)

// New returns an empty DefaultNotifier.
func New() *DefaultNotifier {
	return &DefaultNotifier{
		aroundSubscriptions: make(map[interface{}][]AroundHandler),
		onSubscriptions:     make(map[interface{}][]OnHandler),
	}
}

// Emit calls each subscription for the given name synchronously.
//
// Around subscriptions can pass a context to the provided callback that will
// be passed to the next subscription if there is one, otherwise it is passed
// to f.
func (n *DefaultNotifier) Emit(name interface{}, ctx context.Context, f func(ctx context.Context)) {
	if subscriptions, ok := n.onSubscriptions[name]; ok {
		for _, subscription := range subscriptions {
			subscription(ctx)
		}
	}

	chain := f
	if subscriptions, ok := n.aroundSubscriptions[name]; ok {
		for i := len(subscriptions) - 1; i != -1; i-- {
			subscription := subscriptions[i]
			last := chain
			chain = func(ctx context.Context) {
				subscription(ctx, last)
			}
		}

	}

	chain(ctx)
}

// Around defines a function to run around an event when the given name is
// emitted. In handler, a context.Context can be passed to the provided
// callback which will be passed to either the next subscription if there is
// one, or the function provided to Emit.
//
// The provided handler should handle panics to ensure the underlying Emit
// function is called.
func (n *DefaultNotifier) Around(name interface{}, handler AroundHandler) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if _, ok := n.aroundSubscriptions[name]; !ok {
		n.aroundSubscriptions[name] = make([]AroundHandler, 0, 2)
	}

	n.aroundSubscriptions[name] = append(n.aroundSubscriptions[name], handler)
}

// On defines a function to run when an event with the given `name`. It is
// provided a context.Context to use, but cannot modify it.
//
// The provided handler must not panic, or should handle recovery itself.
func (n *DefaultNotifier) On(name interface{}, handler OnHandler) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if _, ok := n.onSubscriptions[name]; !ok {
		n.onSubscriptions[name] = make([]OnHandler, 0, 2)
	}

	n.onSubscriptions[name] = append(n.onSubscriptions[name], handler)
}

// RemoveOn removes the On subscription for the given name and handler.
func (n *DefaultNotifier) RemoveOn(name interface{}, handler OnHandler) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if subscriptions, ok := n.onSubscriptions[name]; ok {
		for i, subscription := range n.onSubscriptions[name] {
			if reflect.ValueOf(handler).Pointer() == reflect.ValueOf(subscription).Pointer() {
				n.onSubscriptions[name] = append(subscriptions[:i], subscriptions[i+1:]...)

				if len(n.onSubscriptions[name]) == 0 {
					delete(n.onSubscriptions, name)
				}
			}
		}
	}
}

// RemoveAround removes the Around subscription for the given name and handler.
func (n *DefaultNotifier) RemoveAround(name interface{}, handler AroundHandler) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if subscriptions, ok := n.aroundSubscriptions[name]; ok {
		for i, subscription := range n.aroundSubscriptions[name] {
			if reflect.ValueOf(handler).Pointer() == reflect.ValueOf(subscription).Pointer() {
				n.aroundSubscriptions[name] = append(subscriptions[:i], subscriptions[i+1:]...)

				if len(n.aroundSubscriptions[name]) == 0 {
					delete(n.aroundSubscriptions, name)
				}
			}
		}
	}
}
