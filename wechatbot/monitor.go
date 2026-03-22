package wechatbot

import (
	"context"
	"fmt"
	"log"
	"time"
)

const (
	maxConsecutiveFailures = 3
	backoffDelay           = 30 * time.Second
	retryDelay             = 2 * time.Second
	sessionExpiredErrCode  = -14
)

// Start runs a long-poll loop, calling the handler for each inbound message.
// Blocks until ctx is cancelled. Handles retries and backoff automatically.
func (c *Client) Start(ctx context.Context) error {
	h := c.config.Handler
	if h == nil {
		h = &DefaultHandler{}
		c.config.Handler = h
	}
	h.SetClient(c)

	onError := h.OnError
	if onError == nil {
		onError = func(err error) { log.Printf("[weixin-sdk] %v", err) }
	}

	buf := c.config.GetBuf()
	failures := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resp, err := c.GetUpdates(ctx, buf)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			failures++
			onError(fmt.Errorf("getUpdates (%d/%d): %w", failures, maxConsecutiveFailures, err))
			if failures >= maxConsecutiveFailures {
				failures = 0
				sleepCtx(ctx, backoffDelay)
			} else {
				sleepCtx(ctx, retryDelay)
			}
			continue
		}

		// API-level error
		if resp.Ret != 0 || resp.ErrCode != 0 {
			if resp.ErrCode == sessionExpiredErrCode || resp.Ret == sessionExpiredErrCode {
				h.OnSessionExpired()
				onError(fmt.Errorf("session expired (errcode %d), pausing 5 min", sessionExpiredErrCode))
				sleepCtx(ctx, 5*time.Minute)
				continue
			}

			failures++
			onError(fmt.Errorf("getUpdates ret=%d errcode=%d msg=%s (%d/%d)",
				resp.Ret, resp.ErrCode, resp.ErrMsg, failures, maxConsecutiveFailures))
			if failures >= maxConsecutiveFailures {
				failures = 0
				sleepCtx(ctx, backoffDelay)
			} else {
				sleepCtx(ctx, retryDelay)
			}
			continue
		}

		failures = 0

		// Update sync cursor
		if resp.GetUpdatesBuf != "" {
			buf = resp.GetUpdatesBuf
			h.OnBufUpdate(buf)
		}

		// Dispatch messages
		for _, msg := range resp.Msgs {
			// Cache context token for proactive Push
			if msg.ContextToken != "" && msg.FromUserID != "" {
				c.SetContextToken(msg.FromUserID, msg.ContextToken)
			}
			h.OnMessage(ctx, msg)
		}
	}
}

// MonitorOptions configures the long-poll monitor loop.
// Deprecated: Use Config and Handler instead.
type MonitorOptions struct {
	// InitialBuf is the get_updates_buf to resume from (empty for fresh start).
	InitialBuf string

	// OnBufUpdate is called whenever a new sync cursor is received.
	// Persist this value to resume after restart.
	OnBufUpdate func(buf string)

	// OnError is called on non-fatal poll errors. Defaults to log.Printf.
	OnError func(err error)

	// OnSessionExpired is called when server returns errcode -14.
	OnSessionExpired func()
}

// Monitor is deprecated: use Start() instead.
// Deprecated: Use Start() which uses the config's handler and buffer settings.
func (c *Client) Monitor(ctx context.Context, h Handler, opts *MonitorOptions) error {
	if h == nil {
		h = &DefaultHandler{}
	}
	h.SetClient(c)

	// Support legacy MonitorOptions for backwards compatibility
	if opts != nil {
		if dh, ok := h.(*DefaultHandler); ok {
			if opts.OnError != nil {
				dh.FuncOnError = opts.OnError
			}
			if opts.OnBufUpdate != nil {
				dh.FuncOnBufUpdate = opts.OnBufUpdate
			}
			if opts.OnSessionExpired != nil {
				dh.FuncOnSessionExpired = opts.OnSessionExpired
			}
		}
	}

	onError := h.OnError
	if onError == nil {
		onError = func(err error) { log.Printf("[weixin-sdk] %v", err) }
	}

	buf := opts.InitialBuf
	failures := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resp, err := c.GetUpdates(ctx, buf)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			failures++
			onError(fmt.Errorf("getUpdates (%d/%d): %w", failures, maxConsecutiveFailures, err))
			if failures >= maxConsecutiveFailures {
				failures = 0
				sleepCtx(ctx, backoffDelay)
			} else {
				sleepCtx(ctx, retryDelay)
			}
			continue
		}

		// API-level error
		if resp.Ret != 0 || resp.ErrCode != 0 {
			if resp.ErrCode == sessionExpiredErrCode || resp.Ret == sessionExpiredErrCode {
				h.OnSessionExpired()
				onError(fmt.Errorf("session expired (errcode %d), pausing 5 min", sessionExpiredErrCode))
				sleepCtx(ctx, 5*time.Minute)
				continue
			}

			failures++
			onError(fmt.Errorf("getUpdates ret=%d errcode=%d msg=%s (%d/%d)",
				resp.Ret, resp.ErrCode, resp.ErrMsg, failures, maxConsecutiveFailures))
			if failures >= maxConsecutiveFailures {
				failures = 0
				sleepCtx(ctx, backoffDelay)
			} else {
				sleepCtx(ctx, retryDelay)
			}
			continue
		}

		failures = 0

		// Update sync cursor
		if resp.GetUpdatesBuf != "" {
			buf = resp.GetUpdatesBuf
			h.OnBufUpdate(buf)
		}

		// Dispatch messages
		for _, msg := range resp.Msgs {
			// Cache context token for proactive Push
			if msg.ContextToken != "" && msg.FromUserID != "" {
				c.SetContextToken(msg.FromUserID, msg.ContextToken)
			}
			h.OnMessage(ctx, msg)
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
