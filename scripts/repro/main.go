// Command repro packs reproducible release archives and compares two builds.
//
//	go run ./scripts/repro pack <folder> <out.tar.gz|out.zip>   (needs SOURCE_DATE_EPOCH)
//	go run ./scripts/repro compare <left> <right>
//
// compare reads <dir>/archives/*.{tar.gz,zip} and <dir>/image (an OCI image
// layout, or image.json holding the manifest list a registry returned) on both
// sides and prints one line per artifact: identical, or the first difference.
// It exits 1 when anything differs.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jrand0m/corsarr/internal/release"
)

func main() {
	if len(os.Args) == 4 && os.Args[1] == "pack" {
		os.Exit(pack(os.Args[2], os.Args[3]))
	}
	if len(os.Args) == 4 && os.Args[1] == "compare" {
		os.Exit(compare(os.Args[2], os.Args[3]))
	}
	fmt.Fprintln(os.Stderr, "usage: repro pack <folder> <out> | repro compare <left> <right>")
	os.Exit(2)
}

func pack(folder, out string) int {
	sec, err := strconv.ParseInt(os.Getenv("SOURCE_DATE_EPOCH"), 10, 64)
	if err != nil {
		fmt.Fprintln(os.Stderr, "SOURCE_DATE_EPOCH must be the commit time in seconds")
		return 2
	}
	if err := release.Pack(folder, out, time.Unix(sec, 0)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func compare(left, right string) int {
	results, err := release.CompareArchives(filepath.Join(left, "archives"), filepath.Join(right, "archives"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if a, aerr := release.ReadImage(filepath.Join(left, "image")); aerr == nil {
		b, berr := release.ReadImage(filepath.Join(right, "image"))
		if berr != nil {
			fmt.Fprintln(os.Stderr, berr)
			return 1
		}
		results = append(results, release.CompareImages(a, b))
	}
	code := 0
	for _, r := range results {
		fmt.Println(r)
		if !r.Same {
			code = 1
		}
	}
	return code
}
