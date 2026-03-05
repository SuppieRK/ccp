package engine

import "testing"

func TestMaskLinePresetOrder(t *testing.T) {
	in := "uuid 123e4567-e89b-12d3-a456-426614174000 addr 0xDEADBEEF n 1708713600"
	out := maskLine(in)
	if out != "uuid [UUID] addr [ADDR] n [NUM]" {
		t.Fatalf("unexpected masked output: %q", out)
	}
}

func TestMaskLineNumericBoundary(t *testing.T) {
	in := "port 8080 ip 127.0.0.1 ts 1708713600"
	out := maskLine(in)
	if out != "port 8080 ip 127.0.0.1 ts [NUM]" {
		t.Fatalf("unexpected boundary masking: %q", out)
	}
}
