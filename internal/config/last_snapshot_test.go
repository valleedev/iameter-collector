package config

import (
	"testing"

	"github.com/iameter/collector/internal/model"
)

func TestLoadLastSnapshotMissing(t *testing.T) {
	_, ok, err := LoadLastSnapshot(t.TempDir())
	if err != nil {
		t.Fatalf("LoadLastSnapshot() error = %v", err)
	}
	if ok {
		t.Error("ok = true on missing file, want false")
	}
}

func TestSaveLoadLastSnapshotRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := model.UsageSnapshot{
		DeviceID:         "dev_test",
		Provider:         "claude",
		CollectorVersion: "0.1.0-test",
		CapturedAt:       "2026-08-03T21:40:00Z",
		Platform:         model.Platform{OS: "linux", Arch: "amd64"},
		RateLimits: model.RateLimits{
			FiveHour: &model.RateWindow{UsedPercentage: 68.4, ResetsAt: 1785792600},
		},
	}
	if err := SaveLastSnapshot(dir, want); err != nil {
		t.Fatalf("SaveLastSnapshot() error = %v", err)
	}
	got, ok, err := LoadLastSnapshot(dir)
	if err != nil {
		t.Fatalf("LoadLastSnapshot() error = %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if got.DeviceID != want.DeviceID || got.RateLimits.FiveHour.UsedPercentage != 68.4 {
		t.Errorf("got = %+v, want %+v", got, want)
	}
}

func TestSaveLastSnapshotOverwritesPrevious(t *testing.T) {
	dir := t.TempDir()
	first := model.UsageSnapshot{DeviceID: "dev_1", RateLimits: model.RateLimits{FiveHour: &model.RateWindow{UsedPercentage: 10, ResetsAt: 1}}}
	second := model.UsageSnapshot{DeviceID: "dev_1", RateLimits: model.RateLimits{FiveHour: &model.RateWindow{UsedPercentage: 20, ResetsAt: 2}}}
	SaveLastSnapshot(dir, first)
	SaveLastSnapshot(dir, second)

	got, _, err := LoadLastSnapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.RateLimits.FiveHour.UsedPercentage != 20 {
		t.Errorf("UsedPercentage = %v, want 20 (most recent)", got.RateLimits.FiveHour.UsedPercentage)
	}
}
