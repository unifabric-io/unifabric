// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package sflow

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/unifabric-io/unifabric/pkg/config"
)

const sflowSchemaMigrationsTable = "__unifabric_sflow_schema_migrations"

//go:embed migrations/*.sql
var sflowMigrationFiles embed.FS

type clickHouseSchemaConn interface {
	Exec(ctx context.Context, query string, args ...any) error
	Select(ctx context.Context, dest any, query string, args ...any) error
}

type clickHouseMigration struct {
	Version uint64
	Name    string
	File    string
}

type migrationRecord struct {
	Name string `ch:"name"`
}

var sflowClickHouseMigrations = []clickHouseMigration{
	{
		Version: 202606040001,
		Name:    "create_flows_raw",
		File:    "migrations/202606040001_create_flows_raw.sql",
	},
}

func ApplyClickHouseSchema(ctx context.Context, conn clickHouseSchemaConn, cfg config.SFlowClickHouseConfig, log *slog.Logger) error {
	if conn == nil {
		return fmt.Errorf("clickhouse schema connection is nil")
	}

	database, _ := sflowClickHouseTableParts(cfg)
	flowTable := sflowClickHouseTableName(cfg)
	migrationsTable := sflowClickHouseQualifiedName(database, sflowSchemaMigrationsTable)

	if err := conn.Exec(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", sflowClickHouseIdentifier(database))); err != nil {
		return fmt.Errorf("create clickhouse database %s: %w", database, err)
	}
	createMigrationsTableStatements, err := clickHouseSchemaMigrationsTableStatements(migrationsTable)
	if err != nil {
		return err
	}
	for _, statement := range createMigrationsTableStatements {
		if err := conn.Exec(ctx, statement); err != nil {
			return fmt.Errorf("create clickhouse schema migrations table: %w", err)
		}
	}

	for _, migration := range sflowClickHouseMigrations {
		applied, err := clickHouseMigrationApplied(ctx, conn, migrationsTable, migration)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		if log != nil {
			log.Info("apply clickhouse schema migration", "version", migration.Version, "name", migration.Name)
		}
		statements, err := clickHouseMigrationStatements(migration, cfg)
		if err != nil {
			return err
		}
		for _, statement := range statements {
			if err := conn.Exec(ctx, statement); err != nil {
				return fmt.Errorf("apply clickhouse schema migration %d %s: %w", migration.Version, migration.Name, err)
			}
		}
		if err := conn.Exec(ctx, fmt.Sprintf("INSERT INTO %s (version, name) VALUES (?, ?)", migrationsTable), migration.Version, migration.Name); err != nil {
			return fmt.Errorf("record clickhouse schema migration %d %s: %w", migration.Version, migration.Name, err)
		}
	}

	if err := conn.Exec(ctx, modifyFlowsRawTTLSQL(flowTable, cfg.Schema.RetentionDays)); err != nil {
		return fmt.Errorf("reconcile clickhouse flow table ttl: %w", err)
	}
	return nil
}

func clickHouseMigrationApplied(ctx context.Context, conn clickHouseSchemaConn, migrationsTable string, migration clickHouseMigration) (bool, error) {
	var records []migrationRecord
	query := fmt.Sprintf("SELECT name FROM %s WHERE version = ? ORDER BY applied_at DESC LIMIT 1", migrationsTable)
	if err := conn.Select(ctx, &records, query, migration.Version); err != nil {
		return false, fmt.Errorf("read clickhouse schema migration %d: %w", migration.Version, err)
	}
	if len(records) == 0 {
		return false, nil
	}
	if records[0].Name != migration.Name {
		return false, fmt.Errorf("clickhouse schema migration %d was applied as %q, expected %q", migration.Version, records[0].Name, migration.Name)
	}
	return true, nil
}

func clickHouseSchemaMigrationsTableStatements(table string) ([]string, error) {
	data, err := sflowMigrationFiles.ReadFile("migrations/schema_migrations.sql")
	if err != nil {
		return nil, fmt.Errorf("read clickhouse schema migrations table SQL: %w", err)
	}
	sql := strings.ReplaceAll(string(data), "{{migrations_table}}", table)
	return splitClickHouseMigrationStatements(sql), nil
}

func clickHouseMigrationStatements(migration clickHouseMigration, cfg config.SFlowClickHouseConfig) ([]string, error) {
	data, err := sflowMigrationFiles.ReadFile(migration.File)
	if err != nil {
		return nil, fmt.Errorf("read clickhouse schema migration %d %s: %w", migration.Version, migration.Name, err)
	}
	sql := renderClickHouseMigrationSQL(string(data), cfg)
	return splitClickHouseMigrationStatements(sql), nil
}

func renderClickHouseMigrationSQL(sql string, cfg config.SFlowClickHouseConfig) string {
	replacements := map[string]string{
		"{{table}}":          sflowClickHouseTableName(cfg),
		"{{retention_days}}": strconv.Itoa(cfg.Schema.RetentionDays),
	}
	for placeholder, value := range replacements {
		sql = strings.ReplaceAll(sql, placeholder, value)
	}
	return sql
}

func splitClickHouseMigrationStatements(sql string) []string {
	statements := make([]string, 0, 1)
	var statement strings.Builder
	hasSQL := false
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	inLineComment := false
	inBlockComment := false

	appendStatement := func() {
		if !hasSQL {
			statement.Reset()
			return
		}
		text := strings.TrimSpace(statement.String())
		if text != "" {
			statements = append(statements, text)
		}
		statement.Reset()
		hasSQL = false
	}

	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		next := byte(0)
		if i+1 < len(sql) {
			next = sql[i+1]
		}

		if inLineComment {
			if ch == '\n' || ch == '\r' {
				if statement.Len() > 0 {
					statement.WriteByte(ch)
				}
				inLineComment = false
			}
			continue
		}

		if inBlockComment {
			if ch == '*' && next == '/' {
				i++
				inBlockComment = false
			}
			continue
		}

		if inSingleQuote {
			statement.WriteByte(ch)
			if ch == '\\' && i+1 < len(sql) {
				i++
				statement.WriteByte(sql[i])
				continue
			}
			if ch == '\'' {
				if next == '\'' {
					i++
					statement.WriteByte(next)
					continue
				}
				inSingleQuote = false
			}
			continue
		}

		if inDoubleQuote {
			statement.WriteByte(ch)
			if ch == '\\' && i+1 < len(sql) {
				i++
				statement.WriteByte(sql[i])
				continue
			}
			if ch == '"' {
				if next == '"' {
					i++
					statement.WriteByte(next)
					continue
				}
				inDoubleQuote = false
			}
			continue
		}

		if inBacktick {
			statement.WriteByte(ch)
			if ch == '\\' && i+1 < len(sql) {
				i++
				statement.WriteByte(sql[i])
				continue
			}
			if ch == '`' {
				if next == '`' {
					i++
					statement.WriteByte(next)
					continue
				}
				inBacktick = false
			}
			continue
		}

		switch {
		case ch == '-' && next == '-':
			if statement.Len() > 0 {
				statement.WriteByte(' ')
			}
			i++
			inLineComment = true
		case ch == '#':
			if statement.Len() > 0 {
				statement.WriteByte(' ')
			}
			if next == '!' {
				i++
			}
			inLineComment = true
		case ch == '/' && next == '*':
			if statement.Len() > 0 {
				statement.WriteByte(' ')
			}
			i++
			inBlockComment = true
		case ch == '\'':
			statement.WriteByte(ch)
			inSingleQuote = true
			hasSQL = true
		case ch == '"':
			statement.WriteByte(ch)
			inDoubleQuote = true
			hasSQL = true
		case ch == '`':
			statement.WriteByte(ch)
			inBacktick = true
			hasSQL = true
		case ch == ';':
			appendStatement()
		default:
			statement.WriteByte(ch)
			if !clickHouseMigrationWhitespace(ch) {
				hasSQL = true
			}
		}
	}
	appendStatement()
	return statements
}

func clickHouseMigrationWhitespace(ch byte) bool {
	switch ch {
	case ' ', '\n', '\r', '\t', '\f', '\v':
		return true
	default:
		return false
	}
}

func modifyFlowsRawTTLSQL(table string, retentionDays int) string {
	return fmt.Sprintf("ALTER TABLE %s MODIFY TTL time_flow_start + toIntervalDay(%d)", table, retentionDays)
}

func sflowClickHouseTableName(cfg config.SFlowClickHouseConfig) string {
	database, table := sflowClickHouseTableParts(cfg)
	return sflowClickHouseQualifiedName(database, table)
}

func sflowClickHouseTableParts(cfg config.SFlowClickHouseConfig) (string, string) {
	database := strings.TrimSpace(cfg.Database)
	if database == "" {
		database = "default"
	}
	table := strings.TrimSpace(cfg.Table)
	if table == "" {
		table = "flows_raw"
	}
	if parts := strings.Split(table, "."); len(parts) == 2 {
		return parts[0], parts[1]
	}
	return database, table
}

func sflowClickHouseQualifiedName(database, table string) string {
	return fmt.Sprintf("%s.%s", sflowClickHouseIdentifier(database), sflowClickHouseIdentifier(table))
}

func sflowClickHouseIdentifier(identifier string) string {
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
}
