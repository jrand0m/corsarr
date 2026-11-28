package plugin_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/sdk/go/abi"
	"github.com/jrand0m/corsarr/sdk/go/plugin"
)

func TestStatusError(t *testing.T) {
	long := strings.Repeat("x", 200)
	for _, tc := range []struct {
		status int
		body   string
		class  string
		msg    string
	}{
		{200, "fine", "", ""},
		{204, "", "", ""},
		{299, "", "", ""},
		{199, "", abi.ClassParse, "HTTP 199"},
		{300, "", abi.ClassParse, "HTTP 300"},
		{401, "who?", abi.ClassAuth, "HTTP 401: who?"},
		{403, "", abi.ClassAuth, "HTTP 403"},
		{404, "gone", abi.ClassNotFound, "HTTP 404: gone"},
		{429, "", abi.ClassRateLimited, "HTTP 429"},
		{500, "", abi.ClassServerError, "HTTP 500"},
		{503, strings.Repeat("y", 199), abi.ClassServerError, "HTTP 503: " + strings.Repeat("y", 199)},
		{502, long, abi.ClassServerError, "HTTP 502"},
		{418, "teapot", abi.ClassParse, "HTTP 418: teapot"},
		{400, "x", abi.ClassParse, "HTTP 400: x"},
		{499, "", abi.ClassParse, "HTTP 499"},
	} {
		err := plugin.StatusError(tc.status, []byte(tc.body))
		if tc.class == "" {
			if err != nil {
				t.Errorf("%d: %v", tc.status, err)
			}
			continue
		}
		var e *abi.Error
		if !errors.As(err, &e) || e.Class != tc.class || e.Message != tc.msg {
			t.Errorf("%d: got %+v, want %s %q", tc.status, err, tc.class, tc.msg)
		}
	}
}
