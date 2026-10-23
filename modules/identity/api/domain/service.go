package domain

import "context"

type Service interface {
	Foo(ctx context.Context) (string, error)
}
