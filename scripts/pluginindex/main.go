// Command pluginindex builds, signs and checks the plugin catalogue index a
// release publishes next to its modules.
//
//	go run ./scripts/pluginindex build --release vX.Y.Z [--min X.Y.Z] --out <dir> <module.wasm>…
//	go run ./scripts/pluginindex sign --key <index-key.pem> <dir>/plugins-index.json
//	go run ./scripts/pluginindex verify --pub <keyId>.pub <dir>/plugins-index.json
//	go run ./scripts/pluginindex keygen <dir>
//
// build copies each module to <dir>/<id>-<version>.wasm and writes
// <dir>/plugins-index.json (generated = SOURCE_DATE_EPOCH when set); sign
// writes plugins-index.json.sig; keygen writes index-key.pem (keep it secret)
// and <keyId>.pub (commit it under internal/catalogue/trusted/).
package main

import (
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jrand0m/corsarr/internal/catalogue"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "build":
		err = build(os.Args[2:])
	case "sign":
		err = sign(os.Args[2:])
	case "verify":
		err = verify(os.Args[2:])
	case "keygen":
		err = keygen(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "pluginindex:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: pluginindex build|sign|verify|keygen …")
	os.Exit(2)
}

func build(args []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	release := fs.String("release", "", "release tag, vX.Y.Z")
	minVer := fs.String("min", "", "oldest corsarr that can run the modules (default: the release without v)")
	out := fs.String("out", "", "output folder")
	maxMiB := fs.Int64("max-mib", 32, "largest module accepted, in MiB")
	_ = fs.Parse(args)
	if *release == "" || *out == "" || fs.NArg() == 0 {
		return errors.New("build needs --release, --out and at least one module")
	}
	if *minVer == "" {
		*minVer = (*release)[1:]
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	var entries []catalogue.Entry
	for _, path := range fs.Args() {
		mod, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		e, err := catalogue.EntryFromModule(mod, *minVer)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if err := os.WriteFile(filepath.Join(*out, e.URL), mod, 0o644); err != nil {
			return err
		}
		entries = append(entries, e)
	}
	index, err := catalogue.BuildIndex(*release, generated(), entries, *maxMiB<<20)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(*out, "plugins-index.json"), index, 0o644)
}

func generated() time.Time {
	if s, err := strconv.ParseInt(os.Getenv("SOURCE_DATE_EPOCH"), 10, 64); err == nil {
		return time.Unix(s, 0)
	}
	return time.Now()
}

func sign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	keyPath := fs.String("key", "", "PKCS#8 PEM Ed25519 private key")
	_ = fs.Parse(args)
	if *keyPath == "" || fs.NArg() != 1 {
		return errors.New("sign needs --key and the index file")
	}
	pemKey, err := os.ReadFile(*keyPath)
	if err != nil {
		return err
	}
	priv, err := catalogue.ParseSigningKey(pemKey)
	if err != nil {
		return err
	}
	index, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	return os.WriteFile(fs.Arg(0)+".sig", catalogue.Sign(index, priv), 0o644)
}

func verify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	pubPath := fs.String("pub", "", "public key file (<keyId>.pub)")
	_ = fs.Parse(args)
	if *pubPath == "" || fs.NArg() != 1 {
		return errors.New("verify needs --pub and the index file")
	}
	pubText, err := os.ReadFile(*pubPath)
	if err != nil {
		return err
	}
	key, err := catalogue.ParseKey(string(pubText))
	if err != nil {
		return err
	}
	index, err1 := os.ReadFile(fs.Arg(0))
	sig, err2 := os.ReadFile(fs.Arg(0) + ".sig")
	if err := errors.Join(err1, err2); err != nil {
		return err
	}
	if err := catalogue.Verify(index, sig, []catalogue.Key{key}); err != nil {
		return err
	}
	ix, err := catalogue.Decode(index, 0)
	if err != nil {
		return err
	}
	fmt.Printf("ok: %s, %d plugins, signed by %s\n", ix.Release, len(ix.Plugins), key.ID)
	return nil
}

func keygen(args []string) error {
	if len(args) != 1 {
		return errors.New("keygen needs the output folder")
	}
	pemKey, pub, err := catalogue.NewSigningKey(rand.Reader)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(args[0], 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(args[0], "index-key.pem"), pemKey, 0o600); err != nil {
		return err
	}
	fmt.Println("key id:", pub.ID)
	return os.WriteFile(filepath.Join(args[0], pub.ID+".pub"), []byte(catalogue.PublicKeyFile(pub)), 0o644)
}
