// Command transmission is the Transmission download-client module: torrents
// over Transmission's RPC interface (3.x and 4.x). Build it with
//
//	just build plugins transmission
//
// and check it with `corsarr plugin test <file.wasm> --settings=<file.json>`.
package main

import (
	"context"
	"fmt"

	"github.com/jrand0m/corsarr/plugins/transmission/client"
	"github.com/jrand0m/corsarr/sdk/go/abi"
	"github.com/jrand0m/corsarr/sdk/go/plugin"
	"github.com/jrand0m/corsarr/sdk/go/plugin/downloader"
)

// manifest is plugin.json.
var manifest = plugin.Manifest{
	ID: "transmission", Name: "Transmission", Version: "1.0.0", Kind: abi.KindDownloader, ABIVersion: abi.Version,
	Description: "Sends torrents to Transmission 3.x or 4.x over its RPC interface and follows them until they finish.",
	Author:      "corsarr", License: "AGPL-3.0-or-later", Homepage: "https://github.com/jrand0m/corsarr",
	Permissions: abi.Permissions{HTTP: abi.HTTPPermission{Hosts: []string{"$setting:" + client.KeyURL}},
		Secrets: []string{client.KeyPassword}},
	Settings: settingsSchema(),
}

// testAction answers the settings form's test button.
func testAction(string, map[string]any) (string, string) {
	c, err := conn()
	if err != nil {
		return abi.OutcomeFail, "The settings are not usable: " + classify(err).Error()
	}
	r, err := c.Test(context.Background())
	if err != nil {
		return abi.OutcomeFail, "Could not reach Transmission: " + classify(err).Error()
	}
	return abi.OutcomeOK, fmt.Sprintf("Connected to Transmission %s (RPC %d), %.1f GB free in %s.",
		r.Version, r.RPCVersion, float64(r.FreeSpace)/1e9, r.DownloadDir)
}

func init() {
	downloader.Serve(module{})
	plugin.OnAction(testAction)
}

func main() { plugin.Main(manifest) }
