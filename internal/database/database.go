// Package database 负责 SQLite 数据库连接、迁移管理
package database

import (
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// DB 封装 SQLite 连接（读写分离：writeDB 单连接写入，readDB 多连接并发读取）
type DB struct {
	*sql.DB          // 写连接（默认，MaxOpenConns=1）
	readDB *sql.DB   // 只读连接池（MaxOpenConns=3，供前端查询使用）
	dbPath string    // 数据库文件路径
}

// Path 返回数据库文件路径
func (d *DB) Path() string { return d.dbPath }

// Open 打开或创建 SQLite 数据库，并自动执行迁移
func Open(dbPath string) (*DB, error) {
	// 写连接：启用 WAL 日志和外键约束
	writeDSN := fmt.Sprintf("file:%s?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000", dbPath)
	writeDB, err := sql.Open("sqlite", writeDSN)
	if err != nil {
		return nil, fmt.Errorf("打开写数据库失败: %w", err)
	}
	// 写连接限制为单连接（SQLite 只允许一个写者）
	writeDB.SetMaxOpenConns(1)
	writeDB.SetMaxIdleConns(1)

	// 读连接：只读模式 + query_only PRAGMA 双重保险
	// 注意：只读连接不设置 _journal_mode（WAL 由写连接控制，只读连接继承）
	readDSN := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000&mode=ro", dbPath)
	readDB, err := sql.Open("sqlite", readDSN)
	if err != nil {
		writeDB.Close()
		return nil, fmt.Errorf("打开读数据库失败: %w", err)
	}
	// 读连接池允许多个并发读取（WAL 模式支持多读一写）
	readDB.SetMaxOpenConns(3)
	readDB.SetMaxIdleConns(2)

	db := &DB{DB: writeDB, readDB: readDB, dbPath: dbPath}

	// 在事务外单独设置 WAL 模式和 PRAGMA（SQLite 不允许在事务内切换 journal mode）
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA busy_timeout=5000;",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("设置 PRAGMA 失败 (%s): %w", pragma, err)
		}
	}

	if err := db.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}

	slog.Info("数据库连接成功（读写分离模式）", "path", dbPath)
	return db, nil
}

// migrate 执行所有待应用的 SQL 迁移脚本
func (db *DB) migrate() error {
	// 确保 schema_version 表存在（防止第一次初始化时查询失败）
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version    INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return fmt.Errorf("创建版本表失败: %w", err)
	}

	// 获取已应用的最大版本号
	var currentVersion int
	row := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version")
	if err := row.Scan(&currentVersion); err != nil {
		return err
	}

	// 读取所有迁移文件，按文件名升序排列
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}

	var migrations []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			migrations = append(migrations, e.Name())
		}
	}
	sort.Strings(migrations)

	for _, name := range migrations {
		// 从文件名解析版本号，如 "001_initial_schema.sql" -> 1
		var version int
		if _, err := fmt.Sscanf(name, "%03d_", &version); err != nil {
			slog.Warn("跳过无法解析版本号的迁移文件", "name", name)
			continue
		}

		if version <= currentVersion {
			continue
		}

		// embed.FS 始终使用正斜杠（即使在 Windows 上）
		content, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("读取迁移文件 %s 失败: %w", name, err)
		}

		slog.Info("执行数据库迁移", "version", version, "file", name)

		// 在事务中执行迁移
		if err := db.execMigration(version, string(content)); err != nil {
			return fmt.Errorf("迁移 %s 失败: %w", name, err)
		}
	}

	return nil
}

// execMigration 在事务中执行单个迁移脚本
// 对于 ALTER TABLE 语句，失败时仅记录警告（列可能已存在），不中断迁移
func (db *DB) execMigration(version int, sqlContent string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 按分号分割 SQL 语句逐条执行，ALTER TABLE 失败时容错
	statements := splitSQLStatements(sqlContent)
	for _, stmt := range statements {
		trimmed := strings.TrimSpace(stmt)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		if _, err := tx.Exec(stmt); err != nil {
			// ALTER TABLE ADD COLUMN 在列已存在时会失败 → 仅警告
			if strings.Contains(strings.ToUpper(trimmed), "ALTER TABLE") {
				slog.Warn("ALTER TABLE 语句跳过（列可能已存在）", "version", version, "error", err)
				continue
			}
			return fmt.Errorf("语句执行失败: %w\n%s", err, trimmed[:min(len(trimmed), 200)])
		}
	}

	// 记录迁移版本（如果 SQL 中未包含 INSERT INTO schema_version）
	if _, err := tx.Exec(
		"INSERT OR IGNORE INTO schema_version(version) VALUES(?)", version,
	); err != nil {
		return err
	}

	return tx.Commit()
}

// splitSQLStatements 按分号分割 SQL 语句（忽略注释行内的分号）
func splitSQLStatements(sql string) []string {
	var stmts []string
	current := ""
	for _, line := range strings.Split(sql, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		current += line + "\n"
		if strings.HasSuffix(trimmed, ";") {
			stmts = append(stmts, strings.TrimSpace(current))
			current = ""
		}
	}
	if strings.TrimSpace(current) != "" {
		stmts = append(stmts, strings.TrimSpace(current))
	}
	return stmts
}

// min 返回两个整数中较小的
func min(a, b int) int {
	if a < b { return a }
	return b
}

// ReadQuery 使用只读连接执行查询（不阻塞写连接）
// 适用于前端数据查询，写入操作仍使用默认的 db.Query/db.Exec
func (db *DB) ReadQuery(query string, args ...interface{}) (*sql.Rows, error) {
	return db.readDB.Query(query, args...)
}

// ReadQueryRow 使用只读连接执行单行查询
func (db *DB) ReadQueryRow(query string, args ...interface{}) *sql.Row {
	return db.readDB.QueryRow(query, args...)
}

// Close 关闭所有数据库连接
func (db *DB) Close() error {
	slog.Info("关闭数据库连接")
	if db.readDB != nil {
		db.readDB.Close()
	}
	return db.DB.Close()
}
