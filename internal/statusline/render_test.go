package statusline

import (
	"testing"

	"github.com/valleedev/iameter-collector/internal/model"
)

func TestRenderBoth(t *testing.T) {
	rl := model.RateLimits{
		FiveHour: &model.RateWindow{UsedPercentage: 68.4, ResetsAt: 1},
		SevenDay: &model.RateWindow{UsedPercentage: 54.2, ResetsAt: 2},
	}
	got := Render(rl)
	want := "IA METER · 5h 68% · 7d 54%"
	if got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestRenderFiveHourOnly(t *testing.T) {
	rl := model.RateLimits{FiveHour: &model.RateWindow{UsedPercentage: 68, ResetsAt: 1}}
	got := Render(rl)
	want := "IA METER · 5h 68%"
	if got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestRenderSevenDayOnly(t *testing.T) {
	rl := model.RateLimits{SevenDay: &model.RateWindow{UsedPercentage: 54, ResetsAt: 1}}
	got := Render(rl)
	want := "IA METER · 7d 54%"
	if got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestRenderNone(t *testing.T) {
	got := Render(model.RateLimits{})
	want := "IA METER · Consumo no disponible"
	if got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestRenderZeroPercent(t *testing.T) {
	rl := model.RateLimits{FiveHour: &model.RateWindow{UsedPercentage: 0, ResetsAt: 1}}
	got := Render(rl)
	want := "IA METER · 5h 0%"
	if got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}
