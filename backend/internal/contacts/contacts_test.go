package contacts

import "testing"

func TestSplitAddress(t *testing.T) {
	cases := []struct {
		in        string
		wantName  string
		wantEmail string
	}{
		{"Alice <alice@example.com>", "Alice", "alice@example.com"},
		{"bob@example.com", "", "bob@example.com"},
		{"<carol@example.com>", "", "carol@example.com"},
		{"  Alice <A@Example.COM> ", "Alice", "a@example.com"},
		{`"Doe, John" <john@example.com>`, "Doe, John", "john@example.com"},
		{"not-an-email", "", ""},
		{"", "", ""},
		{"   ", "", ""},
	}
	for _, c := range cases {
		name, email := splitAddress(c.in)
		if name != c.wantName || email != c.wantEmail {
			t.Errorf("splitAddress(%q) = (%q, %q), want (%q, %q)", c.in, name, email, c.wantName, c.wantEmail)
		}
	}
}
