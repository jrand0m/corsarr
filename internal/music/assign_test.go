package music

import (
	"math"
	"math/rand/v2"
	"testing"
)

func costOf(c [][]float64, cols []int) float64 {
	sum := 0.0
	for i, j := range cols {
		sum += c[i][j]
	}
	return sum
}

// bruteForce is the cheapest total over every permutation.
func bruteForce(c [][]float64) float64 {
	n := len(c)
	perm := make([]int, n)
	for i := range perm {
		perm[i] = i
	}
	best := math.Inf(1)
	var walk func(k int)
	walk = func(k int) {
		if k == n {
			best = min(best, costOf(c, perm))
			return
		}
		for i := k; i < n; i++ {
			perm[k], perm[i] = perm[i], perm[k]
			walk(k + 1)
			perm[k], perm[i] = perm[i], perm[k]
		}
	}
	walk(0)
	return best
}

func TestAssignBeatsGreedy(t *testing.T) {
	// Greedy takes row 0 → column 0 (cost 1) and leaves row 1 with 10.
	c := [][]float64{{1, 2}, {1, 10}}
	got := assign(c)
	if got[0] != 1 || got[1] != 0 {
		t.Fatalf("got %v", got)
	}
	if got := assign(nil); len(got) != 0 {
		t.Fatalf("empty: %v", got)
	}
}

func TestAssignIsOptimalAndAPermutation(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for trial := 0; trial < 200; trial++ {
		n := 1 + r.IntN(6)
		c := make([][]float64, n)
		for i := range c {
			c[i] = make([]float64, n)
			for j := range c[i] {
				c[i][j] = math.Round(r.Float64()*20) / 20 // ties are common
			}
		}
		got := assign(c)
		seen := map[int]bool{}
		for _, j := range got {
			if j < 0 || j >= n || seen[j] {
				t.Fatalf("not a permutation: %v", got)
			}
			seen[j] = true
		}
		if want := bruteForce(c); math.Abs(costOf(c, got)-want) > 1e-9 {
			t.Fatalf("trial %d: cost %v, optimum %v for %v", trial, costOf(c, got), want, c)
		}
	}
}
