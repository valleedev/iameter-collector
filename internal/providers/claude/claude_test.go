package claude

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/iameter/collector/internal/model"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "statusline", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}

func parseString(t *testing.T, input string) (*model.RateLimits, error) {
	t.Helper()
	return New().Parse(strings.NewReader(input))
}

// 1. Both windows present.
func TestParseBothWindows(t *testing.T) {
	rl, err := parseString(t, fixture(t, "complete.json"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if rl.FiveHour == nil || rl.SevenDay == nil {
		t.Fatalf("expected both windows, got %+v", rl)
	}
	if rl.FiveHour.UsedPercentage != 68.4 || rl.FiveHour.ResetsAt != 1785792600 {
		t.Errorf("five_hour = %+v", rl.FiveHour)
	}
	if rl.SevenDay.UsedPercentage != 54.2 || rl.SevenDay.ResetsAt != 1786107600 {
		t.Errorf("seven_day = %+v", rl.SevenDay)
	}
}

// 2. Only five_hour.
func TestParseFiveHourOnly(t *testing.T) {
	rl, err := parseString(t, fixture(t, "five-hour-only.json"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if rl.FiveHour == nil {
		t.Fatal("expected five_hour present")
	}
	if rl.SevenDay != nil {
		t.Errorf("expected seven_day absent, got %+v", rl.SevenDay)
	}
}

// 3. Only seven_day.
func TestParseSevenDayOnly(t *testing.T) {
	rl, err := parseString(t, fixture(t, "seven-day-only.json"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if rl.SevenDay == nil {
		t.Fatal("expected seven_day present")
	}
	if rl.FiveHour != nil {
		t.Errorf("expected five_hour absent, got %+v", rl.FiveHour)
	}
}

// 4. No rate_limits key at all.
func TestParseNoRateLimits(t *testing.T) {
	rl, err := parseString(t, fixture(t, "no-rate-limits.json"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !rl.Empty() {
		t.Errorf("expected empty RateLimits, got %+v", rl)
	}
}

// 5. rate_limits explicitly null.
func TestParseRateLimitsNull(t *testing.T) {
	rl, err := parseString(t, `{"rate_limits": null}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !rl.Empty() {
		t.Errorf("expected empty RateLimits, got %+v", rl)
	}
}

// 6. Percentage exactly 0 is valid, not treated as absent.
func TestParsePercentageZero(t *testing.T) {
	rl, err := parseString(t, `{"rate_limits":{"five_hour":{"used_percentage":0,"resets_at":1738425600}}}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if rl.FiveHour == nil {
		t.Fatal("expected five_hour present with 0%%")
	}
	if rl.FiveHour.UsedPercentage != 0 {
		t.Errorf("UsedPercentage = %v, want 0", rl.FiveHour.UsedPercentage)
	}
}

// 7. Percentage exactly 100 is valid.
func TestParsePercentageHundred(t *testing.T) {
	rl, err := parseString(t, `{"rate_limits":{"five_hour":{"used_percentage":100,"resets_at":1738425600}}}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if rl.FiveHour == nil || rl.FiveHour.UsedPercentage != 100 {
		t.Errorf("expected five_hour=100, got %+v", rl.FiveHour)
	}
}

// 8. Negative percentage -> window dropped, no error.
func TestParsePercentageNegative(t *testing.T) {
	rl, err := parseString(t, `{"rate_limits":{"five_hour":{"used_percentage":-5,"resets_at":1738425600}}}`)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil (invalid window dropped, not fatal)", err)
	}
	if rl.FiveHour != nil {
		t.Errorf("expected five_hour dropped for negative percentage, got %+v", rl.FiveHour)
	}
}

// 9. Percentage > 100 -> window dropped, no error.
func TestParsePercentageOverHundred(t *testing.T) {
	rl, err := parseString(t, `{"rate_limits":{"five_hour":{"used_percentage":150,"resets_at":1738425600}}}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if rl.FiveHour != nil {
		t.Errorf("expected five_hour dropped for >100%%, got %+v", rl.FiveHour)
	}
}

// 10. Percentage as a string -> window dropped, no error, no crash.
func TestParsePercentageAsString(t *testing.T) {
	rl, err := parseString(t, `{"rate_limits":{"five_hour":{"used_percentage":"68.4","resets_at":1738425600}}}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if rl.FiveHour != nil {
		t.Errorf("expected five_hour dropped for string percentage, got %+v", rl.FiveHour)
	}
}

// 11. resets_at null -> window dropped, never defaults to 0.
func TestParseResetsAtNull(t *testing.T) {
	rl, err := parseString(t, `{"rate_limits":{"five_hour":{"used_percentage":50,"resets_at":null}}}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if rl.FiveHour != nil {
		t.Errorf("expected five_hour dropped for null resets_at (must not default to 0), got %+v", rl.FiveHour)
	}
}

// 12. resets_at invalid (non-integer / wrong type) -> window dropped.
func TestParseResetsAtInvalid(t *testing.T) {
	cases := []string{
		`{"rate_limits":{"five_hour":{"used_percentage":50,"resets_at":"soon"}}}`,
		`{"rate_limits":{"five_hour":{"used_percentage":50,"resets_at":1738425600.5}}}`,
		`{"rate_limits":{"five_hour":{"used_percentage":50,"resets_at":-100}}}`,
	}
	for _, in := range cases {
		rl, err := parseString(t, in)
		if err != nil {
			t.Fatalf("Parse(%s) error = %v", in, err)
		}
		if rl.FiveHour != nil {
			t.Errorf("Parse(%s): expected five_hour dropped, got %+v", in, rl.FiveHour)
		}
	}
}

// 13. Malformed JSON -> error.
func TestParseMalformedJSON(t *testing.T) {
	_, err := parseString(t, fixture(t, "malformed.json"))
	if err == nil {
		t.Fatal("Parse() error = nil, want error for malformed JSON")
	}
}

// 14. Empty input -> error.
func TestParseEmptyInput(t *testing.T) {
	_, err := parseString(t, "")
	if err == nil {
		t.Fatal("Parse() error = nil, want error for empty input")
	}
}

// 15. Oversized input is capture's responsibility (internal/capture), not
// the parser's — tested there. The parser itself must not crash on large
// but well-formed input.
func TestParseLargeButValidInput(t *testing.T) {
	padding := strings.Repeat("x", 100000)
	in := `{"padding":"` + padding + `","rate_limits":{"five_hour":{"used_percentage":50,"resets_at":1738425600}}}`
	rl, err := parseString(t, in)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if rl.FiveHour == nil {
		t.Error("expected five_hour present")
	}
}

// 16. Extra sensitive fields never leak into the parsed result.
func TestParseExtraSensitiveFieldsIgnored(t *testing.T) {
	rl, err := parseString(t, fixture(t, "extra-sensitive-fields.json"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if rl.FiveHour == nil || rl.SevenDay == nil {
		t.Fatalf("expected both windows present, got %+v", rl)
	}
	// model.RateLimits has no fields besides FiveHour/SevenDay, so there is
	// no field to hold cwd/env/git/transcript_path even if we wanted it to.
	// This test documents that guarantee structurally.
}

// 17. Concurrent parsing is safe (Parse holds no shared mutable state).
func TestParseConcurrency(t *testing.T) {
	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := parseString(t, fixture(t, "complete.json"))
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent Parse() error = %v", err)
	}
}

func TestName(t *testing.T) {
	if New().Name() != "claude" {
		t.Errorf("Name() = %q, want claude", New().Name())
	}
}
