package daemon

import (
	"context"
	"io"
)

var HandlerMap = make(map[string]interface{})

type ContentType string

type CommonAction struct {
	Action string `json:"action"`
}

type JsonHandler interface {
	HandleJson(ctx context.Context) (interface{}, error)
}

type StreamHandler interface {
	HandleStream(ctx context.Context, resp io.Writer) error
}
