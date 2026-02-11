// SPDX-License-Identifier: AGPL-3.0-or-later

package buildinfo

import (
	"os"
	"strings"
)

const (
	CoreAppID       = "flwd"
	CoreSpecVersion = "1.0.1"
)

// Version returns the runtime build version exposed by server introspection and metrics.
func Version() string {
	version := strings.TrimSpace(os.Getenv("FLWD_VERSION"))
	if version == "" {
		return "dev"
	}
	return version
}
