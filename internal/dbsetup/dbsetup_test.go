package dbsetup

import (
	"context"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	c := Conn{User: "root"}
	if err := c.Validate(false); err != nil || c.Host != "127.0.0.1" || c.Port != 3306 {
		t.Fatalf("defaults: %v %+v", err, c)
	}
	bad := []Conn{
		{Host: "db.example.com", User: "x'; DROP USER root; --"},
		{Host: "db example", User: "root"},
		{Host: "127.0.0.1", Port: 70000, User: "root"},
		{Host: "127.0.0.1", User: ""},
	}
	for _, b := range bad {
		if b.Validate(false) == nil {
			t.Errorf("should refuse %+v", b)
		}
	}
	if (&Conn{User: "u", Database: "game-db"}).Validate(true) == nil {
		t.Error("should refuse a database name with a dash")
	}
}

func TestCreateRefusesUnsafeNames(t *testing.T) {
	// Names go into SQL as identifiers, so anything but letters, numbers and underscores is refused before connecting.
	for _, n := range []string{"a`b", "a'b", "a b", "", strings.Repeat("a", 33)} {
		if _, err := Create(context.Background(), Conn{Host: "127.0.0.1", Port: 1, User: "root"}, n, "user"); err == nil || !strings.Contains(err.Error(), "letters") {
			t.Errorf("database %q: %v", n, err)
		}
	}
}

func TestRandomPassword(t *testing.T) {
	p := randomPassword()
	if len(p) != 24 || strings.ContainsAny(p, `'"\`+"`") {
		t.Fatalf("password %q", p)
	}
	if p == randomPassword() {
		t.Fatal("not random")
	}
}
