/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package oapi_test

import (
	"testing"

	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc/oapi"
	"github.com/stretchr/testify/require"
)

func TestCheckCredentials(t *testing.T) {
	t.Run("Invalid credentials are rejected with an error", func(t *testing.T) {
		t.Setenv("OSC_ACCESS_KEY", "foo")
		t.Setenv("OSC_SECRET_KEY", "bar")
		api, err := oapi.NewClient("eu-west-2")
		require.NoError(t, err)
		err = api.OAPI().CheckCredentials(t.Context())
		require.ErrorIs(t, err, oapi.ErrInvalidCredentials)
	})
}
