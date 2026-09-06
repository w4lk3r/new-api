package controller_test

import (
	"context"
	"os"
	"testing"

	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHuizhenVideoPluginContracts(t *testing.T) {
	source, err := os.ReadFile("../docs/task-plugins/huizhen-ai/huizhen-ai.plugin.js")
	require.NoError(t, err)
	fixture, err := os.ReadFile("../docs/task-plugins/huizhen-ai/huizhen-ai.fixture.json")
	require.NoError(t, err)

	report, err := pluginruntime.ReplayFixture(context.Background(), string(source), fixture)
	require.NoError(t, err)
	assert.Equal(t, pluginruntime.FixtureReport{Total: 20, Passed: 20}, report)

	plugin, err := pluginruntime.CompilePlugin(string(source), pluginruntime.Options{})
	require.NoError(t, err)
	require.NoError(t, billing_setting.SmokeTestTaskExpr(`tier("per_second", u("seconds") * 0.10)`, plugin.Meta.UsageSchema))
	require.NoError(t, billing_setting.SmokeTestTaskExpr(`u("resolution") == "720p" ? tier("720p", u("seconds") * 0.20) : tier("480p", u("seconds") * 0.10)`, plugin.Meta.UsageSchema))
	require.NoError(t, billing_setting.SmokeTestTaskExpr(`tier("per_call", 0.50)`, plugin.Meta.UsageSchema))
}
