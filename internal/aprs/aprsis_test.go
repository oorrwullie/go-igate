package aprs

import (
	"bufio"
	"context"
	"errors"
	"github.com/oorrwullie/sparky/internal/config"
	"net"
	"fmt"
	"net/textproto"
	"sync"
	"time"
	"strings"
	"testing"
	"sync"
)

func TestVerifyLoginResponseSkipsServerGreeting(t *testing.T) {
	response := textproto.NewReader(bufio.NewReader(strings.NewReader(
		"# aprsc 2.1.20 test server\r\n# logresp N0CALL-10 verified, server test\r\n",
	)))

	if err := verifyLoginResponse(response, "N0CALL-10"); err != nil {
		t.Fatalf("verifyLoginResponse() rejected valid response after greeting: %v", err)
	}
}

func TestVerifyLoginResponseRejectsUnverifiedCallsign(t *testing.T) {
	response := textproto.NewReader(bufio.NewReader(strings.NewReader(
		"# aprsc 2.1.20 test server\r\n# logresp N0CALL-10 unverified, server test\r\n",
	)))

	if err := verifyLoginResponse(response, "N0CALL-10"); err == nil {
		t.Fatal("verifyLoginResponse() accepted an unverified callsign")
	}
}

func TestStopAbortsPendingAPRSISLogin(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		close(accepted)
		// Hold the login unanswered. Stop must interrupt this read, not wait
		// for the ten-second login deadline.
		_, _ = bufio.NewReader(conn).ReadString('\n')
		_, _ = bufio.NewReader(conn).ReadString('\n')
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &AprsIs{
		Callsign: "N0CALL",
		cfg: config.AprsIs{Server: listener.Addr().String(), Passcode: "12345"},
		stop: make(chan struct{}),
		ctx: ctx,
		cancel: cancel,
	}
	connected := make(chan error, 1)
	go func() { connected <- a.Connect() }()
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("server did not accept pending login")
	}
	// Wait until Connect advertises the socket as pending.
	deadline := time.After(time.Second)
	for {
		a.connMu.Lock()
		pending := a.pending != nil
		a.connMu.Unlock()
		if pending { break }
		select {
		case <-deadline:
			t.Fatal("Connect never registered pending login")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	stopped := make(chan struct{})
	go func() { a.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop blocked on pending APRS-IS login")
	}
	select {
	case err := <-connected:
		if err == nil {
			t.Fatal("Connect unexpectedly succeeded after Stop")
		}
	case <-time.After(time.Second):
		t.Fatal("pending login did not exit after Stop")
	}
	if err := a.Connect(); err == nil {
		t.Fatal("Connect succeeded after Stop")
	}
	if err := a.Upload("N0CALL>APRS:test"); err == nil {
		t.Fatal("Upload succeeded after Stop")
	}
	wg.Wait()
}

func TestStopRejectsReconnectRace(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &AprsIs{stop: make(chan struct{}), ctx: ctx, cancel: cancel}
	a.Stop()
	a.Stop()
	if err := a.Connect(); err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Connect after Stop error = %v", err)
	}
}

func TestConcurrentAPRSISUploadsAreCompleteLines(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	a := &AprsIs{
		Conn: textproto.NewConn(client),
		stop: make(chan struct{}),
	}
	const count = 80
	lines := make(chan string, count)
	readDone := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(server)
		for n := 0; n < count; n++ {
			line, err := reader.ReadString('\n')
			if err != nil {
				readDone <- err
				return
			}
			lines <- strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		}
		readDone <- nil
	}()
	var wg sync.WaitGroup
	for n := 0; n < count; n++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if err := a.Upload(fmt.Sprintf("N0CALL>APRS:frame-%03d", id)); err != nil {
				t.Errorf("Upload(%d): %v", id, err)
			}
		}(n)
	}
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(3*time.Second):
		t.Fatal("concurrent Upload calls stalled")
	}
	select {
	case err := <-readDone:
		if err != nil { t.Fatal(err) }
	case <-time.After(3*time.Second):
		t.Fatal("server did not receive all packet lines")
	}
	received := make(map[string]bool)
	for n := 0; n < count; n++ {
		line := <-lines
		if !strings.HasPrefix(line, "N0CALL>APRS:frame-") || len(line) != len("N0CALL>APRS:frame-000") {
			t.Fatalf("interleaved or malformed APRS-IS line %q", line)
		}
		if received[line] {
			t.Fatalf("duplicate packet %q", line)
		}
		received[line] = true
	}
	a.Stop()
}
