// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package sflow

import (
	"context"
	"strings"
	"testing"

	"github.com/unifabric-io/unifabric/pkg/config"
)

func TestApplyClickHouseSchemaAppliesPendingMigration(t *testing.T) {
	cfg := config.SFlowClickHouseConfig{
		Database: "default",
		Table:    "flows_raw",
		Schema: config.SFlowClickHouseSchemaConfig{
			RetentionDays: 5,
		},
	}
	conn := &fakeSchemaConn{}
	if err := ApplyClickHouseSchema(context.Background(), conn, cfg, nil); err != nil {
		t.Fatalf("ApplyClickHouseSchema() error = %v", err)
	}
	if len(conn.execs) != 5 {
		t.Fatalf("exec count = %d, want 5: %#v", len(conn.execs), conn.execs)
	}
	if !strings.Contains(conn.execs[0], "CREATE DATABASE IF NOT EXISTS `default`") {
		t.Fatalf("first exec = %q", conn.execs[0])
	}
	if !strings.Contains(conn.execs[1], "__unifabric_sflow_schema_migrations") {
		t.Fatalf("second exec = %q", conn.execs[1])
	}
	if !strings.Contains(conn.execs[2], "CREATE TABLE IF NOT EXISTS `default`.`flows_raw`") {
		t.Fatalf("third exec = %q", conn.execs[2])
	}
	if strings.Contains(conn.execs[2], "{{") {
		t.Fatalf("unrendered migration placeholder in sql: %q", conn.execs[2])
	}
	if len(conn.execArgs) <= 3 || len(conn.execArgs[3]) != 2 || conn.execArgs[3][0] != uint64(202606040001) {
		t.Fatalf("migration version was not recorded with date version: execs=%#v args=%#v", conn.execs, conn.execArgs)
	}
	if !strings.Contains(conn.execs[2], "toIntervalDay(5)") {
		t.Fatalf("create table ttl missing retention: %q", conn.execs[2])
	}
	if !strings.Contains(conn.execs[4], "ALTER TABLE `default`.`flows_raw` MODIFY TTL") {
		t.Fatalf("ttl exec = %q", conn.execs[4])
	}
}

func TestApplyClickHouseSchemaSkipsAppliedMigrationAndReconcilesTTL(t *testing.T) {
	cfg := config.SFlowClickHouseConfig{
		Database: "analytics",
		Table:    "flows_raw",
		Schema: config.SFlowClickHouseSchemaConfig{
			RetentionDays: 9,
		},
	}
	conn := &fakeSchemaConn{applied: map[uint64]string{202606040001: "create_flows_raw"}}
	if err := ApplyClickHouseSchema(context.Background(), conn, cfg, nil); err != nil {
		t.Fatalf("ApplyClickHouseSchema() error = %v", err)
	}
	if len(conn.execs) != 3 {
		t.Fatalf("exec count = %d, want 3: %#v", len(conn.execs), conn.execs)
	}
	if !strings.Contains(conn.execs[2], "ALTER TABLE `analytics`.`flows_raw` MODIFY TTL time_flow_start + toIntervalDay(9)") {
		t.Fatalf("ttl exec = %q", conn.execs[2])
	}
}

func TestApplyClickHouseSchemaDetectsVersionNameMismatch(t *testing.T) {
	cfg := config.SFlowClickHouseConfig{
		Database: "default",
		Table:    "flows_raw",
		Schema: config.SFlowClickHouseSchemaConfig{
			RetentionDays: 3,
		},
	}
	conn := &fakeSchemaConn{applied: map[uint64]string{202606040001: "other"}}
	err := ApplyClickHouseSchema(context.Background(), conn, cfg, nil)
	if err == nil || !strings.Contains(err.Error(), "expected \"create_flows_raw\"") {
		t.Fatalf("ApplyClickHouseSchema() error = %v, want name mismatch", err)
	}
}

func TestClickHouseMigrationStatementsRenderEmbeddedSQL(t *testing.T) {
	cfg := config.SFlowClickHouseConfig{
		Database: "analytics",
		Table:    "flows_raw",
		Schema: config.SFlowClickHouseSchemaConfig{
			RetentionDays: 11,
		},
	}
	statements, err := clickHouseMigrationStatements(sflowClickHouseMigrations[0], cfg)
	if err != nil {
		t.Fatalf("clickHouseMigrationStatements() error = %v", err)
	}
	if len(statements) != 1 {
		t.Fatalf("statement count = %d, want 1", len(statements))
	}
	if !strings.Contains(statements[0], "CREATE TABLE IF NOT EXISTS `analytics`.`flows_raw`") {
		t.Fatalf("statement table = %q", statements[0])
	}
	if !strings.Contains(statements[0], "toIntervalDay(11)") {
		t.Fatalf("statement retention = %q", statements[0])
	}
	if strings.Contains(statements[0], "{{") {
		t.Fatalf("statement contains unrendered placeholder = %q", statements[0])
	}
}

func TestClickHouseSchemaMigrationsTableStatementsRenderEmbeddedSQL(t *testing.T) {
	statements, err := clickHouseSchemaMigrationsTableStatements("`default`.`__unifabric_sflow_schema_migrations`")
	if err != nil {
		t.Fatalf("clickHouseSchemaMigrationsTableStatements() error = %v", err)
	}
	if len(statements) != 1 {
		t.Fatalf("statement count = %d, want 1", len(statements))
	}
	statement := statements[0]
	if !strings.Contains(statement, "CREATE TABLE IF NOT EXISTS `default`.`__unifabric_sflow_schema_migrations`") {
		t.Fatalf("statement table = %q", statement)
	}
	if !strings.Contains(statement, "version UInt64") {
		t.Fatalf("statement version column = %q", statement)
	}
	if strings.Contains(statement, "{{") {
		t.Fatalf("statement contains unrendered placeholder = %q", statement)
	}
}

func TestSplitClickHouseMigrationStatementsSupportsMultipleStatements(t *testing.T) {
	sql := `
		CREATE TABLE one (id UInt64);
		ALTER TABLE one ADD COLUMN name String DEFAULT 'a;b';
		ALTER TABLE one ADD COLUMN note String DEFAULT "x;y";
		ALTER TABLE one ADD COLUMN ` + "`semi;colon`" + ` UInt8;
	`
	statements := splitClickHouseMigrationStatements(sql)
	if len(statements) != 4 {
		t.Fatalf("statement count = %d, want 4: %#v", len(statements), statements)
	}
	if !strings.Contains(statements[1], "DEFAULT 'a;b'") {
		t.Fatalf("single quoted semicolon was split: %#v", statements)
	}
	if !strings.Contains(statements[2], `DEFAULT "x;y"`) {
		t.Fatalf("double quoted semicolon was split: %#v", statements)
	}
	if !strings.Contains(statements[3], "`semi;colon`") {
		t.Fatalf("backtick quoted semicolon was split: %#v", statements)
	}
}

func TestSplitClickHouseMigrationStatementsIgnoresSemicolonsInComments(t *testing.T) {
	sql := `
		-- comment with ;
		CREATE TABLE one (id UInt64); # comment with ;
		/* block comment with ; */
		ALTER TABLE one ADD COLUMN name String;
	`
	statements := splitClickHouseMigrationStatements(sql)
	if len(statements) != 2 {
		t.Fatalf("statement count = %d, want 2: %#v", len(statements), statements)
	}
	if !strings.Contains(statements[0], "CREATE TABLE one") {
		t.Fatalf("first statement = %q", statements[0])
	}
	if !strings.Contains(statements[1], "ALTER TABLE one") {
		t.Fatalf("second statement = %q", statements[1])
	}
}

type fakeSchemaConn struct {
	execs    []string
	execArgs [][]any
	applied  map[uint64]string
}

func (f *fakeSchemaConn) Exec(_ context.Context, query string, args ...any) error {
	f.execs = append(f.execs, query)
	f.execArgs = append(f.execArgs, args)
	return nil
}

func (f *fakeSchemaConn) Select(_ context.Context, dest any, _ string, args ...any) error {
	records, ok := dest.(*[]migrationRecord)
	if !ok {
		return nil
	}
	if len(args) != 1 {
		return nil
	}
	version, ok := args[0].(uint64)
	if !ok {
		return nil
	}
	if name, ok := f.applied[version]; ok {
		*records = []migrationRecord{{Name: name}}
	}
	return nil
}
