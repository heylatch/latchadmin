package auth

import (
	"testing"
	"time"
)

func TestPassword(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "correct horse battery") || CheckPassword(h, "wrong") || CheckPassword("garbage", "x") {
		t.Fatal("password check wrong")
	}
}

func TestTOTPKnownVector(t *testing.T) {
	// RFC 6238 SHA-1 secret "12345678901234567890", T=59 s → 94287082 (last 6 digits: 287082).
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	got, err := totpAt(secret, 59/30)
	if err != nil || got != "287082" {
		t.Fatalf("got %s %v", got, err)
	}
	now, _ := totpAt(secret, time.Now().Unix()/30)
	if !CheckTOTP(secret, now) || CheckTOTP(secret, "000000x") {
		t.Fatal("CheckTOTP wrong")
	}
}

func TestSetupPINRotatesAfterFiveMisses(t *testing.T) {
	var p SetupPIN
	pin := p.New()
	for i := 0; i < 4; i++ {
		if ok, rot := p.Check("nope"); ok || rot != "" {
			t.Fatal("unexpected")
		}
	}
	ok, rot := p.Check("nope")
	if ok || rot == "" {
		t.Fatal("PIN did not rotate")
	}
	if ok, _ := p.Check(pin); ok && rot != pin {
		t.Fatal("old PIN still works")
	}
	if ok, _ := p.Check(rot); !ok {
		t.Fatal("new PIN refused")
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(2, time.Minute)
	l.Fail("ip")
	if b, _ := l.Blocked("ip"); b {
		t.Fatal("blocked too early")
	}
	l.Fail("ip")
	if b, _ := l.Blocked("ip"); !b {
		t.Fatal("not blocked")
	}
	l.Reset("ip")
	if b, _ := l.Blocked("ip"); b {
		t.Fatal("reset failed")
	}
}
