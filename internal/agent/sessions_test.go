package agent

import (
	"sync"
	"testing"
)

func TestSessionStoreEmpty(t *testing.T) {
	store, err := OpenSessionStore(t.TempDir() + "/missing.json")
	if err != nil {
		t.Fatalf("OpenSessionStore: %v", err)
	}
	if got := store.Get("main"); got != "" {
		t.Errorf("Get on empty store = %q, want empty", got)
	}
	if len(store.All()) != 0 {
		t.Errorf("All on empty store = %v, want empty", store.All())
	}
}

func TestSessionStoreSetGet(t *testing.T) {
	path := t.TempDir() + "/conversations.json"
	store, err := OpenSessionStore(path)
	if err != nil {
		t.Fatalf("OpenSessionStore: %v", err)
	}

	if err := store.Set("main", "sess-1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := store.Get("main"); got != "sess-1" {
		t.Errorf("Get(main) = %q, want sess-1", got)
	}
	if got := store.Get("other"); got != "" {
		t.Errorf("Get(other) = %q, want empty", got)
	}

	// Reopen and confirm persistence.
	reopened, err := OpenSessionStore(path)
	if err != nil {
		t.Fatalf("reopen OpenSessionStore: %v", err)
	}
	if got := reopened.Get("main"); got != "sess-1" {
		t.Errorf("persisted Get(main) = %q, want sess-1", got)
	}
	if all := reopened.All(); len(all) != 1 || all["main"] != "sess-1" {
		t.Errorf("persisted All = %v, want {main: sess-1}", all)
	}
}

func TestSessionStoreOverwrite(t *testing.T) {
	store, err := OpenSessionStore(t.TempDir() + "/c.json")
	if err != nil {
		t.Fatalf("OpenSessionStore: %v", err)
	}
	if err := store.Set("main", "a"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("main", "b"); err != nil {
		t.Fatal(err)
	}
	if got := store.Get("main"); got != "b" {
		t.Errorf("Get after overwrite = %q, want b", got)
	}
}

func TestSessionStoreConcurrentAccess(t *testing.T) {
	store, err := OpenSessionStore(t.TempDir() + "/c.json")
	if err != nil {
		t.Fatalf("OpenSessionStore: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if err := store.Set("conv", "sess"); err != nil {
				t.Errorf("Set: %v", err)
			}
			_ = store.Get("conv")
			_ = store.All()
		}(i)
	}
	wg.Wait()
	if got := store.Get("conv"); got != "sess" {
		t.Errorf("Get after concurrent writes = %q, want sess", got)
	}
}
