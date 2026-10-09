package main

import (
	"slices"
	"testing"
	"time"
)

func TestTrailingMean(t *testing.T) {
	got := trailingMean([]float64{3, 0, 6, 3}, 2)
	if want := []float64{3, 1.5, 3, 4.5}; !slices.Equal(got, want) {
		t.Fatalf("trailingMean = %v, want %v", got, want)
	}
}

func TestGPUNodeBootIncludesModelLoad(t *testing.T) {
	n, err := gpuNode(0.3, 6*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if n.Power.Boot != gpuServerBoot+6*time.Minute {
		t.Fatalf("boot = %v, want server boot plus model load", n.Power.Boot)
	}
	if idle := n.Power.Curve.Idle(); idle != 0.3*gpuPeakWatts {
		t.Fatalf("idle = %v W, want 30%% of %v W", idle, gpuPeakWatts)
	}
}

func TestGPUTraceIsOneWeekPerService(t *testing.T) {
	load, err := readGPUTrace("../../testdata/" + gpuFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"code", "conv"} {
		if len(load[s]) != 7*24*60 {
			t.Fatalf("%s: %d minutes", s, len(load[s]))
		}
	}
}
