package main

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ousiassllc/nefos-inbox/internal/capability"
)

func TestNewLocalAI(t *testing.T) {
	dir := func() (string, error) { return t.TempDir(), nil }
	failing := func() (string, error) { return "", errors.New("no cache dir") }
	logger := slog.New(slog.DiscardHandler)

	t.Run("capability が無い OS では作らない", func(t *testing.T) {
		rt, closeFn := newLocalAI(capability.Android(), dir, nil, logger)
		assert.Nil(t, rt)
		closeFn()
	})
	t.Run("置き場所を決められなければ作らない", func(t *testing.T) {
		rt, closeFn := newLocalAI(capability.Desktop("windows"), failing, nil, logger)
		assert.Nil(t, rt)
		closeFn()
	})
	t.Run("capability があれば作り、close できる", func(t *testing.T) {
		rt, closeFn := newLocalAI(capability.Desktop("windows"), dir, nil, logger)
		assert.NotNil(t, rt)
		closeFn()
		closeFn() // 何度呼んでもよい
	})
}
