package client

import (
	"testing"

	"github.com/hyperjiang/futu/infra"
	"github.com/stretchr/testify/require"
)

func TestClientCloseIsIdempotent(t *testing.T) {
	client := &Client{
		closed: make(chan struct{}),
		hub:    infra.NewDispatcherHub(),
	}

	require.NoError(t, client.Close())
	require.NotPanics(t, func() {
		require.NoError(t, client.Close())
	})
}
