//go:build !(linux && cgo)

package main

import (
	"context"
	"log/slog"
)

func runGio(context.Context, *slog.Logger, gioOptions) error {
	return ErrNeedsCgo
}
