// Command crap reports per-function CRAP scores for Go code (the quality gate).
//
//	CRAP(f) = comp(f)^2 * (1 - cov(f))^3 + comp(f)
//
// comp is cyclomatic complexity from the AST; cov is statement coverage from a
// `go test -coverprofile` file. Exits 1 when any function scores >= -max.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type block struct{ startLine, endLine, stmts, count int }

type fnScore struct {
	name       string
	comp       int
	cov, score float64
}

func main() {
	profile := flag.String("profile", "coverage.out", "coverage profile")
	maxScore := flag.Float64("max", 10, "fail threshold")
	module := flag.String("module", "github.com/jrand0m/corsarr/", "module prefix to strip")
	flag.Parse()

	blocks, err := readProfile(*profile, *module)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var scores []fnScore
	for file, bs := range blocks {
		s, err := scoreFile(file, bs)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		scores = append(scores, s...)
	}
	sort.Slice(scores, func(i, j int) bool { return scores[i].score > scores[j].score })
	failed := 0
	for _, s := range scores {
		if s.score >= *maxScore {
			failed++
			fmt.Printf("CRAP %6.1f  comp %3d  cov %5.1f%%  %s\n", s.score, s.comp, s.cov*100, s.name)
		}
	}
	fmt.Printf("%d functions, %d at or above CRAP %.0f\n", len(scores), failed, *maxScore)
	if failed > 0 {
		os.Exit(1)
	}
}

// readProfile parses "file:l.c,l.c stmts count" lines, grouped by file path.
func readProfile(path, module string) (map[string][]block, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string][]block{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "mode:") {
			continue
		}
		file, b, ok := parseLine(line)
		if !ok {
			return nil, fmt.Errorf("bad profile line: %q", line)
		}
		file = strings.TrimPrefix(file, module)
		out[file] = append(out[file], b)
	}
	return out, sc.Err()
}

func parseLine(line string) (string, block, bool) {
	colon := strings.LastIndex(line, ":")
	fields := strings.Fields(line[colon+1:])
	if colon < 0 || len(fields) != 3 {
		return "", block{}, false
	}
	span := strings.Split(fields[0], ",")
	start, _ := strconv.Atoi(strings.Split(span[0], ".")[0])
	end, _ := strconv.Atoi(strings.Split(span[1], ".")[0])
	stmts, _ := strconv.Atoi(fields[1])
	count, _ := strconv.Atoi(fields[2])
	return line[:colon], block{start, end, stmts, count}, true
}

func scoreFile(file string, bs []block) ([]fnScore, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filepath.Clean(file), nil, 0)
	if err != nil {
		return nil, err
	}
	var out []fnScore
	for _, d := range node.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		from, to := fset.Position(fn.Pos()).Line, fset.Position(fn.End()).Line
		comp := complexity(fn)
		cov := coverage(bs, from, to)
		out = append(out, fnScore{
			name:  fmt.Sprintf("%s:%d %s", file, from, fn.Name.Name),
			comp:  comp,
			cov:   cov,
			score: float64(comp*comp)*math.Pow(1-cov, 3) + float64(comp),
		})
	}
	return out, nil
}

func coverage(bs []block, from, to int) float64 {
	total, hit := 0, 0
	for _, b := range bs {
		if b.startLine >= from && b.endLine <= to {
			total += b.stmts
			if b.count > 0 {
				hit += b.stmts
			}
		}
	}
	if total == 0 {
		return 1
	}
	return float64(hit) / float64(total)
}

func complexity(fn *ast.FuncDecl) int {
	c := 1
	ast.Inspect(fn, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			c++
		case *ast.CaseClause:
			if x.List != nil {
				c++
			}
		case *ast.CommClause:
			if x.Comm != nil {
				c++
			}
		case *ast.BinaryExpr:
			if x.Op == token.LAND || x.Op == token.LOR {
				c++
			}
		}
		return true
	})
	return c
}
