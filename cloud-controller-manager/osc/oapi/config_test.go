package oapi_test

import (
	"os"
	"path"
	"testing"

	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc/oapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const config = `{
    "default": {
        "access_key": "ACCESSKEY",
        "secret_key": "SECRETKEY",
        "region": "eu-west-2"
    }
}`

func TestConfigEnvFromConfigFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	os.Unsetenv("OSC_ACCESS_KEY") //nolint
	os.Unsetenv("OSC_SECRET_KEY") //nolint
	err := os.Mkdir(path.Join(dir, ".osc"), 0700)
	require.NoError(t, err)
	err = os.WriteFile(path.Join(dir, ".osc/config.json"), []byte(config), 0600)
	require.NoError(t, err)
	ecfg, err := oapi.LoadConfig()
	require.NoError(t, err)
	require.NotNil(t, ecfg.AccessKey)
	assert.Equal(t, "ACCESSKEY", *ecfg.AccessKey)
	assert.Equal(t, "SECRETKEY", *ecfg.SecretKey)
}
