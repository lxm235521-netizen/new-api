package service

import (
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeFetcher 模拟 adaptor.FetchTask，可按需延迟、返回错误或 panic。
type fakeFetcher struct {
	delay time.Duration
	resp  *http.Response
	err   error
	panic bool
}

func (f *fakeFetcher) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.panic {
		panic("boom")
	}
	return f.resp, f.err
}

// trackCloser 记录 body 是否被关闭（模拟迟到的响应必须被回收）。
type trackCloser struct {
	once   sync.Once
	closed chan struct{}
}

func newTrackCloser() *trackCloser { return &trackCloser{closed: make(chan struct{})} }

func (c *trackCloser) Read([]byte) (int, error) { return 0, io.EOF }

func (c *trackCloser) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func TestFetchTaskWithTimeout_ReturnsResponse(t *testing.T) {
	body := newTrackCloser()
	fetcher := &fakeFetcher{resp: &http.Response{Body: body}}

	resp, err := FetchTaskWithTimeout(fetcher, "http://upstream", "key", nil, "", 200*time.Millisecond)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, body, resp.Body)
}

func TestFetchTaskWithTimeout_TimesOutAndClosesLateBody(t *testing.T) {
	body := newTrackCloser()
	fetcher := &fakeFetcher{delay: 2 * time.Second, resp: &http.Response{Body: body}}

	started := time.Now()
	resp, err := FetchTaskWithTimeout(fetcher, "http://upstream", "key", nil, "", 50*time.Millisecond)
	elapsed := time.Since(started)

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, errTaskFetchTimeout), "超时错误必须能被熔断逻辑识别: %v", err)
	assert.Less(t, elapsed, time.Second, "必须在超时后立刻返回，不能等待上游")

	// 迟到的响应最终要关闭 body，否则会泄漏连接（甚至重新占满连接池）
	select {
	case <-body.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("迟到的响应 body 没有被关闭")
	}
}

func TestFetchTaskWithTimeout_PanicBecomesError(t *testing.T) {
	fetcher := &fakeFetcher{panic: true}

	resp, err := FetchTaskWithTimeout(fetcher, "http://upstream", "key", nil, "", 200*time.Millisecond)

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "panic")
}

func TestFetchTaskWithTimeout_PassesThroughUpstreamError(t *testing.T) {
	sentinel := errors.New("connection reset by peer")
	fetcher := &fakeFetcher{err: sentinel}

	_, err := FetchTaskWithTimeout(fetcher, "http://upstream", "key", nil, "", 200*time.Millisecond)

	assert.ErrorIs(t, err, sentinel)
}

func TestFetchTaskWithTimeout_ZeroTimeoutUsesDefault(t *testing.T) {
	assert.Greater(t, defaultTaskFetchTimeout, time.Duration(0))

	body := newTrackCloser()
	fetcher := &fakeFetcher{resp: &http.Response{Body: body}}
	resp, err := FetchTaskWithTimeout(fetcher, "http://upstream", "key", nil, "", 0)

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestTaskPollingDefaults_AreSane(t *testing.T) {
	assert.Greater(t, defaultTaskChannelFailureLimit, 0)
	assert.Greater(t, taskPollingSlowRoundWarn, time.Duration(0))
	assert.Greater(t, taskChannelCooldown, time.Duration(0))
}

func TestTaskChannelCooldown(t *testing.T) {
	const channelID = 99991
	now := time.Now()

	setTaskChannelCooldown(channelID, now.Add(80*time.Millisecond))
	until, inCooldown := taskChannelInCooldown(channelID, now)
	require.True(t, inCooldown)
	assert.True(t, until.After(now))

	// 冷却结束后自动解除
	_, inCooldown = taskChannelInCooldown(channelID, now.Add(200*time.Millisecond))
	assert.False(t, inCooldown)

	// 不同渠道互不影响
	_, other := taskChannelInCooldown(channelID+1, now)
	assert.False(t, other)
}
