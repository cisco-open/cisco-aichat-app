// Copyright 2025 Cisco Systems, Inc. and its affiliates
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/stretchr/testify/assert"
)

// clearAllDatabaseEnvVars clears all database-related environment variables
// Use t.Setenv to set new values after calling this
func clearAllDatabaseEnvVars(t *testing.T) {
	t.Helper()
	// Set to empty string - t.Setenv will restore original value after test
	t.Setenv(AIChatDatabaseURL, "")
	t.Setenv(GFDatabaseURL, "")
	t.Setenv(GFDatabaseType, "")
	t.Setenv(GFDatabaseHost, "")
	t.Setenv(GFDatabaseName, "")
	t.Setenv(GFDatabaseUser, "")
	t.Setenv(GFDatabasePassword, "")
	t.Setenv(GFDatabaseSSLMode, "")
	t.Setenv(GFDatabasePath, "")
	t.Setenv(AIChatEnableRuntimeMemoryFallback, "")
	t.Setenv(GFPathsConfig, filepath.Join(t.TempDir(), "missing-grafana.ini"))
	t.Setenv(GFPathsData, "")
}

// TestResolve_ExplicitURL verifies that AICHAT_DATABASE_URL takes highest priority
func TestResolve_ExplicitURL(t *testing.T) {
	clearAllDatabaseEnvVars(t)
	t.Setenv(AIChatDatabaseURL, "postgres://localhost/test")
	// Also set Grafana variables to ensure they're ignored
	t.Setenv(GFDatabaseURL, "postgres://grafana:5432/grafana")

	resolver := NewConfigResolver(log.DefaultLogger)
	config := resolver.Resolve("/tmp/data")

	assert.Equal(t, "explicit", config.Source)
	assert.Equal(t, "postgres://localhost/test", config.DBURL)
	assert.Equal(t, "/tmp/data", config.DataDir)
}

// TestResolve_GrafanaURL verifies GF_DATABASE_URL is used when AICHAT_DATABASE_URL is not set
func TestResolve_GrafanaURL(t *testing.T) {
	clearAllDatabaseEnvVars(t)
	t.Setenv(GFDatabaseURL, "postgres://grafana:5432/grafana")

	resolver := NewConfigResolver(log.DefaultLogger)
	config := resolver.Resolve("/tmp/data")

	assert.Equal(t, "grafana", config.Source)
	assert.Equal(t, "postgres://grafana:5432/grafana", config.DBURL)
}

// TestResolve_GrafanaComponents_Postgres verifies PostgreSQL URL construction from components
func TestResolve_GrafanaComponents_Postgres(t *testing.T) {
	clearAllDatabaseEnvVars(t)
	t.Setenv(GFDatabaseType, "postgres")
	t.Setenv(GFDatabaseHost, "localhost:5432")
	t.Setenv(GFDatabaseName, "grafana")
	t.Setenv(GFDatabaseUser, "admin")
	t.Setenv(GFDatabasePassword, "secret")
	t.Setenv(GFDatabaseSSLMode, "disable")

	resolver := NewConfigResolver(log.DefaultLogger)
	config := resolver.Resolve("/tmp/data")

	assert.Equal(t, "grafana", config.Source)
	assert.Contains(t, config.DBURL, "postgres://")
	assert.Contains(t, config.DBURL, "admin:secret@localhost:5432")
	assert.Contains(t, config.DBURL, "/grafana")
	assert.Contains(t, config.DBURL, "sslmode=disable")
}

// TestResolve_GrafanaComponents_SQLite verifies SQLite URL construction
func TestResolve_GrafanaComponents_SQLite(t *testing.T) {
	clearAllDatabaseEnvVars(t)
	t.Setenv(GFDatabaseType, "sqlite3")
	t.Setenv(GFDatabasePath, "/var/lib/grafana/grafana.db")

	resolver := NewConfigResolver(log.DefaultLogger)
	config := resolver.Resolve("/tmp/data")

	assert.Equal(t, "grafana", config.Source)
	assert.Equal(t, "file:/var/lib/grafana/grafana.db", config.DBURL)
}

// TestResolve_FileStorage verifies file storage is used when no database config and dataDir is writable
func TestResolve_FileStorage(t *testing.T) {
	clearAllDatabaseEnvVars(t)
	// Use t.TempDir() which is guaranteed to be writable
	tmpDir := t.TempDir()

	resolver := NewConfigResolver(log.DefaultLogger)
	config := resolver.Resolve(tmpDir)

	assert.Equal(t, "file", config.Source)
	assert.Equal(t, tmpDir, config.DataDir)
	assert.True(t, config.Writable)
	assert.Empty(t, config.DBURL)
}

// TestResolve_MemoryFallback verifies memory storage is used when filesystem is not writable
func TestResolve_MemoryFallback(t *testing.T) {
	clearAllDatabaseEnvVars(t)
	// Use a path that definitely doesn't exist and can't be created
	nonWritablePath := "/nonexistent/readonly/path"

	resolver := NewConfigResolver(log.DefaultLogger)
	config := resolver.Resolve(nonWritablePath)

	assert.Equal(t, "memory", config.Source)
	assert.False(t, config.Writable)
	assert.Empty(t, config.DBURL)
}

// TestIsWritable_TempDir verifies writable directories are detected correctly
func TestIsWritable_TempDir(t *testing.T) {
	tmpDir := t.TempDir()
	assert.True(t, isWritable(tmpDir), "temp directory should be writable")
}

// TestIsWritable_NonexistentDir verifies non-writable paths are detected correctly
func TestIsWritable_NonexistentDir(t *testing.T) {
	assert.False(t, isWritable("/nonexistent/readonly"), "nonexistent path should not be writable")
}

// TestBuildURL_PostgresWithSpecialChars verifies special characters in credentials are properly encoded
func TestBuildURL_PostgresWithSpecialChars(t *testing.T) {
	clearAllDatabaseEnvVars(t)
	// Password contains special characters that need URL encoding: @ : / ?
	t.Setenv(GFDatabaseType, "postgres")
	t.Setenv(GFDatabaseHost, "db.example.com:5432")
	t.Setenv(GFDatabaseName, "grafana")
	t.Setenv(GFDatabaseUser, "admin@domain")
	t.Setenv(GFDatabasePassword, "p@ss:w/rd?123")

	resolver := NewConfigResolver(log.DefaultLogger)
	config := resolver.Resolve("/tmp/data")

	assert.Equal(t, "grafana", config.Source)
	// Verify the URL is parseable and contains encoded credentials
	assert.Contains(t, config.DBURL, "postgres://")
	assert.Contains(t, config.DBURL, "db.example.com:5432")
	// The @ in username should be encoded as %40
	assert.Contains(t, config.DBURL, "%40")
	// The special chars in password should be encoded
	assert.Contains(t, config.DBURL, "/grafana")
	// Should NOT contain raw special chars that break URL parsing
	assert.NotContains(t, config.DBURL, "@domain:p@ss")
}

// TestBuildURL_PostgresEmptyPassword verifies URL construction works without password
func TestBuildURL_PostgresEmptyPassword(t *testing.T) {
	clearAllDatabaseEnvVars(t)
	t.Setenv(GFDatabaseType, "postgres")
	t.Setenv(GFDatabaseHost, "localhost:5432")
	t.Setenv(GFDatabaseName, "grafana")
	t.Setenv(GFDatabaseUser, "admin")
	// No password set

	resolver := NewConfigResolver(log.DefaultLogger)
	config := resolver.Resolve("/tmp/data")

	assert.Equal(t, "grafana", config.Source)
	assert.Contains(t, config.DBURL, "postgres://admin@localhost:5432/grafana")
	// Should not have password separator
	assert.NotContains(t, config.DBURL, "admin:")
}

// TestBuildURL_PostgresDefaultDatabaseName verifies default database name is used when not specified
func TestBuildURL_PostgresDefaultDatabaseName(t *testing.T) {
	clearAllDatabaseEnvVars(t)
	t.Setenv(GFDatabaseType, "postgres")
	t.Setenv(GFDatabaseHost, "localhost:5432")
	t.Setenv(GFDatabaseUser, "admin")
	// No database name set - should default to "grafana"

	resolver := NewConfigResolver(log.DefaultLogger)
	config := resolver.Resolve("/tmp/data")

	assert.Equal(t, "grafana", config.Source)
	assert.Contains(t, config.DBURL, "/grafana")
}

// TestBuildURL_PostgresMissingHost verifies URL is empty when host is missing
func TestBuildURL_PostgresMissingHost(t *testing.T) {
	clearAllDatabaseEnvVars(t)
	t.Setenv(GFDatabaseType, "postgres")
	// No host set
	t.Setenv(GFDatabaseUser, "admin")

	resolver := NewConfigResolver(log.DefaultLogger)
	config := resolver.Resolve(t.TempDir())

	// Without host, should fall through to file storage
	assert.Equal(t, "file", config.Source)
	assert.Empty(t, config.DBURL)
}

// TestBuildURL_MySQL verifies MySQL DSN format construction
func TestBuildURL_MySQL(t *testing.T) {
	clearAllDatabaseEnvVars(t)
	t.Setenv(GFDatabaseType, "mysql")
	t.Setenv(GFDatabaseHost, "localhost:3306")
	t.Setenv(GFDatabaseName, "grafana")
	t.Setenv(GFDatabaseUser, "admin")
	t.Setenv(GFDatabasePassword, "secret")

	resolver := NewConfigResolver(log.DefaultLogger)
	config := resolver.Resolve("/tmp/data")

	assert.Equal(t, "grafana", config.Source)
	// MySQL DSN format: user:password@tcp(host:port)/dbname
	assert.Equal(t, "admin:secret@tcp(localhost:3306)/grafana", config.DBURL)
}

// TestBuildURL_SQLiteMissingPath verifies SQLite fails gracefully without path
func TestBuildURL_SQLiteMissingPath(t *testing.T) {
	clearAllDatabaseEnvVars(t)
	t.Setenv(GFDatabaseType, "sqlite3")
	// No path set

	resolver := NewConfigResolver(log.DefaultLogger)
	config := resolver.Resolve(t.TempDir())

	// Without path, should fall through to file storage
	assert.Equal(t, "file", config.Source)
	assert.Empty(t, config.DBURL)
}

// TestBuildURL_UnsupportedType verifies unsupported database types are handled
func TestBuildURL_UnsupportedType(t *testing.T) {
	clearAllDatabaseEnvVars(t)
	t.Setenv(GFDatabaseType, "mongodb") // Not supported

	resolver := NewConfigResolver(log.DefaultLogger)
	config := resolver.Resolve(t.TempDir())

	// Unsupported type should fall through to file storage
	assert.Equal(t, "file", config.Source)
	assert.Empty(t, config.DBURL)
}

// TestResolve_PriorityOrder verifies the complete priority chain
func TestResolve_PriorityOrder(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T)
		expectedSrc string
		checkDBURL  func(t *testing.T, dbURL string)
	}{
		{
			name: "explicit overrides all",
			setup: func(t *testing.T) {
				t.Setenv(AIChatDatabaseURL, "postgres://explicit/db")
				t.Setenv(GFDatabaseURL, "postgres://grafana/db")
				t.Setenv(GFDatabaseType, "postgres")
				t.Setenv(GFDatabaseHost, "components")
			},
			expectedSrc: "explicit",
			checkDBURL: func(t *testing.T, dbURL string) {
				assert.Equal(t, "postgres://explicit/db", dbURL)
			},
		},
		{
			name: "GF_DATABASE_URL overrides components",
			setup: func(t *testing.T) {
				t.Setenv(GFDatabaseURL, "postgres://grafana-url/db")
				t.Setenv(GFDatabaseType, "postgres")
				t.Setenv(GFDatabaseHost, "components")
			},
			expectedSrc: "grafana",
			checkDBURL: func(t *testing.T, dbURL string) {
				assert.Equal(t, "postgres://grafana-url/db", dbURL)
			},
		},
		{
			name: "components used when URL not set",
			setup: func(t *testing.T) {
				t.Setenv(GFDatabaseType, "postgres")
				t.Setenv(GFDatabaseHost, "components-host:5432")
				t.Setenv(GFDatabaseName, "mydb")
			},
			expectedSrc: "grafana",
			checkDBURL: func(t *testing.T, dbURL string) {
				assert.True(t, strings.HasPrefix(dbURL, "postgres://"))
				assert.Contains(t, dbURL, "components-host:5432")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearAllDatabaseEnvVars(t)
			tt.setup(t)

			resolver := NewConfigResolver(log.DefaultLogger)
			config := resolver.Resolve("/tmp/data")

			assert.Equal(t, tt.expectedSrc, config.Source)
			tt.checkDBURL(t, config.DBURL)
		})
	}
}

func TestResolve_GrafanaConfigFile_Postgres(t *testing.T) {
	clearAllDatabaseEnvVars(t)

	configPath := filepath.Join(t.TempDir(), "grafana.ini")
	configContents := `[database]
type = postgres
host = db.internal:5432
name = grafana
user = grafana_user
password = grafana_pass
ssl_mode = disable
`
	err := os.WriteFile(configPath, []byte(configContents), 0600)
	assert.NoError(t, err)
	t.Setenv(GFPathsConfig, configPath)

	resolver := NewConfigResolver(log.DefaultLogger)
	config := resolver.Resolve(t.TempDir())

	assert.Equal(t, "grafana", config.Source)
	assert.Contains(t, config.DBURL, "postgres://")
	assert.Contains(t, config.DBURL, "db.internal:5432")
	assert.Contains(t, config.DBURL, "sslmode=disable")
}

func TestResolve_GrafanaConfigFile_SQLiteRelativePath(t *testing.T) {
	clearAllDatabaseEnvVars(t)

	configPath := filepath.Join(t.TempDir(), "grafana.ini")
	configContents := `[database]
type = sqlite3
path = grafana.db

[paths]
data = /var/lib/grafana
`
	err := os.WriteFile(configPath, []byte(configContents), 0600)
	assert.NoError(t, err)
	t.Setenv(GFPathsConfig, configPath)

	resolver := NewConfigResolver(log.DefaultLogger)
	config := resolver.Resolve(t.TempDir())

	assert.Equal(t, "grafana", config.Source)
	assert.Equal(t, "file:/var/lib/grafana/grafana.db", config.DBURL)
}

func TestResolve_RuntimeFallbackFlag_DefaultDisabled(t *testing.T) {
	clearAllDatabaseEnvVars(t)
	t.Setenv(AIChatDatabaseURL, "postgres://localhost/test")

	resolver := NewConfigResolver(log.DefaultLogger)
	config := resolver.Resolve("/tmp/data")

	assert.False(t, config.EnableRuntimeFallback)
}

func TestResolve_RuntimeFallbackFlag_Enabled(t *testing.T) {
	clearAllDatabaseEnvVars(t)
	t.Setenv(AIChatDatabaseURL, "postgres://localhost/test")
	t.Setenv(AIChatEnableRuntimeMemoryFallback, "true")

	resolver := NewConfigResolver(log.DefaultLogger)
	config := resolver.Resolve("/tmp/data")

	assert.True(t, config.EnableRuntimeFallback)
}
