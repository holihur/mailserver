package deliver

import (
	"os"
	"testing"

	"mailserver/internal/db"
	"mailserver/internal/model"

	"gorm.io/gorm"
)

func sieveTestUser(t *testing.T, script string) (*gorm.DB, *model.User) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过")
	}
	g, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	g.Exec("TRUNCATE users, mails, sieve_scripts, mail_rules, mail_folders RESTART IDENTITY CASCADE")
	u := model.User{Email: "sieve@test.local"}
	g.Create(&u)
	g.Create(&model.SieveScript{UserID: u.ID, Name: "main", Active: true, Script: script})
	return g, &u
}

// #16：reject 现在真正拒收（不再静默投递）。
func TestSieveRejectDropsMail(t *testing.T) {
	g, u := sieveTestUser(t, `require ["reject"]; reject "no thanks";`)
	ToUser(g, u, "a@x", u.Email, "", "", "subj", "body", "", "[]")
	var n int64
	g.Model(&model.Mail{}).Where("user_id = ?", u.ID).Count(&n)
	if n != 0 {
		t.Fatalf("reject 不应投递，得到 %d 封", n)
	}
}

// #16：vacation 触发自动回复钩子。
func TestSieveVacationHook(t *testing.T) {
	g, u := sieveTestUser(t, `require ["vacation"]; vacation "out of office";`)
	called := 0
	VacationHook = func(_ *gorm.DB, _ uint, from, subject, text string) { called++ }
	defer func() { VacationHook = nil }()
	ToUser(g, u, "a@x", u.Email, "", "", "subj", "body", "", "[]")
	if called != 1 {
		t.Fatalf("vacation 钩子应调用一次，得到 %d", called)
	}
}
