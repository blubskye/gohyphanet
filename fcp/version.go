// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//
// Source Code: https://github.com/blubskye/gohyphanet

package fcp

import (
	"fmt"
	"runtime"
)

const (
	// Version is the current version of GoHyphanet
	Version = "0.1.0"

	// SourceURL is the URL to the source code repository (AGPL requirement)
	SourceURL = "https://github.com/blubskye/gohyphanet"

	// LicenseName is the name of the license
	LicenseName = "GNU AGPLv3"

	// LicenseURL is the URL to the license text
	LicenseURL = "https://www.gnu.org/licenses/agpl-3.0.txt"
)

// VersionInfo contains version and build information
type VersionInfo struct {
	Version   string
	GoVersion string
	OS        string
	Arch      string
	SourceURL string
	License   string
}

// GetVersionInfo returns complete version information
func GetVersionInfo() VersionInfo {
	return VersionInfo{
		Version:   Version,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		SourceURL: SourceURL,
		License:   LicenseName,
	}
}

// GetVersionString returns a formatted version string
func GetVersionString() string {
	return fmt.Sprintf("GoHyphanet %s", Version)
}

// GetFullVersionString returns a detailed version string with all info
func GetFullVersionString() string {
	info := GetVersionInfo()
	return fmt.Sprintf(`GoHyphanet %s
Go Version: %s
OS/Arch: %s/%s
License: %s
Source: %s`,
		info.Version,
		info.GoVersion,
		info.OS,
		info.Arch,
		info.License,
		info.SourceURL,
	)
}

// PrintLicenseNotice prints the AGPL notice
func PrintLicenseNotice() string {
	return fmt.Sprintf(`GoHyphanet - Freenet/Hyphanet FCP Library and Tools
Copyright (C) 2025 GoHyphanet Contributors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program comes with ABSOLUTELY NO WARRANTY.
This is free software, and you are welcome to redistribute it
under certain conditions; see LICENSE file for details.

Source Code: %s
License: %s (%s)`,
		SourceURL,
		LicenseName,
		LicenseURL,
	)
}
