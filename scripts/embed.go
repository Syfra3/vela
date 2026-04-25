package scripts

import _ "embed"

// LeidenPy is the bundled clustering helper shipped inside the Go binary.
//
//go:embed leiden.py
var LeidenPy []byte
