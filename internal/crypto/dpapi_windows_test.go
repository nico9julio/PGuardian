package crypto

import "testing"

func TestDPAPIRoundTrip(t *testing.T) {
	sample := "SuperSecretSampleValue-NotARealSecret-123!@#"
	enc, err := ProtectString(sample)
	if err != nil {
		t.Fatalf("ProtectString failed: %v", err)
	}
	if enc == "" || enc == sample {
		t.Fatalf("expected encrypted base64 output, got %q", enc)
	}
	dec, err := UnprotectString(enc)
	if err != nil {
		t.Fatalf("UnprotectString failed: %v", err)
	}
	if dec != sample {
		t.Fatalf("mismatch: expected %q, got %q", sample, dec)
	}
}
