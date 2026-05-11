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

package plugin

import (
	"context"
	"errors"
	"testing"

	"github.com/grafana/cisco-aichat-app/pkg/storage"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pingErrorStorage struct {
	storage.Storage
	err error
}

func (s pingErrorStorage) Ping(_ context.Context) error {
	return s.err
}

func TestCheckHealth_OK(t *testing.T) {
	app := &App{
		storage: storage.NewMemoryStorage(),
	}

	res, err := app.CheckHealth(context.Background(), &backend.CheckHealthRequest{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, backend.HealthStatusOk, res.Status)
	assert.Contains(t, res.Message, "healthy")
}

func TestCheckHealth_StorageError(t *testing.T) {
	app := &App{
		storage: pingErrorStorage{
			Storage: storage.NewMemoryStorage(),
			err:     errors.New("primary unavailable"),
		},
	}

	res, err := app.CheckHealth(context.Background(), &backend.CheckHealthRequest{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, backend.HealthStatusError, res.Status)
	assert.Contains(t, res.Message, "Storage check failed")
}

func TestCheckHealth_DegradedResilientStorage(t *testing.T) {
	primary := pingErrorStorage{
		Storage: storage.NewMemoryStorage(),
		err:     errors.New("primary unavailable"),
	}
	fallback := storage.NewMemoryStorage()
	resilient := storage.NewResilientStorage(primary, fallback, log.DefaultLogger)

	// Open the circuit by hitting the failure threshold through Ping.
	for i := 0; i < 5; i++ {
		err := resilient.Ping(context.Background())
		require.NoError(t, err)
	}
	require.True(t, resilient.IsDegraded())

	app := &App{
		storage: resilient,
	}

	res, err := app.CheckHealth(context.Background(), &backend.CheckHealthRequest{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, backend.HealthStatusError, res.Status)
	assert.Contains(t, res.Message, "degraded mode")
}
