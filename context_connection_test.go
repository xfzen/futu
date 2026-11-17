package futu

import (
	"context"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/hyperjiang/futu/client"
	"github.com/hyperjiang/futu/pb/initconnect"
	"google.golang.org/protobuf/proto"
)

func TestContextConnectionRefusesCanceledDial(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan struct{})
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			close(accepted)
			_ = conn.Close()
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() {
		sdk, err := NewSDKWithContext(ctx, client.WithAddr(listener.Addr().String()))
		if sdk != nil {
			_ = sdk.Close()
			err = errors.New("a cancelled dial produced an SDK")
		}
		done <- err
	}()
	select {
	case <-accepted:
		t.Fatal("a cancelled dial opened a TCP connection")
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("dial = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("a cancelled dial did not return")
	}
}

func TestContextConnectionClosesHalfOpenHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		sdk, err := NewSDKWithContext(ctx, client.WithAddr(listener.Addr().String()))
		if sdk != nil {
			_ = sdk.Close()
		}
		done <- err
	}()
	_ = listener.(*net.TCPListener).SetDeadline(time.Now().Add(time.Second))
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	packet := make([]byte, 44)
	if _, err := io.ReadFull(conn, packet); err != nil {
		t.Fatal(err)
	}
	size := binary.LittleEndian.Uint32(packet[12:16])
	if size == 0 || size > 65536 {
		t.Fatalf("bad handshake size: %d", size)
	}
	if _, err := io.CopyN(io.Discard, conn, int64(size)); err != nil {
		t.Fatal(err)
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("handshake = %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("the half-open handshake survived cancellation")
	}
	if _, err := conn.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("the cancelled handshake socket is still open: %v", err)
	}
}

func TestContextConnectionKeepsSuccessfulSession(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	peer := make(chan net.Conn, 1)
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		peer <- conn
		_ = conn.SetDeadline(time.Now().Add(time.Second))
		done <- replyToInitConnect(conn)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	sdk, err := NewSDKWithContext(ctx, client.WithAddr(listener.Addr().String()))
	if err != nil {
		t.Fatal(err)
	}
	defer sdk.Close()
	conn := <-peer
	defer conn.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	cancel()
	_ = conn.SetReadDeadline(time.Now().Add(40 * time.Millisecond))
	_, err = conn.Read(make([]byte, 1))
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("caller cancelled an established session: %v", err)
	}
}

func TestContextConnectionCancelsDuringHandshakeReply(t *testing.T) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	for i := 0; i < 32; i++ {
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				sdk, err := NewSDKWithContext(ctx, client.WithAddr(listener.Addr().String()))
				if sdk != nil {
					_ = sdk.Close()
				}
				done <- err
			}()
			_ = listener.SetDeadline(time.Now().Add(time.Second))
			conn, err := listener.AcceptTCP()
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(time.Second))
			if err := replyToInitConnect(conn); err != nil {
				t.Fatal(err)
			}
			cancel()
			select {
			case err := <-done:
				if err != nil && !errors.Is(err, context.Canceled) {
					t.Fatalf("handshake racing cancellation: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("handshake racing cancellation did not finish")
			}
			if _, err := conn.Read(make([]byte, 1)); !errors.Is(err, io.EOF) && !errors.Is(err, syscall.ECONNRESET) {
				t.Fatalf("handshake racing cancellation left a connection open: %v", err)
			}
		}()
	}
}

func replyToInitConnect(conn net.Conn) error {
	packet := make([]byte, 44)
	if _, err := io.ReadFull(conn, packet); err != nil {
		return err
	}
	if _, err := io.CopyN(io.Discard, conn, int64(binary.LittleEndian.Uint32(packet[12:16]))); err != nil {
		return err
	}
	body, err := proto.Marshal(&initconnect.Response{RetType: proto.Int32(0), S2C: &initconnect.S2C{
		ServerVer: proto.Int32(1), LoginUserID: proto.Uint64(1), ConnID: proto.Uint64(1),
		ConnAESKey: proto.String("0123456789abcdef"), KeepAliveInterval: proto.Int32(0),
	}})
	if err != nil {
		return err
	}
	binary.LittleEndian.PutUint32(packet[12:16], uint32(len(body)))
	hash := sha1.Sum(body)
	copy(packet[16:36], hash[:])
	_, err = conn.Write(append(packet, body...))
	return err
}
