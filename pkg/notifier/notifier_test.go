package notifier

import (
	"fmt"
	"testing"
	"time"

	"context"

	"github.com/stretchr/testify/require"
)

func TestNotifier_On(t *testing.T) {
	done := make(chan struct{}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*200)
	defer cancel()
	called := false

	notifier := New()
	notifier.On("test", func(c context.Context) {
		close(done)
	})

	notifier.Emit("test", ctx, func(ctx context.Context) {
		called = true
	})

	select {
	case <-done:
		require.True(t, called, "expected handler to be called")
	case <-ctx.Done():
		require.Fail(t, ctx.Err().Error())
	}
}

func TestNotifier_Around_PassesContext(t *testing.T) {
	done := make(chan struct{}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*200)
	defer cancel()

	notifier := New()

	notifier.Around("test", func(ctx context.Context, f func(ctx context.Context)) {
		ctx = context.WithValue(ctx, "first", 1)
		f(ctx)
	})

	notifier.Around("test", func(ctx context.Context, f func(ctx context.Context)) {
		ctx = context.WithValue(ctx, "second", 2)
		f(ctx)
	})

	notifier.Emit("test", ctx, func(ctx context.Context) {
		require.Equal(t, 1, ctx.Value("first"))
		require.Equal(t, 2, ctx.Value("second"))
		close(done)
	})

	select {
	case <-done:
	case <-ctx.Done():
		require.Fail(t, ctx.Err().Error())
	}
}

func TestNotifier_Around_Order(t *testing.T) {
	messages := make([]string, 0)

	notifier := New()

	notifier.Around("test", func(ctx context.Context, f func(ctx context.Context)) {
		messages = append(messages, "before first log")
		f(ctx)
		messages = append(messages, "after first log")
	})

	notifier.Around("test", func(ctx context.Context, f func(ctx context.Context)) {
		messages = append(messages, "before second log")
		f(ctx)
		messages = append(messages, "after second log")
	})

	notifier.Around("test", func(ctx context.Context, f func(ctx context.Context)) {
		messages = append(messages, "before third log")
		f(ctx)
		messages = append(messages, "after third log")
	})

	notifier.Emit("test", context.Background(), func(ctx context.Context) {
		messages = append(messages, "executing")
	})

	require.Equal(t, []string{
		"before first log",
		"before second log",
		"before third log",
		"executing",
		"after third log",
		"after second log",
		"after first log",
	}, messages)
}

func TestRemove(t *testing.T) {
	notifier := New()

	handler := func(ctx context.Context) {}

	notifier.On("ignore", handler)
	notifier.On("test", handler)
	require.Len(t, notifier.onSubscriptions["test"], 1)

	notifier.RemoveOn("test", handler)
	require.Len(t, notifier.onSubscriptions["test"], 0)
	require.Len(t, notifier.onSubscriptions, 1)
}

func TestRemoveAround(t *testing.T) {
	notifier := New()

	handler := func(ctx context.Context, f func(context.Context)) {}

	notifier.Around("ignore", handler)
	notifier.Around("test", handler)
	require.Len(t, notifier.aroundSubscriptions["test"], 1)

	notifier.RemoveAround("test", handler)
	require.Len(t, notifier.aroundSubscriptions["test"], 0)
	require.Len(t, notifier.aroundSubscriptions, 1)
}

func Example() {
	notifier := New()

	notifier.On("test", func(ctx context.Context) {
		fmt.Println("running on")
	})

	notifier.Around("test", func(ctx context.Context, f func(ctx context.Context)) {
		fmt.Println("first before")
		f(ctx)
		fmt.Println("first after")
	})

	notifier.Around("test", func(ctx context.Context, f func(ctx context.Context)) {
		fmt.Println("second before")
		ctx = context.WithValue(ctx, "test value", "testing")
		f(ctx)
		fmt.Println("second after")
	})

	notifier.Around("test", func(ctx context.Context, f func(ctx context.Context)) {
		fmt.Println("third before")
		f(ctx)
		fmt.Println("third after")
	})

	notifier.Emit("test", context.Background(), func(ctx context.Context) {
		fmt.Printf("executing with value: %s\n", ctx.Value("test value"))
	})

	// Output:
	// running on
	// first before
	// second before
	// third before
	// executing with value: testing
	// third after
	// second after
	// first after

}
