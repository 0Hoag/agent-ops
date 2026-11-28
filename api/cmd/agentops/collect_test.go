package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestReadOpenCodeSQLiteIsReadOnlyAndMapsTokens(t *testing.T) {
	db := filepath.Join(t.TempDir(), "opencode.db")
	schema := `CREATE TABLE message(id TEXT PRIMARY KEY,session_id TEXT,data TEXT); CREATE TABLE part(message_id TEXT,time_created INTEGER,data TEXT); INSERT INTO message VALUES('m1','s1','{"model":"openai/test"}'); INSERT INTO part VALUES('m1',1761526800000,'{"type":"step-finish","tokens":{"input":7,"output":3,"reasoning":2,"cache":{"read":1,"write":0}}}');`
	if err := exec.Command("sqlite3", db, schema).Run(); err != nil {
		t.Skipf("sqlite3 unavailable: %v", err)
	}
	sessions, err := readOpenCodeSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].ID != "s1" || sessions[0].Usage.Input != 7 || sessions[0].Usage.Output != 3 || sessions[0].Usage.CachedInput != 1 {
		t.Fatalf("sessions=%+v", sessions)
	}
	if _, err := os.Stat(db); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("sqlite3", "-readonly", db, "CREATE TABLE should_fail(x);").Run(); err == nil {
		t.Fatal("read-only control unexpectedly allowed a write")
	}
}
