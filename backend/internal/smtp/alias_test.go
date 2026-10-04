package smtp

import (
	"os"
	"testing"

	"mailserver/internal/db"
	"mailserver/internal/model"
)

// #15：别名递归展开带环路保护。
func TestExpandAliasesLoop(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过")
	}
	g, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	g.Exec("TRUNCATE mail_aliases, users RESTART IDENTITY CASCADE")

	// a <-> b 互指：应展开为空（环路）
	g.Create(&model.MailAlias{Source: "a@test.local", Targets: "b@test.local", Enabled: true})
	g.Create(&model.MailAlias{Source: "b@test.local", Targets: "a@test.local", Enabled: true})
	if got := expandAliases(g, "a@test.local", map[string]bool{}, 0); len(got) != 0 {
		t.Fatalf("环路应展开为空，得到 %v", got)
	}

	// c -> d -> e：应展开到 e
	g.Create(&model.MailAlias{Source: "c@test.local", Targets: "d@test.local", Enabled: true})
	g.Create(&model.MailAlias{Source: "d@test.local", Targets: "e@test.local", Enabled: true})
	got := expandAliases(g, "c@test.local", map[string]bool{}, 0)
	if len(got) != 1 || got[0] != "e@test.local" {
		t.Fatalf("链式应展开到 e，得到 %v", got)
	}
}
