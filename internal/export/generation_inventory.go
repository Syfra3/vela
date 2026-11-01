package export

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/Syfra3/vela/internal/generation"
	"github.com/Syfra3/vela/pkg/types"
)

const InventoryVersion = generation.InventoryVersion
const ExtractorFingerprint = generation.ExtractorFingerprint
const DiscoveryFingerprint = generation.DiscoveryFingerprint
const ChildDiscoveryFingerprint = generation.ChildDiscoveryFingerprint

func CanonicalRoot(root string) (string, error) { return generation.CanonicalRoot(root) }
func Inventory(root string, r types.ManifestRequest) (*types.Manifest, []string, error) {
	return generation.Inventory(root, r)
}
func RequestFingerprint(r types.ManifestRequest) string  { return generation.RequestFingerprint(r) }
func CheckInventory(m *types.Manifest) ([]string, error) { return generation.CheckInventory(m) }
func DiscoverChildRoots(root string) ([]string, error)   { return generation.DiscoverChildRoots(root) }
func digestBytes(data []byte) string                     { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func regularFile(path string) ([]byte, error)            { return generation.RegularFile(path) }
