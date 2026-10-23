package ws

import "testing"

func TestBinaryAssetName(t *testing.T) {
	if got := BinaryAssetName(); got != "ws" {
		t.Fatalf("BinaryAssetName() = %q, want ws", got)
	}
	if got := CLIName(); got != "ws" {
		t.Fatalf("CLIName() = %q, want ws", got)
	}
}
