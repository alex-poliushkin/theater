package theater

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestResourceScopeGetOrCreateInitializesSameKeyOnceConcurrently(t *testing.T) {
	t.Parallel()

	scope := NewResourceScope()
	key := NewResourceKey("test/runtime", "shared")
	shared := &resourceScopeTestValue{id: "shared"}

	var calls atomic.Int32
	var created sync.Once
	start := make(chan struct{})
	factoryStarted := make(chan struct{})
	release := make(chan struct{})
	results := make(chan any, 8)

	for range 8 {
		go func() {
			<-start
			results <- scope.GetOrCreate(key, func() any {
				calls.Add(1)
				created.Do(func() {
					close(factoryStarted)
				})
				<-release
				return shared
			})
		}()
	}

	close(start)

	select {
	case <-factoryStarted:
	case <-time.After(time.Second):
		t.Fatal("shared factory did not start")
	}

	close(release)

	for range 8 {
		select {
		case got := <-results:
			if got != shared {
				t.Fatalf("shared value mismatch: got %v want %v", got, shared)
			}
		case <-time.After(time.Second):
			t.Fatal("concurrent lookup did not finish")
		}
	}

	if got, want := calls.Load(), int32(1); got != want {
		t.Fatalf("factory call count mismatch: got %d want %d", got, want)
	}
}

func TestResourceScopeGetOrCreateDoesNotBlockOtherKeysOnSlowFactory(t *testing.T) {
	t.Parallel()

	scope := NewResourceScope()
	slowKey := NewResourceKey("test/runtime", "slow")
	fastKey := NewResourceKey("test/runtime", "fast")
	slowValue := &resourceScopeTestValue{id: "slow"}
	fastValue := &resourceScopeTestValue{id: "fast"}

	slowStarted := make(chan struct{})
	releaseSlow := make(chan struct{})
	slowDone := make(chan any, 1)
	go func() {
		slowDone <- scope.GetOrCreate(slowKey, func() any {
			close(slowStarted)
			<-releaseSlow
			return slowValue
		})
	}()

	select {
	case <-slowStarted:
	case <-time.After(time.Second):
		t.Fatal("slow factory did not start")
	}

	fastDone := make(chan any, 1)
	go func() {
		fastDone <- scope.GetOrCreate(fastKey, func() any {
			return fastValue
		})
	}()

	select {
	case got := <-fastDone:
		if got != fastValue {
			t.Fatalf("fast value mismatch: got %v want %v", got, fastValue)
		}
	case <-time.After(time.Second):
		t.Fatal("different-key lookup blocked behind slow factory")
	}

	close(releaseSlow)

	select {
	case got := <-slowDone:
		if got != slowValue {
			t.Fatalf("slow value mismatch: got %v want %v", got, slowValue)
		}
	case <-time.After(time.Second):
		t.Fatal("slow lookup did not finish")
	}
}

func TestResourceScopeGetOrCreateAllowsReentrantDifferentKeyLookup(t *testing.T) {
	t.Parallel()

	scope := NewResourceScope()
	outerKey := NewResourceKey("test/runtime", "outer")
	innerKey := NewResourceKey("test/runtime", "inner")
	innerValue := &resourceScopeTestValue{id: "inner"}

	done := make(chan any, 1)
	go func() {
		done <- scope.GetOrCreate(outerKey, func() any {
			return scope.GetOrCreate(innerKey, func() any {
				return innerValue
			})
		})
	}()

	select {
	case got := <-done:
		if got != innerValue {
			t.Fatalf("reentrant value mismatch: got %v want %v", got, innerValue)
		}
	case <-time.After(time.Second):
		t.Fatal("reentrant lookup deadlocked")
	}

	gotInner := scope.GetOrCreate(innerKey, func() any {
		t.Fatal("inner key should already be initialized")
		return nil
	})
	if gotInner != innerValue {
		t.Fatalf("inner cached value mismatch: got %v want %v", gotInner, innerValue)
	}
}

type resourceScopeTestValue struct {
	id string
}
