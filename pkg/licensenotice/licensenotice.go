// Package licensenotice carries the proprietary notice that every NopsAI
// installation must accept before first-install setup can complete.
//
// The notice is held here as a constant rather than embedded from the
// repository root, because go:embed cannot reach outside a package directory.
// TestLicenseNoticeMatchesShippedLicenseFile in the repository root fails if
// this copy ever drifts from the LICENSE file shipped beside the binaries.
package licensenotice

import (
	"crypto/sha256"
	"encoding/hex"
)

// Version identifies the notice document itself, not the product release. Bump
// it whenever Text changes, so existing installations are asked to accept the
// new wording instead of silently inheriting acceptance of the old one.
const Version = "2026-01"

// Text is the proprietary notice, byte-for-byte identical to the repository
// LICENSE file and to the copy shipped at /usr/share/licenses/nopsai/LICENSE
// in every container image.
const Text = `NopsAI Proprietary Software Notice

Copyright (c) 2026 Hossein Yousefi. All rights reserved.

NopsAI and the associated source code, binaries, container images, Helm charts,
configuration, documentation, examples, and other materials (collectively, the
"Software") are proprietary software.

No licence or other right is granted merely by possession of or access to the
Software. Use, installation, execution, copying, modification, distribution,
sublicensing, hosting, managed operation, provision to third parties, or creation
of derivative works is permitted only under a written agreement signed by
Hossein Yousefi or by a successor entity to which the relevant rights have been
assigned.

Nothing in this notice restricts rights that cannot lawfully be excluded or
limited under applicable law.

Third-party components are not licensed under this notice and remain subject to
their applicable licence terms. See THIRD_PARTY_NOTICES.md and the release
materials supplied with the Software.

For licensing enquiries: contact@nopsai.com
`

// SHA256 returns the hex digest of Text. Acceptance records the digest so an
// audit can prove which exact wording an administrator agreed to.
func SHA256() string {
	sum := sha256.Sum256([]byte(Text))
	return hex.EncodeToString(sum[:])
}
