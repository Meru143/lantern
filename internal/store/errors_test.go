package store

import "testing"

func TestFingerprintErrorSameInputsHaveSameFingerprint(t *testing.T) {
	t.Parallel()

	first := FingerprintError("ValueError", "invalid value 123")
	second := FingerprintError("ValueError", "invalid value 123")

	if first != second {
		t.Fatalf("got %q and %q, want equal fingerprints", first, second)
	}
}

func TestFingerprintErrorDifferentExceptionTypeChangesFingerprint(t *testing.T) {
	t.Parallel()

	first := FingerprintError("ValueError", "invalid value 123")
	second := FingerprintError("TypeError", "invalid value 123")

	if first == second {
		t.Fatal("expected different exception types to produce different fingerprints")
	}
}

func TestFingerprintErrorNormalizesUUIDs(t *testing.T) {
	t.Parallel()

	withUUID := FingerprintError("ValueError", "user 550e8400-e29b-41d4-a716-446655440000 not found")
	normalized := FingerprintError("ValueError", "user <uuid> not found")

	if withUUID != normalized {
		t.Fatalf("got %q and %q, want matching fingerprints after normalization", withUUID, normalized)
	}
}
