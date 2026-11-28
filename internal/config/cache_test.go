package config

import (
	"errors"
	"testing"
)

func TestCacheDefaults(t *testing.T) {
	c := mustLoad(t, inputs(nil, nil, nil)).Cache
	want := Cache{Enabled: true, MaxSize: 1 << 30, Images: CacheImages{Quality: 82, MaxWidth: 600, BackdropMaxWidth: 1280,
		Degrade: CacheDegrade{QualityFloor: 60, QualityStep: 10, WidthFloor: 300, WidthStep: 0.75, HighWater: 0.9, LowWater: 0.8}}}
	if c != want {
		t.Fatalf("defaults\n got %+v\nwant %+v", c, want)
	}
	if Live("cache") {
		t.Fatal("cache applies at start")
	}
}

func TestCacheFileValues(t *testing.T) {
	yml := "cache:\n  enabled: false\n  dir: /srv/cache\n  maxSize: 50MiB\n  images:\n    quality: 70\n    degrade:\n      lowWater: 0.5\n"
	c := mustLoad(t, inputs([]string{"--configFile=/c.yml"}, nil, map[string]string{"/c.yml": yml})).Cache
	if c.Enabled || c.Dir != "/srv/cache" || c.MaxSize != 50<<20 || c.Images.Quality != 70 || c.Images.Degrade.LowWater != 0.5 {
		t.Fatalf("%+v", c)
	}
}

func TestCacheEnvValues(t *testing.T) {
	env := []string{"CORSARR_CACHE_MAXSIZE=2GiB", "CORSARR_CACHE_IMAGES_DEGRADE_LOWWATER=0.6", "CORSARR_CACHE_IMAGES_DEGRADE_WIDTHFLOOR=200"}
	c := mustLoad(t, inputs(nil, env, nil)).Cache
	if c.MaxSize != 2<<30 || c.Images.Degrade.LowWater != 0.6 || c.Images.Degrade.WidthFloor != 200 {
		t.Fatalf("%+v", c)
	}
}

func TestCacheRejectsBadValues(t *testing.T) {
	for _, bad := range []string{
		"CORSARR_CACHE_MAXSIZE=lots", "CORSARR_CACHE_MAXSIZE=100KiB", "CORSARR_CACHE_IMAGES_QUALITY=99",
		"CORSARR_CACHE_DIR=relative/path", "CORSARR_CACHE_IMAGES_DEGRADE_WIDTHSTEP=1.5",
		"CORSARR_CACHE_IMAGES_DEGRADE_LOWWATER=0.95", "CORSARR_CACHE_IMAGES_DEGRADE_QUALITYFLOOR=90",
		"CORSARR_CACHE_IMAGES_DEGRADE_WIDTHFLOOR=900",
	} {
		_, err := Load(inputs(nil, []string{bad}, nil), testRegistry(t))
		var errs Errors
		if !errors.As(err, &errs) || len(errs) == 0 {
			t.Errorf("%s accepted (%v)", bad, err)
		}
	}
}

func TestParseSize(t *testing.T) {
	cases := map[string]int64{"1GiB": 1 << 30, "500MB": 500 << 20, "1.5G": 3 << 29, "2048": 2048, " 4 kib ": 4096, "1TB": 1 << 40, "10b": 10}
	for in, want := range cases {
		if got, err := ParseSize(in); err != nil || got != want {
			t.Errorf("%q = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "GiB", "1 PB", "-1MiB", "1e30GiB", "1..2MB"} {
		if _, err := ParseSize(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
