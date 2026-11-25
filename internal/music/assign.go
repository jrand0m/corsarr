package music

import "math"

// assign solves the square assignment problem: it returns, for each row of
// cost, the column given to it so that every column is used once and the
// summed cost is the smallest possible. It grows the assignment one row at
// a time along a shortest augmenting path, keeping dual potentials for rows
// and columns so reduced costs never go negative (O(n³)).
func assign(cost [][]float64) []int {
	n := len(cost)
	s := solver{cost: cost, u: make([]float64, n+1), v: make([]float64, n+1), owner: make([]int, n+1), prev: make([]int, n+1)}
	for row := 1; row <= n; row++ {
		s.addRow(row)
	}
	out := make([]int, n)
	for col := 1; col <= n; col++ {
		out[s.owner[col]-1] = col - 1
	}
	return out
}

// solver holds the state of assign. Indices are 1-based; column 0 is a
// virtual column that holds the row being added.
type solver struct {
	cost  [][]float64
	u, v  []float64 // row and column potentials
	owner []int     // owner[col] is the row holding col (0 = free)
	prev  []int     // prev[col] is the column before col on the current path
}

// addRow assigns row, re-routing earlier rows along the cheapest path.
func (s *solver) addRow(row int) {
	n := len(s.cost)
	s.owner[0] = row
	slack := make([]float64, n+1)
	for i := range slack {
		slack[i] = math.Inf(1)
	}
	used := make([]bool, n+1)
	col := 0
	for s.owner[col] != 0 {
		used[col] = true
		col = s.step(col, slack, used)
	}
	for col != 0 {
		before := s.prev[col]
		s.owner[col] = s.owner[before]
		col = before
	}
}

// step extends the path from col: it lowers the slack of the free columns
// through col's row, then shifts the potentials by the smallest slack and
// returns the column that reaches.
func (s *solver) step(col int, slack []float64, used []bool) int {
	row := s.owner[col]
	next, delta := 0, math.Inf(1)
	for j := 1; j < len(slack); j++ {
		if used[j] {
			continue
		}
		if r := s.cost[row-1][j-1] - s.u[row] - s.v[j]; r < slack[j] {
			slack[j], s.prev[j] = r, col
		}
		if slack[j] < delta {
			delta, next = slack[j], j
		}
	}
	s.shift(delta, slack, used)
	return next
}

// shift moves the potentials by delta: rows and columns on the path keep
// their reduced costs, free columns lose delta of slack.
func (s *solver) shift(delta float64, slack []float64, used []bool) {
	for j := range slack {
		if used[j] {
			s.u[s.owner[j]] += delta
			s.v[j] -= delta
		} else {
			slack[j] -= delta
		}
	}
}
