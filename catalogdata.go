// Package catalogdata embeds the root catalog.yaml so it can live next to
// go.mod for easy editing while remaining available inside the binary.
package catalogdata

import _ "embed"

//go:embed catalog.yaml
var YAML []byte
