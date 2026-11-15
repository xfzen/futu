package client

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenDHandshakeReturnsReadFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(time.Second))
		var header [44]byte
		_, _ = io.ReadFull(conn, header[:])
	}()

	result := make(chan error, 1)
	go func() {
		connection, err := New(WithAddr(listener.Addr().String()))
		if connection != nil {
			_ = connection.Close()
		}
		result <- err
	}()
	select {
	case err := <-result:
		require.ErrorContains(t, err, "OpenD handshake")
	case <-time.After(2 * time.Second):
		t.Fatal("OpenD closed the connection but New is still waiting for the handshake")
	}
}
