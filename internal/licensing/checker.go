// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

// Package licensing defines the pluggable license verification interface.
// The open-source build ships OpenCoreChecker (always valid, community tier).
// The commercial build ships the enterprise implementation from this package's
// enterprise/ subdirectory, which is NOT included in the public repository.
package licensing

// Checker is the license verification interface.
// Open-source default: OpenCoreChecker.
// Commercial: inject enterprise checker at startup.
type Checker interface {
	IsValid(tenantID string) (bool, error)
	Tier(tenantID string) string
	MaxRules(tier string) int
}

// OpenCoreChecker always returns valid + community tier.
// This is the default checker shipped in the AGPL-3.0 open-source build.
type OpenCoreChecker struct{}

func (c *OpenCoreChecker) IsValid(tenantID string) (bool, error) { return true, nil }
func (c *OpenCoreChecker) Tier(tenantID string) string           { return "community" }
func (c *OpenCoreChecker) MaxRules(tier string) int              { return 5 }

// Default is the global checker instance used by the application.
// Override in main.go for enterprise builds.
var Default Checker = &OpenCoreChecker{}
