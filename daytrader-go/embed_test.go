package daytradergo

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// EntityMappingTest.entitiesMapToTheDerbyTablesWithNonNullIds: the JPA annotations are gone;
// the mapping lives in migrations/0001_schema.sql. Same tables, same id columns, NOT NULL and
// primary key. (internal/seed checks the same against a live PostgreSQL.)
func TestSchemaKeepsTheLegacyTablesWithNonNullIDs(t *testing.T) {
	script, err := fs.ReadFile(Migrations, "migrations/0001_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := strings.ToLower(string(script))
	tables := map[string]string{
		"accountejb": "accountid", "accountprofileejb": "userid", "holdingejb": "holdingid",
		"orderejb": "orderid", "quoteejb": "symbol", "keygenejb": "keyname",
	}
	for table, idColumn := range tables {
		body := regexp.MustCompile(`(?s)create table ` + table + ` \((.*?)\n\);`).FindStringSubmatch(schema)
		if body == nil {
			t.Errorf("no CREATE TABLE %s", table)
			continue
		}
		if !regexp.MustCompile(`\b` + idColumn + `\s+\S+(\(\d+\))? not null`).MatchString(body[1]) {
			t.Errorf("%s.%s must be NOT NULL", table, idColumn)
		}
		if !regexp.MustCompile(`primary key \(` + idColumn + `\)`).MatchString(body[1]) {
			t.Errorf("%s: primary key must be %s", table, idColumn)
		}
		if !strings.Contains(schema, "drop table if exists "+table+";") {
			t.Errorf("%s is not dropped first (buildDBTables recreates the schema)", table)
		}
	}
}

func TestEmbeddedFilesShipInTheBinary(t *testing.T) {
	for _, check := range []struct {
		files fs.FS
		name  string
	}{
		{Templates, "templates/layout.html"}, {Static, "static/style.css"}, {Static, "static/index.html"},
		{Migrations, "migrations/0001_schema.sql"},
	} {
		if info, err := fs.Stat(check.files, check.name); err != nil || info.Size() == 0 {
			t.Errorf("%s: %v", check.name, err)
		}
	}
}
