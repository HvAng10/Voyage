package database

import (
	"path/filepath"
	"testing"
)

func TestDatabaseLifecycleAndMigrations(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_voyage_core.db")

	// 1. 测试打开和迁移
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if db.Path() != dbPath {
		t.Errorf("Expected db path %s, got %s", dbPath, db.Path())
	}

	// 2. 测试 schema_version 是否存在，证明 migrate 执行成功
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query schema_version: %v", err)
	}
	
	// 3. 测试基本的写操作 (Exec)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS test_db_table (id INTEGER PRIMARY KEY, name TEXT)`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	_, err = db.Exec(`INSERT INTO test_db_table (name) VALUES (?)`, "voyage_unit_test")
	if err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	// 4. 测试 ReadQuery (只读连接池)
	rows, err := db.ReadQuery(`SELECT name FROM test_db_table WHERE id = ?`, 1)
	if err != nil {
		t.Fatalf("ReadQuery failed: %v", err)
	}
	if !rows.Next() {
		t.Fatal("Expected 1 row, got 0")
	}
	var name string
	if err := rows.Scan(&name); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	rows.Close()
	if name != "voyage_unit_test" {
		t.Errorf("Expected 'voyage_unit_test', got %s", name)
	}

	// 5. 测试 ReadQueryRow
	var name2 string
	err = db.ReadQueryRow(`SELECT name FROM test_db_table WHERE id = ?`, 1).Scan(&name2)
	if err != nil {
		t.Fatalf("ReadQueryRow failed: %v", err)
	}
	if name2 != "voyage_unit_test" {
		t.Errorf("Expected 'voyage_unit_test', got %s", name2)
	}

	// 6. 关闭连接
	if err := db.Close(); err != nil {
		t.Errorf("Failed to close db: %v", err)
	}
}

func TestSplitSQLStatements(t *testing.T) {
	sql := `
-- 此行为注释
CREATE TABLE a (id INTEGER);
-- 这是另一行注释
INSERT INTO a VALUES (1);   
	`
	stmts := splitSQLStatements(sql)
	if len(stmts) != 2 {
		t.Fatalf("Expected 2 statements, got %d", len(stmts))
	}
	if stmts[0] != "CREATE TABLE a (id INTEGER);" {
		t.Errorf("Stmt 1 mismatch: %s", stmts[0])
	}
	if stmts[1] != "INSERT INTO a VALUES (1);" {
		t.Errorf("Stmt 2 mismatch: %s", stmts[1])
	}
}

func TestMinFunc(t *testing.T) {
	if min(1, 2) != 1 {
		t.Error("Expected 1")
	}
	if min(5, 3) != 3 {
		t.Error("Expected 3")
	}
	if min(4, 4) != 4 {
		t.Error("Expected 4")
	}
}
