package alerting

import "context"

// RequestFunc is the narrow Alertmanager transport boundary used by HTTP
// handlers. Keeping protocol transport behind this package makes alerting
// workflows replaceable in tests without changing routes.
type RequestFunc func(context.Context, string, string, interface{}, interface{}) error

type Client struct{ request RequestFunc }

func NewClient(request RequestFunc) *Client { return &Client{request: request} }
func (c *Client) Request(ctx context.Context, method, path string, input, output interface{}) error {
	if c == nil || c.request == nil {
		return context.Canceled
	}
	return c.request(ctx, method, path, input, output)
}
