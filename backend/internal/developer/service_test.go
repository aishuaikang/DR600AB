package developer

import (
	"testing"
	"time"
)

func TestTOTPCodeRFCVector(t *testing.T) {
	secret := []byte("12345678901234567890")
	tests := []struct {
		counter uint64
		want    string
	}{
		{counter: 1, want: "287082"},
		{counter: 37037036, want: "081804"},
		{counter: 37037037, want: "050471"},
		{counter: 41152263, want: "005924"},
		{counter: 66666666, want: "279037"},
		{counter: 666666666, want: "353130"},
	}

	for _, tt := range tests {
		if got := totpCode(secret, tt.counter); got != tt.want {
			t.Fatalf("totpCode(%d) = %q, want %q", tt.counter, got, tt.want)
		}
	}
}

func TestLoginWithCurrentTOTPCode(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	svc, err := NewService(secret, 10*time.Minute)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	rawSecret, err := decodeSecret(secret)
	if err != nil {
		t.Fatalf("decodeSecret() error = %v", err)
	}
	counter := uint64(time.Now().Unix() / int64(timeStep.Seconds()))
	code := totpCode(rawSecret, counter)

	token, expiresAt, err := svc.Login(code)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if token == "" {
		t.Fatal("Login() returned empty token")
	}
	if !expiresAt.After(time.Now()) {
		t.Fatalf("expiresAt = %s, want future time", expiresAt)
	}
	if err := svc.Validate(token); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestLoginWithoutSecret(t *testing.T) {
	svc, err := NewService("", 10*time.Minute)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, _, err = svc.Login("123456")
	if err != ErrNotConfigured {
		t.Fatalf("Login() error = %v, want %v", err, ErrNotConfigured)
	}
}
