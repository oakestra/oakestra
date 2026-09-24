package bestRandomFit

import (
	"errors"
	"scheduler/calculate/schedulers/cpumem"
	"scheduler/calculate/schedulers/interfaces"
	"testing"
)

func res(id string, mem, cpu, cpuPct float64) cpumem.Resources {
	return cpumem.Resources{
		Id:             id,
		Virtualization: []string{"docker"},
		AvailableMem:   mem,
		AvailableCPU:   cpu,
		CPUPercent:     cpuPct,
	}
}

func TestBestRandomFitNoCandidates(t *testing.T) {
	var a BestRandomFit
	_, err := a.Calculate(res("j", 100, 1, 0), nil)
	if !errors.As(err, &interfaces.SchedulingError{}) {
		t.Fatalf("expected scheduling error, got %v", err)
	}
}

func TestBestRandomFitFiltersUnqualified(t *testing.T) {
	var a BestRandomFit
	job := res("j", 1000, 4, 0)
	// Only c2 has enough cpu+mem.
	c1 := res("c1", 500, 1, 0)
	c2 := res("c2", 2000, 8, 0)
	got, err := a.Calculate(job, []cpumem.Resources{c1, c2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Id != "c2" {
		t.Errorf("got %s; want c2", got.Id)
	}
}

// The core behaviour: pick at random ONLY among the best-scoring band, never a
// clearly-worse candidate.
func TestBestRandomFitPicksAmongBestBand(t *testing.T) {
	job := res("j", 100, 1, 0)
	// Two top candidates (identical high score) + one clearly worse.
	top1 := res("top1", 4000, 8, 5) // score = 95 + 4000 = 4095
	top2 := res("top2", 4000, 8, 5) // score = 4095
	worse := res("worse", 500, 2, 90) // score = 10 + 500 = 510
	cands := []cpumem.Resources{worse, top1, top2}

	seen := map[string]int{}
	for i := 0; i < 200; i++ {
		a := BestRandomFit{Tolerance: DefaultTolerance}
		got, err := a.Calculate(job, cands)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		seen[got.Id]++
	}
	if seen["worse"] != 0 {
		t.Errorf("worse candidate was picked %d times; it must never be chosen", seen["worse"])
	}
	if seen["top1"] == 0 || seen["top2"] == 0 {
		t.Errorf("expected both top candidates to be picked across 200 runs; got %v", seen)
	}
}

// With an injected deterministic rng, verify the band membership is exactly the
// candidates within Tolerance of the max.
func TestBestRandomFitBandRespectsTolerance(t *testing.T) {
	job := res("j", 100, 1, 0)
	best := res("best", 1000, 4, 0)  // score 1100
	near := res("near", 1000, 4, 1)  // score 1099 -> within tolerance 2
	far := res("far", 900, 4, 0)     // score 1000 -> outside tolerance 2

	// rng returns the last index, so we can see how big the band is.
	var bandSize int
	a := BestRandomFit{Tolerance: 2.0, rng: func(n int) int { bandSize = n; return n - 1 }}
	_, err := a.Calculate(job, []cpumem.Resources{far, near, best})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bandSize != 2 {
		t.Errorf("band size = %d; want 2 (best + near, far excluded)", bandSize)
	}
}
