package resilience

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/failsafehttp"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

// ErrCircuitOpen is an alias to circuitbreaker.ErrOpen for consumer convenience.
var ErrCircuitOpen = circuitbreaker.ErrOpen

// HTTPCircuitBreakerPredicate evaluates whether an HTTP response or transport error
// should be recorded as a failure against the circuit breaker.
//
// In accordance with Clericot Rule 12:
//   - Transport errors, timeouts, and connection aborts count as failures (true).
//   - HTTP 5xx Server Errors count as failures (true).
//   - HTTP 429 Too Many Requests are EXPLICITLY IGNORED (false) so rate limiting at
//     upstream providers does not falsely trip downstream circuit breakers.
//   - HTTP 4xx Client Errors are ignored (false) because they indicate caller errors, not upstream outages.
//   - HTTP 1xx, 2xx, 3xx are successes (false).
func HTTPCircuitBreakerPredicate(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	if resp == nil {
		return false
	}
	// Explicitly ignore 429 Too Many Requests
	if resp.StatusCode == http.StatusTooManyRequests {
		return false
	}
	// Server errors (500-599) trip the breaker
	if resp.StatusCode >= http.StatusInternalServerError {
		return true
	}
	// 4xx and successful responses do not trip the breaker
	return false
}

// HTTPRetryPredicate determines if an HTTP execution attempt should be retried.
// Retries on transport errors and HTTP 5xx server errors.
func HTTPRetryPredicate(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	if resp == nil {
		return false
	}
	return resp.StatusCode >= http.StatusInternalServerError
}

// Default configuration constants.
const (
	DefaultFailureThreshold = 5
	DefaultSuccessThreshold = 1
	DefaultCircuitDelay     = 10 * time.Second

	DefaultMaxRetries      = 3
	DefaultBackoffDelay    = 100 * time.Millisecond
	DefaultMaxBackoffDelay = 2 * time.Second
	DefaultBackoffFactor   = 2.0
	DefaultJitterFactor    = 0.1
)

// CircuitBreakerConfig holds configuration parameters for circuit breaker construction.
type CircuitBreakerConfig struct {
	FailureThreshold uint
	SuccessThreshold uint
	Delay            time.Duration
	OnStateChanged   func(circuitbreaker.StateChangedEvent)
	OnOpen           func(circuitbreaker.StateChangedEvent)
	OnClose          func(circuitbreaker.StateChangedEvent)
	OnHalfOpen       func(circuitbreaker.StateChangedEvent)
}

// CircuitBreakerOption configures CircuitBreakerConfig.
type CircuitBreakerOption func(*CircuitBreakerConfig)

// WithFailureThreshold sets consecutive failure threshold before opening circuit.
func WithFailureThreshold(threshold uint) CircuitBreakerOption {
	return func(c *CircuitBreakerConfig) {
		c.FailureThreshold = threshold
	}
}

// WithSuccessThreshold sets consecutive success threshold in half-open state before closing.
func WithSuccessThreshold(threshold uint) CircuitBreakerOption {
	return func(c *CircuitBreakerConfig) {
		c.SuccessThreshold = threshold
	}
}

// WithDelay sets open state duration before testing half-open recovery.
func WithDelay(delay time.Duration) CircuitBreakerOption {
	return func(c *CircuitBreakerConfig) {
		c.Delay = delay
	}
}

// WithOnStateChanged registers a callback invoked on any circuit state transition.
func WithOnStateChanged(fn func(circuitbreaker.StateChangedEvent)) CircuitBreakerOption {
	return func(c *CircuitBreakerConfig) {
		c.OnStateChanged = fn
	}
}

// WithOnOpen registers a callback invoked when circuit opens.
func WithOnOpen(fn func(circuitbreaker.StateChangedEvent)) CircuitBreakerOption {
	return func(c *CircuitBreakerConfig) {
		c.OnOpen = fn
	}
}

// WithOnClose registers a callback invoked when circuit closes.
func WithOnClose(fn func(circuitbreaker.StateChangedEvent)) CircuitBreakerOption {
	return func(c *CircuitBreakerConfig) {
		c.OnClose = fn
	}
}

// WithOnHalfOpen registers a callback invoked when circuit half-opens.
func WithOnHalfOpen(fn func(circuitbreaker.StateChangedEvent)) CircuitBreakerOption {
	return func(c *CircuitBreakerConfig) {
		c.OnHalfOpen = fn
	}
}

// NewHTTPCircuitBreaker creates a circuit breaker policy specifically tuned for HTTP operations,
// enforcing Rule 12 by ignoring 429s and other 4xx client errors while tripping on 5xx and transport errors.
func NewHTTPCircuitBreaker(opts ...CircuitBreakerOption) circuitbreaker.CircuitBreaker[*http.Response] {
	cfg := CircuitBreakerConfig{
		FailureThreshold: DefaultFailureThreshold,
		SuccessThreshold: DefaultSuccessThreshold,
		Delay:            DefaultCircuitDelay,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	builder := circuitbreaker.NewBuilder[*http.Response]().
		HandleIf(HTTPCircuitBreakerPredicate).
		WithFailureThreshold(cfg.FailureThreshold).
		WithDelay(cfg.Delay)

	if cfg.SuccessThreshold > 0 {
		builder = builder.WithSuccessThreshold(cfg.SuccessThreshold)
	}
	if cfg.OnStateChanged != nil {
		builder = builder.OnStateChanged(cfg.OnStateChanged)
	}
	if cfg.OnOpen != nil {
		builder = builder.OnOpen(cfg.OnOpen)
	}
	if cfg.OnClose != nil {
		builder = builder.OnClose(cfg.OnClose)
	}
	if cfg.OnHalfOpen != nil {
		builder = builder.OnHalfOpen(cfg.OnHalfOpen)
	}

	return builder.Build()
}

// NewCircuitBreaker creates a generic circuit breaker for any result type R.
func NewCircuitBreaker[R any](opts ...CircuitBreakerOption) circuitbreaker.CircuitBreaker[R] {
	cfg := CircuitBreakerConfig{
		FailureThreshold: DefaultFailureThreshold,
		SuccessThreshold: DefaultSuccessThreshold,
		Delay:            DefaultCircuitDelay,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	builder := circuitbreaker.NewBuilder[R]().
		WithFailureThreshold(cfg.FailureThreshold).
		WithDelay(cfg.Delay)

	if cfg.SuccessThreshold > 0 {
		builder = builder.WithSuccessThreshold(cfg.SuccessThreshold)
	}
	if cfg.OnStateChanged != nil {
		builder = builder.OnStateChanged(cfg.OnStateChanged)
	}
	if cfg.OnOpen != nil {
		builder = builder.OnOpen(cfg.OnOpen)
	}
	if cfg.OnClose != nil {
		builder = builder.OnClose(cfg.OnClose)
	}
	if cfg.OnHalfOpen != nil {
		builder = builder.OnHalfOpen(cfg.OnHalfOpen)
	}

	return builder.Build()
}

// HTTPRetryConfig holds parameters for HTTP retry policy creation.
type HTTPRetryConfig struct {
	MaxRetries        int
	BackoffDelay      time.Duration
	MaxBackoffDelay   time.Duration
	BackoffFactor     float64
	JitterFactor      float64
	HandleIf          func(*http.Response, error) bool
	AbortIf           func(*http.Response, error) bool
	OnRetry           func(failsafe.ExecutionEvent[*http.Response])
	OnRetriesExceeded func(failsafe.ExecutionEvent[*http.Response])
}

// HTTPRetryOption defines a functional option for HTTP retry policies.
type HTTPRetryOption func(*HTTPRetryConfig)

// WithMaxRetries sets the maximum number of retry attempts for HTTP requests.
func WithMaxRetries(maxRetries int) HTTPRetryOption {
	return func(c *HTTPRetryConfig) {
		c.MaxRetries = maxRetries
	}
}

// WithBackoff configures exponential backoff delay bounds and multiplication factor for HTTP requests.
func WithBackoff(initialDelay, maxDelay time.Duration, factor float64) HTTPRetryOption {
	return func(c *HTTPRetryConfig) {
		c.BackoffDelay = initialDelay
		c.MaxBackoffDelay = maxDelay
		c.BackoffFactor = factor
	}
}

// WithJitterFactor sets a random jitter factor on retry delays for HTTP requests.
func WithJitterFactor(jitter float64) HTTPRetryOption {
	return func(c *HTTPRetryConfig) {
		c.JitterFactor = jitter
	}
}

// WithHandleIf configures the failure predicate that triggers a retry for HTTP requests.
func WithHandleIf(predicate func(*http.Response, error) bool) HTTPRetryOption {
	return func(c *HTTPRetryConfig) {
		c.HandleIf = predicate
	}
}

// WithAbortIf configures a predicate where HTTP retries must be immediately aborted.
func WithAbortIf(predicate func(*http.Response, error) bool) HTTPRetryOption {
	return func(c *HTTPRetryConfig) {
		c.AbortIf = predicate
	}
}

// WithOnRetry registers a callback invoked before each HTTP retry attempt.
func WithOnRetry(fn func(failsafe.ExecutionEvent[*http.Response])) HTTPRetryOption {
	return func(c *HTTPRetryConfig) {
		c.OnRetry = fn
	}
}

// WithOnRetriesExceeded registers a callback invoked when maximum HTTP retries are exhausted.
func WithOnRetriesExceeded(fn func(failsafe.ExecutionEvent[*http.Response])) HTTPRetryOption {
	return func(c *HTTPRetryConfig) {
		c.OnRetriesExceeded = fn
	}
}

// NewHTTPRetryPolicy creates a retry policy configured for HTTP requests with exponential backoff.
func NewHTTPRetryPolicy(opts ...HTTPRetryOption) retrypolicy.RetryPolicy[*http.Response] {
	cfg := HTTPRetryConfig{
		MaxRetries:      DefaultMaxRetries,
		BackoffDelay:    DefaultBackoffDelay,
		MaxBackoffDelay: DefaultMaxBackoffDelay,
		BackoffFactor:   DefaultBackoffFactor,
		JitterFactor:    DefaultJitterFactor,
		HandleIf:        HTTPRetryPredicate,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	builder := failsafehttp.NewRetryPolicyBuilder().
		WithMaxRetries(cfg.MaxRetries).
		WithBackoffFactor(cfg.BackoffDelay, cfg.MaxBackoffDelay, cfg.BackoffFactor).
		ReturnLastFailure()

	if cfg.JitterFactor > 0 {
		builder = builder.WithJitterFactor(cfg.JitterFactor)
	}
	if cfg.HandleIf != nil {
		builder = builder.HandleIf(cfg.HandleIf)
	}
	if cfg.AbortIf != nil {
		builder = builder.AbortIf(cfg.AbortIf)
	}
	if cfg.OnRetry != nil {
		builder = builder.OnRetry(cfg.OnRetry)
	}
	if cfg.OnRetriesExceeded != nil {
		builder = builder.OnRetriesExceeded(cfg.OnRetriesExceeded)
	}

	return builder.Build()
}

// GenericRetryConfig holds parameters for generic retry policies.
type GenericRetryConfig[R any] struct {
	MaxRetries        int
	BackoffDelay      time.Duration
	MaxBackoffDelay   time.Duration
	BackoffFactor     float64
	JitterFactor      float64
	HandleIf          func(R, error) bool
	AbortIf           func(R, error) bool
	OnRetry           func(failsafe.ExecutionEvent[R])
	OnRetriesExceeded func(failsafe.ExecutionEvent[R])
}

// GenericRetryOption defines a functional option for generic retry policies.
type GenericRetryOption[R any] func(*GenericRetryConfig[R])

// WithGenericMaxRetries sets the maximum number of retry attempts for generic calls.
func WithGenericMaxRetries[R any](maxRetries int) GenericRetryOption[R] {
	return func(c *GenericRetryConfig[R]) {
		c.MaxRetries = maxRetries
	}
}

// WithGenericBackoff configures exponential backoff delay bounds and multiplication factor for generic calls.
func WithGenericBackoff[R any](initialDelay, maxDelay time.Duration, factor float64) GenericRetryOption[R] {
	return func(c *GenericRetryConfig[R]) {
		c.BackoffDelay = initialDelay
		c.MaxBackoffDelay = maxDelay
		c.BackoffFactor = factor
	}
}

// WithGenericJitterFactor sets a random jitter factor on generic retry delays.
func WithGenericJitterFactor[R any](jitter float64) GenericRetryOption[R] {
	return func(c *GenericRetryConfig[R]) {
		c.JitterFactor = jitter
	}
}

// WithGenericHandleIf configures the failure predicate that triggers a generic retry.
func WithGenericHandleIf[R any](predicate func(R, error) bool) GenericRetryOption[R] {
	return func(c *GenericRetryConfig[R]) {
		c.HandleIf = predicate
	}
}

// WithGenericAbortIf configures a predicate where generic retries must be immediately aborted.
func WithGenericAbortIf[R any](predicate func(R, error) bool) GenericRetryOption[R] {
	return func(c *GenericRetryConfig[R]) {
		c.AbortIf = predicate
	}
}

// WithGenericOnRetry registers a callback invoked before each generic retry attempt.
func WithGenericOnRetry[R any](fn func(failsafe.ExecutionEvent[R])) GenericRetryOption[R] {
	return func(c *GenericRetryConfig[R]) {
		c.OnRetry = fn
	}
}

// WithGenericOnRetriesExceeded registers a callback invoked when maximum generic retries are exhausted.
func WithGenericOnRetriesExceeded[R any](fn func(failsafe.ExecutionEvent[R])) GenericRetryOption[R] {
	return func(c *GenericRetryConfig[R]) {
		c.OnRetriesExceeded = fn
	}
}

// NewRetryPolicy creates a generic retry policy for any result type R.
func NewRetryPolicy[R any](opts ...GenericRetryOption[R]) retrypolicy.RetryPolicy[R] {
	cfg := GenericRetryConfig[R]{
		MaxRetries:      DefaultMaxRetries,
		BackoffDelay:    DefaultBackoffDelay,
		MaxBackoffDelay: DefaultMaxBackoffDelay,
		BackoffFactor:   DefaultBackoffFactor,
		JitterFactor:    DefaultJitterFactor,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	builder := retrypolicy.NewBuilder[R]().
		WithMaxRetries(cfg.MaxRetries).
		WithBackoffFactor(cfg.BackoffDelay, cfg.MaxBackoffDelay, cfg.BackoffFactor).
		ReturnLastFailure()

	if cfg.JitterFactor > 0 {
		builder = builder.WithJitterFactor(cfg.JitterFactor)
	}
	if cfg.HandleIf != nil {
		builder = builder.HandleIf(cfg.HandleIf)
	}
	if cfg.AbortIf != nil {
		builder = builder.AbortIf(cfg.AbortIf)
	}
	if cfg.OnRetry != nil {
		builder = builder.OnRetry(cfg.OnRetry)
	}
	if cfg.OnRetriesExceeded != nil {
		builder = builder.OnRetriesExceeded(cfg.OnRetriesExceeded)
	}

	return builder.Build()
}

// NewRoundTripper wraps an http.RoundTripper with Failsafe-go resilience policies.
// If inner is nil, http.DefaultTransport is used as the base transport.
func NewRoundTripper(inner http.RoundTripper, policies ...failsafe.Policy[*http.Response]) http.RoundTripper {
	if inner == nil {
		inner = http.DefaultTransport
	}
	return failsafehttp.NewRoundTripper(inner, policies...)
}

// NewHTTPClient creates an *http.Client configured with Failsafe-go resilience policies.
func NewHTTPClient(policies ...failsafe.Policy[*http.Response]) *http.Client {
	return NewHTTPClientWithTransport(http.DefaultTransport, policies...)
}

// NewHTTPClientWithTransport creates an *http.Client using the provided base transport and resilience policies.
func NewHTTPClientWithTransport(transport http.RoundTripper, policies ...failsafe.Policy[*http.Response]) *http.Client {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &http.Client{
		Transport: NewRoundTripper(transport, policies...),
	}
}

// Execute wraps an arbitrary function call with Failsafe resilience policies and context propagation.
func Execute[R any](ctx context.Context, fn func(context.Context) (R, error), policies ...failsafe.Policy[R]) (R, error) {
	if fn == nil {
		var zero R
		return zero, errors.New("cannot execute nil function")
	}

	executor := failsafe.With[R](policies...)
	if ctx != nil {
		executor = executor.WithContext(ctx)
	}

	return executor.GetWithExecution(func(exec failsafe.Execution[R]) (R, error) {
		return fn(exec.Context())
	})
}
