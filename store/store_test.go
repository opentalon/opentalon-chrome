package store

import (
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := OpenSQLite(t.TempDir())
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(db)
}

// --- Save / Get ---

func TestStore_SaveAndGet(t *testing.T) {
	s := openTestStore(t)
	cookies := `[{"name":"sid","value":"abc"}]`
	if err := s.Save("entity1", "myapp-work", cookies); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Get("entity1", "myapp-work")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != cookies {
		t.Errorf("Get = %q, want %q", got, cookies)
	}
}

func TestStore_Save_Upsert(t *testing.T) {
	s := openTestStore(t)
	_ = s.Save("entity1", "myapp", `["old"]`)
	if err := s.Save("entity1", "myapp", `["new"]`); err != nil {
		t.Fatalf("Save upsert: %v", err)
	}
	got, err := s.Get("entity1", "myapp")
	if err != nil {
		t.Fatalf("Get after upsert: %v", err)
	}
	if got != `["new"]` {
		t.Errorf("after upsert Get = %q, want [\"new\"]", got)
	}
}

func TestStore_Get_NotFound(t *testing.T) {
	s := openTestStore(t)
	_, err := s.Get("entity1", "missing")
	if err == nil {
		t.Error("expected error for missing credential, got nil")
	}
}

func TestStore_Save_EmptyEntityID(t *testing.T) {
	s := openTestStore(t)
	if err := s.Save("", "name", "[]"); err == nil {
		t.Error("expected error for empty entity_id")
	}
}

func TestStore_Save_EmptyName(t *testing.T) {
	s := openTestStore(t)
	if err := s.Save("entity1", "", "[]"); err == nil {
		t.Error("expected error for empty name")
	}
}

// --- List ---

func TestStore_List(t *testing.T) {
	s := openTestStore(t)
	_ = s.Save("entity1", "b-account", `[]`)
	_ = s.Save("entity1", "a-account", `[]`)
	_ = s.Save("entity2", "other", `[]`) // different entity — must not appear

	names, err := s.List("entity1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("List returned %d names, want 2", len(names))
	}
	// Results must be sorted alphabetically.
	if names[0] != "a-account" || names[1] != "b-account" {
		t.Errorf("List = %v, want [a-account b-account]", names)
	}
}

func TestStore_List_Empty(t *testing.T) {
	s := openTestStore(t)
	names, err := s.List("nobody")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("List for unknown entity = %v, want empty", names)
	}
}

// --- Delete ---

func TestStore_Delete(t *testing.T) {
	s := openTestStore(t)
	_ = s.Save("entity1", "myapp", `[]`)
	if err := s.Delete("entity1", "myapp"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := s.Get("entity1", "myapp")
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestStore_Delete_Nonexistent(t *testing.T) {
	s := openTestStore(t)
	// Deleting a record that doesn't exist should not return an error.
	if err := s.Delete("entity1", "ghost"); err != nil {
		t.Errorf("Delete nonexistent: unexpected error: %v", err)
	}
}

// --- Entity isolation ---

func TestStore_EntityIsolation(t *testing.T) {
	s := openTestStore(t)
	_ = s.Save("alice", "myapp", `["alice"]`)
	_ = s.Save("bob", "myapp", `["bob"]`)

	aliceCookies, err := s.Get("alice", "myapp")
	if err != nil {
		t.Fatalf("Get alice: %v", err)
	}
	bobCookies, err := s.Get("bob", "myapp")
	if err != nil {
		t.Fatalf("Get bob: %v", err)
	}
	if aliceCookies == bobCookies {
		t.Error("entity isolation failed: alice and bob share the same value")
	}
	if aliceCookies != `["alice"]` {
		t.Errorf("alice: got %q, want [\"alice\"]", aliceCookies)
	}
	if bobCookies != `["bob"]` {
		t.Errorf("bob: got %q, want [\"bob\"]", bobCookies)
	}
}

// --- DB / migrations ---

func TestDB_ReopenMigrationsIdempotent(t *testing.T) {
	dir := t.TempDir()
	db1, err := OpenSQLite(dir)
	if err != nil {
		t.Fatalf("first OpenSQLite: %v", err)
	}
	_ = db1.Close()

	db2, err := OpenSQLite(dir)
	if err != nil {
		t.Fatalf("second OpenSQLite (should be idempotent): %v", err)
	}
	_ = db2.Close()
}
