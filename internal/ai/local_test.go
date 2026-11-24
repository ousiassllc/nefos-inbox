package ai

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// local の http.Client は Timeout が 10 分（2 分にしない）で、リダイレクトは追従しない。ほかのプロバイダは 2 分のまま。
func TestHTTPClientForLocalHasLongTimeout(t *testing.T) {
	local := httpClientFor(ProviderLocal)
	assert.Equal(t, 10*time.Minute, local.Timeout)
	assert.Equal(t, http.ErrUseLastResponse, local.CheckRedirect(nil, nil))
	for _, p := range []string{ProviderOpenAI, ProviderAnthropic, ProviderChatGPT} {
		assert.Equal(t, 2*time.Minute, httpClientFor(p).Timeout, p)
	}
}
