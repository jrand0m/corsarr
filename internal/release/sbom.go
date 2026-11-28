package release

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"runtime/debug"
	"strconv"
	"time"
)

// The Go linker stores a module's build information (main module,
// dependencies, build settings) between these markers; WebAssembly builds
// carry it too, although tools that read executables do not look there.
var (
	modInfoStart = []byte{0x30, 0x77, 0xaf, 0x0c, 0x92, 0x74, 0x08, 0x02, 0x41, 0xe1, 0xc1, 0x07, 0xe6, 0xd6, 0x18, 0xe6}
	modInfoEnd   = []byte{0xf9, 0x32, 0x43, 0x31, 0x86, 0x18, 0x20, 0x72, 0x00, 0x82, 0x42, 0x10, 0x41, 0x16, 0xd8, 0xf2}
	goVersionRe  = regexp.MustCompile(`go1\.\d+(\.\d+)?`)
)

// ModuleBuildInfo reads the build information embedded in a Go WebAssembly
// module, with the Go version the runtime carries.
func ModuleBuildInfo(module []byte) (*debug.BuildInfo, error) {
	start := bytes.Index(module, modInfoStart)
	end := bytes.Index(module, modInfoEnd)
	if start < 0 || end < start {
		return nil, errors.New("no Go build information in the module")
	}
	info, err := debug.ParseBuildInfo(string(module[start+len(modInfoStart) : end]))
	if err != nil {
		return nil, err
	}
	if info.GoVersion == "" {
		info.GoVersion = mostFrequent(goVersionRe.FindAllString(string(module), -1))
	}
	return info, nil
}

func mostFrequent(xs []string) string {
	count, best := map[string]int{}, ""
	for _, x := range xs {
		count[x]++
		if count[x] > count[best] || (count[x] == count[best] && x < best) {
			best = x
		}
	}
	return best
}

type spdxDoc struct {
	SPDXVersion       string         `json:"spdxVersion"`
	DataLicense       string         `json:"dataLicense"`
	SPDXID            string         `json:"SPDXID"`
	Name              string         `json:"name"`
	DocumentNamespace string         `json:"documentNamespace"`
	CreationInfo      spdxCreation   `json:"creationInfo"`
	Packages          []spdxPackage  `json:"packages"`
	Relationships     []spdxRelation `json:"relationships"`
}

type spdxCreation struct {
	Created  string   `json:"created"`
	Creators []string `json:"creators"`
}

type spdxPackage struct {
	Name             string         `json:"name"`
	SPDXID           string         `json:"SPDXID"`
	VersionInfo      string         `json:"versionInfo,omitempty"`
	DownloadLocation string         `json:"downloadLocation"`
	FilesAnalyzed    bool           `json:"filesAnalyzed"`
	Checksums        []spdxChecksum `json:"checksums,omitempty"`
	ExternalRefs     []spdxRef      `json:"externalRefs,omitempty"`
}

type spdxChecksum struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"checksumValue"`
}

type spdxRef struct {
	Category string `json:"referenceCategory"`
	Type     string `json:"referenceType"`
	Locator  string `json:"referenceLocator"`
}

type spdxRelation struct {
	Element string `json:"spdxElementId"`
	Type    string `json:"relationshipType"`
	Related string `json:"relatedSpdxElement"`
}

func golangPackage(id, path, version string) spdxPackage {
	p := spdxPackage{Name: path, SPDXID: id, VersionInfo: version, DownloadLocation: "NOASSERTION"}
	if version != "" && version != "(devel)" {
		p.ExternalRefs = []spdxRef{{Category: "PACKAGE-MANAGER", Type: "purl", Locator: "pkg:golang/" + path + "@" + version}}
	}
	return p
}

// ModuleSBOM describes a plugin module as an SPDX 2.3 JSON document: the
// file with its SHA-256, the Go standard library, the main module and every
// dependency linked in, read from the module's own build information.
// created (the release's source date) and namespace keep it reproducible.
func ModuleSBOM(name string, module []byte, created time.Time, namespace string) ([]byte, error) {
	info, err := ModuleBuildInfo(module)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(module)
	doc := spdxDoc{SPDXVersion: "SPDX-2.3", DataLicense: "CC0-1.0", SPDXID: "SPDXRef-DOCUMENT", Name: name,
		DocumentNamespace: namespace,
		CreationInfo:      spdxCreation{Created: created.UTC().Format(time.RFC3339), Creators: []string{"Tool: corsarr-pluginindex"}},
		Packages: []spdxPackage{{Name: name, SPDXID: "SPDXRef-Module", DownloadLocation: "NOASSERTION",
			Checksums: []spdxChecksum{{Algorithm: "SHA256", Value: hex.EncodeToString(sum[:])}}}},
		Relationships: []spdxRelation{{Element: "SPDXRef-DOCUMENT", Type: "DESCRIBES", Related: "SPDXRef-Module"}},
	}
	add := func(p spdxPackage) {
		doc.Packages = append(doc.Packages, p)
		doc.Relationships = append(doc.Relationships, spdxRelation{Element: "SPDXRef-Module", Type: "CONTAINS", Related: p.SPDXID})
	}
	if info.GoVersion != "" {
		add(golangPackage("SPDXRef-Go-stdlib", "stdlib", info.GoVersion))
	}
	add(golangPackage("SPDXRef-Main", info.Main.Path, info.Main.Version))
	for i, d := range info.Deps {
		if d.Replace != nil {
			d = d.Replace
		}
		add(golangPackage("SPDXRef-Dep-"+strconv.Itoa(i+1), d.Path, d.Version))
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	return append(b, '\n'), err
}
