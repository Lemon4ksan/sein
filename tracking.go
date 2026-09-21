package sein

import (
	"net"
)

func (s *Server) trackConn(c net.Conn, add bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.activeConns == nil {
		s.activeConns = make(map[net.Conn]struct{})
	}

	if add {
		s.activeConns[c] = struct{}{}
	} else {
		delete(s.activeConns, c)
	}
}
