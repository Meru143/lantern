package store

import "testing"

func TestNormalizeSQLReplacesNumericLiterals(t *testing.T) {
	t.Parallel()

	got := NormalizeSQL("SELECT * FROM users WHERE id = 123")
	want := "SELECT * FROM users WHERE id = ?"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeSQLReplacesStringLiterals(t *testing.T) {
	t.Parallel()

	got := NormalizeSQL("SELECT * FROM users WHERE email = 'alice@example.com'")
	want := "SELECT * FROM users WHERE email = '?'"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
