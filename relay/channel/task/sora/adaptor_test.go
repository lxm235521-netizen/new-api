package sora

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEstimateBillingUsesResolvedInputDuration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Duration: 16,
		// A client-supplied shorter value must be ignored because the resolver
		// clears Seconds before EstimateBilling runs.
		Seconds: "",
	})

	ratios := (&TaskAdaptor{}).EstimateBilling(c, &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	})
	require.Equal(t, 16.0, ratios["seconds"])
}
