// Copyright (c) 2016 the quic-go authors. All rights reserved.
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quic

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"net"
	"sync/atomic"

	"github.com/lemon4ksan/sein/internal/quic/internal/handshake"
	"github.com/lemon4ksan/sein/internal/quic/internal/protocol"
	"github.com/lemon4ksan/sein/internal/quic/internal/utils"
	"github.com/lemon4ksan/sein/internal/quic/internal/wire"
)

// ErrServerClosed is returned by Listener.Accept when Close is called.
var ErrServerClosed = errors.New("quic: server closed")

// Listener listens for incoming QUIC connections.
type Listener struct {
	tr          *Transport
	tlsConf     *tls.Config
	config      *Config
	connChan    chan *Conn
	errChan     chan error
	closeChan   chan struct{}
	isClosed    atomic.Bool
	tokenGen    *handshake.TokenGenerator
	logger      utils.Logger
	createdConn bool
}

var _ packetHandler = &Listener{}

// ListenAddr creates a new QUIC listener on the specified UDP address.
func ListenAddr(addr string, tlsConf *tls.Config, conf *Config) (*Listener, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}
	tr := &Transport{
		Conn:        conn,
		createdConn: true,
		isSingleUse: true,
	}
	ln, err := tr.Listen(tlsConf, conf)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	ln.createdConn = true
	return ln, nil
}

// Listen creates a new QUIC listener on an existing PacketConn.
func Listen(conn net.PacketConn, tlsConf *tls.Config, conf *Config) (*Listener, error) {
	tr := &Transport{
		Conn:        conn,
		isSingleUse: true,
	}
	return tr.Listen(tlsConf, conf)
}

// Listen starts listening for incoming QUIC connections on this Transport.
func (t *Transport) Listen(tlsConf *tls.Config, conf *Config) (*Listener, error) {
	if tlsConf == nil {
		return nil, errors.New("quic: nil tls.Config")
	}
	if err := t.init(true); err != nil {
		return nil, err
	}

	conf = populateConfig(conf)
	logger := utils.DefaultLogger.WithPrefix("server")

	var tokenGenKey handshake.TokenProtectorKey
	if _, err := rand.Read(tokenGenKey[:]); err != nil {
		return nil, err
	}
	tokenGen := handshake.NewTokenGenerator(tokenGenKey)

	ln := &Listener{
		tr:        t,
		tlsConf:   tlsConf,
		config:    conf,
		connChan:  make(chan *Conn, 32),
		errChan:   make(chan error, 1),
		closeChan: make(chan struct{}),
		tokenGen:  tokenGen,
		logger:    logger,
	}

	t.mutex.Lock()
	t.serverHandler = ln
	t.mutex.Unlock()

	return ln, nil
}

// Accept returns newly accepted QUIC connections.
func (l *Listener) Accept(ctx context.Context) (*Conn, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-l.closeChan:
		return nil, ErrServerClosed
	case err := <-l.errChan:
		return nil, err
	case conn := <-l.connChan:
		return conn, nil
	}
}

// Addr returns the listener's network address.
func (l *Listener) Addr() net.Addr {
	return l.tr.Conn.LocalAddr()
}

// Close closes the listener.
func (l *Listener) Close() error {
	if l.isClosed.Swap(true) {
		return nil
	}
	close(l.closeChan)

	l.tr.mutex.Lock()
	if l.tr.serverHandler == packetHandler(l) {
		l.tr.serverHandler = nil
	}
	l.tr.mutex.Unlock()

	if l.createdConn {
		return l.tr.Close()
	}
	return nil
}

func (l *Listener) destroy(err error) {
	_ = l.Close()
}

func (l *Listener) closeWithTransportError(code TransportErrorCode) {
	_ = l.Close()
}

func (l *Listener) handlePacket(p receivedPacket) {
	if l.isClosed.Load() {
		p.buffer.MaybeRelease()
		return
	}

	if len(p.data) == 0 {
		p.buffer.MaybeRelease()
		return
	}

	// Only initial packets can create new connections
	hdr, _, _, err := wire.ParsePacket(p.data)
	if err != nil || hdr.Type != protocol.PacketTypeInitial {
		p.buffer.MaybeRelease()
		return
	}

	destConnID := hdr.DestConnectionID
	srcConnID := hdr.SrcConnectionID

	var serverConnID protocol.ConnectionID
	if l.tr.connIDGenerator != nil {
		var err error
		serverConnID, err = l.tr.connIDGenerator.GenerateConnectionID()
		if err != nil {
			p.buffer.MaybeRelease()
			return
		}
	} else {
		serverConnID, _ = protocol.GenerateConnectionID(l.tr.connIDLen)
	}

	sendConn := newSendConn(l.tr.conn, p.remoteAddr, p.info, l.logger)
	wConn := newServerConnection(
		context.Background(),
		sendConn,
		(*packetHandlerMap)(l.tr),
		destConnID,
		nil,
		destConnID,
		srcConnID,
		serverConnID,
		l.tr.connIDGenerator,
		l.config,
		l.tlsConf,
		l.tokenGen,
		false,
		l.logger,
		hdr.Version,
	)

	if !(*packetHandlerMap)(l.tr).AddWithConnID(destConnID, serverConnID, wConn) {
		p.buffer.MaybeRelease()
		return
	}

	go func() {
		_ = wConn.run()
	}()

	wConn.handlePacket(p)

	select {
	case l.connChan <- wConn.Conn:
	default:
		l.logger.Debugf("dropping incoming connection, backlog full")
	}
}
