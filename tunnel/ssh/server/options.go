// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

// Option configures an SSH Server.
type Option func(*Server) error

// WithAddr sets the listening network address.
func WithAddr(addr string) Option {
	return func(s *Server) error {
		s.Addr = addr
		return nil
	}
}

// WithHandler sets the session handler function.
func WithHandler(handler Handler) Option {
	return func(s *Server) error {
		s.Handler = handler
		return nil
	}
}

// WithHostKeySigner appends a host key signer.
func WithHostKeySigner(signer ssh.Signer) Option {
	return func(s *Server) error {
		s.AddHostKey(signer)
		return nil
	}
}

// WithHostKeyPEM appends a host key from PEM encoded private key bytes.
func WithHostKeyPEM(pemBytes []byte) Option {
	return func(s *Server) error {
		signer, err := ssh.ParsePrivateKey(pemBytes)
		if err != nil {
			return err
		}

		s.AddHostKey(signer)

		return nil
	}
}

// WithHostKeyFile appends a host key from a private key file path.
func WithHostKeyFile(path string) Option {
	return func(s *Server) error {
		// #nosec G304 -- Admin loaded host key path
		pemBytes, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return err
		}

		return WithHostKeyPEM(pemBytes)(s)
	}
}

// WithPasswordAuth configures password authentication callback.
func WithPasswordAuth(handler PasswordHandler) Option {
	return func(s *Server) error {
		s.PasswordHandler = handler
		return nil
	}
}

// WithPublicKeyAuth configures public key authentication callback.
func WithPublicKeyAuth(handler PublicKeyHandler) Option {
	return func(s *Server) error {
		s.PublicKeyHandler = handler
		return nil
	}
}

// WithSubsystem registers a subsystem handler (e.g. SFTP).
func WithSubsystem(name string, handler SubsystemHandler) Option {
	return func(s *Server) error {
		s.SetSubsystem(name, handler)
		return nil
	}
}

// WithVersion sets the server software version string.
func WithVersion(version string) Option {
	return func(s *Server) error {
		s.Version = version
		return nil
	}
}

// GlobalRequestHandler handles SSH global requests (e.g. tcpip-forward for reverse tunnels).
type GlobalRequestHandler func(ctx Context, reqs <-chan *ssh.Request, conn *ssh.ServerConn)

// WithGlobalRequestHandler configures a global request handler callback for the SSH server.
func WithGlobalRequestHandler(handler GlobalRequestHandler) Option {
	return func(s *Server) error {
		s.GlobalRequestHandler = handler
		return nil
	}
}

// WithUserCAKeys registers trusted User Certificate Authority public keys for validating client certificates.
func WithUserCAKeys(caKeys ...ssh.PublicKey) Option {
	return func(s *Server) error {
		s.UserCAKeys = append(s.UserCAKeys, caKeys...)
		return nil
	}
}

// WithUserCAPEM registers trusted User CA public keys parsed from PEM or authorized_keys payload.
func WithUserCAPEM(pemBytes []byte) Option {
	return func(s *Server) error {
		keys, err := parsePublicKeys(pemBytes)
		if err != nil {
			return err
		}

		s.UserCAKeys = append(s.UserCAKeys, keys...)

		return nil
	}
}

// WithUserCAFile registers trusted User CA public keys loaded from file path.
func WithUserCAFile(path string) Option {
	return func(s *Server) error {
		// #nosec G304 -- Admin loaded CA path
		data, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return err
		}

		return WithUserCAPEM(data)(s)
	}
}

// WithHostCertificate registers a signed Host Certificate and private key for the server.
func WithHostCertificate(certBytes, keyBytes []byte, passphrase string) Option {
	return func(s *Server) error {
		signer, err := parseCertKey(certBytes, keyBytes, passphrase)
		if err != nil {
			return err
		}

		s.AddHostKey(signer)

		return nil
	}
}

func parseCertKey(certBytes, keyBytes []byte, passphrase string) (ssh.Signer, error) {
	var (
		keySigner ssh.Signer
		err       error
	)

	if passphrase != "" {
		keySigner, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(passphrase))
	} else {
		keySigner, err = ssh.ParsePrivateKey(keyBytes)
	}

	if err != nil {
		return nil, err
	}

	pubKey, _, _, _, err := ssh.ParseAuthorizedKey(certBytes)
	if err != nil {
		return nil, err
	}

	cert, ok := pubKey.(*ssh.Certificate)
	if !ok {
		return nil, ErrInvalidPublicKey
	}

	return ssh.NewCertSigner(cert, keySigner)
}

func parsePublicKeys(data []byte) ([]ssh.PublicKey, error) {
	var keys []ssh.PublicKey
	for len(data) > 0 {
		pubKey, _, _, rest, err := ssh.ParseAuthorizedKey(data)
		if err != nil {
			break
		}

		if pubKey != nil {
			keys = append(keys, pubKey)
		}

		data = rest
	}

	if len(keys) == 0 {
		return nil, ErrInvalidPublicKey
	}

	return keys, nil
}
