// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws_test

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"
	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/ws"
)

type RoomQuery struct {
	Room string `query:"room,required"`
	User string `query:"user"`
}

var ErrCustomAuthFailed = errors.New("custom authentication failed")

func TestWS_Handle_WithDTO(t *testing.T) {
	app := sein.New()

	ws.MapCloseError(ErrCustomAuthFailed, 4003, "custom auth rejected")

	userChan := make(chan string, 1)

	ws.Handle(app, "/ws/chat", func(ctx context.Context, conn *ws.Conn, q RoomQuery) error {
		userChan <- q.User
		if q.User == "banned" {
			return ErrCustomAuthFailed
		}

		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				return nil
			}

			if err := conn.WriteMessage(msgType, data); err != nil {
				return err
			}
		}
	})

	t.Run("Missing required query param returns 400 Bad Request before upgrade", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/ws/chat", nil)
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Valid query param upgrades and echoes", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer ln.Close()

		addr := ln.Addr().String()
		go func() {
			_ = app.Serve(ln)
		}()

		clientConn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		require.NoError(t, err)
		defer clientConn.Close()

		nonce := make([]byte, 16)
		_, _ = rand.Read(nonce)
		clientKey := base64.StdEncoding.EncodeToString(nonce)

		reqStr := "GET /ws/chat?room=general&user=alice HTTP/1.1\r\n" +
			"Host: " + addr + "\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"Sec-WebSocket-Version: 13\r\n" +
			"Sec-WebSocket-Key: " + clientKey + "\r\n\r\n"

		_, err = clientConn.Write([]byte(reqStr))
		require.NoError(t, err)

		br := bufio.NewReader(clientConn)
		statusLine, err := br.ReadString('\n')
		require.NoError(t, err)
		assert.Contains(t, statusLine, "101 Switching Protocols")

		for {
			line, err := br.ReadString('\n')
			require.NoError(t, err)
			if line == "\r\n" {
				break
			}
		}

		select {
		case user := <-userChan:
			assert.Equal(t, "alice", user)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for user from handler")
		}
	})

	t.Run("Handler returning mapped error auto-closes socket with mapped status code", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer ln.Close()

		addr := ln.Addr().String()
		go func() {
			_ = app.Serve(ln)
		}()

		clientConn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		require.NoError(t, err)
		defer clientConn.Close()

		nonce := make([]byte, 16)
		_, _ = rand.Read(nonce)
		clientKey := base64.StdEncoding.EncodeToString(nonce)

		reqStr := "GET /ws/chat?room=general&user=banned HTTP/1.1\r\n" +
			"Host: " + addr + "\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"Sec-WebSocket-Version: 13\r\n" +
			"Sec-WebSocket-Key: " + clientKey + "\r\n\r\n"

		_, err = clientConn.Write([]byte(reqStr))
		require.NoError(t, err)

		br := bufio.NewReader(clientConn)
		statusLine, err := br.ReadString('\n')
		require.NoError(t, err)
		assert.Contains(t, statusLine, "101 Switching Protocols")

		for {
			line, err := br.ReadString('\n')
			require.NoError(t, err)
			if line == "\r\n" {
				break
			}
		}

		// Read Close Frame from server (2 bytes header + 2 bytes code + payload)
		var closeHdr [2]byte
		_, err = br.Read(closeHdr[:])
		require.NoError(t, err)

		assert.Equal(t, byte(0x88), closeHdr[0]) // OpClose (0x8) with FIN (0x80)

		var closeCodeBuf [2]byte
		_, err = br.Read(closeCodeBuf[:])
		require.NoError(t, err)

		closeCode := int(closeCodeBuf[0])<<8 | int(closeCodeBuf[1])
		assert.Equal(t, 4003, closeCode)
	})
}
