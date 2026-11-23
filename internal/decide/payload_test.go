package decide

import (
	"reflect"
	"testing"
)

func TestPayloadVerdict(t *testing.T) {
	var shipped PayloadPolicy
	cases := []struct {
		name      string
		files     []PayloadFile
		wantVideo bool
		want      PayloadVerdict
	}{
		{"no file list yet", nil, true, PayloadVerdict{Status: PayloadUnknown}},
		{"plain video", []PayloadFile{{"Movie/movie.mkv", 5e9}, {"Movie/movie.srt", 1e5}}, true, PayloadVerdict{Status: PayloadSafe}},
		{"screensaver malware", []PayloadFile{{"Movie.2020.1080p/Movie.2020.1080p.scr", 2e6}}, true,
			PayloadVerdict{Status: PayloadDangerous, Files: []string{"Movie.2020.1080p/Movie.2020.1080p.scr"}}},
		{"double extension", FilesFromNames([]string{"movie.mkv.exe", "sample.mkv"}), true,
			PayloadVerdict{Status: PayloadDangerous, Files: []string{"movie.mkv.exe"}}},
		{"upper case and trailing dot", FilesFromNames([]string{`Movie\SETUP.EXE.`}), true,
			PayloadVerdict{Status: PayloadDangerous, Files: []string{`Movie\SETUP.EXE.`}}},
		{"executable listed inside an archive", FilesFromNames([]string{"movie.rar/codec.msi"}), true,
			PayloadVerdict{Status: PayloadDangerous, Files: []string{"movie.rar/codec.msi"}}},
		{"right-to-left override disguise", FilesFromNames([]string{"movie‮vkm.txt"}), true,
			PayloadVerdict{Status: PayloadDangerous, Files: []string{"movie‮vkm.txt"}}},
		{"largest file is not video", []PayloadFile{{"movie.zip", 4e9}, {"sample.mkv", 5e7}}, true,
			PayloadVerdict{Status: PayloadNotVideo, Files: []string{"movie.zip"}}},
		{"largest-file rule only for video", []PayloadFile{{"album.zip", 4e8}}, false, PayloadVerdict{Status: PayloadSafe}},
		{"sizes unknown skip largest-file rule", FilesFromNames([]string{"movie.zip"}), true, PayloadVerdict{Status: PayloadSafe}},
		{"disc image counts as video", []PayloadFile{{"movie.iso", 4e10}}, true, PayloadVerdict{Status: PayloadSafe}},
	}
	for _, c := range cases {
		if got := shipped.Verdict(c.files, c.wantVideo); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestPayloadPolicy_UserAdditionsOnly(t *testing.T) {
	p, err := NewPayloadPolicy(".XYZ", " cab ")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"a.xyz", "b.cab", "c.exe"} {
		if p.Verdict(FilesFromNames([]string{f}), true).Status != PayloadDangerous {
			t.Errorf("%s should be dangerous", f)
		}
	}
	for _, bad := range []string{"", ".", "e x e", "waytoolongextension1"} {
		if _, err := NewPayloadPolicy(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestDangerousExtensions_ShipsTheMinimum(t *testing.T) {
	got := map[string]bool{}
	for _, e := range DangerousExtensions() {
		got[e] = true
	}
	for _, must := range []string{"exe", "scr", "bat", "cmd", "com", "msi", "pif", "vbs", "vbe", "js", "jse",
		"wsf", "ps1", "jar", "lnk", "apk", "dll", "hta", "cpl", "dmg", "app"} {
		if !got[must] {
			t.Errorf("missing %s", must)
		}
	}
}
