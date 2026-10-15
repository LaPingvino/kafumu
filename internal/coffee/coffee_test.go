package coffee

import "testing"

func TestMoney(t *testing.T) {
	for in, want := range map[string]int64{"2": 2_000_000, "2.5": 2_500_000, "2,50": 2_500_000, "0.000001": 1, ".5": 500_000, "12.345678": 12_345_678} {
		if got, ok := ParseMoney(in); !ok || got != want {
			t.Errorf("ParseMoney(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "0", "-1", "1.2345678", "abc", "1.2.3", "1e3", " . "} {
		if _, ok := ParseMoney(in); ok {
			t.Errorf("ParseMoney(%q) accepted", in)
		}
	}
	if Money(12_345_678) != "12.34" || Money(-1_500_000) != "-1.50" || Money(0) != "0.00" {
		t.Errorf("Money: %s %s %s", Money(12_345_678), Money(-1_500_000), Money(0))
	}
}
