package cloud_test

import (
	"testing"

	"github.com/outscale/cloud-provider-osc/ccm/cloud"
	"github.com/outscale/goutils/sdk/metadata/mocks_metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteInit(t *testing.T) {
	// Setup mock metadata server, with no configured call, should fail on all calls.
	mocks_metadata.Setup()
	defer mocks_metadata.Teardown()
	c, err := cloud.New(t.Context(), "foo", true)
	require.NoError(t, err)
	assert.True(t, c.HasClusterID())
}
