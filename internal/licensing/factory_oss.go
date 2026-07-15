// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

//go:build !enterprise

package licensing

func NewChecker(keyProvider func() string) Checker { return &OpenCoreChecker{} }
