// Package db abre o banco SQLite da QuadrinhoTeca e aplica as migrações.
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite" // driver "sqlite" (Go puro, sem CGO)
)

//go:embed migrations/*.sql
var migrations embed.FS

// Open abre (ou cria) o banco em path e o deixa na versão mais recente.
func Open(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// migrate aplica, em ordem, os arquivos NNNN_nome.sql com número maior que PRAGMA user_version.
func migrate(conn *sql.DB) error {
	var current int
	if err := conn.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("lendo versão do banco: %w", err)
	}

	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)

	for _, f := range files {
		version, err := strconv.Atoi(strings.SplitN(strings.TrimPrefix(f, "migrations/"), "_", 2)[0])
		if err != nil {
			return fmt.Errorf("nome de migração inválido %q: %w", f, err)
		}
		if version <= current {
			continue
		}
		body, err := migrations.ReadFile(f)
		if err != nil {
			return err
		}
		tx, err := conn.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migração %s: %w", f, err)
		}
		// PRAGMA não aceita parâmetro; version vem de um nome de arquivo embutido.
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		current = version
	}
	return nil
}

// Version devolve a versão de esquema atual (PRAGMA user_version).
func Version(conn *sql.DB) (int, error) {
	var v int
	err := conn.QueryRow("PRAGMA user_version").Scan(&v)
	return v, err
}
