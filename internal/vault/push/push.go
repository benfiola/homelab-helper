package push

import (
	"context"
)

type Opts struct {
	Address string
}

type Pusher struct {
	Address string
}

func New(opts *Opts) (*Pusher, error) {
	pusher := Pusher{
		Address: opts.Address,
	}
	return &pusher, nil
}

func (p *Pusher) Push(ctx context.Context) error {
	return nil
}
