package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// packInto writes tree-based archives under dir; edit may change the tree first.
func packInto(t *testing.T, dir string, edit func(root string)) {
	t.Helper()
	root := tree(t, time.Now(), 0o755)
	if edit != nil {
		edit(root)
	}
	must(t, os.MkdirAll(dir, 0o755))
	must(t, Pack(root, filepath.Join(dir, "a.tar.gz"), epoch))
	must(t, Pack(root, filepath.Join(dir, "a.zip"), epoch))
}

func TestCompareArchives(t *testing.T) {
	a, b, c := t.TempDir(), t.TempDir(), t.TempDir()
	packInto(t, a, nil)
	packInto(t, b, nil)
	packInto(t, c, func(root string) { must(t, os.WriteFile(filepath.Join(root, "sub", "x"), []byte("y"), 0o644)) })
	got, err := CompareArchives(a, b)
	must(t, err)
	if len(got) != 2 || !got[0].Same || !got[1].Same || !strings.Contains(got[0].String(), "identical") {
		t.Fatalf("%v", got)
	}
	got, err = CompareArchives(a, c)
	must(t, err)
	for _, r := range got {
		if r.Same || r.Detail != "pkg/sub/x (content)" || !strings.Contains(r.String(), "DIFFERS") {
			t.Fatalf("%v", r)
		}
	}
	if _, err := CompareArchives(a, filepath.Join(a, "missing")); err == nil {
		t.Fatal("missing folder accepted")
	}
}

func TestComparePackages(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	for _, n := range []string{"corsarr_1.0.0_amd64.deb", "corsarr-1.0.0-1.x86_64.rpm", "corsarr_1.0.0_x86_64.apk", "notes.txt"} {
		must(t, os.WriteFile(filepath.Join(a, n), []byte("same"), 0o644))
		must(t, os.WriteFile(filepath.Join(b, n), []byte("same"), 0o644))
	}
	must(t, os.WriteFile(filepath.Join(b, "corsarr_1.0.0_amd64.deb"), []byte("other!"), 0o644))
	got, err := CompareArchives(a, b)
	must(t, err)
	if len(got) != 3 || !got[0].Same || !got[2].Same || got[1].Detail != "package bytes differ (4 vs 6 bytes); diffoscope shows where" {
		t.Fatalf("%v", got)
	}
}

func TestCompareArchiveDetails(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	packInto(t, a, nil)
	packInto(t, b, func(root string) { must(t, os.WriteFile(filepath.Join(root, "extra"), nil, 0o644)) })
	got, _ := CompareArchives(a, b)
	if !strings.Contains(got[0].Detail, "entry 4 is pkg/sub/ on the left and pkg/extra on the right") {
		t.Fatalf("%v", got)
	}
	must(t, os.Remove(filepath.Join(b, "a.zip")))
	must(t, os.WriteFile(filepath.Join(a, "only.zip"), []byte("junk"), 0o644))
	got, _ = CompareArchives(a, b)
	if !strings.HasPrefix(got[1].Detail, "missing on the right") || !strings.HasPrefix(got[2].Detail, "missing on the right") {
		t.Fatalf("%v", got)
	}
	got, _ = CompareArchives(b, a)
	if !strings.HasPrefix(got[1].Detail, "missing on the left") {
		t.Fatalf("%v", got)
	}
}

func TestFirstEntryDiffCases(t *testing.T) {
	if d := firstEntryDiff("x.zip", []byte("a"), []byte("b")); !strings.HasPrefix(d, "unreadable") {
		t.Fatal(d)
	}
	if d := firstEntryDiff("x.tar.gz", []byte("a"), []byte("b")); !strings.HasPrefix(d, "unreadable") {
		t.Fatal(d)
	}
	root := tree(t, time.Now(), 0o755)
	dir := t.TempDir()
	must(t, Pack(root, filepath.Join(dir, "1.tar.gz"), epoch))
	must(t, Pack(root, filepath.Join(dir, "2.tar.gz"), epoch.Add(time.Hour)))
	one, _ := os.ReadFile(filepath.Join(dir, "1.tar.gz"))
	two, _ := os.ReadFile(filepath.Join(dir, "2.tar.gz"))
	if d := firstEntryDiff("x.tar.gz", one, two); !strings.Contains(d, "header") {
		t.Fatal(d)
	}
	if d := firstEntryDiff("x.tar.gz", one, one); !strings.Contains(d, "compression") {
		t.Fatal(d)
	}
	must(t, os.Remove(filepath.Join(root, "sub", "x")))
	must(t, Pack(root, filepath.Join(dir, "3.tar.gz"), epoch))
	three, _ := os.ReadFile(filepath.Join(dir, "3.tar.gz"))
	if d := firstEntryDiff("x.tar.gz", one, three); !strings.Contains(d, "5 entries on the left, 4") {
		t.Fatal(d)
	}
}

const ociIndex = `{"schemaVersion":2,"manifests":[
 {"digest":"sha256:aaa","platform":{"os":"linux","architecture":"amd64"}},
 {"digest":"sha256:bbb","platform":{"os":"linux","architecture":"arm","variant":"v7"}},
 {"digest":"sha256:ccc","platform":{"os":"unknown","architecture":"unknown"}}]}`

func writeLayout(t *testing.T, index string) string {
	t.Helper()
	dir := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(dir, "blobs", "sha256"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "blobs", "sha256", "top"), []byte(index), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "index.json"), []byte(`{"manifests":[{"digest":"sha256:top"}]}`), 0o644))
	return dir
}

func TestReadAndCompareImages(t *testing.T) {
	local, err := ReadImage(writeLayout(t, ociIndex))
	must(t, err)
	if local.Digest != "sha256:top" || len(platforms(local)) != 2 {
		t.Fatalf("%+v", local)
	}
	pub := t.TempDir()
	must(t, os.WriteFile(filepath.Join(pub, "image.json"), []byte(`{"digest":"sha256:top","manifests":[]}`), 0o644))
	published, err := ReadImage(pub)
	must(t, err)
	if r := CompareImages(local, published); !r.Same {
		t.Fatalf("%v", r)
	}
	published.Digest = "sha256:other"
	if r := CompareImages(local, published); r.Same || !strings.Contains(r.Detail, "linux/amd64 manifest sha256:aaa vs (none)") {
		t.Fatalf("%v", r)
	}
	local.Manifests, published.Manifests = nil, nil
	if r := CompareImages(local, published); !strings.Contains(r.Detail, "index metadata") {
		t.Fatalf("%v", r)
	}
	if shortDigest("abc") != "abc" {
		t.Fatal("short digest")
	}
}

func TestReadImageErrors(t *testing.T) {
	if _, err := ReadImage(t.TempDir()); err == nil {
		t.Fatal("empty folder accepted")
	}
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "index.json"), []byte(`{"manifests":[]}`), 0o644))
	if _, err := ReadImage(dir); err == nil {
		t.Fatal("empty layout accepted")
	}
	if err := readJSON(filepath.Join(dir, "missing"), &Index{}); err == nil {
		t.Fatal("missing file accepted")
	}
}
