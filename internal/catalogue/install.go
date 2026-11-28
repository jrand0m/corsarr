package catalogue

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"

	"github.com/jrand0m/corsarr/internal/wasm"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// VerifyError is a module download that does not match its index entry;
// nothing from it may be installed.
type VerifyError struct{ Reason string }

func (e *VerifyError) Error() string { return "module refused: " + e.Reason }

func refused(format string, a ...any) error { return &VerifyError{Reason: fmt.Sprintf(format, a...)} }

// Download fetches the module of e from the catalogue at indexURL and
// verifies it: exact size, SHA-256 and an embedded manifest whose identity
// and permissions equal the entry. The bytes returned are safe to hand to the
// module store with e.SHA256.
func Download(ctx context.Context, doer Doer, indexURL string, e Entry) ([]byte, error) {
	u, err := ResolveURL(indexURL, e)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("module download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("module download answered HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, e.Size+1))
	if err != nil {
		return nil, fmt.Errorf("module download: %w", err)
	}
	return body, VerifyModule(body, e)
}

// VerifyModule checks downloaded bytes against their entry (pure).
func VerifyModule(module []byte, e Entry) error {
	if int64(len(module)) != e.Size {
		return refused("it is %d bytes, the catalogue says %d", len(module), e.Size)
	}
	if sum := wasm.SHA256(module); sum != e.SHA256 {
		return refused("its SHA-256 is %s, the catalogue says %s", sum, e.SHA256)
	}
	raw, ok := wasm.CustomSection(module, abi.ManifestSection)
	if !ok {
		return refused("it has no embedded manifest")
	}
	man, err := wasm.ParseManifest(raw)
	if err != nil {
		return refused("%v", err)
	}
	if field := manifestDiffers(man, e); field != "" {
		return refused("its manifest %s differs from the catalogue entry", field)
	}
	return nil
}

func manifestDiffers(m abi.Manifest, e Entry) string {
	pairs := []struct {
		name string
		same bool
	}{
		{"id", m.ID == e.ID}, {"version", m.Version == e.Version}, {"kind", wasm.ManifestKind(m) == e.Kind},
		{"abiVersion", m.ABIVersion == e.ABIVersion}, {"name", m.Name == e.Name},
		{"description", m.Description == e.Description}, {"author", m.Author == e.Author},
		{"license", m.License == e.License}, {"homepage", m.Homepage == e.Homepage},
		{"permissions", samePermissions(m.Permissions, e.Permissions)},
	}
	for _, p := range pairs {
		if !p.same {
			return p.name
		}
	}
	return ""
}

func samePermissions(a, b abi.Permissions) bool {
	return slices.Equal(a.HTTP.Hosts, b.HTTP.Hosts) && slices.Equal(a.Secrets, b.Secrets)
}
