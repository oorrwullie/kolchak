package minimize

import "context"

type Predicate[T any] func(context.Context, []T) (bool, error)

type Minimizer[T any] interface {
	Minimize(context.Context, []T, Predicate[T]) ([]T, error)
}
