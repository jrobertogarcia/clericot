package resilience_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"clericot/internal/platform/resilience"
)

func TestHTTPCircuitBreakerPredicate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		err        error
		expected   bool
	}{
		{
			name:       "transport error trips breaker",
			statusCode: 0,
			err:        errors.New("connection reset by peer"),
			expected:   true,
		},
		{
			name:       "500 Internal Server Error trips breaker",
			statusCode: http.StatusInternalServerError,
			err:        nil,
			expected:   true,
		},
		{
			name:       "502 Bad Gateway trips breaker",
			statusCode: http.StatusBadGateway,
			err:        nil,
			expected:   true,
		},
		{
			name:       "503 Service Unavailable trips breaker",
			statusCode: http.StatusServiceUnavailable,
			err:        nil,
			expected:   true,
		},
		{
			name:       "504 Gateway Timeout trips breaker",
			statusCode: http.StatusGatewayTimeout,
			err:        nil,
			expected:   true,
		},
		{
			name:       "429 Too Many Requests EXPLICITLY IGNORED (does not trip breaker)",
			statusCode: http.StatusTooManyRequests,
			err:        nil,
			expected:   false,
		},
		{
			name:       "400 Bad Request does not trip breaker",
			statusCode: http.StatusBadRequest,
			err:        nil,
			expected:   false,
		},
		{
			name:       "401 Unauthorized does not trip breaker",
			statusCode: http.StatusUnauthorized,
			err:        nil,
			expected:   false,
		},
		{
			name:       "403 Forbidden does not trip breaker",
			statusCode: http.StatusForbidden,
			err:        nil,
			expected:   false,
		},
		{
			name:       "404 Not Found does not trip breaker",
			statusCode: http.StatusNotFound,
			err:        nil,
			expected:   false,
		},
		{
			name:       "200 OK does not trip breaker",
			statusCode: http.StatusOK,
			err:        nil,
			expected:   false,
		},
		{
			name:       "201 Created does not trip breaker",
			statusCode: http.StatusCreated,
			err:        nil,
			expected:   false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var resp *http.Response
			if tt.statusCode > 0 {
				resp = &http.Response{StatusCode: tt.statusCode}
			}
			result := resilience.HTTPCircuitBreakerPredicate(resp, tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCircuitBreaker_429ResponsesDoNotTrip(t *testing.T) {
	t.Parallel()

	var reqCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqCount, 1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error": "rate limit exceeded"}`))
	}))
	defer server.Close()

	cb := resilience.NewHTTPCircuitBreaker(
		resilience.WithFailureThreshold(3),
		resilience.WithDelay(1*time.Second),
	)

	client := resilience.NewHTTPClient(cb)

	// Send 10 consecutive HTTP 429 requests
	for i := 0; i < 10; i++ {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
		_ = resp.Body.Close()
	}

	// Verify circuit breaker is still CLOSED
	assert.True(t, cb.IsClosed(), "circuit breaker should remain closed after receiving 429 responses")
	assert.False(t, cb.IsOpen(), "circuit breaker must not open on 429 responses")
	assert.Equal(t, int32(10), atomic.LoadInt32(&reqCount))
}

func TestCircuitBreaker_500ResponsesTripBreaker(t *testing.T) {
	t.Parallel()

	var reqCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer server.Close()

	var opened atomic.Bool
	cb := resilience.NewHTTPCircuitBreaker(
		resilience.WithFailureThreshold(3),
		resilience.WithDelay(2*time.Second),
		resilience.WithOnOpen(func(event circuitbreaker.StateChangedEvent) {
			opened.Store(true)
		}),
	)

	client := resilience.NewHTTPClient(cb)

	// Requests 1, 2, 3 return 500 and should trip the circuit breaker on the 3rd failure
	for i := 0; i < 3; i++ {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		_ = resp.Body.Close()
	}

	assert.True(t, cb.IsOpen(), "circuit breaker should be open after reaching failure threshold")
	assert.True(t, opened.Load(), "onOpen callback should have been triggered")

	// 4th request must immediately fail with circuitbreaker.ErrOpen without hitting the server
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	_, err = client.Do(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, circuitbreaker.ErrOpen) || errors.Is(err, resilience.ErrCircuitOpen))
	assert.Equal(t, int32(3), atomic.LoadInt32(&reqCount), "server should not have received 4th request")
}

func TestRetryPolicy_TransientErrors(t *testing.T) {
	t.Parallel()

	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`transient outage`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "recovered"}`))
	}))
	defer server.Close()

	var retriesLogged int32
	retryPol := resilience.NewHTTPRetryPolicy(
		resilience.WithMaxRetries(3),
		resilience.WithBackoff(10*time.Millisecond, 50*time.Millisecond, 2.0),
		resilience.WithOnRetry(func(event failsafe.ExecutionEvent[*http.Response]) {
			atomic.AddInt32(&retriesLogged, 1)
		}),
	)

	client := resilience.NewHTTPClient(retryPol)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.JSONEq(t, `{"status": "recovered"}`, string(body))
	assert.Equal(t, int32(3), atomic.LoadInt32(&attempts))
	assert.Equal(t, int32(2), atomic.LoadInt32(&retriesLogged))
}

func TestExecute_GenericHelper(t *testing.T) {
	t.Parallel()

	t.Run("successful execution", func(t *testing.T) {
		t.Parallel()
		res, err := resilience.Execute(context.Background(), func(ctx context.Context) (string, error) {
			return "success", nil
		})
		require.NoError(t, err)
		assert.Equal(t, "success", res)
	})

	t.Run("nil function returns error", func(t *testing.T) {
		t.Parallel()
		_, err := resilience.Execute[string](context.Background(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot execute nil function")
	})

	t.Run("generic retry on error", func(t *testing.T) {
		t.Parallel()
		var count int32
		retry := resilience.NewRetryPolicy[string](
			resilience.WithGenericMaxRetries[string](2),
			resilience.WithGenericBackoff[string](5*time.Millisecond, 20*time.Millisecond, 2.0),
		)

		res, err := resilience.Execute(context.Background(), func(ctx context.Context) (string, error) {
			c := atomic.AddInt32(&count, 1)
			if c < 3 {
				return "", errors.New("temporary error")
			}
			return "final value", nil
		}, retry)

		require.NoError(t, err)
		assert.Equal(t, "final value", res)
		assert.Equal(t, int32(3), count)
	})

	t.Run("generic circuit breaker", func(t *testing.T) {
		t.Parallel()
		cb := resilience.NewCircuitBreaker[int](
			resilience.WithFailureThreshold(2),
			resilience.WithDelay(500*time.Millisecond),
		)

		for i := 0; i < 2; i++ {
			_, _ = resilience.Execute(context.Background(), func(ctx context.Context) (int, error) {
				return 0, errors.New("failure")
			}, cb)
		}

		assert.True(t, cb.IsOpen())

		_, err := resilience.Execute(context.Background(), func(ctx context.Context) (int, error) {
			return 100, nil
		}, cb)

		require.Error(t, err)
		assert.True(t, errors.Is(err, circuitbreaker.ErrOpen))
	})
}
