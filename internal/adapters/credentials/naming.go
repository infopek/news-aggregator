// Package credentials provides OS credential-vault adapters and opaque,
// source-stable credential references.
package credentials

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/infopek/news-aggregator/internal/domain"
)

const ApplicationNamespace = "infopek.news-aggregator.v1"

// MaxSecretBytes matches the Windows generic-credential blob limit supported
// by the MVP target.
const MaxSecretBytes = 5 * 512

// ReferenceForSource is stable across source display-name changes. Source IDs
// are immutable and the digest avoids disclosing them in the operating-system
// credential inventory.
func ReferenceForSource(sourceID domain.SourceID) domain.CredentialID {
	digest := sha256.Sum256([]byte(ApplicationNamespace + "\x00" + string(sourceID)))
	return domain.CredentialID("source-" + hex.EncodeToString(digest[:]))
}

func targetName(id domain.CredentialID) string {
	digest := sha256.Sum256([]byte(ApplicationNamespace + "\x00credential\x00" + string(id)))
	return ApplicationNamespace + "/" + hex.EncodeToString(digest[:])
}
