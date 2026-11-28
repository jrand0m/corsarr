// Command pluginindex writes, signs and checks the plugin index a release
// publishes, and describes plugin modules as SBOMs.
//
//	go run ./scripts/pluginindex build -release vX.Y.Z [-min-corsarr X.Y.Z] -epoch <unix> -o plugins-index.json <module.wasm>…
//	go run ./scripts/pluginindex sign -key index-key.pem -o plugins-index.json.sig plugins-index.json
//	go run ./scripts/pluginindex verify -pub <keyId>.pub plugins-index.json plugins-index.json.sig
//	go run ./scripts/pluginindex keygen -o index-key.pem    (prints the key id and the public key line)
//	go run ./scripts/pluginindex sbom -epoch <unix> -namespace <url> -o <module>.spdx.json <module.wasm>
//
// Module files must be named <id>-<version>.wasm after their embedded
// manifest. The signed message is "corsarr-plugin-index-v1\n" followed by the
// exact index bytes.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jrand0m/corsarr/internal/release"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmds := map[string]func([]string) error{"build": build, "sign": sign, "verify": verify, "keygen": keygen, "sbom": sbom}
	run, ok := cmds[os.Args[1]]
	if !ok {
		usage()
	}
	if err := run(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "pluginindex:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: pluginindex build|sign|verify|keygen|sbom … (see the package comment)")
	os.Exit(2)
}

func epoch(v int64) (time.Time, error) {
	if v < 0 {
		return time.Time{}, errors.New("-epoch must be the source date in Unix seconds")
	}
	return time.Unix(v, 0), nil
}

func build(args []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	rel := fs.String("release", "", "release tag")
	minVersion := fs.String("min-corsarr", "", "oldest corsarr that runs the modules (the version being released, without v)")
	ep := fs.Int64("epoch", -1, "source date (SOURCE_DATE_EPOCH)")
	out := fs.String("o", "plugins-index.json", "output file")
	_ = fs.Parse(args)
	when, err := epoch(*ep)
	if err != nil || *rel == "" {
		return errors.Join(err, errors.New("-release and -epoch are required"))
	}
	files := make([]release.ModuleFile, 0, fs.NArg())
	for _, p := range fs.Args() {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files = append(files, release.ModuleFile{Name: filepath.Base(p), Data: b})
	}
	idx, err := release.BuildIndex(*rel, *minVersion, when, files)
	if err != nil {
		return err
	}
	return os.WriteFile(*out, idx, 0o644)
}

func sign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	keyFile := fs.String("key", "", "Ed25519 private key, PKCS#8 PEM")
	out := fs.String("o", "", "signature file (default: <index>.sig)")
	_ = fs.Parse(args)
	if fs.NArg() != 1 || *keyFile == "" {
		return errors.New("sign -key <key.pem> [-o file] <index.json>")
	}
	pemBytes, err := os.ReadFile(*keyFile)
	if err != nil {
		return err
	}
	key, err := release.ParsePrivateKey(pemBytes)
	if err != nil {
		return err
	}
	index, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	sig, err := release.SignIndex(index, key)
	if err != nil {
		return err
	}
	if *out == "" {
		*out = fs.Arg(0) + ".sig"
	}
	return os.WriteFile(*out, sig, 0o644)
}

func verify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	var pubs []string
	fs.Func("pub", "trusted public key file (repeatable)", func(s string) error { pubs = append(pubs, s); return nil })
	_ = fs.Parse(args)
	if fs.NArg() != 2 || len(pubs) == 0 {
		return errors.New("verify -pub <key.pub> <index.json> <index.json.sig>")
	}
	keys := map[string]ed25519.PublicKey{}
	for _, p := range pubs {
		line, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		pub, err := release.ParsePublicKey(line)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		keys[release.KeyID(pub)] = pub
	}
	index, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	sig, err := os.ReadFile(fs.Arg(1))
	if err != nil {
		return err
	}
	if err := release.VerifyIndex(index, sig, keys); err != nil {
		return err
	}
	fmt.Println("signature ok")
	return nil
}

func keygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	out := fs.String("o", "index-key.pem", "private key file (created, mode 0600)")
	_ = fs.Parse(args)
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	pemBytes, err := release.MarshalPrivateKey(key)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(pemBytes); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Printf("keyId %s\n%s.pub: %s", release.KeyID(pub), release.KeyID(pub), release.PublicKeyLine(pub))
	return nil
}

func sbom(args []string) error {
	fs := flag.NewFlagSet("sbom", flag.ExitOnError)
	ep := fs.Int64("epoch", -1, "source date (SOURCE_DATE_EPOCH)")
	ns := fs.String("namespace", "", "SPDX document namespace (a URL unique to this file)")
	out := fs.String("o", "", "output file (default: <module>.spdx.json)")
	_ = fs.Parse(args)
	when, err := epoch(*ep)
	if err != nil || fs.NArg() != 1 || *ns == "" {
		return errors.Join(err, errors.New("sbom -epoch <unix> -namespace <url> [-o file] <module.wasm>"))
	}
	b, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	doc, err := release.ModuleSBOM(filepath.Base(fs.Arg(0)), b, when, *ns)
	if err != nil {
		return err
	}
	if *out == "" {
		*out = fs.Arg(0) + ".spdx.json"
	}
	return os.WriteFile(*out, doc, 0o644)
}
