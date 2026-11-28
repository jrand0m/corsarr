// Command rtorrent is the rTorrent download-client module: torrents over
// rTorrent's XML-RPC interface exposed over HTTP (the /RPC2 endpoint of a web
// front end or reverse proxy). Build it with
//
//	just build plugins rtorrent
//
// and check it with `corsarr plugin test <file.wasm> --settings=<file.json>`.
package main

import (
	"context"
	"fmt"

	"github.com/jrand0m/corsarr/sdk/go/abi"
	"github.com/jrand0m/corsarr/sdk/go/plugin"
	"github.com/jrand0m/corsarr/sdk/go/plugin/downloader"
)

// manifest is plugin.json.
var manifest = plugin.Manifest{
	ID: "rtorrent", Name: "rTorrent", Version: "1.0.0", Kind: abi.KindDownloader, ABIVersion: abi.Version,
	Description: "Sends torrents to rTorrent over XML-RPC (the /RPC2 endpoint of a web front end or reverse proxy) and follows them until they finish.",
	Author:      "corsarr", License: "AGPL-3.0-or-later", Homepage: "https://github.com/jrand0m/corsarr",
	Permissions: abi.Permissions{HTTP: abi.HTTPPermission{Hosts: []string{"$setting:url"}}, Secrets: []string{"password"}},
	Settings:    settingsSchema(),
}

// testAction answers the settings form's test button.
func testAction(string, map[string]any) (string, string) {
	c, err := conn()
	if err != nil {
		return abi.OutcomeFail, "The settings are not usable: " + classify(err).Error()
	}
	ctx := context.Background()
	v, err := c.Version(ctx)
	if err != nil {
		return abi.OutcomeFail, "Could not reach rTorrent: " + classify(err).Error()
	}
	text := fmt.Sprintf("Connected to rTorrent %s (libtorrent %s).", v.Client, v.Library)
	if free, err := c.FreeSpace(ctx); err == nil && free >= 0 {
		text += fmt.Sprintf(" %.1f GB free.", float64(free)/1e9)
	}
	if erase, err := c.SupportsEraseData(ctx); err == nil && !erase {
		return abi.OutcomeWarn, text + " Removing a download keeps its files: no data-erasing handler is installed."
	}
	return abi.OutcomeOK, text
}

func init() {
	downloader.Serve(module{})
	plugin.OnAction(testAction)
}

func main() { plugin.Main(manifest) }
